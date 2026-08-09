//go:build integration

package retention

import (
	"context"
	"database/sql"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// Lane S5-F's two day-1 gates.
//
// Both land before a line of production code, both are recorded in
// .loom/worklog/2026-08-09-lane-s5-f-day-1-gates.md with their predicted
// results, and each one kills one half of the framing the lane inherited:
//
//	"The purge works; role separation is the remaining hardening."
//
// TestPurgeThroughput_BacklogExceedsOneBatchPerTick must FAIL on unmodified
// main, at exactly one batch. TestPurgeRoleSeparation_ApplicationRoleCanDrop
// ItsOwnGuardToday must PASS on unmodified main. A gate that produces the other
// answer, or the predicted answer for a different reason, disconfirms
// .loom/33-sprint5-execution-specs.md and the lane corrects that file before
// writing code.

const (
	// throughputSeededEvents is deliberately larger than defaultBatchSize and
	// not a multiple of it, so a partial drain is distinguishable from a
	// rounded one: 500 = 200 + 200 + 100.
	throughputSeededEvents = 500

	throughputReceiptID = "rcpt-throughput-gate"
	throughputSentinel  = "MRN-THROUGHPUT-GATE"
)

// TestPurgeThroughput_BacklogExceedsOneBatchPerTick is Lane S5-F's first day-1
// gate, and it is the reproduction of found defect D1.
//
// D1 (.loom/33-sprint5-execution-specs.md): Purger.Run calls PurgeOnce once per
// ticker tick (purger.go:142-159) against statements that every carry LIMIT $3
// (store.go:311,339,363,409,441,474,512) bound to defaultBatchSize = 200
// (store.go:33), on a defaultRetentionCadence of one hour
// (cmd/fi-fhir/retention_runtime.go:22-23). There is no continue-on-full-batch,
// so the sustained ceiling is 200 records per class per hour — 0.056/sec — on
// what store.go:31-33 itself calls "the busiest table in the system".
//
// The gate asserts the property a retention control must have and does not: ONE
// purge pass drains the backlog it is given. On unmodified main it fails at
// exactly defaultBatchSize, and the failure message says so, because "partial
// for some other reason" would be a different defect and must not be mistaken
// for this one.
//
// It is deliberately narrow. The policy configures the canonical-event class
// only, so nothing but the per-pass bound can explain a partial result: the
// delivery interlock is unreachable (no attempts are seeded), the tombstone
// exemption is intact (the second subtest drains the remainder through the same
// code path), and the rows are dependent-free.
//
// NON-VACUITY. The gate asserts up front that the purger is running the SHIPPED
// batch size. Without that, a hypothetical defaultBatchSize of 1 would fail this
// test too — for a different reason — and the reproduction would be worthless.
// The .loom/33 note is precise about this: every retention test today seeds two
// rows per class and calls PurgeOnce once, so defaultBatchSize = 1 would leave
// the whole suite green.
func TestPurgeThroughput_BacklogExceedsOneBatchPerTick(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	db := newRetentionGateSchema(t, ctx, requireRetentionPostgres(t), "purge_throughput_gate")
	migrateDurableSchema(t, ctx, db)
	seedExpiredCanonicalEventBacklog(t, ctx, db, throughputSeededEvents)

	store := newRetentionStore(t, db)
	if _, err := store.PutPolicy(ctx, throughputPolicy()); err != nil {
		t.Fatalf("PutPolicy: %v", err)
	}

	// BatchSize zero means the store's own default, which is what
	// cmd/fi-fhir/retention_runtime.go:95 also resolves to when
	// FI_FHIR_RETENTION_PURGE_BATCH_SIZE is unset. Interval matches the shipped
	// defaultRetentionCadence so the arithmetic below is the deployed one.
	purger, err := NewPurger(PurgerConfig{Store: store, Interval: time.Hour})
	if err != nil {
		t.Fatalf("NewPurger: %v", err)
	}
	if got := purger.BatchSize(); got != defaultBatchSize {
		t.Fatalf("purger batch size = %d, want the shipped defaultBatchSize %d; "+
			"this gate only reproduces D1 against the shipped configuration", got, defaultBatchSize)
	}
	if throughputSeededEvents <= defaultBatchSize {
		t.Fatalf("the fixture seeds %d events against a batch size of %d, so a single pass "+
			"could drain it and the gate would pass vacuously",
			throughputSeededEvents, defaultBatchSize)
	}
	if remaining := countPurgeableCanonicalEvents(t, ctx, db); remaining != throughputSeededEvents {
		t.Fatalf("fixture seeded %d purgeable events, want %d", remaining, throughputSeededEvents)
	}

	t.Run("one_purge_pass_drains_the_backlog_it_is_given", func(t *testing.T) {
		result, err := purger.PurgeOnce(ctx)
		if err != nil {
			t.Fatalf("PurgeOnce: %v", err)
		}
		tombstoned := countTombstonedCanonicalEvents(t, ctx, db)
		if tombstoned == int64(throughputSeededEvents) {
			t.Logf("one pass tombstoned all %d events in %s", tombstoned, result.Duration)
			return
		}
		if tombstoned == int64(defaultBatchSize) {
			t.Fatalf("DAY-1 GATE CONFIRMED — D1 reproduced. One purge pass tombstoned "+
				"exactly %d of %d seeded events: one batch, then the pass returned and "+
				"Purger.Run blocks on a %s tick (purger.go:148-158). Sustained ceiling "+
				"%d records/class/hour on integration_canonical_events. "+
				"reported counts=%+v duration=%s",
				tombstoned, throughputSeededEvents, time.Hour, defaultBatchSize,
				result.PurgeCounts, result.Duration)
		}
		t.Fatalf("one purge pass tombstoned %d of %d seeded events — partial, but NOT at the "+
			"batch boundary of %d. That is not D1 as .loom/33 describes it; diagnose before "+
			"assuming the spec is right. reported counts=%+v",
			tombstoned, throughputSeededEvents, defaultBatchSize, result.PurgeCounts)
	})

	// The diagnosis. It distinguishes "the bound is per call" from "these rows
	// are unpurgeable", and it holds both before and after the D1 repair: on
	// unmodified main the first subtest leaves 300 behind and this drains them
	// in two more passes; once the drain loop lands the first subtest leaves
	// nothing and this observes a zero backlog immediately.
	t.Run("the_bound_is_per_call_not_a_property_of_the_rows", func(t *testing.T) {
		maxPasses := 1 + (throughputSeededEvents+defaultBatchSize-1)/defaultBatchSize
		var sequence []int64
		for pass := 0; pass < maxPasses; pass++ {
			if countPurgeableCanonicalEvents(t, ctx, db) == 0 {
				t.Logf("backlog reached zero; per-pass tombstone sequence was %v "+
					"(each element is one PurgeOnce, i.e. one hourly tick under Purger.Run)",
					sequence)
				return
			}
			result, err := purger.PurgeOnce(ctx)
			if err != nil {
				t.Fatalf("PurgeOnce pass %d: %v", pass, err)
			}
			sequence = append(sequence, result.CanonicalEvents)
		}
		t.Fatalf("backlog did not reach zero in %d passes; sequence=%v. The rows are not "+
			"drainable by repetition either, so the defect is not the per-pass bound alone",
			maxPasses, sequence)
	})
}

// TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday is Lane S5-F's
// second day-1 gate. It must PASS on unmodified main.
//
// docs/operations/PHI-RETENTION.md:293 asserts this in prose — "every migration
// runs on the same connection the runtime uses, so the application role owns the
// tables it guards and can drop any trigger. The schema-enforced exemption is a
// guard against programmatic error, not against a hostile database role" — and
// nothing in the tree demonstrates it. .loom/40-decisions.md:1631-1632,1659,1667
// repeats it. This test demonstrates it in three statements, and it is the whole
// argument for the slice: if the application role could NOT disarm its own
// guard, the filed follow-up would already be satisfied.
//
// NON-VACUITY, and this is the part that matters. The CI service container's
// POSTGRES_USER is a superuser, and a superuser can drop anything, so running
// this as POSTGRES_TEST_URL's own role would prove nothing about ownership. The
// test therefore provisions an ORDINARY role — NOSUPERUSER, NOCREATEDB,
// NOCREATEROLE, granted nothing but USAGE and CREATE on one scratch schema —
// runs the shipped migrators through it exactly as runServe does on the runtime
// connection (cmd/fi-fhir/retention_runtime.go:67-80 and the five other
// migrators), and asserts both that the role is not a superuser and that it
// ended up owning the guarded table. Only then does it disarm the guard.
//
// The three disarm shapes are the three the lane's acceptance criteria must
// invert once the role topology lands: DROP TRIGGER, ALTER TABLE ... DISABLE
// TRIGGER, and ALTER TABLE ... OWNER TO.
func TestPurgeRoleSeparation_ApplicationRoleCanDropItsOwnGuardToday(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	app := newApplicationRoleSchema(t, ctx, requireRetentionPostgres(t), "purge_role_gate")

	// The runtime's own startup path: the application connection applies the
	// migrations, so the application role becomes the owner of every table,
	// function, and trigger they create.
	migrateDurableSchema(t, ctx, app.db)

	t.Run("the_role_is_ordinary_and_owns_the_guarded_table", func(t *testing.T) {
		var superuser, createRole bool
		if err := app.db.QueryRowContext(ctx,
			`SELECT rolsuper, rolcreaterole FROM pg_roles WHERE rolname = current_user`,
		).Scan(&superuser, &createRole); err != nil {
			t.Fatalf("read current role attributes: %v", err)
		}
		if superuser || createRole {
			t.Fatalf("the connected role is superuser=%t createrole=%t; every assertion below "+
				"would prove PostgreSQL's superuser bypass rather than table ownership",
				superuser, createRole)
		}
		var owner string
		if err := app.db.QueryRowContext(ctx, `
			SELECT pg_get_userbyid(c.relowner)
			FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
			WHERE n.nspname = $1 AND c.relname = 'integration_canonical_events'
		`, app.schema).Scan(&owner); err != nil {
			t.Fatalf("read table owner: %v", err)
		}
		if owner != app.role {
			t.Fatalf("integration_canonical_events is owned by %q, not by the application role %q; "+
				"correction 54 does not hold in this fixture and the gate would be measuring "+
				"something else", owner, app.role)
		}
		t.Logf("application role %q is NOSUPERUSER and owns integration_canonical_events", app.role)
	})

	seedRawCanonicalEvent(t, ctx, app.db, "rcpt-role-gate", "evt-role-gate", throughputSentinel)

	const forbiddenMutation = `
		UPDATE integration_canonical_events
		SET payload_json = '{"mrn":"REWRITTEN-BY-THE-APPLICATION-ROLE"}'::jsonb
		WHERE event_id = 'evt-role-gate'`

	t.Run("a_the_guard_is_armed_before_the_disarm", func(t *testing.T) {
		_, err := app.db.ExecContext(ctx, forbiddenMutation)
		assertRaised(t, err, "append-only",
			"an arbitrary payload rewrite succeeded with the guard in place; "+
				"Slice 4.1e's exemption is wider than its own kill-test claims")
	})

	t.Run("b_the_application_role_drops_its_own_guard", func(t *testing.T) {
		if _, err := app.db.ExecContext(ctx,
			`DROP TRIGGER integration_canonical_events_purge_only ON integration_canonical_events`,
		); err != nil {
			t.Fatalf("DAY-1 GATE DISCONFIRMED — the application role could NOT drop the guard: %v. "+
				"PHI-RETENTION.md:293 and .loom/33 correction 54 are wrong, and the filed "+
				"follow-up needs re-scoping before any GRANT is written", err)
		}
		t.Log("DROP TRIGGER integration_canonical_events_purge_only succeeded as the application role")
	})

	t.Run("c_and_the_mutation_the_guard_forbade_now_succeeds", func(t *testing.T) {
		result, err := app.db.ExecContext(ctx, forbiddenMutation)
		if err != nil {
			t.Fatalf("the guard was dropped but the mutation still failed: %v — the refusal in "+
				"subtest a was not attributable to the trigger", err)
		}
		affected, err := result.RowsAffected()
		if err != nil || affected != 1 {
			t.Fatalf("payload rewrite affected %d rows (err=%v), want 1", affected, err)
		}
		var payload string
		if err := app.db.QueryRowContext(ctx,
			`SELECT payload_json::text FROM integration_canonical_events WHERE event_id = 'evt-role-gate'`,
		).Scan(&payload); err != nil {
			t.Fatalf("read rewritten payload: %v", err)
		}
		if !strings.Contains(payload, "REWRITTEN-BY-THE-APPLICATION-ROLE") {
			t.Fatalf("payload is %s; the rewrite did not land", payload)
		}
		t.Log("DAY-1 GATE CONFIRMED — the application role disarmed its own immutability guard " +
			"and then performed the mutation the guard existed to forbid")
	})

	// The other disarm shapes, so every acceptance criterion that must invert
	// once the role topology lands has a day-1 baseline.
	t.Run("d_disable_trigger_is_also_available_to_the_application_role", func(t *testing.T) {
		if _, err := app.db.ExecContext(ctx,
			`ALTER TABLE integration_receipts DISABLE TRIGGER integration_receipts_undeletable`,
		); err != nil {
			t.Fatalf("ALTER TABLE ... DISABLE TRIGGER was refused: %v", err)
		}
		t.Log("ALTER TABLE ... DISABLE TRIGGER succeeded as the application role")
	})

	// The sharpest shape, and the one with a design consequence: the guard
	// functions are owned by the application role too, so ONE
	// CREATE OR REPLACE FUNCTION disarms every trigger that shares a function
	// without touching a single trigger. reject_integration_submission_mutation
	// backs four of them (processor/migrations/0004_audit_immutability.sql:30-47).
	//
	// A role topology that revokes only ALTER TABLE would leave this route open.
	// Recorded in the role-topology decision as a constraint on task 5's GRANTs.
	t.Run("e_replacing_the_shared_guard_function_disarms_every_trigger_using_it", func(t *testing.T) {
		seedGuardedLineageRow(t, ctx, app.db)
		const forbiddenLineageMutation = `
			UPDATE integration_message_lineage SET trace_id = 'rewritten'
			WHERE lineage_id = 'lineage-role-gate'`

		_, err := app.db.ExecContext(ctx, forbiddenLineageMutation)
		assertRaised(t, err, "append-only",
			"the lineage guard was not armed, so replacing its function would prove nothing")

		if _, err := app.db.ExecContext(ctx, `
			CREATE OR REPLACE FUNCTION reject_integration_submission_mutation()
			RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NEW; END; $$
		`); err != nil {
			t.Fatalf("CREATE OR REPLACE FUNCTION was refused: %v", err)
		}
		if _, err := app.db.ExecContext(ctx, forbiddenLineageMutation); err != nil {
			t.Fatalf("the guard function was replaced but the mutation still failed: %v", err)
		}
		t.Log("CREATE OR REPLACE FUNCTION reject_integration_submission_mutation() succeeded as " +
			"the application role, disarming all four triggers that share it in one statement")
	})

	t.Run("f_and_so_is_taking_the_table_away_entirely", func(t *testing.T) {
		if _, err := app.db.ExecContext(ctx,
			`ALTER TABLE integration_canonical_events OWNER TO `+pq.QuoteIdentifier(app.peerRole),
		); err != nil {
			t.Fatalf("ALTER TABLE ... OWNER TO was refused: %v", err)
		}
		t.Logf("ALTER TABLE ... OWNER TO %s succeeded as the application role", app.peerRole)
	})
}

// applicationRoleSchema is one scratch schema plus the ordinary, non-superuser
// role that owns everything inside it.
type applicationRoleSchema struct {
	db       *sql.DB
	schema   string
	role     string
	peerRole string
}

// newApplicationRoleSchema models the deployment PHI-RETENTION.md:293 describes:
// one connection, used both to apply migrations and to serve traffic, held by an
// ordinary role with no privilege beyond its own schema.
//
// peerRole exists only so ALTER TABLE ... OWNER TO has a legal target.
// PostgreSQL requires two things of that statement, and neither is a privilege
// check against the table: the issuing role must be a member of the new owner,
// and the NEW OWNER must hold CREATE on the schema. Both are granted here, so a
// refusal is attributable to the application role's own privileges rather than
// to the fixture. (Without the second grant the statement fails with
// "permission denied for schema", which says nothing about what the application
// role may do.)
func newApplicationRoleSchema(t *testing.T, ctx context.Context, base, prefix string) applicationRoleSchema {
	t.Helper()
	admin, err := sql.Open("postgres", base)
	if err != nil {
		t.Fatalf("open admin connection: %v", err)
	}
	suffix := time.Now().UnixNano()
	out := applicationRoleSchema{
		schema:   fmt.Sprintf("%s_%d", prefix, suffix),
		role:     fmt.Sprintf("%s_app_%d", prefix, suffix),
		peerRole: fmt.Sprintf("%s_peer_%d", prefix, suffix),
	}
	const password = "role-gate-password"
	setup := []string{
		`CREATE ROLE ` + pq.QuoteIdentifier(out.peerRole) + ` NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE`,
		`CREATE ROLE ` + pq.QuoteIdentifier(out.role) + ` LOGIN PASSWORD ` + pq.QuoteLiteral(password) +
			` NOSUPERUSER NOCREATEDB NOCREATEROLE`,
		`GRANT ` + pq.QuoteIdentifier(out.peerRole) + ` TO ` + pq.QuoteIdentifier(out.role),
		`CREATE SCHEMA ` + pq.QuoteIdentifier(out.schema) + ` AUTHORIZATION ` + pq.QuoteIdentifier(out.role),
		`GRANT USAGE, CREATE ON SCHEMA ` + pq.QuoteIdentifier(out.schema) +
			` TO ` + pq.QuoteIdentifier(out.peerRole),
	}
	for _, statement := range setup {
		if _, err := admin.ExecContext(ctx, statement); err != nil {
			t.Fatalf("provision application role fixture (%s): %v", statement, err)
		}
	}

	parsed, err := url.Parse(base)
	if err != nil {
		t.Fatalf("parse POSTGRES_TEST_URL: %v", err)
	}
	parsed.User = url.UserPassword(out.role, password)
	query := parsed.Query()
	query.Set("search_path", out.schema)
	parsed.RawQuery = query.Encode()
	db, err := sql.Open("postgres", parsed.String())
	if err != nil {
		t.Fatalf("open application-role connection: %v", err)
	}
	if err := db.PingContext(ctx); err != nil {
		t.Fatalf("connect as application role %s: %v", out.role, err)
	}
	out.db = db

	t.Cleanup(func() {
		background := context.Background()
		_ = db.Close()
		_, _ = admin.ExecContext(background,
			`DROP SCHEMA IF EXISTS `+pq.QuoteIdentifier(out.schema)+` CASCADE`)
		for _, role := range []string{out.role, out.peerRole} {
			_, _ = admin.ExecContext(background, `DROP OWNED BY `+pq.QuoteIdentifier(role)+` CASCADE`)
			_, _ = admin.ExecContext(background, `DROP ROLE IF EXISTS `+pq.QuoteIdentifier(role))
		}
		_ = admin.Close()
	})
	return out
}

// throughputPolicy configures the canonical-event class and nothing else, so a
// partial drain has exactly one candidate explanation.
func throughputPolicy() Policy {
	return Policy{
		TenantID:             "tenant-a",
		CanonicalEventRetain: time.Hour,
		Principal: integration.Principal{
			ID: "privacy-officer-7", Kind: integration.PrincipalKindHuman, AuthMethod: "oidc",
		},
		Reason:         "Lane S5-F day-1 throughput gate",
		DocumentDigest: "sha256:" + strings.Repeat("d", 64),
	}
}

// seedExpiredCanonicalEventBacklog writes one receipt and count dependent-free
// canonical events, all recorded 30 days ago so every one of them is expired
// against the one-hour window above. No delivery attempt is seeded, so the
// interlock in purgeCanonicalEvents (store.go:389-405) cannot be what withholds
// a row.
func seedExpiredCanonicalEventBacklog(t *testing.T, ctx context.Context, db *sql.DB, count int) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_receipts (
			tenant_id, receipt_id, idempotency_key, request_fingerprint, integration_revision,
			status, recorded_at, correlation_id, raw_retention_mode, principal_json, reason, result_json
		) VALUES ('tenant-a', $1, 'idem-'||$1, 'fp-'||$1, '{}'::jsonb,
			'accepted', now() - interval '30 days', 'corr-'||$1, 'ephemeral',
			'{"id":"svc"}'::jsonb, '', '{}'::jsonb)
	`, throughputReceiptID); err != nil {
		t.Fatalf("seed throughput receipt: %v", err)
	}
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_canonical_events (
			tenant_id, event_id, receipt_id, event_type, source_message_id,
			correlation_id, classification, payload_json, recorded_at
		)
		SELECT 'tenant-a', 'evt-throughput-'||lpad(g::text, 6, '0'), $1, 'patient_admit',
			'msg-'||g, 'corr-'||g, 'phi',
			jsonb_build_object('mrn', $2::text||'-'||g),
			now() - interval '30 days'
		FROM generate_series(1, $3) AS g
	`, throughputReceiptID, throughputSentinel, count); err != nil {
		t.Fatalf("seed %d expired canonical events: %v", count, err)
	}
}

