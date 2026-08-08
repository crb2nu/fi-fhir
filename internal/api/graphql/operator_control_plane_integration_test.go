//go:build integration

package graphql_test

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	_ "github.com/lib/pq"

	graphqlapi "gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/resolvers"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity/oidctest"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/delivery"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/lifecycle"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/operator"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// rawPHISentinel is planted as a *value* inside the seeded canonical event.
// The control plane may render the structure of that document but never its
// content, so this string must not appear in any GraphQL response body.
const rawPHISentinel = "RAW-PHI-SENTINEL-9F2C"

const operatorTenant = "tenant-a"

// TestOperatorControlPlane_FailureReplayAndAuditGoldenJourneys is the Slice
// 4.2a kill-test. It proves the failure/replay and operator-audit golden
// journeys complete over the real GraphQL handler with a verified OIDC
// operator identity, against durable Slice 2.3 records, with no SQL and no
// filesystem access in the journey itself.
func TestOperatorControlPlane_FailureReplayAndAuditGoldenJourneys(t *testing.T) {
	ctx := t.Context()
	db := openOperatorDatabase(t, ctx)

	submissionStore, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submissionStore.Migrate(ctx); err != nil {
		t.Fatalf("migrate submission schema: %v", err)
	}
	if err := submissionStore.Migrate(ctx); err != nil {
		t.Fatalf("migrate submission schema (idempotent): %v", err)
	}

	seededAt := time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)
	clock := &operatorClock{now: seededAt}
	seedFailedDelivery(t, db, operatorTenant, "receipt-a", "attempt-a", "outbox-a", seededAt)
	// A second tenant's identical failure proves isolation is data-scoped, not
	// merely a claim check.
	seedFailedDelivery(t, db, "tenant-b", "receipt-b", "attempt-b", "outbox-b", seededAt)

	catalog, revision := seedDeployedIntegration(t, ctx, db, seededAt)

	reads, err := operator.NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}
	recovery, err := delivery.NewPostgresStore(db, clock.Now)
	if err != nil {
		t.Fatalf("delivery.NewPostgresStore: %v", err)
	}
	controlPlane, err := operator.NewService(reads, recovery, catalog, operatorTenant)
	if err != nil {
		t.Fatalf("operator.NewService: %v", err)
	}

	issuer, err := oidctest.New()
	if err != nil {
		t.Fatalf("new OIDC issuer: %v", err)
	}
	t.Cleanup(issuer.Close)
	authenticator, err := requestsecurity.NewOIDCAuthenticator(issuer.Context(), requestsecurity.OIDCConfig{
		IssuerURL: issuer.IssuerURL(),
		Audience:  "fi-fhir-graphql",
		TenantID:  operatorTenant,
	})
	if err != nil {
		t.Fatalf("NewOIDCAuthenticator: %v", err)
	}
	config := graphqlapi.DefaultServerConfig()
	config.PlaygroundEnabled = false
	config.AllowedOrigins = []string{"https://ide.example.test"}
	config.MaxRequestBodyBytes = 16 * 1024
	config.Authenticator = authenticator
	server, err := graphqlapi.NewServer(
		resolvers.NewResolver(resolvers.WithOperatorControlPlane(controlPlane)),
		config,
	)
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}

	bodies := &responseRecorder{}
	client := &operatorClient{
		t:       t,
		handler: server.Handler(),
		path:    config.Path,
		issuer:  issuer,
		bodies:  bodies,
	}

	operatorRoles := []string{
		graphqlapi.GraphQLOperatorRole,
		operator.ReadRole,
		delivery.OperatorRole,
		operator.DeploymentOperatorRole,
	}

	// ---------------------------------------------------------------------
	// Journey 5 (operator audit): browse durable records through GraphQL only.
	// ---------------------------------------------------------------------
	receiptsBody := client.query(operatorRoles, `
		query {
			operatorReceipts(page: {first: 50}) {
				nodes {
					tenantId receiptId status correlationId rawRetentionMode
					integrationRevision { artifactId revisionId digest }
					principal { id kind authMethod roles }
					eventCount attemptCount failedAttemptCount deadLetterCount
				}
				pageInfo { hasNextPage endCursor }
			}
		}`)
	receipts := receiptsBody.path("data", "operatorReceipts", "nodes").array()
	if len(receipts) != 1 {
		t.Fatalf("operator saw %d receipts, want exactly this tenant's 1: %s", len(receipts), receiptsBody.raw)
	}
	firstReceipt := jsonValue{receipts[0]}
	if got := firstReceipt.path("receiptId").str(); got != "receipt-a" {
		t.Fatalf("receipt id = %q, want receipt-a", got)
	}
	if got := firstReceipt.path("tenantId").str(); got != operatorTenant {
		t.Fatalf("receipt tenant = %q, want %q", got, operatorTenant)
	}
	if got := firstReceipt.path("deadLetterCount").number(); got != 1 {
		t.Fatalf("receipt deadLetterCount = %v, want 1", got)
	}
	if got := firstReceipt.path("failedAttemptCount").number(); got != 1 {
		t.Fatalf("receipt failedAttemptCount = %v, want 1", got)
	}

	traceBody := client.query(operatorRoles, `
		query {
			operatorMessageTrace(receiptId: "receipt-a") {
				receipt { receiptId correlationId }
				events {
					eventId eventType sourceMessageId correlationId classification
					payloadFields { path kind repeated }
					payloadTruncated
				}
				lineage {
					lineageId traceId sourceMessageId
					artifactRevisions {
						source { artifactId revisionId digest }
						profile { artifactId revisionId digest }
						workflow { artifactId revisionId digest }
					}
					routes { route matched skipped transformCount plannedActions diagnosticCodes }
					diagnostics { severity stage code path classification }
				}
				attempts {
					attemptId status attemptCount outboxStatus lastErrorCode
					destination { artifactId revisionId digest class }
					deadLetter { attemptId active failureCode replayCount resolution }
				}
				audit { auditId attemptId eventKind reason principal { id } }
			}
		}`)
	trace := traceBody.path("data", "operatorMessageTrace")
	if trace.value == nil {
		t.Fatalf("operatorMessageTrace returned null: %s", traceBody.raw)
	}
	traceEvents := trace.path("events").array()
	if len(traceEvents) != 1 {
		t.Fatalf("trace events = %d, want 1: %s", len(traceEvents), traceBody.raw)
	}
	event := jsonValue{traceEvents[0]}
	if got := event.path("sourceMessageId").str(); got != "MSH10-A" {
		t.Fatalf("trace source message id = %q, want MSH10-A", got)
	}
	payloadPaths := make([]string, 0)
	for _, field := range event.path("payloadFields").array() {
		payloadPaths = append(payloadPaths, jsonValue{field}.path("path").str())
	}
	if len(payloadPaths) == 0 {
		t.Fatal("semantic payload rendering produced no field coordinates")
	}
	if !containsString(payloadPaths, "patient.mrn") {
		t.Fatalf("payload field coordinates = %v, want a patient.mrn coordinate", payloadPaths)
	}

	traceLineage := trace.path("lineage").array()
	if len(traceLineage) != 1 {
		t.Fatalf("trace lineage = %d, want 1: %s", len(traceLineage), traceBody.raw)
	}
	lineage := jsonValue{traceLineage[0]}
	if got := lineage.path("traceId").str(); got != "trace-a" {
		t.Fatalf("lineage trace id = %q, want trace-a", got)
	}
	if got := lineage.path("artifactRevisions", "workflow", "artifactId").str(); got != "workflow-adt" {
		t.Fatalf("lineage workflow artifact = %q, want workflow-adt", got)
	}
	if routes := lineage.path("routes").array(); len(routes) != 1 {
		t.Fatalf("lineage routes = %d, want 1", len(routes))
	}
	if diagnostics := lineage.path("diagnostics").array(); len(diagnostics) != 1 {
		t.Fatalf("lineage diagnostics = %d, want 1", len(diagnostics))
	}

	traceAttempts := trace.path("attempts").array()
	if len(traceAttempts) != 1 {
		t.Fatalf("trace attempts = %d, want 1", len(traceAttempts))
	}
	attempt := jsonValue{traceAttempts[0]}
	if attempt.path("status").str() != "failed" || attempt.path("outboxStatus").str() != "failed" {
		t.Fatalf("trace attempt state = %s", traceBody.raw)
	}
	if !attempt.path("deadLetter", "active").boolean() {
		t.Fatalf("trace attempt dead letter is not active: %s", traceBody.raw)
	}

	dlqBody := client.query(operatorRoles, `
		query {
			operatorDeadLetters(activeOnly: true, page: {first: 50}) {
				nodes { attemptId active failureCode failureDetail replayCount resolution }
				pageInfo { hasNextPage }
			}
		}`)
	deadLetters := dlqBody.path("data", "operatorDeadLetters", "nodes").array()
	if len(deadLetters) != 1 {
		t.Fatalf("DLQ browse = %s, want exactly this tenant's 1 entry", dlqBody.raw)
	}
	if got := (jsonValue{deadLetters[0]}).path("attemptId").str(); got != "attempt-a" {
		t.Fatalf("DLQ browse attempt = %q, want attempt-a", got)
	}

	circuitBody := client.query(operatorRoles, `
		query { operatorCircuits { state consecutiveFailures destination { artifactId } } }`)
	if circuits := circuitBody.path("data", "operatorCircuits").array(); len(circuits) != 1 {
		t.Fatalf("operator circuits = %s, want exactly this tenant's 1", circuitBody.raw)
	}

	// ---------------------------------------------------------------------
	// Journey 2 (failure and replay): replay once, with a reason, over GraphQL.
	// ---------------------------------------------------------------------
	beforeReplay := readDeliveryState(t, db, operatorTenant, "attempt-a")
	if beforeReplay.attemptStatus != "failed" || !beforeReplay.dlqActive {
		t.Fatalf("seeded state = %#v, want a failed, actively dead-lettered attempt", beforeReplay)
	}

	replayBody := client.query(operatorRoles, `
		mutation {
			replayDelivery(input: {
				attemptId: "attempt-a",
				reason: "Destination outage repaired; replaying once",
				idempotencyKey: "operator-replay-1"
			}) {
				kind sourceAttemptId resultAttemptId reason idempotencyKey
				actor { id kind authMethod }
				attempt { attemptId status attemptCount outboxStatus deadLetter { active resolution } }
			}
		}`)
	replay := replayBody.path("data", "replayDelivery")
	if replay.value == nil {
		t.Fatalf("replayDelivery returned no data: %s", replayBody.raw)
	}
	if got := replay.path("resultAttemptId").str(); got != "attempt-a" {
		t.Fatalf("replay result attempt = %q, want attempt-a (requeue in place)", got)
	}
	if got := replay.path("actor", "id").str(); got != "clinician-1" {
		t.Fatalf("replay actor = %q, want the verified token subject", got)
	}
	if got := replay.path("attempt", "status").str(); got != "queued" {
		t.Fatalf("replayed attempt status = %q, want queued", got)
	}
	if replay.path("attempt", "deadLetter", "active").boolean() {
		t.Fatalf("replay did not resolve the dead letter: %s", replayBody.raw)
	}
	if got := replay.path("attempt", "deadLetter", "resolution").str(); got != "replayed" {
		t.Fatalf("dead letter resolution = %q, want replayed", got)
	}

	afterReplay := readDeliveryState(t, db, operatorTenant, "attempt-a")
	if afterReplay.attemptStatus != "queued" || afterReplay.outboxStatus != "pending" || afterReplay.dlqActive {
		t.Fatalf("durable state after replay = %#v", afterReplay)
	}

	// The append-only audit trail carries actor and reason; the idempotent
	// operation ledger carries the key that makes a repeat a no-op.
	auditRows := readAudit(t, db, operatorTenant, "attempt-a", "replayed")
	if len(auditRows) != 1 {
		t.Fatalf("replay audit rows = %d, want exactly 1", len(auditRows))
	}
	if auditRows[0].principalID != "clinician-1" {
		t.Fatalf("audit principal = %q, want the verified token subject", auditRows[0].principalID)
	}
	if auditRows[0].reason != "Destination outage repaired; replaying once" {
		t.Fatalf("audit reason = %q", auditRows[0].reason)
	}
	if !containsString(auditRows[0].principalRoles, delivery.OperatorRole) {
		t.Fatalf("audit principal roles = %v, want the delivery operator role", auditRows[0].principalRoles)
	}
	operations := readOperations(t, db, operatorTenant)
	if len(operations) != 1 {
		t.Fatalf("operation ledger rows = %d, want 1", len(operations))
	}
	if operations[0].kind != "replay" || operations[0].idempotencyKey != "operator-replay-1" ||
		operations[0].reason != "Destination outage repaired; replaying once" ||
		operations[0].principalID != "clinician-1" {
		t.Fatalf("operation ledger row = %#v", operations[0])
	}

	// Same key, same request: the durable machinery must not execute twice.
	duplicateBody := client.query(operatorRoles, `
		mutation {
			replayDelivery(input: {
				attemptId: "attempt-a",
				reason: "Destination outage repaired; replaying once",
				idempotencyKey: "operator-replay-1"
			}) { resultAttemptId attempt { attemptId status attemptCount } }
		}`)
	if got := duplicateBody.path("data", "replayDelivery", "resultAttemptId").str(); got != "attempt-a" {
		t.Fatalf("duplicate replay result = %s", duplicateBody.raw)
	}
	afterDuplicate := readDeliveryState(t, db, operatorTenant, "attempt-a")
	if afterDuplicate != afterReplay {
		t.Fatalf("duplicate replay changed durable state: %#v -> %#v", afterReplay, afterDuplicate)
	}
	if rows := readAudit(t, db, operatorTenant, "attempt-a", "replayed"); len(rows) != 1 {
		t.Fatalf("duplicate replay appended %d audit rows, want the original 1", len(rows))
	}
	if ops := readOperations(t, db, operatorTenant); len(ops) != 1 {
		t.Fatalf("duplicate replay created %d operation rows, want 1", len(ops))
	}

	// Reusing one key for a different action must be refused, not silently
	// reinterpreted.
	conflictBody := client.query(operatorRoles, `
		mutation {
			resubmitMessage(input: {
				attemptId: "attempt-a",
				reason: "Trying to reuse a spent key",
				idempotencyKey: "operator-replay-1"
			}) { resultAttemptId }
		}`)
	if !conflictBody.hasError("idempotency conflict") {
		t.Fatalf("reused idempotency key was accepted: %s", conflictBody.raw)
	}

	auditPageBody := client.query(operatorRoles, `
		query {
			operatorAttemptAudit(attemptId: "attempt-a", page: {first: 50}) {
				nodes { eventKind reason principal { id roles } detail }
				pageInfo { hasNextPage }
			}
		}`)
	auditKinds := make([]string, 0)
	for _, record := range auditPageBody.path("data", "operatorAttemptAudit", "nodes").array() {
		auditKinds = append(auditKinds, jsonValue{record}.path("eventKind").str())
	}
	if !containsString(auditKinds, "replayed") || !containsString(auditKinds, "dlq_entered") {
		t.Fatalf("audit browse kinds = %v, want the durable dlq_entered and replayed history", auditKinds)
	}

	// ---------------------------------------------------------------------
	// Deployment/channel controls: reason + expected version, conflicts surfaced.
	// ---------------------------------------------------------------------
	deploymentsBody := client.query(operatorRoles, `
		query {
			operatorDeployments {
				definitionRevision { artifactId revisionId digest }
				state version health validationPassed
				updatedBy { id } updatedReason
			}
		}`)
	deployments := deploymentsBody.path("data", "operatorDeployments").array()
	if len(deployments) != 1 {
		t.Fatalf("operator deployments = %s, want this tenant's 1", deploymentsBody.raw)
	}
	deployed := jsonValue{deployments[0]}
	if got := deployed.path("state").str(); got != "deployed" {
		t.Fatalf("seeded deployment state = %q, want deployed", got)
	}
	deployedVersion := int(deployed.path("version").number())

	staleBody := client.query(operatorRoles, fmt.Sprintf(`
		mutation {
			pauseIntegrationDeployment(input: {
				definitionId: %q, revisionId: %q,
				expectedVersion: %d, reason: "Stale operator view"
			}) { state version }
		}`, revision.DefinitionID, revision.RevisionID, deployedVersion-1))
	if !staleBody.hasError("version conflict") {
		t.Fatalf("stale expected version was accepted: %s", staleBody.raw)
	}

	pauseBody := client.query(operatorRoles, fmt.Sprintf(`
		mutation {
			pauseIntegrationDeployment(input: {
				definitionId: %q, revisionId: %q,
				expectedVersion: %d, reason: "Destination outage: pausing intake"
			}) { state version updatedBy { id kind authMethod } updatedReason }
		}`, revision.DefinitionID, revision.RevisionID, deployedVersion))
	paused := pauseBody.path("data", "pauseIntegrationDeployment")
	if got := paused.path("state").str(); got != "paused" {
		t.Fatalf("pause state = %s", pauseBody.raw)
	}
	if got := paused.path("updatedBy", "id").str(); got != "clinician-1" {
		t.Fatalf("pause actor = %q, want the verified token subject", got)
	}
	if got := paused.path("updatedReason").str(); got != "Destination outage: pausing intake" {
		t.Fatalf("pause reason = %q", got)
	}
	pausedVersion := int(paused.path("version").number())

	resumeBody := client.query(operatorRoles, fmt.Sprintf(`
		mutation {
			resumeIntegrationDeployment(input: {
				definitionId: %q, revisionId: %q,
				expectedVersion: %d, reason: "Destination restored: resuming intake"
			}) { state version }
		}`, revision.DefinitionID, revision.RevisionID, pausedVersion))
	if got := resumeBody.path("data", "resumeIntegrationDeployment", "state").str(); got != "deployed" {
		t.Fatalf("resume state = %s", resumeBody.raw)
	}

	eventsBody := client.query(operatorRoles, fmt.Sprintf(`
		query {
			operatorDeploymentEvents(definitionId: %q, revisionId: %q) {
				version action fromState toState reason actor { id }
			}
		}`, revision.DefinitionID, revision.RevisionID))
	actions := make([]string, 0)
	for _, record := range eventsBody.path("data", "operatorDeploymentEvents").array() {
		actions = append(actions, jsonValue{record}.path("action").str())
	}
	if !containsString(actions, "pause") || !containsString(actions, "resume") {
		t.Fatalf("lifecycle history actions = %v, want the pause and resume records", actions)
	}

	// ---------------------------------------------------------------------
	// Authorization: unprivileged roles never reach control-plane data.
	// ---------------------------------------------------------------------
	previewOnly := client.query([]string{"integration:preview"}, `
		query { operatorReceipts(page: {first: 5}) { nodes { receiptId } } }`)
	if !previewOnly.hasErrorCode("FORBIDDEN") {
		t.Fatalf("preview role was not stopped by operation authorization: %s", previewOnly.raw)
	}
	if strings.Contains(previewOnly.raw, "receipt-a") {
		t.Fatalf("preview role saw resolver data: %s", previewOnly.raw)
	}

	// A caller that clears the legacy GraphQL gate but lacks the control-plane
	// role must still be refused before any record is read.
	noOperatorRole := client.query([]string{graphqlapi.GraphQLOperatorRole}, `
		query { operatorReceipts(page: {first: 5}) { nodes { receiptId } } }`)
	if !noOperatorRole.hasError("forbidden") {
		t.Fatalf("missing operator read role was accepted: %s", noOperatorRole.raw)
	}
	if strings.Contains(noOperatorRole.raw, "receipt-a") {
		t.Fatalf("unprivileged role saw resolver data: %s", noOperatorRole.raw)
	}

	readOnly := client.query([]string{graphqlapi.GraphQLOperatorRole, operator.ReadRole}, `
		mutation {
			replayDelivery(input: {
				attemptId: "attempt-a", reason: "Read-only operator",
				idempotencyKey: "operator-replay-readonly"
			}) { resultAttemptId }
		}`)
	if !readOnly.hasError("forbidden") {
		t.Fatalf("read-only operator performed a control action: %s", readOnly.raw)
	}
	if ops := readOperations(t, db, operatorTenant); len(ops) != 1 {
		t.Fatalf("refused control action wrote an operation row: %d", len(ops))
	}

	// ---------------------------------------------------------------------
	// Tenant isolation: another tenant's failure is invisible and unmutatable.
	// ---------------------------------------------------------------------
	crossTenantTrace := client.query(operatorRoles, `
		query { operatorMessageTrace(receiptId: "receipt-b") { receipt { receiptId } } }`)
	if crossTenantTrace.path("data", "operatorMessageTrace").value != nil {
		t.Fatalf("cross-tenant trace returned data: %s", crossTenantTrace.raw)
	}
	crossTenantAttempt := client.query(operatorRoles, `
		query { operatorDeliveryAttempt(attemptId: "attempt-b") { attemptId status } }`)
	if crossTenantAttempt.path("data", "operatorDeliveryAttempt").value != nil {
		t.Fatalf("cross-tenant attempt returned data: %s", crossTenantAttempt.raw)
	}

	beforeCrossTenantWrite := readDeliveryState(t, db, "tenant-b", "attempt-b")
	crossTenantReplay := client.query(operatorRoles, `
		mutation {
			replayDelivery(input: {
				attemptId: "attempt-b", reason: "Cross-tenant replay attempt",
				idempotencyKey: "operator-replay-cross"
			}) { resultAttemptId }
		}`)
	if !crossTenantReplay.hasError("not dead-lettered") {
		t.Fatalf("cross-tenant replay was not refused: %s", crossTenantReplay.raw)
	}
	afterCrossTenantWrite := readDeliveryState(t, db, "tenant-b", "attempt-b")
	if beforeCrossTenantWrite != afterCrossTenantWrite {
		t.Fatalf("cross-tenant replay changed another tenant's durable state: %#v -> %#v",
			beforeCrossTenantWrite, afterCrossTenantWrite)
	}
	if ops := readOperations(t, db, "tenant-b"); len(ops) != 0 {
		t.Fatalf("cross-tenant replay wrote %d operation rows for the other tenant", len(ops))
	}

	// A token asserting another tenant never authenticates at all.
	crossTenantToken := client.rawQuery(func(claims map[string]any) {
		claims["tenant_id"] = "tenant-b"
		claims["roles"] = operatorRoles
	}, `query { operatorReceipts(page: {first: 5}) { nodes { receiptId } } }`)
	if crossTenantToken.status != http.StatusUnauthorized {
		t.Fatalf("cross-tenant token status = %d, want 401: %s", crossTenantToken.status, crossTenantToken.raw)
	}

	// ---------------------------------------------------------------------
	// Resubmit forks one idempotent child attempt from the same dead letter.
	// ---------------------------------------------------------------------
	failAttemptAgain(t, db, operatorTenant, "attempt-a", "outbox-a", clock.Now())
	resubmitBody := client.query(operatorRoles, `
		mutation {
			resubmitMessage(input: {
				attemptId: "attempt-a",
				reason: "Vendor asked for a fresh delivery attempt",
				idempotencyKey: "operator-resubmit-1"
			}) {
				kind sourceAttemptId resultAttemptId
				attempt { attemptId parentAttemptId status attemptCount outboxStatus }
			}
		}`)
	resubmit := resubmitBody.path("data", "resubmitMessage")
	childAttemptID := resubmit.path("resultAttemptId").str()
	if childAttemptID == "" || childAttemptID == "attempt-a" {
		t.Fatalf("resubmit did not fork a child attempt: %s", resubmitBody.raw)
	}
	if got := resubmit.path("attempt", "parentAttemptId").str(); got != "attempt-a" {
		t.Fatalf("child attempt parent = %q, want attempt-a", got)
	}
	if got := resubmit.path("attempt", "status").str(); got != "queued" {
		t.Fatalf("child attempt status = %q, want queued", got)
	}
	sourceState := readDeliveryState(t, db, operatorTenant, "attempt-a")
	if sourceState.dlqActive || sourceState.dlqResolution != "resubmitted" {
		t.Fatalf("resubmit dead-letter resolution = %#v, want an inactive resubmitted entry", sourceState)
	}
	if rows := readAudit(t, db, operatorTenant, childAttemptID, "resubmitted"); len(rows) != 1 ||
		rows[0].principalID != "clinician-1" {
		t.Fatalf("resubmit audit rows = %#v", rows)
	}
	duplicateResubmit := client.query(operatorRoles, `
		mutation {
			resubmitMessage(input: {
				attemptId: "attempt-a",
				reason: "Vendor asked for a fresh delivery attempt",
				idempotencyKey: "operator-resubmit-1"
			}) { resultAttemptId }
		}`)
	if got := duplicateResubmit.path("data", "resubmitMessage", "resultAttemptId").str(); got != childAttemptID {
		t.Fatalf("duplicate resubmit forked a second child: %s", duplicateResubmit.raw)
	}
	if ops := readOperations(t, db, operatorTenant); len(ops) != 2 {
		t.Fatalf("operation ledger rows = %d, want the replay and resubmit records only", len(ops))
	}

	// ---------------------------------------------------------------------
	// Discard closes the remaining path with the same attributable ledger.
	// ---------------------------------------------------------------------
	failAttemptAgain(t, db, operatorTenant, "attempt-a", "outbox-a", clock.Now())
	discardBody := client.query(operatorRoles, `
		mutation {
			discardDeadLetter(input: {
				attemptId: "attempt-a",
				reason: "Duplicate of a message already delivered by the vendor",
				idempotencyKey: "operator-discard-1"
			}) {
				kind resultAttemptId reason actor { id }
				attempt { status deadLetter { active resolution } }
			}
		}`)
	discard := discardBody.path("data", "discardDeadLetter")
	if got := discard.path("attempt", "deadLetter", "resolution").str(); got != "discarded" {
		t.Fatalf("discard resolution = %s", discardBody.raw)
	}
	if discard.path("attempt", "deadLetter", "active").boolean() {
		t.Fatalf("discard left the dead letter active: %s", discardBody.raw)
	}
	if got := discard.path("attempt", "status").str(); got != "failed" {
		t.Fatalf("discard requeued the attempt: %s", discardBody.raw)
	}
	if rows := readAudit(t, db, operatorTenant, "attempt-a", "discarded"); len(rows) != 1 ||
		rows[0].principalID != "clinician-1" ||
		rows[0].reason != "Duplicate of a message already delivered by the vendor" {
		t.Fatalf("discard audit rows = %#v", rows)
	}

	// ---------------------------------------------------------------------
	// Raw-PHI sentinel: it is in the durable payload and in no response body.
	// ---------------------------------------------------------------------
	assertSentinelIsDurablyStored(t, db)
	if leaked := bodies.leaks(rawPHISentinel); leaked != "" {
		t.Fatalf("a GraphQL response exposed the raw-PHI sentinel: %s", leaked)
	}
	if bodies.count() < 15 {
		t.Fatalf("sentinel scan covered only %d responses; the journey did not run", bodies.count())
	}
}

