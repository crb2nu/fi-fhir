//go:build integration

package mllp

import (
	"bufio"
	"context"
	"crypto/tls"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

const (
	identitySenderAURI   = "spiffe://hospital-a/mllp/sender-a"
	identitySenderBURI   = "spiffe://hospital-a/mllp/sender-b"
	identitySenderCURI   = "spiffe://hospital-a/mllp/sender-c"
	identityUnmappedURI  = "spiffe://hospital-a/mllp/sender-z"
	identityObserverRole = "integration:observer"
)

// TestPostgresMLLPRuntime_CertificateIdentityAuthorization is the Slice 4.1b2
// kill test. It drives the real MLLP listener over real TLS 1.3 mutual
// authentication against PostgreSQL 16 with the production durable processor
// and transaction-scoped runnable admission, and proves that:
//
//  1. two allowlisted certificates remain two distinct verified service
//     subjects at the authorization decision even when the second sender's
//     in-band MSH provenance impersonates the first;
//  2. a CA-valid certificate absent from the identity map is rejected before
//     artifact loading and before any durable record exists;
//  3. an allowlisted identity without a recognized submit grant stops before
//     artifact loading or durability for the exact tenant/revision/source;
//  4. compatibility mode without an identity map preserves current behavior.
func TestPostgresMLLPRuntime_CertificateIdentityAuthorization(t *testing.T) {
	ctx := t.Context()
	dsn := os.Getenv("POSTGRES_TEST_URL")
	if dsn == "" {
		if os.Getenv("CI") != "" {
			t.Fatal("POSTGRES_TEST_URL is required in CI")
		}
		t.Skip("POSTGRES_TEST_URL is required for MLLP certificate identity tests")
	}
	schema := fmt.Sprintf("mllp_identity_%d", time.Now().UnixNano())
	createMLLPSchema(t, dsn, schema)
	db := openMLLPDB(t, mllpSchemaDSN(t, dsn, schema))

	authority := newIdentityAuthority(t)
	mappedSource := identityRuntimeSource(t, "mllp-source-mapped", "adt-mapped", []ClientIdentity{
		{Subject: "svc-sender-a", URISAN: identitySenderAURI, Grants: []string{authorizationMLLPGrant}},
		{Subject: "svc-sender-b", URISAN: identitySenderBURI, Grants: []string{authorizationHTTPGrant}},
		{Subject: "svc-sender-c", URISAN: identitySenderCURI, Grants: []string{identityObserverRole}},
	})
	compatibilitySource := identityRuntimeSource(t, "mllp-source-compat", "adt-compat", nil)

	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{
		ValidateConnection: func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
			return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE", "AUTH_OK"}}, nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	mappedRevision := identityMLLPRevision(t, "integration-mllp-identity", mappedSource)
	compatibilityRevision := identityMLLPRevision(t, "integration-mllp-compat", compatibilitySource)
	deployMLLPRevision(t, catalog, mappedRevision)
	deployMLLPRevision(t, catalog, compatibilityRevision)

	loads := &atomic.Int64{}
	artifacts, err := processor.NewRevisionResolver("tenant-a", &countingArtifactLoader{
		profile: []byte(integrationProfileJSON), workflow: []byte(integrationWorkflowYAML), loads: loads,
	})
	if err != nil {
		t.Fatal(err)
	}
	admitted := &identityRecorder{}
	store, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{
		Authorize: func(ctx context.Context, tx *sql.Tx, request integration.ProcessRequest, exact integration.IntegrationDefinitionRevision) error {
			err := catalog.AuthorizeRunnableSubmission(ctx, tx, request, exact)
			if err == nil {
				admitted.record(request.Security)
			}
			return err
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Migrate(ctx); err != nil {
		t.Fatal(err)
	}
	definitions, err := processor.NewDefinitionRevisionResolver("tenant-a", catalog)
	if err != nil {
		t.Fatal(err)
	}
	durableProcessor, err := processor.NewDurableMessageProcessor(definitions, artifacts, store)
	if err != nil {
		t.Fatal(err)
	}

	mappedAddress, stopMapped := identityRuntimeServer(t, mappedSource, mappedRevision.DefinitionID, catalog, durableProcessor, authority.material)
	defer stopMapped()

	// 1. Two allowlisted certificates stay two distinct verified subjects even
	//    when the second sender spoofs the first sender's in-band provenance.
	senderA := authority.clientConfig(t, identitySenderAURI)
	senderB := authority.clientConfig(t, identitySenderBURI)
	if code := identityRoundTrip(t, mappedAddress, senderA, mappedSource, identityHL7("SENDER-A", "FAC-A", "identity-1")); code != "AA" {
		t.Fatalf("mapped sender A ACK = %s", code)
	}
	if code := identityRoundTrip(t, mappedAddress, senderB, mappedSource, identityHL7("SENDER-A", "FAC-A", "identity-2")); code != "AA" {
		t.Fatalf("mapped sender B ACK = %s", code)
	}
	assertMLLPCounts(t, db, 2)
	subjects := admitted.subjects()
	if len(subjects) != 2 || subjects[0] != "svc-sender-a" || subjects[1] != "svc-sender-b" {
		t.Fatalf("admitted subjects = %v, want distinct certificate identities", subjects)
	}
	for index, security := range admitted.snapshot() {
		if security.Principal.Kind != integration.PrincipalKindService ||
			security.Principal.AuthMethod != AuthMethodCertificateIdentity ||
			security.Principal.SourceID != mappedSource.SourceID ||
			security.TenantID != "tenant-a" {
			t.Fatalf("admitted security[%d] = %#v", index, security.Principal)
		}
	}
	if grants := admitted.snapshot()[0].Principal.Roles; len(grants) != 1 || grants[0] != authorizationMLLPGrant {
		t.Fatalf("sender A grants = %v", grants)
	}
	if grants := admitted.snapshot()[1].Principal.Roles; len(grants) != 1 || grants[0] != authorizationHTTPGrant {
		t.Fatalf("sender B grants = %v", grants)
	}
	loadsAfterAccepted := loads.Load()
	if loadsAfterAccepted == 0 {
		t.Fatal("accepted submissions never loaded artifacts")
	}

	// 2. A CA-valid certificate absent from the identity map is closed before
	//    any frame is processed and leaves no durable record.
	unmapped := authority.clientConfig(t, identityUnmappedURI)
	identityExpectNoAcknowledgement(t, mappedAddress, unmapped, mappedSource, identityHL7("SENDER-A", "FAC-A", "identity-3"))
	assertMLLPCounts(t, db, 2)
	if loads.Load() != loadsAfterAccepted {
		t.Fatalf("unmapped certificate loaded artifacts: %d -> %d", loadsAfterAccepted, loads.Load())
	}
	if len(admitted.subjects()) != 2 {
		t.Fatalf("unmapped certificate reached durable admission: %v", admitted.subjects())
	}

	// 3. A mapped identity without a recognized submit grant is denied for the
	//    exact tenant, revision, and source before artifacts or durability.
	senderC := authority.clientConfig(t, identitySenderCURI)
	if code := identityRoundTrip(t, mappedAddress, senderC, mappedSource, identityHL7("SENDER-C", "FAC-C", "identity-4")); code != "AE" {
		t.Fatalf("ungranted identity ACK = %s, want AE", code)
	}
	assertMLLPCounts(t, db, 2)
	if loads.Load() != loadsAfterAccepted {
		t.Fatalf("ungranted identity loaded artifacts: %d -> %d", loadsAfterAccepted, loads.Load())
	}
	if len(admitted.subjects()) != 2 {
		t.Fatalf("ungranted identity reached durable admission: %v", admitted.subjects())
	}

	// 4. Compatibility mode without an identity map keeps current behavior: the
	//    same CA-valid certificate that the mapped listener rejects is admitted
	//    under the deployment-fixed principal and server-issued grant.
	compatibilityAddress, stopCompatibility := identityRuntimeServer(
		t, compatibilitySource, compatibilityRevision.DefinitionID, catalog, durableProcessor, authority.material,
	)
	defer stopCompatibility()
	if code := identityRoundTrip(t, compatibilityAddress, unmapped, compatibilitySource, identityHL7("SENDER-A", "FAC-A", "identity-5")); code != "AA" {
		t.Fatalf("compatibility ACK = %s", code)
	}
	assertMLLPCounts(t, db, 3)
	compatibility := admitted.snapshot()
	if len(compatibility) != 3 {
		t.Fatalf("compatibility submission did not reach admission: %d", len(compatibility))
	}
	principal := compatibility[2].Principal
	if principal.ID != "mllp-listener" || principal.AuthMethod != AuthMethodMutualTLS ||
		principal.SourceID != compatibilitySource.SourceID ||
		len(principal.Roles) != 1 || principal.Roles[0] != SubmitRole {
		t.Fatalf("compatibility principal = %#v", principal)
	}
	assertMLLPPersistenceSafe(t, db)
}

const (
	authorizationMLLPGrant = "integration:mllp"
	authorizationHTTPGrant = "integration:submit"
)

type identityRecorder struct {
	mu      sync.Mutex
	records []integration.SecurityContext
}

func (r *identityRecorder) record(security integration.SecurityContext) {
	r.mu.Lock()
	defer r.mu.Unlock()
	clone := security
	clone.Principal.Roles = append([]string(nil), security.Principal.Roles...)
	r.records = append(r.records, clone)
}

func (r *identityRecorder) snapshot() []integration.SecurityContext {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]integration.SecurityContext(nil), r.records...)
}

func (r *identityRecorder) subjects() []string {
	values := make([]string, 0, 4)
	for _, record := range r.snapshot() {
		values = append(values, record.Principal.ID)
	}
	return values
}

type countingArtifactLoader struct {
	profile  []byte
	workflow []byte
	loads    *atomic.Int64
}

func (l *countingArtifactLoader) LoadProfileRevision(context.Context, string, string) ([]byte, error) {
	l.loads.Add(1)
	return append([]byte(nil), l.profile...), nil
}

func (l *countingArtifactLoader) LoadWorkflowRevision(context.Context, string, string) ([]byte, error) {
	l.loads.Add(1)
	return append([]byte(nil), l.workflow...), nil
}

func identityRuntimeSource(t *testing.T, artifactID, sourceID string, identities []ClientIdentity) SourceRevision {
	t.Helper()
	revision, err := NewSourceRevision(SourceRevisionInput{
		ArtifactID: artifactID, RevisionID: "source-v1", SourceID: sourceID,
		ListenAddress: "127.0.0.1:2575", Encoding: "utf-8",
		Framing:  FramingPolicy{StartByte: StandardStartByte, EndByte: StandardEndByte, TrailerByte: StandardTrailerByte},
		Timeouts: TimeoutPolicy{ReadSeconds: 5, WriteSeconds: 5, IdleSeconds: 5, ProcessSeconds: 10},
		TLS: TLSPolicy{
			Mode: TLSModeMutual, ServerCertificateBinding: "mllp-server-cert",
			ServerPrivateKeyBinding: "mllp-server-key", ClientCABinding: "mllp-client-ca",
		},
		Clients:          ClientPolicy{AllowedCIDRs: []string{"127.0.0.0/8"}, Identities: identities},
		Acknowledgements: AcknowledgementPolicy{Mode: AcknowledgementModeApplication, IncludeErrorSegment: true},
		MaxMessageBytes:  4096, MaxConnections: 8,
	})
	if err != nil {
		t.Fatalf("create identity source revision: %v", err)
	}
	return revision
}

func identityMLLPRevision(t *testing.T, definitionID string, source SourceRevision) integration.IntegrationDefinitionRevision {
	t.Helper()
	profileRef, err := processor.NewProfileRevisionReference("profile-adt", 1, []byte(integrationProfileJSON))
	if err != nil {
		t.Fatal(err)
	}
	workflowRef, err := processor.NewWorkflowRevisionReference("workflow-adt", "workflow-1", []byte(integrationWorkflowYAML))
	if err != nil {
		t.Fatal(err)
	}
	deployment := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 300},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health:               integration.HealthPolicy{StartupGraceSeconds: 5, CheckIntervalSeconds: 10, TimeoutSeconds: 2, FailureThreshold: 3},
		Capacity:             integration.CapacityPolicy{MaxInFlight: 2, MaxQueued: 8, MaxMessagesPerSecond: 100},
	}
	secretReference := integration.SecretReference{Provider: integration.SecretProviderFile, Key: "/var/run/secrets/mllp"}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: definitionID, RevisionID: "definition-v1", TenantID: "tenant-a",
		Source: integration.SourceRevisionRef{ArtifactRevisionRef: source.Reference(), SourceID: source.SourceID},
		Format: events.FormatHL7v2, Profile: profileRef, Workflow: workflowRef,
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: "fhir-primary", RevisionID: "destination-v1",
				Digest: "sha256:" + strings.Repeat("d", 64),
			},
			Class: integration.DestinationClassProduction,
		}},
		SecretBindings: []integration.SecretBinding{
			{Name: "mllp-server-cert", Reference: secretReference},
			{Name: "mllp-server-key", Reference: secretReference},
			{Name: "mllp-client-ca", Reference: secretReference},
		},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: &deployment,
		Created: integration.AuditEnvelope{
			TenantID: "tenant-a",
			Principal: integration.Principal{
				ID: "engineer", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"integration:engineer"},
			},
			Reason: "create MLLP certificate identity fixture", OccurredAt: time.Now().UTC(),
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return revision
}