// seedGuardedLineageRow writes one integration_message_lineage row for the
// role-gate fixture's receipt and event. The lineage table's UPDATE guard uses
// reject_integration_submission_mutation(), the function shared by four
// triggers, which is the subject of subtest e.
func seedGuardedLineageRow(t *testing.T, ctx context.Context, db *sql.DB) {
	t.Helper()
	if _, err := db.ExecContext(ctx, `
		INSERT INTO integration_message_lineage (
			tenant_id, lineage_id, receipt_id, event_id, trace_id, correlation_id,
			source_message_id, artifact_revisions_json, routes_json, diagnostics_json, recorded_at
		) VALUES ('tenant-a', 'lineage-role-gate', 'rcpt-role-gate', 'evt-role-gate',
			'trace-role-gate', 'corr-role-gate', 'msg-role-gate',
			'{}'::jsonb, '[]'::jsonb, '[]'::jsonb, now())
	`); err != nil {
		t.Fatalf("seed guarded lineage row: %v", err)
	}
}

func countTombstonedCanonicalEvents(t *testing.T, ctx context.Context, db *sql.DB) int64 {
	t.Helper()
	var count int64
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM integration_canonical_events WHERE purged_at IS NOT NULL`,
	).Scan(&count); err != nil {
		t.Fatalf("count tombstoned canonical events: %v", err)
	}
	return count
}

func countPurgeableCanonicalEvents(t *testing.T, ctx context.Context, db *sql.DB) int {
	t.Helper()
	var count int
	if err := db.QueryRowContext(ctx,
		`SELECT count(*) FROM integration_canonical_events WHERE purged_at IS NULL`,
	).Scan(&count); err != nil {
		t.Fatalf("count purgeable canonical events: %v", err)
	}
	return count
}