// --------------------------------------------------------------------------
// GraphQL client
// --------------------------------------------------------------------------

type operatorClient struct {
	t       *testing.T
	handler http.Handler
	path    string
	issuer  *oidctest.Fixture
	bodies  *responseRecorder
}

func (c *operatorClient) query(roles []string, document string) graphQLResponse {
	c.t.Helper()
	return c.rawQuery(func(claims map[string]any) { claims["roles"] = roles }, document)
}

func (c *operatorClient) rawQuery(mutate func(map[string]any), document string) graphQLResponse {
	c.t.Helper()
	claims := c.issuer.Claims()
	if mutate != nil {
		mutate(claims)
	}
	token, err := c.issuer.Sign(claims, "RS256")
	if err != nil {
		c.t.Fatalf("sign operator token: %v", err)
	}
	payload, err := json.Marshal(map[string]any{"query": document})
	if err != nil {
		c.t.Fatalf("marshal GraphQL request: %v", err)
	}
	request := httptest.NewRequest(http.MethodPost, c.path, bytes.NewReader(payload))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Authorization", "Bearer "+token)
	recorder := httptest.NewRecorder()
	c.handler.ServeHTTP(recorder, request)

	body := recorder.Body.String()
	c.bodies.add(body)
	response := graphQLResponse{raw: body, status: recorder.Code}
	if recorder.Code == http.StatusOK {
		var decoded map[string]any
		if err := json.Unmarshal(recorder.Body.Bytes(), &decoded); err != nil {
			c.t.Fatalf("decode GraphQL response: %v (%s)", err, body)
		}
		response.decoded = decoded
	}
	return response
}

