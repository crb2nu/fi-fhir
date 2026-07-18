package session

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	publicationSchemaVersion = "fi-fhir.integration-session-publication/v1"
	publicationDigestDomain  = "fi-fhir/integration-session-publication/v1\x00"
	publicationSigningDomain = "fi-fhir/integration-session-publication-signature/v1\x00"
	publicationAlgorithm     = "Ed25519"
	maxPublicationReason     = 1024
)

var (
	ErrPublicationUnavailable = errors.New("integration session publication unavailable")
	ErrPublicationMismatch    = errors.New("integration session publication artifact mismatch")
	ErrPublicationSignature   = errors.New("integration session publication signature invalid")
)

// PublicationLifecycle is the closed production lifecycle surface used by a
// signed session publication. Implementations must preserve optimistic version
// checks and immutable definition revisions.
type PublicationLifecycle interface {
	LoadDefinitionRevision(context.Context, string, string, string) ([]byte, error)
	GetSnapshot(context.Context, string, string, string) (lifecycle.Snapshot, error)
	Approve(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Publish(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
	Deploy(context.Context, lifecycle.Command) (lifecycle.Snapshot, error)
}

// PublicationCrypto signs manifests with one local key and verifies them only
// against explicitly configured trust roots.
type PublicationCrypto struct {
	signingKeyID string
	privateKey   ed25519.PrivateKey
	trustedKeys  map[string]ed25519.PublicKey
}

func NewPublicationCrypto(signingKeyID string, privateKey ed25519.PrivateKey, trustedKeys map[string]ed25519.PublicKey) (*PublicationCrypto, error) {
	if !validPublicationIdentity(signingKeyID) || len(privateKey) != ed25519.PrivateKeySize || len(trustedKeys) == 0 {
		return nil, fmt.Errorf("%w: signing key and trust roots are required", ErrPublicationUnavailable)
	}
	clonedTrust := make(map[string]ed25519.PublicKey, len(trustedKeys))
	for keyID, publicKey := range trustedKeys {
		if !validPublicationIdentity(keyID) || len(publicKey) != ed25519.PublicKeySize {
			return nil, fmt.Errorf("%w: trust root is invalid", ErrPublicationUnavailable)
		}
		clonedTrust[keyID] = append(ed25519.PublicKey(nil), publicKey...)
	}
	if _, trusted := clonedTrust[signingKeyID]; !trusted {
		return nil, fmt.Errorf("%w: signing key is not trusted", ErrPublicationUnavailable)
	}
	derivedPublicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok || !bytes.Equal(derivedPublicKey, clonedTrust[signingKeyID]) {
		return nil, fmt.Errorf("%w: signing key does not match its trust root", ErrPublicationUnavailable)
	}
	return &PublicationCrypto{
		signingKeyID: signingKeyID,
		privateKey:   append(ed25519.PrivateKey(nil), privateKey...),
		trustedKeys:  clonedTrust,
	}, nil
}

func (c *PublicationCrypto) sign(manifest []byte) ([]byte, error) {
	if c == nil || len(c.privateKey) != ed25519.PrivateKeySize {
		return nil, ErrPublicationUnavailable
	}
	return ed25519.Sign(c.privateKey, publicationSigningBytes(manifest)), nil
}

func (c *PublicationCrypto) verify(keyID, algorithm string, manifest, signature []byte) error {
	if c == nil || algorithm != publicationAlgorithm || !validPublicationIdentity(keyID) || len(signature) != ed25519.SignatureSize {
		return ErrPublicationSignature
	}
	publicKey, trusted := c.trustedKeys[keyID]
	if !trusted || !ed25519.Verify(publicKey, publicationSigningBytes(manifest), signature) {
		return ErrPublicationSignature
	}
	return nil
}

// PublicationService creates and promotes PHI-minimal, signed evidence for an
// exact tested session and an exact validated production definition.
type PublicationService struct {
	store     Store
	tenantID  string
	resolver  *processor.RevisionResolver
	lifecycle PublicationLifecycle
	crypto    *PublicationCrypto
	clock     func() time.Time
}

func NewPublicationService(store Store, tenantID string, resolver *processor.RevisionResolver, catalog PublicationLifecycle, crypto *PublicationCrypto, clock func() time.Time) (*PublicationService, error) {
	if store == nil || !validPublicationIdentity(tenantID) || resolver == nil || catalog == nil || crypto == nil {
		return nil, ErrPublicationUnavailable
	}
	if clock == nil {
		clock = time.Now
	}
	return &PublicationService{store: store, tenantID: tenantID, resolver: resolver, lifecycle: catalog, crypto: crypto, clock: clock}, nil
}

// Publish proves that exact session bytes are content-equivalent to the exact
// artifacts referenced by a validated production definition, then signs and
// appends the bounded evidence record.
func (s *PublicationService) Publish(ctx context.Context, req PublishRequest) (*Publication, error) {
	if err := s.available(ctx); err != nil {
		return nil, err
	}
	if !validPublicationIdentity(req.SessionID) || !validPublicationIdentity(req.ProfileRevisionID) ||
		!validPublicationIdentity(req.WorkflowSimulationID) || !validPublicationIdentity(req.DefinitionID) ||
		!validPublicationIdentity(req.DefinitionRevisionID) || !validPublicationIdentity(req.PublishedBy) ||
		strings.TrimSpace(req.Reason) == "" || len(req.Reason) > maxPublicationReason {
		return nil, fmt.Errorf("%w: publication request is incomplete", ErrInvalid)
	}

	profile, err := s.store.GetArtifactRevision(ctx, req.SessionID, req.ProfileRevisionID)
	if err != nil {
		return nil, err
	}
	if profile.Kind != ArtifactKindMappingProfile || profile.RevisionID != req.ProfileRevisionID || profile.Digest != recordDigest(profile.Content) {
		return nil, ErrImmutable
	}
	simulation, err := s.store.GetWorkflowSimulation(ctx, req.SessionID, req.WorkflowSimulationID)
	if err != nil {
		return nil, err
	}
	workflowRevision, err := s.store.GetArtifactRevision(ctx, req.SessionID, simulation.WorkflowRevisionID)
	if err != nil {
		return nil, err
	}
	if workflowRevision.Kind != ArtifactKindWorkflowDraft || workflowRevision.ID != simulation.WorkflowArtifactID ||
		workflowRevision.RevisionID != simulation.WorkflowRevisionID || workflowRevision.Digest != simulation.WorkflowRevisionDigest ||
		workflowRevision.Digest != recordDigest(workflowRevision.Content) {
		return nil, ErrImmutable
	}

	definitionJSON, err := s.lifecycle.LoadDefinitionRevision(ctx, s.tenantID, req.DefinitionID, req.DefinitionRevisionID)
	if err != nil {
		return nil, err
	}
	definition, err := integration.DecodeIntegrationDefinitionRevision(bytes.NewReader(definitionJSON))
	if err != nil || definition.ValidateForDeployment() != nil || definition.TenantID != s.tenantID ||
		definition.DefinitionID != req.DefinitionID || definition.RevisionID != req.DefinitionRevisionID {
		return nil, lifecycle.ErrImmutableRecord
	}
	snapshot, err := s.lifecycle.GetSnapshot(ctx, s.tenantID, req.DefinitionID, req.DefinitionRevisionID)
	if err != nil {
		return nil, err
	}
	if snapshot.DefinitionRevision != definition.Reference() {
		return nil, lifecycle.ErrImmutableRecord
	}
	if snapshot.State != integration.DeploymentStateValidated {
		return nil, fmt.Errorf("%w: production definition must be validated", lifecycle.ErrInvalidTransition)
	}

	resolved, err := s.resolver.Resolve(ctx, s.tenantID, definition.Profile, definition.Workflow)
	if err != nil {
		return nil, err
	}
	if err := verifyProductionContent(profile.Content, workflowRevision.Content, definition, resolved); err != nil {
		return nil, err
	}

	fixtures, err := s.publicationFixtures(ctx, req.SessionID, profile, simulation)
	if err != nil {
		return nil, err
	}
	sets := workflowSimulationKeySets(*simulation)
	publicationID := newID("publication")
	createdAt := s.clock().UTC()
	manifest := PublicationManifest{
		SchemaVersion: publicationSchemaVersion, PublicationID: publicationID, SessionID: req.SessionID,
		SessionProfile:       integration.ArtifactRevisionRef{ArtifactID: profile.ID, RevisionID: profile.RevisionID, Digest: profile.Digest},
		SessionWorkflow:      integration.ArtifactRevisionRef{ArtifactID: workflowRevision.ID, RevisionID: workflowRevision.RevisionID, Digest: workflowRevision.Digest},
		WorkflowSimulationID: simulation.ID, DefinitionRevision: definition.Reference(),
		DefinitionVersion: snapshot.Version,
		ProductionProfile: resolved.ProfileReference(), ProductionWorkflow: resolved.WorkflowReference(),
		Fixtures: fixtures, ExpectedMatchedRoutes: sortedSet(sets.routes), ExpectedTransforms: sortedSet(sets.transforms),
		ExpectedActions: sortedSet(sets.actions), PublishedBy: strings.TrimSpace(req.PublishedBy),
		Reason: strings.TrimSpace(req.Reason), CreatedAt: createdAt,
	}
	manifestBytes, err := json.Marshal(manifest)
	if err != nil {
		return nil, fmt.Errorf("encode publication manifest: %w", err)
	}
	signature, err := s.crypto.sign(manifestBytes)
	if err != nil {
		return nil, err
	}
	record, err := s.store.CreatePublication(ctx, req.SessionID, CreatePublicationRequest{
		ID: publicationID, ProfileArtifactID: profile.ID, ProfileRevisionID: profile.RevisionID,
		ProfileRevisionDigest: profile.Digest, WorkflowArtifactID: workflowRevision.ID,
		WorkflowRevisionID: workflowRevision.RevisionID, WorkflowRevisionDigest: workflowRevision.Digest,
		WorkflowSimulationID: simulation.ID, DefinitionRevision: definition.Reference(),
		DefinitionVersion: snapshot.Version,
		ProductionProfile: resolved.ProfileReference(), ProductionWorkflow: resolved.WorkflowReference(),
		SourceRunIDs: simulation.SourceRunIDs, Manifest: manifestBytes, ManifestDigest: publicationDigest(manifestBytes),
		Signature: signature, SignatureAlgorithm: publicationAlgorithm, SigningKeyID: s.crypto.signingKeyID,
		PublishedBy: manifest.PublishedBy, Reason: manifest.Reason, CreatedAt: createdAt,
	})
	if err != nil {
		return nil, err
	}
	return record, nil
}

// Verify checks the detached signature, digest, canonical manifest bytes, and
// duplicated query fields before the record is trusted for promotion.
func (s *PublicationService) Verify(ctx context.Context, publication Publication) (PublicationManifest, error) {
	if err := s.available(ctx); err != nil {
		return PublicationManifest{}, err
	}
	if err := validateStoredPublication(publication); err != nil {
		return PublicationManifest{}, err
	}
	if publication.ManifestDigest != publicationDigest(publication.Manifest) {
		return PublicationManifest{}, ErrPublicationSignature
	}
	if err := s.crypto.verify(publication.SigningKeyID, publication.SignatureAlgorithm, publication.Manifest, publication.Signature); err != nil {
		return PublicationManifest{}, err
	}
	var manifest PublicationManifest
	decoder := json.NewDecoder(bytes.NewReader(publication.Manifest))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&manifest); err != nil {
		return PublicationManifest{}, ErrPublicationSignature
	}
	canonical, err := json.Marshal(manifest)
	if err != nil || !bytes.Equal(canonical, publication.Manifest) {
		return PublicationManifest{}, ErrPublicationSignature
	}
	if manifest.SchemaVersion != publicationSchemaVersion || manifest.PublicationID != publication.ID || manifest.SessionID != publication.SessionID ||
		manifest.SessionProfile != (integration.ArtifactRevisionRef{ArtifactID: publication.ProfileArtifactID, RevisionID: publication.ProfileRevisionID, Digest: publication.ProfileRevisionDigest}) ||
		manifest.SessionWorkflow != (integration.ArtifactRevisionRef{ArtifactID: publication.WorkflowArtifactID, RevisionID: publication.WorkflowRevisionID, Digest: publication.WorkflowRevisionDigest}) ||
		manifest.WorkflowSimulationID != publication.WorkflowSimulationID || manifest.DefinitionRevision != publication.DefinitionRevision ||
		manifest.DefinitionVersion != publication.DefinitionVersion ||
		manifest.ProductionProfile != publication.ProductionProfile || manifest.ProductionWorkflow != publication.ProductionWorkflow ||
		manifest.PublishedBy != publication.PublishedBy || manifest.Reason != publication.Reason || !manifest.CreatedAt.Equal(publication.CreatedAt) ||
		!equalStrings(manifestFixtureRunIDs(manifest.Fixtures), publication.SourceRunIDs) {
		return PublicationManifest{}, ErrPublicationSignature
	}
	return manifest, nil
}

// Approve verifies the publication before advancing its exact validated
// production definition. Later states are returned idempotently.
func (s *PublicationService) Approve(ctx context.Context, req PromotePublicationRequest) (lifecycle.Snapshot, error) {
	publication, manifest, snapshot, err := s.loadPromotion(ctx, req)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	_ = publication
	if snapshot.State == integration.DeploymentStateApproved || snapshot.State == integration.DeploymentStatePublished || snapshot.State == integration.DeploymentStateDeployed {
		return snapshot, nil
	}
	if snapshot.State != integration.DeploymentStateValidated {
		return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
	}
	return s.lifecycle.Approve(ctx, promotionCommand(s.tenantID, manifest.DefinitionRevision, req))
}

// Deploy verifies the publication immediately before every irreversible
// promotion step. It safely resumes from an already-published release.
func (s *PublicationService) Deploy(ctx context.Context, req PromotePublicationRequest) (lifecycle.Snapshot, error) {
	publication, manifest, snapshot, err := s.loadPromotion(ctx, req)
	if err != nil {
		return lifecycle.Snapshot{}, err
	}
	if snapshot.State == integration.DeploymentStateDeployed {
		return snapshot, nil
	}
	if snapshot.State == integration.DeploymentStateApproved {
		if _, err := s.Verify(ctx, *publication); err != nil {
			return lifecycle.Snapshot{}, err
		}
		snapshot, err = s.lifecycle.Publish(ctx, promotionCommandAtVersion(s.tenantID, manifest.DefinitionRevision, req, snapshot.Version))
		if err != nil {
			return lifecycle.Snapshot{}, err
		}
	}
	if snapshot.State != integration.DeploymentStatePublished {
		return lifecycle.Snapshot{}, lifecycle.ErrInvalidTransition
	}
	if _, err := s.Verify(ctx, *publication); err != nil {
		return lifecycle.Snapshot{}, err
	}
	return s.lifecycle.Deploy(ctx, promotionCommandAtVersion(s.tenantID, manifest.DefinitionRevision, req, snapshot.Version))
}

func (s *PublicationService) loadPromotion(ctx context.Context, req PromotePublicationRequest) (*Publication, PublicationManifest, lifecycle.Snapshot, error) {
	if err := s.available(ctx); err != nil {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, err
	}
	if !validPublicationIdentity(req.SessionID) || !validPublicationIdentity(req.PublicationID) || req.ExpectedVersion <= 0 {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, lifecycle.ErrInvalidCommand
	}
	publication, err := s.store.GetPublication(ctx, req.SessionID, req.PublicationID)
	if err != nil {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, err
	}
	manifest, err := s.Verify(ctx, *publication)
	if err != nil {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, err
	}
	snapshot, err := s.lifecycle.GetSnapshot(ctx, s.tenantID, manifest.DefinitionRevision.ArtifactID, manifest.DefinitionRevision.RevisionID)
	if err != nil {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, err
	}
	if snapshot.DefinitionRevision != manifest.DefinitionRevision {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, lifecycle.ErrImmutableRecord
	}
	if snapshot.Version != req.ExpectedVersion {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	if !publicationLifecycleVersionMatches(manifest.DefinitionVersion, snapshot.State, snapshot.Version) {
		return nil, PublicationManifest{}, lifecycle.Snapshot{}, lifecycle.ErrVersionConflict
	}
	return publication, manifest, snapshot, nil
}

func publicationLifecycleVersionMatches(publishedVersion int64, state integration.DeploymentState, currentVersion int64) bool {
	var transitions int64
	switch state {
	case integration.DeploymentStateValidated:
		transitions = 0
	case integration.DeploymentStateApproved:
		transitions = 1
	case integration.DeploymentStatePublished:
		transitions = 2
	case integration.DeploymentStateDeployed:
		transitions = 3
	default:
		return false
	}
	return publishedVersion > 0 && currentVersion == publishedVersion+transitions
}

func (s *PublicationService) publicationFixtures(ctx context.Context, sessionID string, profile *ArtifactDraft, simulation *WorkflowSimulation) ([]PublicationFixture, error) {
	if len(simulation.SourceRunIDs) == 0 || len(simulation.SourceRunIDs) > maxWorkflowSimulationRuns {
		return nil, ErrImmutable
	}
	fixtures := make([]PublicationFixture, 0, len(simulation.SourceRunIDs))
	seen := make(map[string]struct{}, len(simulation.SourceRunIDs))
	for _, runID := range simulation.SourceRunIDs {
		if _, duplicate := seen[runID]; duplicate {
			return nil, ErrImmutable
		}
		seen[runID] = struct{}{}
		run, err := s.store.GetRun(ctx, sessionID, runID)
		if err != nil {
			return nil, err
		}
		if run.Status != RunStatusSucceeded || len(run.Events) == 0 || run.ProfileID != profile.ID ||
			run.ProfileRevisionID != profile.RevisionID || run.ProfileRevisionDigest != profile.Digest {
			return nil, fmt.Errorf("%w: source run was not produced by the selected profile", ErrPublicationMismatch)
		}
		sample, err := s.store.GetSample(ctx, sessionID, run.SampleID)
		if err != nil {
			return nil, err
		}
		if sample.PHIPolicy != PHIPolicyRedact {
			return nil, fmt.Errorf("%w: publication fixtures must be redacted", ErrInvalid)
		}
		eventTypes := make([]string, 0, len(run.Events))
		for _, event := range run.Events {
			if event.ID == "" || event.Type == "" {
				return nil, ErrImmutable
			}
			eventTypes = append(eventTypes, event.Type)
		}
		diagnosticCodes := make([]string, 0, len(run.Diagnostics))
		for _, diagnostic := range run.Diagnostics {
			if diagnostic.Code == "" {
				return nil, ErrImmutable
			}
			diagnosticCodes = append(diagnosticCodes, diagnostic.Code)
		}
		fixtures = append(fixtures, PublicationFixture{
			RunID: run.ID, SampleID: sample.ID, SampleFormat: sample.Format, SampleDigest: recordDigest([]byte(sample.Raw)),
			ExpectedEventTypes: sortedUnique(eventTypes), ExpectedDiagnosticCodes: sortedUnique(diagnosticCodes),
		})
	}
	return fixtures, nil
}

func verifyProductionContent(profileJSON, workflowYAML []byte, definition integration.IntegrationDefinitionRevision, resolved processor.ResolvedArtifactRevisions) error {
	profileRevisionID, err := strconv.Atoi(definition.Profile.RevisionID)
	if err != nil || profileRevisionID <= 0 {
		return fmt.Errorf("%w: production profile revision is invalid", ErrPublicationMismatch)
	}
	profileRef, err := processor.NewProfileRevisionReference(definition.Profile.ArtifactID, profileRevisionID, profileJSON)
	if err != nil || profileRef != definition.Profile || profileRef != resolved.ProfileReference() {
		return fmt.Errorf("%w: session and production profile content differ", ErrPublicationMismatch)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference(definition.Workflow.ArtifactID, definition.Workflow.RevisionID, workflowYAML)
	if err != nil || workflowRef != definition.Workflow || workflowRef != resolved.WorkflowReference() {
		return fmt.Errorf("%w: session and production workflow content differ", ErrPublicationMismatch)
	}
	if !bytes.Equal(resolved.ProfileJSON(), profileJSON) {
		// Canonical JSON equivalence was proven by the reference above. Exact bytes
		// may differ only in insignificant JSON formatting.
		var sessionValue, productionValue any
		if json.Unmarshal(profileJSON, &sessionValue) != nil || json.Unmarshal(resolved.ProfileJSON(), &productionValue) != nil || !jsonValuesEqual(sessionValue, productionValue) {
			return fmt.Errorf("%w: session and production profile content differ", ErrPublicationMismatch)
		}
	}
	if !bytes.Equal(resolved.WorkflowYAML(), workflowYAML) {
		return fmt.Errorf("%w: session and production workflow content differ", ErrPublicationMismatch)
	}
	return nil
}

func validateCreatePublicationRequest(sessionID string, req CreatePublicationRequest) error {
	publication := Publication{
		ID: req.ID, SessionID: sessionID, Version: 1, ProfileArtifactID: req.ProfileArtifactID,
		ProfileRevisionID: req.ProfileRevisionID, ProfileRevisionDigest: req.ProfileRevisionDigest,
		WorkflowArtifactID: req.WorkflowArtifactID, WorkflowRevisionID: req.WorkflowRevisionID,
		WorkflowRevisionDigest: req.WorkflowRevisionDigest, WorkflowSimulationID: req.WorkflowSimulationID,
		DefinitionRevision: req.DefinitionRevision, ProductionProfile: req.ProductionProfile,
		DefinitionVersion:  req.DefinitionVersion,
		ProductionWorkflow: req.ProductionWorkflow, SourceRunIDs: req.SourceRunIDs, Manifest: req.Manifest,
		ManifestDigest: req.ManifestDigest, Signature: req.Signature, SignatureAlgorithm: req.SignatureAlgorithm,
		SigningKeyID: req.SigningKeyID, PublishedBy: req.PublishedBy, Reason: req.Reason, CreatedAt: req.CreatedAt,
	}
	return validateStoredPublication(publication)
}

func validateStoredPublication(publication Publication) error {
	identities := []string{publication.ID, publication.SessionID, publication.ProfileArtifactID, publication.ProfileRevisionID,
		publication.WorkflowArtifactID, publication.WorkflowRevisionID, publication.WorkflowSimulationID,
		publication.DefinitionRevision.ArtifactID, publication.DefinitionRevision.RevisionID,
		publication.ProductionProfile.ArtifactID, publication.ProductionProfile.RevisionID,
		publication.ProductionWorkflow.ArtifactID, publication.ProductionWorkflow.RevisionID,
		publication.SigningKeyID, publication.PublishedBy}
	for _, identity := range identities {
		if !validPublicationIdentity(identity) {
			return fmt.Errorf("%w: publication identity is invalid", ErrInvalid)
		}
	}
	if publication.Version <= 0 || publication.DefinitionVersion <= 0 || len(publication.SourceRunIDs) == 0 || len(publication.SourceRunIDs) > maxWorkflowSimulationRuns ||
		len(publication.Manifest) == 0 || len(publication.Signature) != ed25519.SignatureSize || publication.SignatureAlgorithm != publicationAlgorithm ||
		publication.ProfileRevisionDigest == "" || publication.WorkflowRevisionDigest == "" || publication.ManifestDigest == "" ||
		publication.DefinitionRevision.Digest == "" || publication.ProductionProfile.Digest == "" || publication.ProductionWorkflow.Digest == "" ||
		strings.TrimSpace(publication.Reason) == "" || len(publication.Reason) > maxPublicationReason || publication.CreatedAt.IsZero() {
		return fmt.Errorf("%w: publication record is incomplete", ErrInvalid)
	}
	seen := make(map[string]struct{}, len(publication.SourceRunIDs))
	for _, runID := range publication.SourceRunIDs {
		if !validPublicationIdentity(runID) {
			return fmt.Errorf("%w: source run identity is invalid", ErrInvalid)
		}
		if _, duplicate := seen[runID]; duplicate {
			return fmt.Errorf("%w: source run identities must be unique", ErrInvalid)
		}
		seen[runID] = struct{}{}
	}
	return nil
}

func (s *PublicationService) available(ctx context.Context) error {
	if s == nil || s.store == nil || s.resolver == nil || s.lifecycle == nil || s.crypto == nil || s.clock == nil || ctx == nil || ctx.Err() != nil {
		return ErrPublicationUnavailable
	}
	return nil
}

func promotionCommand(tenantID string, ref integration.ArtifactRevisionRef, req PromotePublicationRequest) lifecycle.Command {
	return promotionCommandAtVersion(tenantID, ref, req, req.ExpectedVersion)
}

func promotionCommandAtVersion(tenantID string, ref integration.ArtifactRevisionRef, req PromotePublicationRequest, version int64) lifecycle.Command {
	return lifecycle.Command{TenantID: tenantID, DefinitionID: ref.ArtifactID, RevisionID: ref.RevisionID, ExpectedVersion: version, Principal: req.Actor, Reason: req.Reason}
}

func publicationDigest(manifest []byte) string {
	digest := sha256.Sum256(append([]byte(publicationDigestDomain), manifest...))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func publicationSigningBytes(manifest []byte) []byte {
	return append(append([]byte(nil), publicationSigningDomain...), manifest...)
}

func sortedSet(values map[string]struct{}) []string {
	out := make([]string, 0, len(values))
	for value := range values {
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func sortedUnique(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	sort.Strings(values)
	out := values[:0]
	for _, value := range values {
		if len(out) == 0 || out[len(out)-1] != value {
			out = append(out, value)
		}
	}
	return out
}

func manifestFixtureRunIDs(fixtures []PublicationFixture) []string {
	out := make([]string, len(fixtures))
	for index := range fixtures {
		out[index] = fixtures[index].RunID
	}
	return out
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func jsonValuesEqual(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftJSON, rightJSON)
}

func validPublicationIdentity(value string) bool {
	if value == "" || len(value) > 256 || strings.TrimSpace(value) != value {
		return false
	}
	for _, character := range value {
		if character < 0x20 || character == 0x7f {
			return false
		}
	}
	return true
}