func identityRuntimeServer(
	t *testing.T,
	source SourceRevision,
	definitionID string,
	catalog *lifecycle.PostgresCatalog,
	messageProcessor MessageProcessor,
	material TLSMaterial,
) (string, func()) {
	t.Helper()
	server, err := NewServer(ServerConfig{
		Service: ServiceConfig{
			TenantID: "tenant-a", DefinitionID: definitionID, PrincipalID: "mllp-listener",
			Source: source, Resolver: catalog, Processor: messageProcessor,
		},
		TLSMaterial: material,
	})
	if err != nil {
		t.Fatal(err)
	}
	return serveTestServer(t, server)
}

func identityHL7(sendingApplication, sendingFacility, controlID string) []byte {
	segments := []string{
		"MSH|^~\\&|" + sendingApplication + "|" + sendingFacility +
			"|FI-FHIR|FAC|20260715120000-0400||ADT^A01^ADT_A01|" + controlID + "|P|2.5.1",
		"EVN|A01|20260715120000||||20260715115900-0400",
		"PID|1||MRN-123^^^HOSP^MR||Patient^Test||19800101|F",
		"PV1|1|I|UNIT^101^A^FAC||||||||||||||||visit-" + controlID + "|||||||||||||||||||||||||20260715120000",
	}
	return []byte(strings.Join(segments, "\r") + "\r")
}

