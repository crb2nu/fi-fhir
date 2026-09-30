package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/integration"
)

// ListCanonicalEvents browses durable canonical events newest first, each
// joined to its admitting receipt and, when present, its lineage row. The
// payload is read only to summarize its structure (summarizePayload); no
// stored value leaves this function. Served by
// integration_canonical_events_browse_idx (0006).
func (s *PostgresReadStore) ListCanonicalEvents(
	ctx context.Context,
	tenantID string,
	filter CanonicalEventFilter,
	request PageRequest,
) (Page[CanonicalEventRecord], error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return Page[CanonicalEventRecord]{}, ErrUnavailable
	}
	if err := filter.validate(); err != nil {
		return Page[CanonicalEventRecord]{}, err
	}
	size, cursorTime, cursorID, err := normalizePage(request)
	if err != nil {
		return Page[CanonicalEventRecord]{}, err
	}
	rows, err := s.db.QueryContext(ctx, `
		SELECT e.tenant_id, e.event_id, e.receipt_id, e.event_type,
			e.source_message_id, e.correlation_id, e.classification,
			e.recorded_at, e.payload_json, e.purge_after, e.purged_at,
			r.status, r.integration_revision, l.artifact_revisions_json
		FROM integration_canonical_events e
		JOIN integration_receipts r
		  ON r.tenant_id = e.tenant_id AND r.receipt_id = e.receipt_id
		LEFT JOIN integration_message_lineage l
		  ON l.tenant_id = e.tenant_id AND l.receipt_id = e.receipt_id
		 AND l.event_id = e.event_id
		WHERE e.tenant_id = $1
		  AND ($2 = '' OR e.event_type = $2)
		  AND ($3 = '' OR r.integration_revision->>'artifact_id' = $3)
		  AND ($4 = '' OR e.receipt_id = $4)
		  AND ($5 = '' OR e.source_message_id = $5)
		  AND ($6 = '' OR e.correlation_id = $6)
		  AND ($7::timestamptz IS NULL OR e.recorded_at >= $7)
		  AND ($8::timestamptz IS NULL OR e.recorded_at <= $8)
		  AND ($9 OR e.purged_at IS NULL)
		  AND ($10::timestamptz IS NULL OR (e.recorded_at, e.event_id) < ($10, $11))
		ORDER BY e.recorded_at DESC, e.event_id DESC
		LIMIT $12
	`,
		tenantID, filter.EventType, filter.DefinitionID, filter.ReceiptID,
		filter.SourceMessageID, filter.CorrelationID, filter.From, filter.To,
		filter.IncludePurged, nullableTime(cursorTime), cursorID, size+1,
	)
	if err != nil {
		return Page[CanonicalEventRecord]{}, fmt.Errorf("list operator canonical events: %w", err)
	}
	defer func() { _ = rows.Close() }()
	records := make([]CanonicalEventRecord, 0, size)
	for rows.Next() {
		record, err := scanCanonicalEventRecord(rows)
		if err != nil {
			return Page[CanonicalEventRecord]{}, err
		}
		records = append(records, record)
	}
	if err := rows.Err(); err != nil {
		return Page[CanonicalEventRecord]{}, fmt.Errorf("iterate operator canonical events: %w", err)
	}
	return paginate(records, size, func(item CanonicalEventRecord) (time.Time, string) {
		return item.Event.RecordedAt, item.Event.EventID
	}), nil
}

func scanCanonicalEventRecord(rows *sql.Rows) (CanonicalEventRecord, error) {
	var record CanonicalEventRecord
	var payload, revisionJSON, artifactsJSON []byte
	var purgeAfter, purgedAt sql.NullTime
	event := &record.Event
	if err := rows.Scan(
		&event.TenantID, &event.EventID, &event.ReceiptID, &event.EventType,
		&event.SourceMessageID, &event.CorrelationID, &event.Classification,
		&event.RecordedAt, &payload, &purgeAfter, &purgedAt,
		&record.ReceiptStatus, &revisionJSON, &artifactsJSON,
	); err != nil {
		return CanonicalEventRecord{}, fmt.Errorf("scan operator canonical event: %w", err)
	}
	event.RecordedAt = event.RecordedAt.UTC()
	if purgeAfter.Valid {
		event.PurgeAfter = optionalTime(purgeAfter.Time)
	}
	if purgedAt.Valid {
		event.PurgedAt = optionalTime(purgedAt.Time)
	}
	event.PayloadFields, event.PayloadTruncated = summarizePayload(payload)
	if err := json.Unmarshal(revisionJSON, &record.Definition); err != nil {
		return CanonicalEventRecord{}, fmt.Errorf("decode operator canonical event definition: %w", err)
	}
	if len(artifactsJSON) > 0 {
		var artifacts integration.ExecutionArtifactRevisions
		if err := json.Unmarshal(artifactsJSON, &artifacts); err != nil {
			return CanonicalEventRecord{}, fmt.Errorf("decode operator canonical event lineage: %w", err)
		}
		if artifacts.Source.ArtifactID != "" {
			source := artifacts.Source
			record.Source = &source
		}
	}
	return record, nil
}