type graphQLResponse struct {
	raw     string
	status  int
	decoded map[string]any
}

func (r graphQLResponse) path(keys ...string) jsonValue {
	var current any = r.decoded
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return jsonValue{}
		}
		current = object[key]
	}
	return jsonValue{current}
}

func (r graphQLResponse) hasError(fragment string) bool {
	errorsValue, ok := r.decoded["errors"].([]any)
	if !ok || len(errorsValue) == 0 {
		return false
	}
	return strings.Contains(strings.ToLower(r.raw), strings.ToLower(fragment))
}

func (r graphQLResponse) hasErrorCode(code string) bool {
	return strings.Contains(r.raw, `"code":"`+code+`"`)
}

type jsonValue struct{ value any }

func (v jsonValue) path(keys ...string) jsonValue {
	current := v.value
	for _, key := range keys {
		object, ok := current.(map[string]any)
		if !ok {
			return jsonValue{}
		}
		current = object[key]
	}
	return jsonValue{current}
}

func (v jsonValue) array() []any {
	values, _ := v.value.([]any)
	return values
}

func (v jsonValue) str() string {
	text, _ := v.value.(string)
	return text
}

func (v jsonValue) number() float64 {
	number, _ := v.value.(float64)
	return number
}

func (v jsonValue) boolean() bool {
	flag, _ := v.value.(bool)
	return flag
}

