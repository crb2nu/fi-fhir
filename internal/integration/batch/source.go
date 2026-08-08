// Package batch implements deployed-release S3 and SFTP batch ingestion.
package batch

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"path"
	"strconv"
	"strings"
	"unicode"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/authorization"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	SourceSchemaVersion   = "1"
	SubmitRole            = authorization.BatchSubmitGrant
	maxSourceRevisionSize = 1 << 20
	sourceDigestDomain    = "fi-fhir/batch-source/v1\x00"
)

type ProviderType string

const (
	ProviderS3   ProviderType = "s3"
	ProviderSFTP ProviderType = "sftp"
)

var (
	ErrInvalidSourceRevision = errors.New("invalid batch source revision")
	ErrSourceMismatch        = errors.New("batch source does not match deployed release")
)

// S3Policy contains non-secret S3 connection and object-prefix configuration.
type S3Policy struct {
	Endpoint               string `json:"endpoint"`
	Region                 string `json:"region,omitempty"`
	Bucket                 string `json:"bucket"`
	InputPrefix            string `json:"input_prefix"`
	ArchivePrefix          string `json:"archive_prefix"`
	UseTLS                 bool   `json:"use_tls"`
	AccessKeyBinding       string `json:"access_key_binding"`
	SecretAccessKeyBinding string `json:"secret_access_key_binding"`
}

// SFTPPolicy contains non-secret SFTP connection and directory configuration.
type SFTPPolicy struct {
	Host                  string `json:"host"`
	Port                  int    `json:"port"`
	Username              string `json:"username"`
	InputDirectory        string `json:"input_directory"`
	ArchiveDirectory      string `json:"archive_directory"`
	KnownHostsBinding     string `json:"known_hosts_binding"`
	PasswordBinding       string `json:"password_binding,omitempty"`
	PrivateKeyBinding     string `json:"private_key_binding,omitempty"`
	PrivateKeyPassBinding string `json:"private_key_passphrase_binding,omitempty"`
}

// SourceRevisionInput supplies semantic fields for a content-addressed source.
type SourceRevisionInput struct {
	ArtifactID      string
	RevisionID      string
	SourceID        string
	Provider        ProviderType
	S3              *S3Policy
	SFTP            *SFTPPolicy
	Workload        *WorkloadIdentity
	PollSeconds     int64
	LeaseSeconds    int64
	ProcessSeconds  int64
	MaxFilesPerPoll int
	MaxMessageBytes int64
}

// SourceRevision is the immutable runtime contract for one batch source.
// Secret values remain out of band; only lifecycle binding names are stored.
// An absent workload block selects compatibility mode and is omitted from the
// canonical digest input, so existing revisions keep their exact digest.
type SourceRevision struct {
	SchemaVersion   string            `json:"schema_version"`
	ArtifactID      string            `json:"artifact_id"`
	RevisionID      string            `json:"revision_id"`
	SourceID        string            `json:"source_id"`
	Provider        ProviderType      `json:"provider"`
	S3              *S3Policy         `json:"s3,omitempty"`
	SFTP            *SFTPPolicy       `json:"sftp,omitempty"`
	Workload        *WorkloadIdentity `json:"workload,omitempty"`
	PollSeconds     int64             `json:"poll_seconds"`
	LeaseSeconds    int64             `json:"lease_seconds"`
	ProcessSeconds  int64             `json:"process_seconds"`
	MaxFilesPerPoll int               `json:"max_files_per_poll"`
	MaxMessageBytes int64             `json:"max_message_bytes"`
	Digest          string            `json:"digest"`
}

func NewSourceRevision(input SourceRevisionInput) (SourceRevision, error) {
	revision := SourceRevision{
		SchemaVersion: SourceSchemaVersion, ArtifactID: input.ArtifactID,
		RevisionID: input.RevisionID, SourceID: input.SourceID, Provider: input.Provider,
		S3: cloneS3(input.S3), SFTP: cloneSFTP(input.SFTP),
		Workload:     cloneWorkloadIdentity(input.Workload),
		PollSeconds:  input.PollSeconds,
		LeaseSeconds: input.LeaseSeconds, ProcessSeconds: input.ProcessSeconds,
		MaxFilesPerPoll: input.MaxFilesPerPoll, MaxMessageBytes: input.MaxMessageBytes,
	}
	if err := revision.validateSemanticFields(); err != nil {
		return SourceRevision{}, err
	}
	digest, err := revision.semanticDigest()
	if err != nil {
		return SourceRevision{}, fmt.Errorf("%w: compute digest", ErrInvalidSourceRevision)
	}
	revision.Digest = digest
	return revision, nil
}

