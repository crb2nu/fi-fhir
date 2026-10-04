package resolvers

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
	enginesession "gitlab.flexinfer.ai/libs/fi-fhir/internal/integration/session"
)

// Unimplemented Store methods panic through the nil embedding. A summary
// resolver that starts loading session children therefore fails this proof.
type summaryOnlyStore struct {
	enginesession.Store
	ctx  context.Context
	opts enginesession.SessionSummaryOptions
	page enginesession.SessionSummaryPage
	err  error
}

func (s *summaryOnlyStore) ListSessionSummaries(ctx context.Context, opts enginesession.SessionSummaryOptions) (enginesession.SessionSummaryPage, error) {
	s.ctx, s.opts = ctx, opts
	if err := ctx.Err(); err != nil {
		return enginesession.SessionSummaryPage{}, err
	}
	return s.page, s.err
}

func TestIntegrationSessionSummariesProjectWithoutChildren(t *testing.T) {
	now := time.Date(2026, 10, 3, 12, 0, 0, 0, time.UTC)
	next := 26
	store := &summaryOnlyStore{page: enginesession.SessionSummaryPage{
		HasMore: true, NextOffset: &next,
		Nodes: []enginesession.SessionSummary{
			{ID: "s-1", Name: "One", CreatedAt: now, UpdatedAt: now, LatestRun: &enginesession.SessionRunSummary{ID: "r-1", Status: enginesession.RunStatusSucceeded, CreatedAt: now}},
			{ID: "s-2", Name: "Two", Archived: true, CreatedAt: now, UpdatedAt: now},
		},
	}}
	resolver := &queryResolver{NewResolver(WithIntegrationSessionStore(store))}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	page, err := resolver.IntegrationSessionSummaries(ctx, nil)
	if err != nil {
		t.Fatal(err)
	}
	if store.ctx != ctx || store.opts.Limit != 25 || store.opts.Offset != 0 || store.opts.IncludeArchived || store.opts.HasRuns != nil {
		t.Fatalf("context/defaults = %#v", store.opts)
	}
	if !page.HasMore || page.NextOffset == nil || *page.NextOffset != next || len(page.Nodes) != 2 || page.Nodes[0].LatestRun.Status != "completed" || page.Nodes[1].LatestRun != nil || !page.Nodes[1].Archived {
		t.Fatalf("projected page = %#v", page)
	}
	if page.Nodes[0].LatestRun.CreatedAt != now || page.Nodes[0].CreatedAt != now || page.Nodes[0].UpdatedAt != now {
		t.Fatal("summary changed its timestamps")
	}
	search, hasRuns := "literal %_ search", false
	input := &model.IntegrationSessionSummaryInput{Search: &search, IncludeArchived: true, HasRuns: &hasRuns, Limit: 100, Offset: 26}
	if _, err := resolver.IntegrationSessionSummaries(ctx, input); err != nil {
		t.Fatal(err)
	}
	want := enginesession.SessionSummaryOptions{Search: search, IncludeArchived: true, HasRuns: &hasRuns, Limit: 100, Offset: 26}
	if !reflect.DeepEqual(store.opts, want) {
		t.Fatalf("options = %#v, want %#v", store.opts, want)
	}
	cancel()
	if _, err := resolver.IntegrationSessionSummaries(ctx, nil); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request = %v", err)
	}
}

func TestIntegrationSessionSummariesAvailabilityAndSafeValidation(t *testing.T) {
	store := &summaryOnlyStore{err: enginesession.ErrInvalid}
	resolver := NewResolver(WithIntegrationSessionStore(store))
	query := &queryResolver{resolver}
	if _, err := query.IntegrationSessionSummaries(context.Background(), nil); err == nil || err.Error() != "invalid integration session summary request" {
		t.Fatalf("invalid input error = %v", err)
	}
	resolver.legacyUnsafeExecution = false
	resolver.durableSessionWorkspace = false
	store.ctx = nil
	if _, err := query.IntegrationSessionSummaries(context.Background(), nil); !errors.Is(err, ErrLegacyExecutionUnavailable) {
		t.Fatalf("disabled workspace error = %v", err)
	}
	if store.ctx != nil {
		t.Fatal("disabled workspace queried its store")
	}
}