type responseRecorder struct {
	mu     sync.Mutex
	bodies []string
}

func (r *responseRecorder) add(body string) {
	r.mu.Lock()
	r.bodies = append(r.bodies, body)
	r.mu.Unlock()
}

func (r *responseRecorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.bodies)
}

func (r *responseRecorder) leaks(sentinel string) string {
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, body := range r.bodies {
		if strings.Contains(body, sentinel) {
			return body
		}
	}
	return ""
}

// --------------------------------------------------------------------------
// Durable fixtures and direct-SQL assertions (test-only, never in the journey)
// --------------------------------------------------------------------------

type operatorClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *operatorClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(time.Second)
	return c.now
}

func openOperatorDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		t.Skip("POSTGRES_TEST_URL is required for operator control-plane integration tests")
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open PostgreSQL admin: %v", err)
	}
	schema := fmt.Sprintf("operator_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		_ = admin.Close()
		t.Fatalf("create operator schema: %v", err)
	}
	t.Cleanup(func() {
		_, _ = admin.ExecContext(context.Background(), `DROP SCHEMA IF EXISTS `+schema+` CASCADE`)
		_ = admin.Close()
	})
	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse PostgreSQL URL: %v", err)
	}
	query := parsed.Query()
	query.Set("search_path", schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatalf("open PostgreSQL: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	db.SetMaxOpenConns(8)
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	return db
}

func seedFailedDelivery(t *testing.T, db *sql.DB, tenantID, receiptID, attemptID, outboxID string, now time.Time) {
	t.Helper()
	destination := integration.DestinationRevisionRef{
		ArtifactRevisionRef: integration.ArtifactRevisionRef{
			ArtifactID: "destination-fhir", RevisionID: "destination-1",
			Digest: "sha256:" + strings.Repeat("4", 64),
		},
		Class: integration.DestinationClassProduction,
	}
	destinationJSON, err := json.Marshal(destination)
	if err != nil {
		t.Fatalf("marshal destination: %v", err)
	}
	revisionJSON, err := json.Marshal(integration.ArtifactRevisionRef{
		ArtifactID: "adt-http", RevisionID: "rev-1", Digest: "sha256:" + strings.Repeat("0", 64),
	})
	if err != nil {
		t.Fatalf("marshal integration revision: %v", err)
	}
	principalJSON, err := json.Marshal(integration.Principal{
		ID: "adt-gateway", Kind: integration.PrincipalKindService,
		AuthMethod: "oauth2-client-credentials", Roles: []string{"integration:submit"},
	})
	if err != nil {
		t.Fatalf("marshal principal: %v", err)
	}
	// The canonical event payload holds the raw-PHI sentinel as a VALUE.
	eventPayload := fmt.Sprintf(
		`{"id":%q,"type":"patient_admit","patient":{"mrn":%q,"name":{"family":%q}},"encounter":{"class":"inpatient"}}`,
		"event-"+attemptID, rawPHISentinel, rawPHISentinel,
	)
	artifactsJSON, err := json.Marshal(integration.ExecutionArtifactRevisions{
		Source:   integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: "sha256:" + strings.Repeat("1", 64)},
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: "sha256:" + strings.Repeat("2", 64)},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: "sha256:" + strings.Repeat("3", 64)},
	})
	if err != nil {
		t.Fatalf("marshal artifact revisions: %v", err)
	}
	// Routes and diagnostics are marshalled from the production contract types
	// so the seeded rows are byte-compatible with what the shared processor
	// persists, including the diagnostic message/code invariant.
	routesJSON, err := json.Marshal([]integration.RouteResult{{
		TenantID: tenantID, EventID: "event-" + attemptID, Route: "admit",
		Matched: true, TransformCount: 2,
		PlannedActions: []string{"send-fhir"}, DiagnosticCodes: []string{"MISSING_PV1"},
	}})
	if err != nil {
		t.Fatalf("marshal routes: %v", err)
	}
	diagnostic, err := integration.NewDiagnostic(integration.DiagnosticInput{
		TenantID: tenantID, Severity: integration.DiagnosticSeverityWarning,
		Stage: "parse", Code: "MISSING_PV1", Path: "PID-7",
		Classification: integration.DataClassificationPHI,
	})
	if err != nil {
		t.Fatalf("NewDiagnostic: %v", err)
	}
	diagnosticsJSON, err := json.Marshal([]integration.Diagnostic{diagnostic})
	if err != nil {
		t.Fatalf("marshal diagnostics: %v", err)
	}
	commandPayload := fmt.Sprintf(
		`{"schema":"integration.delivery.v1","tenant_id":%q,"receipt_id":%q,"event_id":%q,"trace_id":"trace-a","attempt_id":%q,"destination":%s,"route":"admit","action":"send-fhir","attempt_count":2}`,
		tenantID, receiptID, "event-"+attemptID, attemptID, destinationJSON,
	)

	tx, err := db.Begin()
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []struct {
		name string
		sql  string
		args []any
	}{
		{"receipt", `
			INSERT INTO integration_receipts (
				tenant_id, receipt_id, idempotency_key, request_fingerprint,
				integration_revision, status, recorded_at, correlation_id,
				raw_retention_mode, principal_json, reason, result_json
			) VALUES ($1, $2, $3, 'fingerprint', $4, 'accepted', $5, $6,
				'ephemeral', $7, 'production ingress', '{}')`,
			[]any{tenantID, receiptID, "key-" + receiptID, revisionJSON, now,
				"correlation-" + receiptID, principalJSON}},
		{"event", `
			INSERT INTO integration_canonical_events (
				tenant_id, event_id, receipt_id, event_type, source_message_id,
				correlation_id, classification, payload_json, recorded_at
			) VALUES ($1, $2, $3, 'patient_admit', $4, $5, 'phi', $6, $7)`,
			[]any{tenantID, "event-" + attemptID, receiptID, "MSH10-" + strings.ToUpper(strings.TrimPrefix(tenantID, "tenant-")),
				"correlation-" + receiptID, eventPayload, now}},
		{"lineage", `
			INSERT INTO integration_message_lineage (
				tenant_id, lineage_id, receipt_id, event_id, trace_id, correlation_id,
				source_message_id, artifact_revisions_json, routes_json,
				diagnostics_json, recorded_at
			) VALUES ($1, $2, $3, $4, 'trace-a', $5, $6, $7, $8, $9, $10)`,
			[]any{tenantID, "lineage-" + attemptID, receiptID, "event-" + attemptID,
				"correlation-" + receiptID,
				"MSH10-" + strings.ToUpper(strings.TrimPrefix(tenantID, "tenant-")),
				artifactsJSON, routesJSON, diagnosticsJSON, now}},
		{"attempt", `
			INSERT INTO integration_delivery_attempts (
				tenant_id, attempt_id, receipt_id, event_id, trace_id,
				destination_revision_json, route_name, action_id, status,
				attempt_count, recorded_at, scheduled_at, completed_at,
				last_error_code, last_error_detail
			) VALUES ($1, $2, $3, $4, 'trace-a', $5, 'admit', 'send-fhir', 'failed',
				2, $6, $6, $6, 'DESTINATION_UNAVAILABLE',
				'FHIR destination refused the connection')`,
			[]any{tenantID, attemptID, receiptID, "event-" + attemptID, destinationJSON, now}},
		{"outbox", `
			INSERT INTO integration_delivery_outbox (
				tenant_id, outbox_id, attempt_id, topic, status, payload_json,
				created_at, scheduled_at, updated_at
			) VALUES ($1, $2, $3, 'integration.delivery.v1', 'failed', $4, $5, $5, $5)`,
			[]any{tenantID, outboxID, attemptID, commandPayload, now}},
		{"dlq", `
			INSERT INTO integration_delivery_dlq (
				tenant_id, attempt_id, outbox_id, failure_code, failure_detail,
				failed_at, active, replay_count, resolution, resolved_at
			) VALUES ($1, $2, $3, 'DESTINATION_UNAVAILABLE',
				'FHIR destination refused the connection', $4, true, 0, '', NULL)`,
			[]any{tenantID, attemptID, outboxID, now}},
		{"circuit", `
			INSERT INTO integration_delivery_circuits (
				tenant_id, destination_artifact_id, destination_revision_id,
				destination_digest, state, consecutive_failures, open_until, updated_at
			) VALUES ($1, 'destination-fhir', 'destination-1', $2, 'closed', 3, NULL, $3)`,
			[]any{tenantID, "sha256:" + strings.Repeat("4", 64), now}},
		{"dlq audit", `
			INSERT INTO integration_delivery_audit (
				tenant_id, attempt_id, event_kind, attempt_count, principal_json,
				reason, detail_json, recorded_at
			) VALUES ($1, $2, 'dlq_entered', 2, '{}', '',
				jsonb_build_object('code', 'DESTINATION_UNAVAILABLE'), $3)`,
			[]any{tenantID, attemptID, now}},
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement.sql, statement.args...); err != nil {
			t.Fatalf("seed %s for %s: %v", statement.name, tenantID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed for %s: %v", tenantID, err)
	}
}