func DecodeSourceRevision(reader io.Reader) (SourceRevision, error) {
	if reader == nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	raw, err := io.ReadAll(io.LimitReader(reader, maxSourceRevisionSize+1))
	if err != nil || len(raw) == 0 || len(raw) > maxSourceRevisionSize || rejectDuplicateJSONKeys(raw) != nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var revision SourceRevision
	if err := decoder.Decode(&revision); err != nil {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err != io.EOF {
		return SourceRevision{}, ErrInvalidSourceRevision
	}
	if err := revision.Validate(); err != nil {
		return SourceRevision{}, err
	}
	return revision, nil
}

func (r SourceRevision) Validate() error {
	if err := r.validateSemanticFields(); err != nil {
		return err
	}
	expected, err := r.semanticDigest()
	if err != nil || r.Digest != expected {
		return ErrInvalidSourceRevision
	}
	return nil
}

func (r SourceRevision) Reference() integration.ArtifactRevisionRef {
	return integration.ArtifactRevisionRef{ArtifactID: r.ArtifactID, RevisionID: r.RevisionID, Digest: r.Digest}
}

func (r SourceRevision) ValidateAgainst(binding lifecycle.RunnableBinding) error {
	if r.Validate() != nil || binding.SourceRevision != r.Reference() ||
		binding.SourceID != r.SourceID || binding.Format != events.FormatHL7v2 ||
		binding.IntegrationRevision.ArtifactID == "" || binding.Deployment.Validate() != nil {
		return ErrSourceMismatch
	}
	for _, name := range r.secretBindingNames() {
		if !hasSecretBinding(binding.SecretBindings, name) {
			return ErrSourceMismatch
		}
	}
	return nil
}

func (r SourceRevision) validateSemanticFields() error {
	if r.SchemaVersion != SourceSchemaVersion || !validIdentity(r.ArtifactID) ||
		!validIdentity(r.RevisionID) || !validIdentity(r.SourceID) ||
		r.PollSeconds < 1 || r.PollSeconds > 3600 ||
		r.ProcessSeconds < 1 || r.ProcessSeconds > 300 ||
		r.LeaseSeconds <= r.ProcessSeconds || r.LeaseSeconds > 3600 ||
		r.MaxFilesPerPoll < 1 || r.MaxFilesPerPoll > 1000 ||
		r.MaxMessageBytes < 1 || r.MaxMessageBytes > processor.MaxPreviewSourceBytes {
		return ErrInvalidSourceRevision
	}
	switch r.Provider {
	case ProviderS3:
		if r.S3 == nil || r.SFTP != nil || validateS3(*r.S3) != nil {
			return ErrInvalidSourceRevision
		}
	case ProviderSFTP:
		if r.SFTP == nil || r.S3 != nil || validateSFTP(*r.SFTP) != nil {
			return ErrInvalidSourceRevision
		}
	default:
		return ErrInvalidSourceRevision
	}
	return validateWorkloadIdentity(r.Workload)
}

func validateS3(policy S3Policy) error {
	if !validEndpoint(policy.Endpoint) || !validIdentity(policy.Bucket) || (!policy.UseTLS && !loopbackEndpoint(policy.Endpoint)) ||
		!validRelativePrefix(policy.InputPrefix) || !validRelativePrefix(policy.ArchivePrefix) ||
		prefixesOverlap(policy.InputPrefix, policy.ArchivePrefix) ||
		!validIdentity(policy.AccessKeyBinding) || !validIdentity(policy.SecretAccessKeyBinding) ||
		policy.AccessKeyBinding == policy.SecretAccessKeyBinding {
		return ErrInvalidSourceRevision
	}
	return nil
}

func loopbackEndpoint(value string) bool {
	host := value
	if parsedHost, _, err := net.SplitHostPort(value); err == nil {
		host = parsedHost
	}
	host = strings.Trim(host, "[]")
	return host == "localhost" || net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback()
}

func validateSFTP(policy SFTPPolicy) error {
	if !validHost(policy.Host) || policy.Port < 1 || policy.Port > 65535 ||
		!validIdentity(policy.Username) || !validAbsolutePath(policy.InputDirectory) ||
		!validAbsolutePath(policy.ArchiveDirectory) ||
		prefixesOverlap(policy.InputDirectory, policy.ArchiveDirectory) ||
		!validIdentity(policy.KnownHostsBinding) {
		return ErrInvalidSourceRevision
	}
	password := policy.PasswordBinding != ""
	privateKey := policy.PrivateKeyBinding != ""
	if password == privateKey || (password && policy.PrivateKeyPassBinding != "") {
		return ErrInvalidSourceRevision
	}
	if password && !validIdentity(policy.PasswordBinding) {
		return ErrInvalidSourceRevision
	}
	if privateKey && !validIdentity(policy.PrivateKeyBinding) {
		return ErrInvalidSourceRevision
	}
	if policy.PrivateKeyPassBinding != "" && !validIdentity(policy.PrivateKeyPassBinding) {
		return ErrInvalidSourceRevision
	}
	return nil
}

func (r SourceRevision) secretBindingNames() []string {
	if r.S3 != nil {
		return []string{r.S3.AccessKeyBinding, r.S3.SecretAccessKeyBinding}
	}
	if r.SFTP == nil {
		return nil
	}
	names := []string{r.SFTP.KnownHostsBinding}
	for _, value := range []string{r.SFTP.PasswordBinding, r.SFTP.PrivateKeyBinding, r.SFTP.PrivateKeyPassBinding} {
		if value != "" {
			names = append(names, value)
		}
	}
	return names
}

func (r SourceRevision) semanticDigest() (string, error) {
	canonical := r
	canonical.Digest = ""
	canonical.Workload = canonicalWorkloadIdentity(r.Workload)
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", err
	}
	hasher := sha256.New()
	_, _ = hasher.Write([]byte(sourceDigestDomain))
	_, _ = hasher.Write(encoded)
	return "sha256:" + hex.EncodeToString(hasher.Sum(nil)), nil
}

