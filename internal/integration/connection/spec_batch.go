package connection

import (
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/batch"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
)

// Batch spec bounds, restated from batch.SourceRevision.validateSemanticFields.
const (
	batchMaxPollSeconds    = 3600
	batchMaxProcessSeconds = 300
	batchMaxLeaseSeconds   = 3600
	batchMaxFilesPerPoll   = 1000
	batchMaxWorkloadGrants = 16
)

// batchMaxMessageBytes is the processor kernel limit the batch source bounds
// max_message_bytes by.
const batchMaxMessageBytes = processor.MaxPreviewSourceBytes

// BatchS3Spec is the `batch_s3` connection spec: batch.SourceRevisionInput
// with Provider s3, minus the identifiers the catalog assigns.
type BatchS3Spec struct {
	SourceID        string        `json:"source_id"`
	S3              *S3Spec       `json:"s3"`
	Workload        *WorkloadSpec `json:"workload,omitempty"`
	PollSeconds     *int64        `json:"poll_seconds"`
	LeaseSeconds    *int64        `json:"lease_seconds"`
	ProcessSeconds  *int64        `json:"process_seconds"`
	MaxFilesPerPoll *int64        `json:"max_files_per_poll"`
	MaxMessageBytes *int64        `json:"max_message_bytes"`
}

// S3Spec is batch.S3Policy. use_tls is required: an explicit false is only
// accepted for a loopback endpoint.
type S3Spec struct {
	Endpoint               string `json:"endpoint"`
	Region                 string `json:"region,omitempty"`
	Bucket                 string `json:"bucket"`
	InputPrefix            string `json:"input_prefix"`
	ArchivePrefix          string `json:"archive_prefix"`
	UseTLS                 *bool  `json:"use_tls"`
	AccessKeyBinding       string `json:"access_key_binding"`
	SecretAccessKeyBinding string `json:"secret_access_key_binding"`
}

// BatchSFTPSpec is the `batch_sftp` connection spec: batch.SourceRevisionInput
// with Provider sftp.
type BatchSFTPSpec struct {
	SourceID        string        `json:"source_id"`
	SFTP            *SFTPSpec     `json:"sftp"`
	Workload        *WorkloadSpec `json:"workload,omitempty"`
	PollSeconds     *int64        `json:"poll_seconds"`
	LeaseSeconds    *int64        `json:"lease_seconds"`
	ProcessSeconds  *int64        `json:"process_seconds"`
	MaxFilesPerPoll *int64        `json:"max_files_per_poll"`
	MaxMessageBytes *int64        `json:"max_message_bytes"`
}

// SFTPSpec is batch.SFTPPolicy. Exactly one of password_binding and
// private_key_binding is set; a passphrase binding only with a private key.
type SFTPSpec struct {
	Host                        string `json:"host"`
	Port                        *int64 `json:"port"`
	Username                    string `json:"username"`
	InputDirectory              string `json:"input_directory"`
	ArchiveDirectory            string `json:"archive_directory"`
	KnownHostsBinding           string `json:"known_hosts_binding"`
	PasswordBinding             string `json:"password_binding,omitempty"`
	PrivateKeyBinding           string `json:"private_key_binding,omitempty"`
	PrivateKeyPassphraseBinding string `json:"private_key_passphrase_binding,omitempty"`
}

// WorkloadSpec is batch.WorkloadIdentity.
type WorkloadSpec struct {
	Subject string   `json:"subject"`
	Grants  []string `json:"grants,omitempty"`
}

// batchSchedule is the part of both batch specs that is not provider policy.
type batchSchedule struct {
	sourceID        string
	workload        *WorkloadSpec
	pollSeconds     *int64
	leaseSeconds    *int64
	processSeconds  *int64
	maxFilesPerPoll *int64
	maxMessageBytes *int64
}