// failAttemptAgain returns a replayed attempt to the dead-letter queue so the
// discard path has an active dead letter to close.
func failAttemptAgain(t *testing.T, db *sql.DB, tenantID, attemptID, outboxID string, now time.Time) {
	t.Helper()
	statements := []string{
		`UPDATE integration_delivery_attempts SET status = 'failed', completed_at = $3
		 WHERE tenant_id = $1 AND attempt_id = $2`,
		`UPDATE integration_delivery_outbox SET status = 'failed', lease_owner = '',
		 lease_expires_at = NULL, updated_at = $3 WHERE tenant_id = $1 AND outbox_id = $2`,
	}
	for _, statement := range statements {
		id := attemptID
		if strings.Contains(statement, "outbox") {
			id = outboxID
		}
		if _, err := db.Exec(statement, tenantID, id, now); err != nil {
			t.Fatalf("re-fail delivery: %v", err)
		}
	}
	if _, err := db.Exec(`
		UPDATE integration_delivery_dlq
		SET active = true, resolution = '', resolved_at = NULL, failed_at = $3
		WHERE tenant_id = $1 AND attempt_id = $2
	`, tenantID, attemptID, now); err != nil {
		t.Fatalf("reopen dead letter: %v", err)
	}
}

func seedDeployedIntegration(
	t *testing.T,
	ctx context.Context,
	db *sql.DB,
	now time.Time,
) (*lifecycle.PostgresCatalog, integration.IntegrationDefinitionRevision) {
	t.Helper()
	catalog, err := lifecycle.NewPostgresCatalog(db, lifecycle.Config{
		Clock: func() time.Time { return now },
		ValidateConnection: func(context.Context, integration.IntegrationDefinitionRevision) (lifecycle.ConnectionValidationOutcome, error) {
			return lifecycle.ConnectionValidationOutcome{Passed: true, Codes: []string{"SOURCE_REACHABLE"}}, nil
		},
	})
	if err != nil {
		t.Fatalf("NewPostgresCatalog: %v", err)
	}
	if err := catalog.Migrate(ctx); err != nil {
		t.Fatalf("migrate lifecycle catalog: %v", err)
	}
	revision := operatorLifecycleRevision(t, now)
	if _, err := catalog.CreateDraft(ctx, revision); err != nil {
		t.Fatalf("CreateDraft: %v", err)
	}
	steps := []struct {
		name string
		run  func(lifecycle.Command) (lifecycle.Snapshot, error)
	}{
		{"validate", func(command lifecycle.Command) (lifecycle.Snapshot, error) {
			return catalog.ValidateConnection(ctx, command)
		}},
		{"approve", func(command lifecycle.Command) (lifecycle.Snapshot, error) {
			return catalog.Approve(ctx, command)
		}},
		{"publish", func(command lifecycle.Command) (lifecycle.Snapshot, error) {
			return catalog.Publish(ctx, command)
		}},
		{"deploy", func(command lifecycle.Command) (lifecycle.Snapshot, error) {
			return catalog.Deploy(ctx, command)
		}},
	}
	version := int64(1)
	for _, step := range steps {
		snapshot, err := step.run(lifecycle.Command{
			TenantID: revision.TenantID, DefinitionID: revision.DefinitionID,
			RevisionID: revision.RevisionID, ExpectedVersion: version,
			Principal: integration.Principal{
				ID: "engineer-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"integration:engineer"},
			},
			Reason: "seed deployed integration for the operator control plane",
		})
		if err != nil {
			t.Fatalf("lifecycle %s: %v", step.name, err)
		}
		version = snapshot.Version
	}
	return catalog, revision
}

