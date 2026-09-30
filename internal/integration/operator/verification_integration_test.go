//go:build integration

package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/requestsecurity"
	"gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/processor"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// verificationSentinel is planted as a VALUE in every seeded canonical event.
// The verification reads may describe the payload's structure and must never
// return its content.
const verificationSentinel = "VERIFICATION-PHI-SENTINEL-7Q4W"

// seededAdmission is one receipt, at most one canonical event, and at most one
// delivery attempt, written straight into the submission schema.
type seededAdmission struct {
	tenant        string
	receiptID     string
	status        string
	definition    integration.ArtifactRevisionRef
	recordedAt    time.Time
	eventType     string // "" = no canonical event
	withLineage   bool
	purgeAfter    *time.Time
	purgedAt      *time.Time
	attemptStatus string // "" = no attempt
	destination   string
}

// TestVerificationReads_BrowseAndStatisticsOverDurableAdmissions proves the two
// .loom/42 E-2 reads against the real submission schema: the browse filters,
// pages and joins exactly, hides tombstoned rows unless asked, never returns a
// payload value and never another tenant's row; the statistics count the same
// rows from columns over a half-open window, bucket them in UTC, and total
// what they bucket; and the migration that added their indexes is applied.
func TestVerificationReads_BrowseAndStatisticsOverDurableAdmissions(t *testing.T) {
	ctx := t.Context()
	db := openVerificationDatabase(t, ctx)
	submission, err := processor.NewPostgresSubmissionStore(db, processor.PostgresSubmissionConfig{})
	if err != nil {
		t.Fatalf("NewPostgresSubmissionStore: %v", err)
	}
	if err := submission.Migrate(ctx); err != nil {
		t.Fatalf("migrate submission schema: %v", err)
	}

	var applied int
	if err := db.QueryRowContext(ctx, `SELECT max(version) FROM integration_submission_schema_migrations`).Scan(&applied); err != nil {
		t.Fatalf("read submission ledger: %v", err)
	}
	if applied != processor.SchemaVersion || applied < 6 {
		t.Fatalf("submission ledger at %d, want SchemaVersion %d (>= 6)", applied, processor.SchemaVersion)
	}
	for _, index := range []string{"integration_canonical_events_browse_idx", "integration_canonical_events_receipt_idx"} {
		var present bool
		if err := db.QueryRowContext(ctx,
			`SELECT EXISTS (SELECT 1 FROM pg_indexes WHERE schemaname = current_schema() AND indexname = $1)`,
			index,
		).Scan(&present); err != nil || !present {
			t.Fatalf("index %s present = %v (%v)", index, present, err)
		}
	}

	t0 := time.Date(2026, 9, 28, 10, 15, 0, 0, time.UTC)
	purgeAfter := t0.Add(30 * 24 * time.Hour)
	purgedAt := t0.Add(3 * time.Hour)
	stampedAfter := t0.Add(90 * 24 * time.Hour)
	adt := integration.ArtifactRevisionRef{ArtifactID: "adt-http", RevisionID: "rev-1", Digest: "sha256:" + strings.Repeat("a", 64)}
	batch := integration.ArtifactRevisionRef{ArtifactID: "adt-batch", RevisionID: "v2", Digest: "sha256:" + strings.Repeat("b", 64)}
	admissions := []seededAdmission{
		{tenant: testTenant, receiptID: "receipt-1", status: "accepted", definition: adt, recordedAt: t0,
			eventType: "patient_admit", withLineage: true, attemptStatus: "queued", destination: "fhir-primary"},
		{tenant: testTenant, receiptID: "receipt-2", status: "accepted", definition: adt, recordedAt: t0.Add(time.Hour),
			eventType: "lab_result", purgeAfter: &stampedAfter, attemptStatus: "succeeded", destination: "fhir-primary"},
		{tenant: testTenant, receiptID: "receipt-3", status: "accepted", definition: batch, recordedAt: t0.Add(2 * time.Hour),
			eventType: "patient_admit", withLineage: true, purgeAfter: &purgeAfter, purgedAt: &purgedAt,
			attemptStatus: "failed", destination: "kafka-archive"},
		{tenant: testTenant, receiptID: "receipt-4", status: "rejected", definition: batch, recordedAt: t0.Add(25 * time.Hour)},
		// Another tenant's identical admission: isolation is data-scoped.
		{tenant: "tenant-b", receiptID: "receipt-b", status: "accepted", definition: adt, recordedAt: t0.Add(30 * time.Minute),
			eventType: "patient_admit", withLineage: true, attemptStatus: "queued", destination: "fhir-primary"},
	}
	for _, admission := range admissions {
		seedAdmission(t, ctx, db, admission)
	}

	reads, err := NewPostgresReadStore(db)
	if err != nil {
		t.Fatalf("NewPostgresReadStore: %v", err)
	}
	service, err := NewService(reads, &recordingLedger{}, &countingRecovery{}, &countingCatalog{}, testTenant)
	if err != nil {
		t.Fatalf("NewService: %v", err)
	}
	ctx = requestsecurity.WithSecurityContext(ctx, securityContext(testTenant, ReadRole))
	var responses []any

	browse := func(filter CanonicalEventFilter, page PageRequest) Page[CanonicalEventRecord] {
		t.Helper()
		result, err := service.ListCanonicalEvents(ctx, filter, page)
		if err != nil {
			t.Fatalf("ListCanonicalEvents(%+v): %v", filter, err)
		}
		responses = append(responses, result)
		return result
	}
	receiptsOf := func(page Page[CanonicalEventRecord]) string {
		ids := make([]string, 0, len(page.Items))
		for _, item := range page.Items {
			ids = append(ids, item.Event.ReceiptID)
		}
		return strings.Join(ids, ",")
	}

	// Default browse: intact rows only, newest first, this tenant only.
	all := browse(CanonicalEventFilter{}, PageRequest{})
	if got := receiptsOf(all); got != "receipt-2,receipt-1" || all.HasMore {
		t.Fatalf("default browse = %q (more %v), want receipt-2,receipt-1", got, all.HasMore)
	}
	first := all.Items[1]
	if first.ReceiptStatus != "accepted" || first.Definition != adt || first.Event.EventType != "patient_admit" ||
		first.Event.SourceMessageID != "MSH10-receipt-1" || first.Event.CorrelationID != "correlation-receipt-1" ||
		first.Event.Classification != "phi" || !first.Event.RecordedAt.Equal(t0) {
		t.Fatalf("receipt-1 row = %+v", first)
	}
	if first.Source == nil || first.Source.ArtifactID != "source-adt" || first.Source.RevisionID != "source-1" {
		t.Fatalf("receipt-1 source = %+v, want source-adt@source-1 from lineage", first.Source)
	}
	if all.Items[0].Source != nil {
		t.Fatalf("receipt-2 has no lineage row but source = %+v", all.Items[0].Source)
	}
	if all.Items[0].Event.PurgeAfter == nil || !all.Items[0].Event.PurgeAfter.Equal(stampedAfter) || all.Items[0].Event.PurgedAt != nil {
		t.Fatalf("receipt-2 retention marks = %v / %v", all.Items[0].Event.PurgeAfter, all.Items[0].Event.PurgedAt)
	}
	paths := map[string]string{}
	for _, field := range first.Event.PayloadFields {
		paths[field.Path] = field.Kind
	}
	if paths["patient.mrn"] != "string" || paths["encounter.class"] != "string" {
		t.Fatalf("receipt-1 payload structure = %+v", first.Event.PayloadFields)
	}

	// includePurged admits the tombstoned row, with its marks.
	withPurged := browse(CanonicalEventFilter{IncludePurged: true}, PageRequest{})
	if got := receiptsOf(withPurged); got != "receipt-3,receipt-2,receipt-1" {
		t.Fatalf("includePurged browse = %q", got)
	}
	tombstone := withPurged.Items[0].Event
	if tombstone.PurgedAt == nil || !tombstone.PurgedAt.Equal(purgedAt) || tombstone.PurgeAfter == nil {
		t.Fatalf("tombstoned row marks = %v / %v", tombstone.PurgeAfter, tombstone.PurgedAt)
	}

	// Every filter is an exact column match.
	filters := []struct {
		name   string
		filter CanonicalEventFilter
		want   string
	}{
		{"event type", CanonicalEventFilter{EventType: "patient_admit"}, "receipt-1"},
		{"event type with purged", CanonicalEventFilter{EventType: "patient_admit", IncludePurged: true}, "receipt-3,receipt-1"},
		{"unknown event type", CanonicalEventFilter{EventType: "vital_sign"}, ""},
		{"definition", CanonicalEventFilter{DefinitionID: "adt-batch", IncludePurged: true}, "receipt-3"},
		{"receipt", CanonicalEventFilter{ReceiptID: "receipt-2"}, "receipt-2"},
		{"another tenant's receipt", CanonicalEventFilter{ReceiptID: "receipt-b"}, ""},
		{"source message", CanonicalEventFilter{SourceMessageID: "MSH10-receipt-1"}, "receipt-1"},
		{"correlation", CanonicalEventFilter{CorrelationID: "correlation-receipt-2"}, "receipt-2"},
		{"window", CanonicalEventFilter{From: ptrTime(t0.Add(time.Minute)), To: ptrTime(t0.Add(time.Hour)), IncludePurged: true}, "receipt-2"},
	}
	for _, testCase := range filters {
		if got := receiptsOf(browse(testCase.filter, PageRequest{})); got != testCase.want {
			t.Errorf("%s: browse = %q, want %q", testCase.name, got, testCase.want)
		}
	}

	// Keyset paging walks every row exactly once.
	var walked []string
	page := PageRequest{First: 1}
	for guard := 0; guard < 5; guard++ {
		result := browse(CanonicalEventFilter{IncludePurged: true}, page)
		walked = append(walked, receiptsOf(result))
		if !result.HasMore {
			break
		}
		page.Cursor = result.NextCursor
	}
	if got := strings.Join(walked, "|"); got != "receipt-3|receipt-2|receipt-1" {
		t.Fatalf("paged walk = %q", got)
	}

	// Statistics over two days, by day.
	byDay, err := service.AdmissionStatistics(ctx, StatisticsRequest{
		From: t0.Add(-15 * time.Minute), To: t0.Add(47 * time.Hour), Bucket: StatisticsBucketDay,
	})
	if err != nil {
		t.Fatalf("AdmissionStatistics by day: %v", err)
	}
	responses = append(responses, byDay)
	if byDay.AcceptedReceipts != 3 || byDay.RejectedReceipts != 1 || byDay.CanonicalEvents != 3 ||
		byDay.PurgedEvents != 1 || byDay.ScheduledForPurge != 1 || byDay.QueuedAttempts != 1 ||
		byDay.SucceededAttempts != 1 || byDay.FailedAttempts != 1 || byDay.GroupsTruncated {
		t.Fatalf("daily totals = %+v", byDay)
	}
	if fmt.Sprint(byDay.EventsByType) != "[{patient_admit 2} {lab_result 1}]" {
		t.Fatalf("events by type = %v", byDay.EventsByType)
	}
	if fmt.Sprint(byDay.ReceiptsByDefinition) != "[{adt-batch v2 1 1} {adt-http rev-1 2 0}]" {
		t.Fatalf("receipts by definition = %v", byDay.ReceiptsByDefinition)
	}
	if fmt.Sprint(byDay.AttemptsByDestination) != "[{fhir-primary 1 1 0} {kafka-archive 0 0 1}]" {
		t.Fatalf("attempts by destination = %v", byDay.AttemptsByDestination)
	}
	wantDays := []StatisticsBucketCounts{
		{Start: time.Date(2026, 9, 28, 0, 0, 0, 0, time.UTC), Accepted: 3, Queued: 1, Succeeded: 1, Failed: 1},
		{Start: time.Date(2026, 9, 29, 0, 0, 0, 0, time.UTC), Rejected: 1},
		{Start: time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)},
	}
	assertSeries(t, "daily", byDay.Series, wantDays)

	// By hour, the window is half-open: receipt-2 at exactly To is excluded.
	byHour, err := service.AdmissionStatistics(ctx, StatisticsRequest{From: t0, To: t0.Add(time.Hour), Bucket: StatisticsBucketHour})
	if err != nil {
		t.Fatalf("AdmissionStatistics by hour: %v", err)
	}
	responses = append(responses, byHour)
	if byHour.AcceptedReceipts != 1 || byHour.CanonicalEvents != 1 || byHour.QueuedAttempts != 1 || byHour.SucceededAttempts != 0 {
		t.Fatalf("hourly totals = %+v", byHour)
	}
	assertSeries(t, "hourly", byHour.Series, []StatisticsBucketCounts{
		{Start: time.Date(2026, 9, 28, 10, 0, 0, 0, time.UTC), Accepted: 1, Queued: 1},
		{Start: time.Date(2026, 9, 28, 11, 0, 0, 0, time.UTC)},
	})

	// An empty window answers zeros and a zero-filled series, never an error.
	empty, err := service.AdmissionStatistics(ctx, StatisticsRequest{
		From: t0.AddDate(0, 1, 0), To: t0.AddDate(0, 1, 0).Add(3 * time.Hour), Bucket: StatisticsBucketHour,
	})
	if err != nil || empty.AcceptedReceipts != 0 || len(empty.Series) != 4 || len(empty.EventsByType) != 0 {
		t.Fatalf("empty window = %+v (%v)", empty, err)
	}

	encoded, err := json.Marshal(responses)
	if err != nil {
		t.Fatalf("marshal responses: %v", err)
	}
	if strings.Contains(string(encoded), verificationSentinel) {
		t.Fatalf("a verification read returned a payload value: %s", encoded)
	}
	if strings.Contains(string(encoded), "receipt-b") || strings.Contains(string(encoded), "tenant-b") {
		t.Fatalf("a verification read returned another tenant's row: %s", encoded)
	}
	if !strings.Contains(string(encoded), "patient.mrn") {
		t.Fatalf("negative control: the structural projection is missing from the responses, so the sentinel check proves nothing")
	}
}

