package session

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"
)

const (
	DefaultSessionSummaryLimit = 25
	MaxSessionSummaryLimit     = 100
	MaxSessionSummaryOffset    = 10000
	MaxSessionSummarySearch    = 256
)

// SessionSummaryOptions filters the whole store before applying bounded paging.
// HasRuns nil includes every session, true requires a run, false excludes runs.
type SessionSummaryOptions struct {
	Search          string
	IncludeArchived bool
	HasRuns         *bool
	Limit           int
	Offset          int
}

type SessionRunSummary struct {
	ID        string
	Status    RunStatus
	CreatedAt time.Time
}

type SessionSummary struct {
	ID        string
	Name      string
	Archived  bool
	CreatedAt time.Time
	UpdatedAt time.Time
	LatestRun *SessionRunSummary
}

type SessionSummaryPage struct {
	Nodes      []SessionSummary
	HasMore    bool
	NextOffset *int
}

func normalizeSummaryOptions(opts SessionSummaryOptions) (SessionSummaryOptions, error) {
	opts.Search = strings.TrimSpace(opts.Search)
	if opts.Limit < 1 || opts.Limit > MaxSessionSummaryLimit || opts.Offset < 0 || opts.Offset > MaxSessionSummaryOffset ||
		len(opts.Search) > MaxSessionSummarySearch || strings.ContainsRune(opts.Search, 0) {
		return SessionSummaryOptions{}, fmt.Errorf("%w: session summary bounds", ErrInvalid)
	}
	return opts, nil
}

func summaryPage(nodes []SessionSummary, opts SessionSummaryOptions) SessionSummaryPage {
	page := SessionSummaryPage{Nodes: nodes, HasMore: len(nodes) > opts.Limit}
	if page.HasMore {
		page.Nodes = nodes[:opts.Limit]
		next := opts.Offset + len(page.Nodes)
		if next <= MaxSessionSummaryOffset {
			page.NextOffset = &next
		}
	}
	return page
}

// ListSessionSummaries copies only metadata; no run payload, sample, or artifact
// is loaded or cloned for browsing. A store instance owns one security domain.
func (s *MemoryStore) ListSessionSummaries(ctx context.Context, options SessionSummaryOptions) (SessionSummaryPage, error) {
	opts, err := normalizeSummaryOptions(options)
	if err != nil {
		return SessionSummaryPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return SessionSummaryPage{}, err
	}
	s.mu.RLock()
	defer s.mu.RUnlock()

	latest := make(map[string]SessionRunSummary)
	for _, run := range s.runs {
		if err := ctx.Err(); err != nil {
			return SessionSummaryPage{}, err
		}
		previous, exists := latest[run.SessionID]
		if !exists || run.CreatedAt.After(previous.CreatedAt) || (run.CreatedAt.Equal(previous.CreatedAt) && run.ID > previous.ID) {
			latest[run.SessionID] = SessionRunSummary{ID: run.ID, Status: run.Status, CreatedAt: run.CreatedAt}
		}
	}
	needle := strings.ToLower(opts.Search)
	nodes := make([]SessionSummary, 0)
	for _, record := range s.sessions {
		if err := ctx.Err(); err != nil {
			return SessionSummaryPage{}, err
		}
		if record.Status == SessionStatusArchived && !opts.IncludeArchived {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(record.Name), needle) && !strings.Contains(strings.ToLower(record.ID), needle) {
			continue
		}
		run, hasRun := latest[record.ID]
		if opts.HasRuns != nil && *opts.HasRuns != hasRun {
			continue
		}
		node := SessionSummary{ID: record.ID, Name: record.Name, Archived: record.Status == SessionStatusArchived, CreatedAt: record.CreatedAt, UpdatedAt: record.UpdatedAt}
		if hasRun {
			node.LatestRun = &run
		}
		nodes = append(nodes, node)
	}
	sort.Slice(nodes, func(i, j int) bool {
		if nodes[i].UpdatedAt.Equal(nodes[j].UpdatedAt) {
			return nodes[i].ID > nodes[j].ID
		}
		return nodes[i].UpdatedAt.After(nodes[j].UpdatedAt)
	})
	if err := ctx.Err(); err != nil {
		return SessionSummaryPage{}, err
	}
	if opts.Offset >= len(nodes) {
		return SessionSummaryPage{Nodes: []SessionSummary{}}, nil
	}
	end := min(len(nodes), opts.Offset+opts.Limit+1)
	return summaryPage(nodes[opts.Offset:end], opts), nil
}