func operatorLifecycleRevision(t *testing.T, createdAt time.Time) integration.IntegrationDefinitionRevision {
	t.Helper()
	digest := func(value byte) string { return "sha256:" + strings.Repeat(string(value), 64) }
	policy := integration.IntegrationDeploymentPolicy{
		ConnectionValidation: integration.ConnectionValidationPolicy{TimeoutSeconds: 5, MaxAgeSeconds: 86400},
		Schedule:             integration.SchedulePolicy{Mode: integration.ScheduleModeContinuous},
		Health: integration.HealthPolicy{
			StartupGraceSeconds: 30, CheckIntervalSeconds: 15,
			TimeoutSeconds: 5, FailureThreshold: 3,
		},
		Capacity: integration.CapacityPolicy{
			MaxInFlight: 32, MaxQueued: 1024, MaxMessagesPerSecond: 250,
		},
	}
	revision, err := integration.NewIntegrationDefinitionRevision(integration.IntegrationDefinitionRevisionInput{
		DefinitionID: "adt-http", RevisionID: "rev-1", TenantID: operatorTenant,
		Source: integration.SourceRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: digest('1')},
			SourceID:            "adt-east",
		},
		Format:   events.FormatHL7v2,
		Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: digest('2')},
		Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "workflow-1", Digest: digest('3')},
		Destinations: []integration.DestinationRevisionRef{{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{ArtifactID: "destination-fhir", RevisionID: "destination-1", Digest: digest('4')},
			Class:               integration.DestinationClassProduction,
		}},
		SecretBindings: []integration.SecretBinding{{
			Name: "source_credential",
			Reference: integration.SecretReference{
				Provider: integration.SecretProviderKubernetes,
				Key:      "fi-fhir/source-credential",
				Version:  "1",
			},
		}},
		Policy: integration.IntegrationPolicy{
			Classification: integration.DataClassificationPHI,
			RawRetention:   integration.RawRetentionPolicy{Mode: integration.RawRetentionModeEphemeral},
		},
		Deployment: &policy,
		Created: integration.AuditEnvelope{
			TenantID: operatorTenant,
			Principal: integration.Principal{
				ID: "engineer-1", Kind: integration.PrincipalKindHuman,
				AuthMethod: "oidc", Roles: []string{"integration:engineer"},
			},
			Reason: "create operator control-plane fixture", OccurredAt: createdAt,
		},
	})
	if err != nil {
		t.Fatalf("NewIntegrationDefinitionRevision: %v", err)
	}
	return revision
}