// AdmissionStatistics counts one window from columns only, inside one
// read-only REPEATABLE READ transaction so every count describes the same
// snapshot: an admission committing mid-read cannot appear in the series and
// not in the totals.
func (s *PostgresReadStore) AdmissionStatistics(
	ctx context.Context,
	tenantID string,
	request StatisticsRequest,
) (AdmissionStatistics, error) {
	if s == nil || s.db == nil || ctx == nil || !validToken(tenantID, 256) {
		return AdmissionStatistics{}, ErrUnavailable
	}
	window, starts, err := request.normalize()
	if err != nil {
		return AdmissionStatistics{}, err
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		return AdmissionStatistics{}, fmt.Errorf("begin operator statistics: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	stats := AdmissionStatistics{From: window.From, To: window.To, Bucket: window.Bucket}
	series := make([]StatisticsBucketCounts, len(starts))
	index := make(map[int64]int, len(starts))
	for position, start := range starts {
		series[position] = StatisticsBucketCounts{Start: start}
		index[start.Unix()] = position
	}
	bucketOf := func(value time.Time) (*StatisticsBucketCounts, error) {
		position, ok := index[value.UTC().Unix()]
		if !ok {
			return nil, fmt.Errorf("operator statistics bucket %s outside the window", value.UTC().Format(time.RFC3339))
		}
		return &series[position], nil
	}
	args := []any{tenantID, window.From, window.To}

	// Receipts per bucket and status; the receipt totals are their sum.
	if err := forEachRow(ctx, tx, "receipt series", `
		SELECT date_trunc($4, r.recorded_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
			r.status, COUNT(*)
		FROM integration_receipts r
		WHERE r.tenant_id = $1 AND r.recorded_at >= $2 AND r.recorded_at < $3
		GROUP BY 1, 2
	`, append(args, string(window.Bucket)), func(rows *sql.Rows) error {
		var start time.Time
		var status string
		var count int
		if err := rows.Scan(&start, &status, &count); err != nil {
			return err
		}
		bucket, err := bucketOf(start)
		if err != nil {
			return err
		}
		switch status {
		case "accepted":
			bucket.Accepted += count
			stats.AcceptedReceipts += count
		case "rejected":
			bucket.Rejected += count
			stats.RejectedReceipts += count
		default:
			// The schema's CHECK admits only these two; a third would be
			// silently missing from every total, so the read fails instead.
			return fmt.Errorf("unknown receipt status %q", status)
		}
		return nil
	}); err != nil {
		return AdmissionStatistics{}, err
	}

	// Delivery attempts per bucket and status; the attempt totals are their sum.
	if err := forEachRow(ctx, tx, "attempt series", `
		SELECT date_trunc($4, a.recorded_at AT TIME ZONE 'UTC') AT TIME ZONE 'UTC',
			a.status, COUNT(*)
		FROM integration_delivery_attempts a
		WHERE a.tenant_id = $1 AND a.recorded_at >= $2 AND a.recorded_at < $3
		GROUP BY 1, 2
	`, append(args, string(window.Bucket)), func(rows *sql.Rows) error {
		var start time.Time
		var status string
		var count int
		if err := rows.Scan(&start, &status, &count); err != nil {
			return err
		}
		bucket, err := bucketOf(start)
		if err != nil {
			return err
		}
		switch status {
		case "queued":
			bucket.Queued += count
			stats.QueuedAttempts += count
		case "succeeded":
			bucket.Succeeded += count
			stats.SucceededAttempts += count
		case "failed":
			bucket.Failed += count
			stats.FailedAttempts += count
		default:
			return fmt.Errorf("unknown delivery attempt status %q", status)
		}
		return nil
	}); err != nil {
		return AdmissionStatistics{}, err
	}
	stats.Series = series

	// Canonical event totals, including the retention marks.
	if err := tx.QueryRowContext(ctx, `
		SELECT COUNT(*),
			COUNT(*) FILTER (WHERE e.purged_at IS NOT NULL),
			COUNT(*) FILTER (WHERE e.purge_after IS NOT NULL AND e.purged_at IS NULL)
		FROM integration_canonical_events e
		WHERE e.tenant_id = $1 AND e.recorded_at >= $2 AND e.recorded_at < $3
	`, args...).Scan(&stats.CanonicalEvents, &stats.PurgedEvents, &stats.ScheduledForPurge); err != nil {
		return AdmissionStatistics{}, fmt.Errorf("count operator canonical events: %w", err)
	}

	limit := MaxStatisticsGroups + 1
	stats.EventsByType = make([]KeyCount, 0)
	if err := forEachRow(ctx, tx, "events by type", `
		SELECT e.event_type, COUNT(*)
		FROM integration_canonical_events e
		WHERE e.tenant_id = $1 AND e.recorded_at >= $2 AND e.recorded_at < $3
		GROUP BY 1
		ORDER BY 2 DESC, 1
		LIMIT $4
	`, append(args, limit), func(rows *sql.Rows) error {
		var group KeyCount
		if err := rows.Scan(&group.Key, &group.Count); err != nil {
			return err
		}
		stats.EventsByType = append(stats.EventsByType, group)
		return nil
	}); err != nil {
		return AdmissionStatistics{}, err
	}

	stats.ReceiptsByDefinition = make([]DefinitionAdmissions, 0)
	if err := forEachRow(ctx, tx, "receipts by definition", `
		SELECT COALESCE(r.integration_revision->>'artifact_id', ''),
			COALESCE(r.integration_revision->>'revision_id', ''),
			COUNT(*) FILTER (WHERE r.status = 'accepted'),
			COUNT(*) FILTER (WHERE r.status = 'rejected')
		FROM integration_receipts r
		WHERE r.tenant_id = $1 AND r.recorded_at >= $2 AND r.recorded_at < $3
		GROUP BY 1, 2
		ORDER BY COUNT(*) DESC, 1, 2
		LIMIT $4
	`, append(args, limit), func(rows *sql.Rows) error {
		var group DefinitionAdmissions
		if err := rows.Scan(&group.DefinitionID, &group.RevisionID, &group.Accepted, &group.Rejected); err != nil {
			return err
		}
		stats.ReceiptsByDefinition = append(stats.ReceiptsByDefinition, group)
		return nil
	}); err != nil {
		return AdmissionStatistics{}, err
	}

	stats.AttemptsByDestination = make([]DestinationAttempts, 0)
	if err := forEachRow(ctx, tx, "attempts by destination", `
		SELECT COALESCE(a.destination_revision_json->>'artifact_id', ''),
			COUNT(*) FILTER (WHERE a.status = 'queued'),
			COUNT(*) FILTER (WHERE a.status = 'succeeded'),
			COUNT(*) FILTER (WHERE a.status = 'failed')
		FROM integration_delivery_attempts a
		WHERE a.tenant_id = $1 AND a.recorded_at >= $2 AND a.recorded_at < $3
		GROUP BY 1
		ORDER BY COUNT(*) DESC, 1
		LIMIT $4
	`, append(args, limit), func(rows *sql.Rows) error {
		var group DestinationAttempts
		if err := rows.Scan(&group.DestinationArtifactID, &group.Queued, &group.Succeeded, &group.Failed); err != nil {
			return err
		}
		stats.AttemptsByDestination = append(stats.AttemptsByDestination, group)
		return nil
	}); err != nil {
		return AdmissionStatistics{}, err
	}

	if len(stats.EventsByType) > MaxStatisticsGroups {
		stats.EventsByType = stats.EventsByType[:MaxStatisticsGroups]
		stats.GroupsTruncated = true
	}
	if len(stats.ReceiptsByDefinition) > MaxStatisticsGroups {
		stats.ReceiptsByDefinition = stats.ReceiptsByDefinition[:MaxStatisticsGroups]
		stats.GroupsTruncated = true
	}
	if len(stats.AttemptsByDestination) > MaxStatisticsGroups {
		stats.AttemptsByDestination = stats.AttemptsByDestination[:MaxStatisticsGroups]
		stats.GroupsTruncated = true
	}
	if err := tx.Commit(); err != nil {
		return AdmissionStatistics{}, fmt.Errorf("commit operator statistics: %w", err)
	}
	return stats, nil
}

// forEachRow runs one statistics query inside the snapshot transaction and
// hands every row to scan.
func forEachRow(
	ctx context.Context,
	tx *sql.Tx,
	name string,
	query string,
	args []any,
	scan func(*sql.Rows) error,
) error {
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return fmt.Errorf("operator statistics %s: %w", name, err)
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return fmt.Errorf("operator statistics %s: %w", name, err)
		}
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("operator statistics %s: %w", name, err)
	}
	return nil
}
