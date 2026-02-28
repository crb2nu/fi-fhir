package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"github.com/google/uuid"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/api/graphql/model"
)

// PostgresEventStore is a PostgreSQL-backed implementation of EventStore.
// Events are stored in a graphql_events table with JSONB payloads;
// patient demographics are denormalized into graphql_patients for fast lookups.
type PostgresEventStore struct {
	db *sql.DB

	// In-process subscription hub (same pattern as MemoryStore).
	subMu       sync.RWMutex
	subscribers []chan model.Event
}

// NewPostgresEventStore creates a new PostgreSQL-backed event store.
func NewPostgresEventStore(db *sql.DB) *PostgresEventStore {
	return &PostgresEventStore{db: db}
}

// InitSchema creates the graphql_events and graphql_patients tables idempotently.
func (s *PostgresEventStore) InitSchema(ctx context.Context) error {
	schema := `
		CREATE TABLE IF NOT EXISTS graphql_events (
			id TEXT PRIMARY KEY,
			event_type TEXT NOT NULL,
			source TEXT NOT NULL DEFAULT '',
			source_format TEXT,
			correlation_id TEXT,
			timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
			payload JSONB NOT NULL
		);

		CREATE INDEX IF NOT EXISTS idx_graphql_events_type ON graphql_events(event_type);
		CREATE INDEX IF NOT EXISTS idx_graphql_events_source ON graphql_events(source);
		CREATE INDEX IF NOT EXISTS idx_graphql_events_timestamp ON graphql_events(timestamp);
		CREATE INDEX IF NOT EXISTS idx_graphql_events_correlation ON graphql_events(correlation_id) WHERE correlation_id IS NOT NULL;

		CREATE TABLE IF NOT EXISTS graphql_patients (
			mrn TEXT PRIMARY KEY,
			family_name TEXT NOT NULL DEFAULT '',
			given_name TEXT NOT NULL DEFAULT '',
			payload JSONB NOT NULL,
			updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
		);

		CREATE INDEX IF NOT EXISTS idx_graphql_patients_name ON graphql_patients(lower(family_name), lower(given_name));
	`
	_, err := s.db.ExecContext(ctx, schema)
	return err
}

// SaveEvent stores an event in PostgreSQL and notifies subscribers.
func (s *PostgresEventStore) SaveEvent(ctx context.Context, event model.Event) (string, error) {
	id := event.GetID()
	if id == "" {
		id = "evt_" + strings.ReplaceAll(uuid.NewString(), "-", "")
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return "", fmt.Errorf("marshal event payload: %w", err)
	}

	var sourceFormat *string
	if sf := event.GetSourceFormat(); sf != nil {
		s := sf.String()
		sourceFormat = &s
	}

	_, err = s.db.ExecContext(ctx, `
		INSERT INTO graphql_events (id, event_type, source, source_format, correlation_id, timestamp, payload)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET payload = EXCLUDED.payload
	`, id, string(event.GetType()), event.GetSource(), sourceFormat,
		event.GetCorrelationID(), event.GetTimestamp().UTC(), string(payload))
	if err != nil {
		return "", fmt.Errorf("insert event: %w", err)
	}

	// Upsert patient if this event carries patient demographics.
	if patient := extractPatient(event); patient != nil {
		patientPayload, err := json.Marshal(patient)
		if err == nil {
			_, _ = s.db.ExecContext(ctx, `
				INSERT INTO graphql_patients (mrn, family_name, given_name, payload, updated_at)
				VALUES ($1, $2, $3, $4, NOW())
				ON CONFLICT (mrn) DO UPDATE SET
					family_name = EXCLUDED.family_name,
					given_name  = EXCLUDED.given_name,
					payload     = EXCLUDED.payload,
					updated_at  = NOW()
			`, patient.MRN, patient.FamilyName, patient.GivenName, string(patientPayload))
		}
	}

	// Fan out to in-process subscribers.
	s.broadcast(event)

	return id, nil
}