type deliveryState struct {
	attemptStatus string
	attemptCount  int
	outboxStatus  string
	dlqActive     bool
	dlqResolution string
	replayCount   int
}

func readDeliveryState(t *testing.T, db *sql.DB, tenantID, attemptID string) deliveryState {
	t.Helper()
	var state deliveryState
	if err := db.QueryRow(`
		SELECT a.status, a.attempt_count, o.status,
			COALESCE(d.active, false), COALESCE(d.resolution, ''),
			COALESCE(d.replay_count, 0)
		FROM integration_delivery_attempts a
		JOIN integration_delivery_outbox o
		  ON o.tenant_id = a.tenant_id AND o.attempt_id = a.attempt_id
		LEFT JOIN integration_delivery_dlq d
		  ON d.tenant_id = a.tenant_id AND d.attempt_id = a.attempt_id
		WHERE a.tenant_id = $1 AND a.attempt_id = $2
	`, tenantID, attemptID).Scan(
		&state.attemptStatus, &state.attemptCount, &state.outboxStatus,
		&state.dlqActive, &state.dlqResolution, &state.replayCount,
	); err != nil {
		t.Fatalf("read delivery state for %s/%s: %v", tenantID, attemptID, err)
	}
	return state
}

type auditRow struct {
	principalID    string
	principalRoles []string
	reason         string
}

