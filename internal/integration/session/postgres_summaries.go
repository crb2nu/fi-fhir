package session

import (
	"context"
	"database/sql"
	"fmt"
)

// ListSessionSummaries pages session metadata before looking up the latest run.
// The existing run index supports both EXISTS and the descending latest lookup.
// JSON timestamps are cast for chronological ordering, not sorted as strings.
func (s *PostgresStore) ListSessionSummaries(ctx context.Context, options SessionSummaryOptions) (SessionSummaryPage, error) {
	opts, err := normalizeSummaryOptions(options)
	if err != nil {
		return SessionSummaryPage{}, err
	}
	if err := ctx.Err(); err != nil {
		return SessionSummaryPage{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		WITH page AS (
			SELECT s.tenant_id, s.session_id, s.record_json->>'name' AS name,
			       s.status, s.created_at, (s.record_json->>'updated_at')::timestamptz AS updated_at
			FROM integration_sessions s
			WHERE s.tenant_id = $1
			  AND ($2 OR s.status = 'active')
			  AND ($3 = '' OR strpos(lower(s.record_json->>'name'), lower($3)) > 0
			               OR strpos(lower(s.session_id), lower($3)) > 0)
			  AND ($4::boolean IS NULL OR $4 = EXISTS (
			      SELECT 1 FROM integration_session_runs r
			      WHERE r.tenant_id = s.tenant_id AND r.session_id = s.session_id
			  ))
			ORDER BY updated_at DESC, s.session_id COLLATE "C" DESC
			LIMIT $5 OFFSET $6
		)
		SELECT p.session_id, p.name, p.status, p.created_at, p.updated_at,
		       latest.run_id, latest.status, latest.created_at
		FROM page p
		LEFT JOIN LATERAL (
			SELECT r.run_id, r.status, r.created_at FROM integration_session_runs r
			WHERE r.tenant_id = p.tenant_id AND r.session_id = p.session_id
			ORDER BY r.created_at DESC, r.run_id COLLATE "C" DESC LIMIT 1
		) latest ON true
		ORDER BY p.updated_at DESC, p.session_id COLLATE "C" DESC
	`, s.tenantID, opts.IncludeArchived, opts.Search, opts.HasRuns, opts.Limit+1, opts.Offset)
	if err != nil {
		return SessionSummaryPage{}, fmt.Errorf("list integration session summaries: %w", err)
	}
	defer func() { _ = rows.Close() }()

	nodes := make([]SessionSummary, 0, opts.Limit+1)
	for rows.Next() {
		var node SessionSummary
		var status SessionStatus
		var runID, runStatus sql.NullString
		var runCreated sql.NullTime
		if err := rows.Scan(&node.ID, &node.Name, &status, &node.CreatedAt, &node.UpdatedAt, &runID, &runStatus, &runCreated); err != nil {
			return SessionSummaryPage{}, fmt.Errorf("scan integration session summary: %w", err)
		}
		node.Archived = status == SessionStatusArchived
		if runID.Valid {
			node.LatestRun = &SessionRunSummary{ID: runID.String, Status: RunStatus(runStatus.String), CreatedAt: runCreated.Time}
		}
		nodes = append(nodes, node)
	}
	if err := rows.Err(); err != nil {
		return SessionSummaryPage{}, fmt.Errorf("read integration session summaries: %w", err)
	}
	return summaryPage(nodes, opts), nil
}
