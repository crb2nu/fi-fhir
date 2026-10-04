package session

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"time"
)

func summaryFixtures() ([]Session, []Run) {
	base := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	makeSession := func(id, name string, updated int) Session {
		return Session{ID: id, Name: name, Status: SessionStatusActive, CreatedAt: base, UpdatedAt: base.Add(time.Duration(updated) * time.Second)}
	}
	sessions := []Session{
		makeSession("session-old", "Old name", 0),
		makeSession("session-id-needle", "Renamed late", 3),
		makeSession("session-tie-a", `Literal %_\ Feed`, 2),
		makeSession("session-tie-z", "Other Feed", 2),
		makeSession("session-empty", "No runs", 1),
		makeSession("session-archived", "Archived Feed", 4),
	}
	sessions[5].Status = SessionStatusArchived
	runs := []Run{
		{ID: "run-old", SessionID: "session-tie-a", Status: RunStatusFailed, CreatedAt: base, UpdatedAt: base.Add(time.Hour)},
		{ID: "run-a", SessionID: "session-tie-a", Status: RunStatusPending, CreatedAt: base.Add(time.Second)},
		{ID: "run-z", SessionID: "session-tie-a", Status: RunStatusSucceeded, CreatedAt: base.Add(time.Second)},
		{ID: "run-active", SessionID: "session-old", Status: RunStatusRunning, CreatedAt: base.Add(time.Minute)},
	}
	return sessions, runs
}

func assertSummaryBrowse(t *testing.T, store Store) {
	t.Helper()
	ctx := context.Background()
	withRuns, withoutRuns := true, false
	for _, tc := range []struct {
		name string
		opts SessionSummaryOptions
		ids  []string
	}{
		{"metadata order and ID tie", SessionSummaryOptions{Limit: 25}, []string{"session-id-needle", "session-tie-z", "session-tie-a", "session-empty", "session-old"}},
		{"archive inclusion", SessionSummaryOptions{Limit: 25, IncludeArchived: true}, []string{"session-archived", "session-id-needle", "session-tie-z", "session-tie-a", "session-empty", "session-old"}},
		{"literal and trimmed search", SessionSummaryOptions{Limit: 25, Search: `  literal %_\ fEED  `}, []string{"session-tie-a"}},
		{"percent is literal", SessionSummaryOptions{Limit: 25, Search: "%"}, []string{"session-tie-a"}},
		{"ID substring", SessionSummaryOptions{Limit: 25, Search: "ID-NEEDLE"}, []string{"session-id-needle"}},
		{"has runs", SessionSummaryOptions{Limit: 25, HasRuns: &withRuns}, []string{"session-tie-a", "session-old"}},
		{"no runs", SessionSummaryOptions{Limit: 25, HasRuns: &withoutRuns}, []string{"session-id-needle", "session-tie-z", "session-empty"}},
		{"combined filters", SessionSummaryOptions{Limit: 25, Search: "feed", IncludeArchived: true, HasRuns: &withoutRuns}, []string{"session-archived", "session-tie-z"}},
		{"no matches", SessionSummaryOptions{Limit: 25, Search: "not present"}, []string{}},
		{"offset after matches", SessionSummaryOptions{Limit: 25, Offset: 100}, []string{}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.ListSessionSummaries(ctx, tc.opts)
			if err != nil {
				t.Fatal(err)
			}
			ids := make([]string, 0, len(page.Nodes))
			for _, node := range page.Nodes {
				ids = append(ids, node.ID)
				if node.ID == "session-tie-a" && (node.LatestRun == nil || node.LatestRun.ID != "run-z" || node.LatestRun.Status != RunStatusSucceeded) {
					t.Fatalf("latest run = %#v, want deterministic created-time/ID winner", node.LatestRun)
				}
				if node.ID == "session-id-needle" && node.LatestRun != nil {
					t.Fatalf("no-run session acquired a run: %#v", node.LatestRun)
				}
				if node.Archived != (node.ID == "session-archived") {
					t.Fatalf("wrong archived flag: %#v", node)
				}
			}
			if !reflect.DeepEqual(ids, tc.ids) || page.HasMore || page.NextOffset != nil || page.Nodes == nil {
				t.Fatalf("page = %#v; IDs = %v, want %v", page, ids, tc.ids)
			}
		})
	}
	for _, tc := range []struct {
		offset int
		ids    []string
		next   int
	}{
		{0, []string{"session-id-needle", "session-tie-z"}, 2},
		{2, []string{"session-tie-a", "session-empty"}, 4},
		{4, []string{"session-old"}, 0},
	} {
		page, err := store.ListSessionSummaries(ctx, SessionSummaryOptions{Limit: 2, Offset: tc.offset})
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, len(page.Nodes))
		for _, node := range page.Nodes {
			ids = append(ids, node.ID)
		}
		if !reflect.DeepEqual(ids, tc.ids) || page.HasMore != (tc.next != 0) || (tc.next == 0 && page.NextOffset != nil) || (tc.next != 0 && (page.NextOffset == nil || *page.NextOffset != tc.next)) {
			t.Fatalf("offset %d: page = %#v; IDs = %v", tc.offset, page, ids)
		}
	}
	for _, opts := range []SessionSummaryOptions{
		{Limit: 0}, {Limit: -1}, {Limit: 101}, {Limit: 25, Offset: -1}, {Limit: 25, Offset: 10001},
		{Limit: 25, Search: strings.Repeat("é", 129)}, {Limit: 25, Search: "bad\x00search"},
	} {
		if _, err := store.ListSessionSummaries(ctx, opts); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid options %#v error = %v", opts, err)
		}
	}
	cancelled, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := store.ListSessionSummaries(cancelled, SessionSummaryOptions{Limit: 25}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request error = %v", err)
	}
}

func TestMemorySessionSummaries(t *testing.T) {
	store := NewMemoryStore()
	sessions, runs := summaryFixtures()
	for _, record := range sessions {
		store.sessions[record.ID] = &record
	}
	for _, run := range runs {
		store.runs[run.ID] = &run
	}
	assertSummaryBrowse(t, store)
	other := NewMemoryStore()
	page, err := other.ListSessionSummaries(context.Background(), SessionSummaryOptions{Limit: 25})
	if err != nil || len(page.Nodes) != 0 {
		t.Fatalf("another security domain's page = %#v, %v", page, err)
	}
}

func TestMemorySessionSummariesBrowseCap(t *testing.T) {
	store := NewMemoryStore()
	for i := 0; i < 10026; i++ {
		id := fmt.Sprintf("session-%05d", i)
		store.sessions[id] = &Session{ID: id, Name: id, Status: SessionStatusActive}
	}
	page, err := store.ListSessionSummaries(context.Background(), SessionSummaryOptions{Limit: 25, Offset: 10000})
	if err != nil || len(page.Nodes) != 25 || !page.HasMore || page.NextOffset != nil {
		t.Fatalf("browse cap page = %#v, %v", page, err)
	}
	page, err = store.ListSessionSummaries(context.Background(), SessionSummaryOptions{Limit: 25, Search: "session-00000"})
	if err != nil || len(page.Nodes) != 1 || page.Nodes[0].ID != "session-00000" {
		t.Fatalf("search beyond browsing cap = %#v, %v", page, err)
	}
}