func assertSeries(t *testing.T, name string, got, want []StatisticsBucketCounts) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("%s series = %+v, want %+v", name, got, want)
	}
	for index := range want {
		if !got[index].Start.Equal(want[index].Start) || got[index].Start.Location() != time.UTC {
			t.Fatalf("%s bucket %d start = %v, want %v", name, index, got[index].Start, want[index].Start)
		}
		got[index].Start = want[index].Start
		if got[index] != want[index] {
			t.Fatalf("%s bucket %d = %+v, want %+v", name, index, got[index], want[index])
		}
	}
}

func ptrTime(value time.Time) *time.Time { return &value }

func seedAdmission(t *testing.T, ctx context.Context, db *sql.DB, admission seededAdmission) {
	t.Helper()
	revisionJSON, _ := json.Marshal(admission.definition)
	principalJSON, _ := json.Marshal(integration.Principal{
		ID: "adt-gateway", Kind: integration.PrincipalKindService, AuthMethod: "bearer",
	})
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		t.Fatalf("begin seed: %v", err)
	}
	defer func() { _ = tx.Rollback() }()
	exec := func(name, statement string, args ...any) {
		t.Helper()
		if _, err := tx.ExecContext(ctx, statement, args...); err != nil {
			t.Fatalf("seed %s for %s: %v", name, admission.receiptID, err)
		}
	}
	exec("receipt", `
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint,
			integration_revision, status, recorded_at, correlation_id,
			raw_retention_mode, principal_json, reason, result_json
		) VALUES ($1, $2, $3, 'fingerprint', $4, $5, $6, $7, 'ephemeral', $8, '', '{}')`,
		admission.tenant, admission.receiptID, "key-"+admission.receiptID, revisionJSON,
		admission.status, admission.recordedAt, "correlation-"+admission.receiptID, principalJSON)
	if admission.eventType == "" {
		if err := tx.Commit(); err != nil {
			t.Fatalf("commit seed: %v", err)
		}
		return
	}
	eventID := "event-" + admission.receiptID
	payload := fmt.Sprintf(`{"type":%q,"patient":{"mrn":%q,"name":{"family":%q}},"encounter":{"class":"inpatient"}}`,
		admission.eventType, verificationSentinel, verificationSentinel)
	if admission.purgedAt != nil {
		payload = `{"purged": true, "purge_schema": "fi-fhir.retention.tombstone.v1"}`
	}
	exec("event", `
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at,
			purge_after, purged_at
		) VALUES ($1, $2, $3, $4, $5, $6, 'phi', $7, $8, $9, $10)`,
		admission.tenant, eventID, admission.receiptID, admission.eventType,
		"MSH10-"+admission.receiptID, "correlation-"+admission.receiptID, payload,
		admission.recordedAt, admission.purgeAfter, admission.purgedAt)
	if admission.withLineage {
		artifactsJSON, _ := json.Marshal(integration.ExecutionArtifactRevisions{
			Source:   integration.ArtifactRevisionRef{ArtifactID: "source-adt", RevisionID: "source-1", Digest: "sha256:" + strings.Repeat("1", 64)},
			Profile:  integration.ArtifactRevisionRef{ArtifactID: "profile-adt", RevisionID: "1", Digest: "sha256:" + strings.Repeat("2", 64)},
			Workflow: integration.ArtifactRevisionRef{ArtifactID: "workflow-adt", RevisionID: "1", Digest: "sha256:" + strings.Repeat("3", 64)},
		})
		exec("lineage", `
			INSERT INTO integration_message_lineage (
				tenant_id, lineage_id, receipt_id, event_id, trace_id, correlation_id,
				source_message_id, artifact_revisions_json, routes_json,
				diagnostics_json, recorded_at
			) VALUES ($1, $2, $3, $4, 'trace', $5, $6, $7, '[]', '[]', $8)`,
			admission.tenant, "lineage-"+admission.receiptID, admission.receiptID, eventID,
			"correlation-"+admission.receiptID, "MSH10-"+admission.receiptID, artifactsJSON, admission.recordedAt)
	}
	if admission.attemptStatus != "" {
		destinationJSON, _ := json.Marshal(integration.DestinationRevisionRef{
			ArtifactRevisionRef: integration.ArtifactRevisionRef{
				ArtifactID: admission.destination, RevisionID: "d1", Digest: "sha256:" + strings.Repeat("4", 64),
			},
			Class: integration.DestinationClassProduction,
		})
		var completedAt *time.Time
		if admission.attemptStatus != "queued" {
			completedAt = ptrTime(admission.recordedAt.Add(time.Minute))
		}
		exec("attempt", `
			INSERT INTO integration_delivery_attempts (
				tenant_id, attempt_id, receipt_id, event_id, trace_id,
				destination_revision_json, route_name, action_id, status,
				attempt_count, recorded_at, scheduled_at, completed_at
			) VALUES ($1, $2, $3, $4, 'trace', $5, 'admit', 'send', $6, 1, $7, $7, $8)`,
			admission.tenant, "attempt-"+admission.receiptID, admission.receiptID, eventID,
			destinationJSON, admission.attemptStatus, admission.recordedAt, completedAt)
	}
	if err := tx.Commit(); err != nil {
		t.Fatalf("commit seed: %v", err)
	}
}

func openVerificationDatabase(t *testing.T, ctx context.Context) *sql.DB {
	t.Helper()
	base := os.Getenv("POSTGRES_TEST_URL")
	if base == "" {
		t.Skip("POSTGRES_TEST_URL is required for the verification read proofs")
	}
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open PostgreSQL admin: %v", err)
	}
	schema := fmt.Sprintf("verification_%d", time.Now().UnixNano())
	if _, err := admin.ExecContext(ctx, `CREATE SCHEMA `+schema); err != nil {
		_ = admin.Close()
		t.Fatalf("create verification schema: %v", err)
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
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("ping PostgreSQL: %v", err)
	}
	return db
}