func validEndpoint(value string) bool {
	if value == "" || strings.Contains(value, "://") || strings.TrimSpace(value) != value || strings.ContainsAny(value, "/\r\n\x00") {
		return false
	}
	host, port, err := net.SplitHostPort(value)
	if err == nil {
		number, parseErr := strconv.Atoi(port)
		return validHost(host) && parseErr == nil && number > 0 && number <= 65535 && strconv.Itoa(number) == port
	}
	return validHost(value)
}

func validHost(value string) bool {
	if value == "" || strings.TrimSpace(value) != value || strings.ContainsAny(value, "\r\n\x00/\\") {
		return false
	}
	if ip := net.ParseIP(strings.Trim(value, "[]")); ip != nil {
		return true
	}
	for _, label := range strings.Split(value, ".") {
		if label == "" || len(label) > 63 || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, character := range label {
			if !unicode.IsLetter(character) && !unicode.IsDigit(character) && character != '-' {
				return false
			}
		}
	}
	return true
}

func validRelativePrefix(value string) bool {
	return value != "" && value == path.Clean(value) && !path.IsAbs(value) && value != "." &&
		!strings.HasPrefix(value, "../") && !strings.ContainsAny(value, "\r\n\x00\\")
}

func validAbsolutePath(value string) bool {
	return value != "" && path.IsAbs(value) && value == path.Clean(value) && value != "/" &&
		!strings.ContainsAny(value, "\r\n\x00\\")
}

func prefixesOverlap(left, right string) bool {
	left = strings.TrimSuffix(left, "/")
	right = strings.TrimSuffix(right, "/")
	return left == right || strings.HasPrefix(left, right+"/") || strings.HasPrefix(right, left+"/")
}

func validIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if unicode.IsControl(character) || unicode.IsSpace(character) {
			return false
		}
	}
	return true
}

func hasSecretBinding(bindings []integration.SecretBinding, name string) bool {
	for _, binding := range bindings {
		if binding.Name == name {
			return true
		}
	}
	return false
}

func cloneS3(policy *S3Policy) *S3Policy {
	if policy == nil {
		return nil
	}
	clone := *policy
	return &clone
}

func cloneSFTP(policy *SFTPPolicy) *SFTPPolicy {
	if policy == nil {
		return nil
	}
	clone := *policy
	return &clone
}

func rejectDuplicateJSONKeys(raw []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	var walk func() error
	walk = func() error {
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		delimiter, ok := token.(json.Delim)
		if !ok {
			return nil
		}
		switch delimiter {
		case '{':
			seen := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return err
				}
				key, ok := keyToken.(string)
				if !ok {
					return ErrInvalidSourceRevision
				}
				if _, duplicate := seen[key]; duplicate {
					return ErrInvalidSourceRevision
				}
				seen[key] = struct{}{}
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		case '[':
			for decoder.More() {
				if err := walk(); err != nil {
					return err
				}
			}
			_, err = decoder.Token()
			return err
		default:
			return ErrInvalidSourceRevision
		}
	}
	return walk()
}