func readAudit(t *testing.T, db *sql.DB, tenantID, attemptID, eventKind string) []auditRow {
	t.Helper()
	rows, err := db.Query(`
		SELECT principal_json, reason FROM integration_delivery_audit
		WHERE tenant_id = $1 AND attempt_id = $2 AND event_kind = $3
		ORDER BY audit_id
	`, tenantID, attemptID, eventKind)
	if err != nil {
		t.Fatalf("read audit rows: %v", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]auditRow, 0)
	for rows.Next() {
		var principalJSON []byte
		var record auditRow
		if err := rows.Scan(&principalJSON, &record.reason); err != nil {
			t.Fatalf("scan audit row: %v", err)
		}
		var principal integration.Principal
		if err := json.Unmarshal(principalJSON, &principal); err != nil {
			t.Fatalf("decode audit principal: %v", err)
		}
		record.principalID = principal.ID
		record.principalRoles = principal.Roles
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate audit rows: %v", err)
	}
	return records
}

type operationRow struct {
	kind           string
	idempotencyKey string
	reason         string
	principalID    string
	sourceAttempt  string
	resultAttempt  string
}

func readOperations(t *testing.T, db *sql.DB, tenantID string) []operationRow {
	t.Helper()
	rows, err := db.Query(`
		SELECT operation_kind, idempotency_key, reason, principal_json,
			source_attempt_id, result_attempt_id
		FROM integration_delivery_operations
		WHERE tenant_id = $1
		ORDER BY recorded_at, operation_id
	`, tenantID)
	if err != nil {
		t.Fatalf("read operation ledger: %v", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]operationRow, 0)
	for rows.Next() {
		var record operationRow
		var principalJSON []byte
		if err := rows.Scan(&record.kind, &record.idempotencyKey, &record.reason,
			&principalJSON, &record.sourceAttempt, &record.resultAttempt); err != nil {
			t.Fatalf("scan operation row: %v", err)
		}
		var principal integration.Principal
		if err := json.Unmarshal(principalJSON, &principal); err != nil {
			t.Fatalf("decode operation principal: %v", err)
		}
		record.principalID = principal.ID
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("iterate operation ledger: %v", err)
	}
	return records
}

// assertSentinelIsDurablyStored proves the leak test is real: the sentinel
// exists in the durable payload the operator browsed, so its absence from
// every response body is a property of the projection, not of the fixture.
func assertSentinelIsDurablyStored(t *testing.T, db *sql.DB) {
	t.Helper()
	var stored int
	if err := db.QueryRow(`
		SELECT COUNT(*) FROM integration_canonical_events
		WHERE payload_json::text LIKE '%' || $1 || '%'
	`, rawPHISentinel).Scan(&stored); err != nil {
		t.Fatalf("verify durable sentinel: %v", err)
	}
	if stored == 0 {
		t.Fatal("no seeded canonical event contains the raw-PHI sentinel; the leak test is vacuous")
	}
}

func containsString(values []string, wanted string) bool {
	for _, value := range values {
		if value == wanted {
			return true
		}
	}
	return false
}