func identityRoundTrip(t *testing.T, address string, config *tls.Config, source SourceRevision, payload []byte) string {
	t.Helper()
	connection := identityDial(t, address, config)
	defer connection.Close()
	framed, err := framePayload(payload, source.Framing)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := connection.Write(framed); err != nil {
		t.Fatal(err)
	}
	_ = connection.SetReadDeadline(time.Now().Add(10 * time.Second))
	acknowledgement, err := readFrame(bufio.NewReader(connection), source.Framing, source.MaxMessageBytes)
	if err != nil {
		t.Fatalf("read acknowledgement: %v", err)
	}
	return acknowledgementCodeFromPayload(t, acknowledgement)
}

// identityExpectNoAcknowledgement proves the certificate is genuinely CA-valid
// — the mutual-TLS handshake must succeed — and that the listener still refuses
// to answer because the identity is unmapped.
func identityExpectNoAcknowledgement(t *testing.T, address string, config *tls.Config, source SourceRevision, payload []byte) {
	t.Helper()
	connection := identityDial(t, address, config)
	defer connection.Close()
	framed, frameErr := framePayload(payload, source.Framing)
	if frameErr != nil {
		t.Fatal(frameErr)
	}
	_, _ = connection.Write(framed)
	_ = connection.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, readErr := connection.Read(make([]byte, 1)); readErr == nil {
		t.Fatal("unmapped CA-valid certificate received an MLLP acknowledgement")
	}
}

func identityDial(t *testing.T, address string, config *tls.Config) net.Conn {
	t.Helper()
	dialer := &net.Dialer{Timeout: 5 * time.Second}
	connection, err := tls.DialWithDialer(dialer, "tcp", address, config)
	if err != nil {
		t.Fatalf("dial %s: %v", address, err)
	}
	if connection.ConnectionState().Version != tls.VersionTLS13 {
		t.Fatalf("TLS version = %x", connection.ConnectionState().Version)
	}
	return connection
}