func (b batchSchedule) check(c *checker) {
	c.identity("source_id", b.sourceID)
	c.intRange("poll_seconds", b.pollSeconds, 1, batchMaxPollSeconds)
	processOK := c.intRange("process_seconds", b.processSeconds, 1, batchMaxProcessSeconds)
	if b.leaseSeconds == nil {
		c.add(CodeRequired, "lease_seconds", "is required")
	} else if *b.leaseSeconds > batchMaxLeaseSeconds || *b.leaseSeconds < 1 ||
		(processOK && *b.leaseSeconds <= *b.processSeconds) {
		c.add(CodeOutOfRange, "lease_seconds", "must be longer than process_seconds and at most 3600")
	}
	c.intRange("max_files_per_poll", b.maxFilesPerPoll, 1, batchMaxFilesPerPoll)
	c.intRange("max_message_bytes", b.maxMessageBytes, 1, batchMaxMessageBytes)
	if b.workload != nil {
		c.identity("workload.subject", b.workload.Subject)
		c.grants("workload.grants", b.workload.Grants, 0, batchMaxWorkloadGrants)
	}
}

func (b batchSchedule) input(artifactID, revisionID string, provider batch.ProviderType) batch.SourceRevisionInput {
	input := batch.SourceRevisionInput{
		ArtifactID:      artifactID,
		RevisionID:      revisionID,
		SourceID:        b.sourceID,
		Provider:        provider,
		PollSeconds:     int64Value(b.pollSeconds),
		LeaseSeconds:    int64Value(b.leaseSeconds),
		ProcessSeconds:  int64Value(b.processSeconds),
		MaxFilesPerPoll: int(int64Value(b.maxFilesPerPoll)),
		MaxMessageBytes: int64Value(b.maxMessageBytes),
	}
	if b.workload != nil {
		input.Workload = &batch.WorkloadIdentity{
			Subject: b.workload.Subject,
			Grants:  append([]string(nil), b.workload.Grants...),
		}
	}
	return input
}

func (s *BatchS3Spec) schedule() batchSchedule {
	return batchSchedule{
		sourceID: s.SourceID, workload: s.Workload, pollSeconds: s.PollSeconds,
		leaseSeconds: s.LeaseSeconds, processSeconds: s.ProcessSeconds,
		maxFilesPerPoll: s.MaxFilesPerPoll, maxMessageBytes: s.MaxMessageBytes,
	}
}

func (s *BatchS3Spec) check(c *checker) {
	s.schedule().check(c)
	if s.S3 == nil {
		c.add(CodeRequired, "s3", "is required")
		return
	}
	policy := s.S3
	switch {
	case policy.Endpoint == "":
		c.add(CodeRequired, "s3.endpoint", "is required")
	case !validEndpoint(policy.Endpoint):
		c.add(CodeInvalidAddress, "s3.endpoint", "must be host or host:port, without a scheme or path")
	case policy.UseTLS != nil && !*policy.UseTLS && !loopbackEndpoint(policy.Endpoint):
		c.add(CodeConflict, "s3.use_tls", "may be false only for a loopback endpoint")
	}
	if policy.UseTLS == nil {
		c.add(CodeRequired, "s3.use_tls", "is required")
	}
	c.identity("s3.bucket", policy.Bucket)
	inputOK := relativePrefix(c, "s3.input_prefix", policy.InputPrefix)
	archiveOK := relativePrefix(c, "s3.archive_prefix", policy.ArchivePrefix)
	if inputOK && archiveOK && prefixesOverlap(policy.InputPrefix, policy.ArchivePrefix) {
		c.add(CodeConflict, "s3.archive_prefix", "must not overlap input_prefix")
	}
	accessOK := c.binding("s3.access_key_binding", policy.AccessKeyBinding)
	secretOK := c.binding("s3.secret_access_key_binding", policy.SecretAccessKeyBinding)
	if accessOK && secretOK && policy.AccessKeyBinding == policy.SecretAccessKeyBinding {
		c.add(CodeConflict, "s3.secret_access_key_binding", "must name a different binding than access_key_binding")
	}
}

func relativePrefix(c *checker, path, value string) bool {
	switch {
	case value == "":
		c.add(CodeRequired, path, "is required")
		return false
	case !validRelativePrefix(value):
		c.add(CodeInvalidValue, path, "must be a clean relative prefix such as incoming")
		return false
	}
	return true
}

func absoluteDirectory(c *checker, path, value string) bool {
	switch {
	case value == "":
		c.add(CodeRequired, path, "is required")
		return false
	case !validAbsolutePath(value):
		c.add(CodeInvalidValue, path, "must be a clean absolute directory other than /")
		return false
	}
	return true
}

