package destination

import (
	"context"
	"database/sql"
	"errors"
	"reflect"
	"testing"

	_ "github.com/lib/pq"
)

// TestListDeliveriesForAttemptRefusesBeforeQuerying pins the read's argument
// contract. The handle points at a closed port, so any case that reached the
// database would fail with a connection error rather than the sentinel.
func TestListDeliveriesForAttemptRefusesBeforeQuerying(t *testing.T) {
	db, err := sql.Open("postgres", "postgres://unused:unused@127.0.0.1:1/unused?sslmode=disable")
	if err != nil {
		t.Fatalf("open placeholder database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	provenance, err := NewPostgresProvenance(db)
	if err != nil {
		t.Fatalf("NewPostgresProvenance: %v", err)
	}
	ctx := context.Background()

	for name, run := range map[string]func() ([]DeliverySummary, error){
		"empty tenant": func() ([]DeliverySummary, error) {
			return provenance.ListDeliveriesForAttempt(ctx, "", "attempt-a", 5)
		},
		"zero limit": func() ([]DeliverySummary, error) {
			return provenance.ListDeliveriesForAttempt(ctx, "tenant-a", "attempt-a", 0)
		},
		"limit over the ceiling": func() ([]DeliverySummary, error) {
			return provenance.ListDeliveriesForAttempt(ctx, "tenant-a", "attempt-a", MaxDeliveryReadLimit+1)
		},
		"nil store": func() ([]DeliverySummary, error) {
			var missing *PostgresProvenance
			return missing.ListDeliveriesForAttempt(ctx, "tenant-a", "attempt-a", 5)
		},
	} {
		if _, err := run(); !errors.Is(err, ErrProvenanceUnavailable) {
			t.Fatalf("%s: error = %v, want %v", name, err, ErrProvenanceUnavailable)
		}
	}

	// An attempt id RecordDelivery would refuse cannot carry a row, so the
	// answer is an empty list without a query — not an error that would fail
	// the operator's whole read.
	for _, attemptID := range []string{"", "attempt a", " attempt-a"} {
		deliveries, err := provenance.ListDeliveriesForAttempt(ctx, "tenant-a", attemptID, 5)
		if err != nil || deliveries == nil || len(deliveries) != 0 {
			t.Fatalf("attempt %q: deliveries = %#v, err = %v; want an empty list", attemptID, deliveries, err)
		}
	}
}

func TestSplitLedgerList(t *testing.T) {
	cases := map[string][]string{
		"":                        {},
		"Patient":                 {"Patient"},
		"Patient,Encounter":       {"Patient", "Encounter"},
		"invalid,not-found,value": {"invalid", "not-found", "value"},
	}
	for column, want := range cases {
		got := splitLedgerList(column)
		if got == nil || !reflect.DeepEqual(got, want) {
			t.Fatalf("splitLedgerList(%q) = %#v, want %#v", column, got, want)
		}
	}
}