// GetEvent retrieves a single event by ID, deserializing from JSONB.
func (s *PostgresEventStore) GetEvent(ctx context.Context, id string) (model.Event, error) {
	var eventType string
	var payload []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT event_type, payload FROM graphql_events WHERE id = $1
	`, id).Scan(&eventType, &payload)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("event not found: %s", id)
	}
	if err != nil {
		return nil, fmt.Errorf("get event: %w", err)
	}

	return deserializeEvent(model.EventType(eventType), payload)
}

// QueryEvents retrieves events matching the filter with cursor-based pagination.
func (s *PostgresEventStore) QueryEvents(ctx context.Context, filter *model.EventFilter, first int, after *string, orderBy *model.EventOrderBy) (*model.EventConnection, error) {
	// Build dynamic WHERE clause.
	where, args, _ := buildEventWhere(filter, 1)

	// Count total.
	countQuery := "SELECT COUNT(*) FROM graphql_events" + where
	var totalCount int
	if err := s.db.QueryRowContext(ctx, countQuery, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count events: %w", err)
	}

	// Build ORDER BY.
	orderCol, orderDir := "timestamp", "DESC"
	if orderBy != nil {
		switch orderBy.Field {
		case model.EventOrderFieldTimestamp:
			orderCol = "timestamp"
		case model.EventOrderFieldType:
			orderCol = "event_type"
		case model.EventOrderFieldSource:
			orderCol = "source"
		}
		if orderBy.Direction == model.OrderDirectionAsc {
			orderDir = "ASC"
		}
	}

	// Cursor-based pagination: decode after cursor to get the after-ID,
	// then fetch from DB with OFFSET semantics via a subquery.
	// For simplicity, we use offset-based: decode cursor → find position.
	// (Matches the MemoryStore approach for API compatibility.)
	query := fmt.Sprintf(
		"SELECT id, event_type, payload FROM graphql_events%s ORDER BY %s %s, id ASC",
		where, orderCol, orderDir,
	)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// Collect all matching events.
	type eventRow struct {
		id        string
		eventType string
		payload   []byte
	}
	var allRows []eventRow
	for rows.Next() {
		var r eventRow
		if err := rows.Scan(&r.id, &r.eventType, &r.payload); err != nil {
			return nil, fmt.Errorf("scan event: %w", err)
		}
		allRows = append(allRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate events: %w", err)
	}

	// Apply cursor-based pagination.
	startIdx := 0
	if after != nil && *after != "" {
		afterID, err := decodeCursor(*after)
		if err == nil {
			for i, r := range allRows {
				if r.id == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}

	endIdx := startIdx + first
	if endIdx > len(allRows) {
		endIdx = len(allRows)
	}

	// Build edges.
	edges := make([]model.EventEdge, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		evt, err := deserializeEvent(model.EventType(allRows[i].eventType), allRows[i].payload)
		if err != nil {
			continue // skip corrupt rows
		}
		edges = append(edges, model.EventEdge{
			Cursor: encodeCursor(allRows[i].id),
			Node:   evt,
		})
	}

	var startCursor, endCursor *string
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.EventConnection{
		Edges: edges,
		PageInfo: model.PageInfo{
			HasNextPage:     endIdx < len(allRows),
			HasPreviousPage: startIdx > 0,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}, nil
}

// GetPatient retrieves a patient by MRN.
func (s *PostgresEventStore) GetPatient(ctx context.Context, mrn string) (*model.Patient, error) {
	var payload []byte
	err := s.db.QueryRowContext(ctx, `
		SELECT payload FROM graphql_patients WHERE mrn = $1
	`, mrn).Scan(&payload)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("patient not found: %s", mrn)
	}
	if err != nil {
		return nil, fmt.Errorf("get patient: %w", err)
	}

	var patient model.Patient
	if err := json.Unmarshal(payload, &patient); err != nil {
		return nil, fmt.Errorf("unmarshal patient: %w", err)
	}
	return &patient, nil
}

// QueryPatients retrieves patients matching the filter with pagination.
func (s *PostgresEventStore) QueryPatients(ctx context.Context, filter *model.PatientFilter, first int, after *string) (*model.PatientConnection, error) {
	where := " WHERE 1=1"
	args := make([]any, 0, 4)
	argIdx := 1

	if filter != nil {
		if filter.MRN != nil {
			where += fmt.Sprintf(" AND lower(mrn) LIKE $%d", argIdx)
			args = append(args, "%"+strings.ToLower(*filter.MRN)+"%")
			argIdx++
		}
		if filter.FamilyName != nil {
			where += fmt.Sprintf(" AND lower(family_name) LIKE $%d", argIdx)
			args = append(args, "%"+strings.ToLower(*filter.FamilyName)+"%")
			argIdx++
		}
		if filter.GivenName != nil {
			where += fmt.Sprintf(" AND lower(given_name) LIKE $%d", argIdx)
			args = append(args, "%"+strings.ToLower(*filter.GivenName)+"%")
			argIdx++
		}
	}

	// Count total.
	var totalCount int
	if err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM graphql_patients"+where, args...).Scan(&totalCount); err != nil {
		return nil, fmt.Errorf("count patients: %w", err)
	}

	query := "SELECT mrn, payload FROM graphql_patients" + where + " ORDER BY mrn ASC"
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query patients: %w", err)
	}
	defer func() { _ = rows.Close() }()

	type patientRow struct {
		mrn     string
		payload []byte
	}
	var allRows []patientRow
	for rows.Next() {
		var r patientRow
		if err := rows.Scan(&r.mrn, &r.payload); err != nil {
			return nil, fmt.Errorf("scan patient: %w", err)
		}
		allRows = append(allRows, r)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate patients: %w", err)
	}

	// Cursor pagination.
	startIdx := 0
	if after != nil && *after != "" {
		afterID, err := decodeCursor(*after)
		if err == nil {
			for i, r := range allRows {
				if r.mrn == afterID {
					startIdx = i + 1
					break
				}
			}
		}
	}

	endIdx := startIdx + first
	if endIdx > len(allRows) {
		endIdx = len(allRows)
	}

	edges := make([]model.PatientEdge, 0, endIdx-startIdx)
	for i := startIdx; i < endIdx; i++ {
		var patient model.Patient
		if err := json.Unmarshal(allRows[i].payload, &patient); err != nil {
			continue
		}
		edges = append(edges, model.PatientEdge{
			Cursor: encodeCursor(allRows[i].mrn),
			Node:   patient,
		})
	}

	var startCursor, endCursor *string
	if len(edges) > 0 {
		startCursor = &edges[0].Cursor
		endCursor = &edges[len(edges)-1].Cursor
	}

	return &model.PatientConnection{
		Edges: edges,
		PageInfo: model.PageInfo{
			HasNextPage:     endIdx < len(allRows),
			HasPreviousPage: startIdx > 0,
			StartCursor:     startCursor,
			EndCursor:       endCursor,
		},
		TotalCount: totalCount,
	}, nil
}

// Subscribe creates a channel for receiving events matching the filter.
func (s *PostgresEventStore) Subscribe(ctx context.Context, filter *model.EventFilter) (<-chan model.Event, error) {
	ch := make(chan model.Event, 100)

	s.subMu.Lock()
	s.subscribers = append(s.subscribers, ch)
	s.subMu.Unlock()

	// Clean up on context cancellation.
	go func() {
		<-ctx.Done()
		s.subMu.Lock()
		for i, sub := range s.subscribers {
			if sub == ch {
				s.subscribers = append(s.subscribers[:i], s.subscribers[i+1:]...)
				break
			}
		}
		s.subMu.Unlock()
		close(ch)
	}()

	// Filter relay.
	filtered := make(chan model.Event, 100)
	go func() {
		for event := range ch {
			if matchesFilter(event, filter) {
				select {
				case filtered <- event:
				case <-ctx.Done():
					return
				}
			}
		}
		close(filtered)
	}()

	return filtered, nil
}

// SubscribePatient creates a channel for events related to a specific patient.
func (s *PostgresEventStore) SubscribePatient(ctx context.Context, mrn string) (<-chan model.Event, error) {
	filter := &model.EventFilter{
		PatientMrn: &mrn,
	}
	return s.Subscribe(ctx, filter)
}

// broadcast sends an event to all in-process subscribers.
func (s *PostgresEventStore) broadcast(event model.Event) {
	s.subMu.RLock()
	defer s.subMu.RUnlock()
	for _, ch := range s.subscribers {
		select {
		case ch <- event:
		default:
		}
	}
}

// --- helpers ---

// buildEventWhere builds a SQL WHERE clause from an EventFilter.
func buildEventWhere(filter *model.EventFilter, startIdx int) (string, []any, int) {
	if filter == nil {
		return "", nil, startIdx
	}

	where := ""
	args := make([]any, 0, 6)
	argIdx := startIdx

	clauses := make([]string, 0, 6)

	if len(filter.Types) > 0 {
		placeholders := make([]string, len(filter.Types))
		for i, t := range filter.Types {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, string(t))
			argIdx++
		}
		clauses = append(clauses, fmt.Sprintf("event_type IN (%s)", strings.Join(placeholders, ",")))
	}

	if len(filter.Sources) > 0 {
		placeholders := make([]string, len(filter.Sources))
		for i, src := range filter.Sources {
			placeholders[i] = fmt.Sprintf("$%d", argIdx)
			args = append(args, src)
			argIdx++
		}
		clauses = append(clauses, fmt.Sprintf("source IN (%s)", strings.Join(placeholders, ",")))
	}

	if filter.FromTimestamp != nil {
		clauses = append(clauses, fmt.Sprintf("timestamp >= $%d", argIdx))
		args = append(args, filter.FromTimestamp.UTC())
		argIdx++
	}

	if filter.ToTimestamp != nil {
		clauses = append(clauses, fmt.Sprintf("timestamp <= $%d", argIdx))
		args = append(args, filter.ToTimestamp.UTC())
		argIdx++
	}

	if filter.CorrelationID != nil {
		clauses = append(clauses, fmt.Sprintf("correlation_id = $%d", argIdx))
		args = append(args, *filter.CorrelationID)
		argIdx++
	}

	if filter.PatientMrn != nil {
		clauses = append(clauses, fmt.Sprintf("payload->'patient'->>'mrn' = $%d", argIdx))
		args = append(args, *filter.PatientMrn)
		argIdx++
	}

	if len(clauses) > 0 {
		where = " WHERE " + strings.Join(clauses, " AND ")
	}

	return where, args, argIdx
}

// extractPatient extracts the Patient from events that carry one.
func extractPatient(event model.Event) *model.Patient {
	switch e := event.(type) {
	case *model.PatientAdmitEvent:
		return &e.Patient
	case *model.PatientDischargeEvent:
		return &e.Patient
	case *model.LabResultEvent:
		return &e.Patient
	case *model.VitalSignEvent:
		return &e.Patient
	case *model.ConditionEvent:
		return &e.Patient
	case *model.ProcedureEvent:
		return &e.Patient
	case *model.ImmunizationEvent:
		return &e.Patient
	case *model.AppointmentEvent:
		return &e.Patient
	case *model.DocumentEvent:
		return e.Patient
	}
	return nil
}

// deserializeEvent converts a stored event_type + JSONB payload back into a concrete model.Event.
func deserializeEvent(eventType model.EventType, payload []byte) (model.Event, error) {
	switch eventType {
	case model.EventTypePatientAdmit:
		var e model.PatientAdmitEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypePatientDischarge:
		var e model.PatientDischargeEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeLabResult:
		var e model.LabResultEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeVitalSign:
		var e model.VitalSignEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeCondition:
		var e model.ConditionEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeProcedure:
		var e model.ProcedureEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeImmunization:
		var e model.ImmunizationEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeAppointmentScheduled, model.EventTypeAppointmentCancelled, model.EventTypeAppointmentNoshow:
		var e model.AppointmentEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	case model.EventTypeDocument:
		var e model.DocumentEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, err
		}
		return &e, nil
	default:
		// For event types without a dedicated struct (PATIENT_TRANSFER, PATIENT_UPDATE,
		// LAB_ORDERED, CLAIM_SUBMITTED, CLAIM_ADJUDICATED), store as a generic event.
		// We attempt PatientAdmitEvent as a fallback since it has the most fields.
		var e model.PatientAdmitEvent
		if err := json.Unmarshal(payload, &e); err != nil {
			return nil, fmt.Errorf("unknown event type %q: %w", eventType, err)
		}
		return &e, nil
	}
}

// sortEvents is re-exported from store.go — needed by both MemoryStore and PostgresEventStore.
// The function is already defined in store.go so we don't redefine it here.
// Same for encodeCursor, decodeCursor, matchesFilter.

// Ensure PostgresEventStore implements EventStore at compile time.
var _ EventStore = (*PostgresEventStore)(nil)

// SavePatient stores or updates a patient record in PostgreSQL.
// This mirrors the MemoryStore.SavePatient method used by the resolver.
func (s *PostgresEventStore) SavePatient(patient *model.Patient) {
	if patient == nil {
		return
	}
	payload, err := json.Marshal(patient)
	if err != nil {
		return
	}
	_, _ = s.db.Exec(`
		INSERT INTO graphql_patients (mrn, family_name, given_name, payload, updated_at)
		VALUES ($1, $2, $3, $4, NOW())
		ON CONFLICT (mrn) DO UPDATE SET
			family_name = EXCLUDED.family_name,
			given_name  = EXCLUDED.given_name,
			payload     = EXCLUDED.payload,
			updated_at  = NOW()
	`, patient.MRN, patient.FamilyName, patient.GivenName, string(payload))
}

// Close closes the underlying database connection.
func (s *PostgresEventStore) Close() error {
	return s.db.Close()
}