func (s *BatchS3Spec) input(artifactID, revisionID string) batch.SourceRevisionInput {
	input := s.schedule().input(artifactID, revisionID, batch.ProviderS3)
	if s.S3 != nil {
		input.S3 = &batch.S3Policy{
			Endpoint:               s.S3.Endpoint,
			Region:                 s.S3.Region,
			Bucket:                 s.S3.Bucket,
			InputPrefix:            s.S3.InputPrefix,
			ArchivePrefix:          s.S3.ArchivePrefix,
			UseTLS:                 boolValue(s.S3.UseTLS),
			AccessKeyBinding:       s.S3.AccessKeyBinding,
			SecretAccessKeyBinding: s.S3.SecretAccessKeyBinding,
		}
	}
	return input
}

func (s *BatchSFTPSpec) schedule() batchSchedule {
	return batchSchedule{
		sourceID: s.SourceID, workload: s.Workload, pollSeconds: s.PollSeconds,
		leaseSeconds: s.LeaseSeconds, processSeconds: s.ProcessSeconds,
		maxFilesPerPoll: s.MaxFilesPerPoll, maxMessageBytes: s.MaxMessageBytes,
	}
}

func (s *BatchSFTPSpec) check(c *checker) {
	s.schedule().check(c)
	if s.SFTP == nil {
		c.add(CodeRequired, "sftp", "is required")
		return
	}
	policy := s.SFTP
	switch {
	case policy.Host == "":
		c.add(CodeRequired, "sftp.host", "is required")
	case !validHost(policy.Host):
		c.add(CodeInvalidAddress, "sftp.host", "must be a host name or IP address")
	}
	c.intRange("sftp.port", policy.Port, 1, 65535)
	c.identity("sftp.username", policy.Username)
	inputOK := absoluteDirectory(c, "sftp.input_directory", policy.InputDirectory)
	archiveOK := absoluteDirectory(c, "sftp.archive_directory", policy.ArchiveDirectory)
	if inputOK && archiveOK && prefixesOverlap(policy.InputDirectory, policy.ArchiveDirectory) {
		c.add(CodeConflict, "sftp.archive_directory", "must not overlap input_directory")
	}
	c.binding("sftp.known_hosts_binding", policy.KnownHostsBinding)
	password := policy.PasswordBinding != ""
	privateKey := policy.PrivateKeyBinding != ""
	switch {
	case password && privateKey:
		c.add(CodeConflict, "sftp.private_key_binding", "set exactly one of password_binding and private_key_binding")
	case !password && !privateKey:
		c.add(CodeRequired, "sftp.password_binding", "set exactly one of password_binding and private_key_binding")
	}
	if password {
		c.binding("sftp.password_binding", policy.PasswordBinding)
	}
	if privateKey {
		c.binding("sftp.private_key_binding", policy.PrivateKeyBinding)
	}
	if policy.PrivateKeyPassphraseBinding != "" {
		if password {
			c.add(CodeForbidden, "sftp.private_key_passphrase_binding", "applies only with private_key_binding")
		} else {
			c.binding("sftp.private_key_passphrase_binding", policy.PrivateKeyPassphraseBinding)
		}
	}
}

func (s *BatchSFTPSpec) input(artifactID, revisionID string) batch.SourceRevisionInput {
	input := s.schedule().input(artifactID, revisionID, batch.ProviderSFTP)
	if s.SFTP != nil {
		input.SFTP = &batch.SFTPPolicy{
			Host:                  s.SFTP.Host,
			Port:                  int(int64Value(s.SFTP.Port)),
			Username:              s.SFTP.Username,
			InputDirectory:        s.SFTP.InputDirectory,
			ArchiveDirectory:      s.SFTP.ArchiveDirectory,
			KnownHostsBinding:     s.SFTP.KnownHostsBinding,
			PasswordBinding:       s.SFTP.PasswordBinding,
			PrivateKeyBinding:     s.SFTP.PrivateKeyBinding,
			PrivateKeyPassBinding: s.SFTP.PrivateKeyPassphraseBinding,
		}
	}
	return input
}
