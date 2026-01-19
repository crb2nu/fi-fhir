package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"reflect"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/config"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
)

func loadTerminologyPinConfigFromEnv() (dbURL string, pins map[string]string, policy string) {
	cfg := config.LoadFromEnv()
	dbURL = strings.TrimSpace(cfg.Terminology.DBURL)
	if dbURL == "" {
		dbURL = strings.TrimSpace(os.Getenv("FI_FHIR_DATABASE_URL"))
	}

	pins = cfg.Terminology.Pins
	if pins == nil {
		pins = make(map[string]string)
	}

	policy = strings.ToLower(strings.TrimSpace(cfg.Terminology.Policy))
	if policy == "" {
		policy = "warn"
	}
	return dbURL, pins, policy
}

func checkTerminologyPins(ctx context.Context, dbURL string, pins map[string]string, policy string) ([]events.ParseWarning, error) {
	policy = strings.ToLower(strings.TrimSpace(policy))
	switch policy {
	case "", "warn":
		policy = "warn"
	case "pass", "error":
	default:
		return nil, fmt.Errorf("invalid terminology pin policy: %q", policy)
	}

	if policy == "pass" || dbURL == "" || len(pins) == 0 {
		return nil, nil
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to terminology database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	statusCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	migrator := db.NewMigrator(conn)
	statuses, err := migrator.CheckPinnedReleases(statusCtx, pins)
	if err != nil {
		return nil, fmt.Errorf("failed to check terminology pins: %w", err)
	}

	var (
		warnings  []events.ParseWarning
		hasIssues bool
	)

	severity := "warning"
	if policy == "error" {
		severity = "error"
	}

	for _, s := range statuses {
		if s.Match {
			continue
		}
		hasIssues = true

		if !s.ActiveReleaseSet {
			warnings = append(warnings, events.ParseWarning{
				Phase:    "terminology",
				Code:     "PIN_NOT_LOADED",
				Message:  fmt.Sprintf("%s active release not set (expected %s)", s.Vocabulary, s.ExpectedVersion),
				Severity: severity,
			})
			continue
		}

		warnings = append(warnings, events.ParseWarning{
			Phase:    "terminology",
			Code:     "PIN_MISMATCH",
			Message:  fmt.Sprintf("%s active release %s does not match pinned %s", s.Vocabulary, s.ActiveVersion, s.ExpectedVersion),
			Severity: severity,
		})
	}

	if hasIssues && policy == "error" {
		return warnings, fmt.Errorf("terminology pins do not match active releases (set FI_FHIR_TERMINOLOGY_POLICY=warn to continue)")
	}

	return warnings, nil
}

func appendParseWarningsToOutputData(outputData interface{}, extra []events.ParseWarning) {
	if outputData == nil || len(extra) == 0 {
		return
	}

	// Handle structs with an Events field (e.g. CSV ParseResult)
	rv := reflect.ValueOf(outputData)
	if rv.IsValid() && rv.Kind() == reflect.Pointer && !rv.IsNil() && rv.Elem().Kind() == reflect.Struct {
		eventsField := rv.Elem().FieldByName("Events")
		if eventsField.IsValid() && eventsField.Kind() == reflect.Slice {
			for i := 0; i < eventsField.Len(); i++ {
				appendParseWarningsToEvent(eventsField.Index(i).Interface(), extra)
			}
		}
	}

	switch v := outputData.(type) {
	case []interface{}:
		for _, evt := range v {
			appendParseWarningsToEvent(evt, extra)
		}
	case map[string]interface{}:
		if evt, ok := v["event"]; ok {
			appendParseWarningsToEvent(evt, extra)
		}
		if evts, ok := v["events"].([]interface{}); ok {
			for _, evt := range evts {
				appendParseWarningsToEvent(evt, extra)
			}
		}
	default:
		appendParseWarningsToEvent(v, extra)
	}
}

func appendParseWarningsToEvent(evt interface{}, extra []events.ParseWarning) bool {
	if evt == nil || len(extra) == 0 {
		return false
	}

	v := reflect.ValueOf(evt)
	for v.IsValid() && (v.Kind() == reflect.Interface) {
		v = v.Elem()
	}

	if !v.IsValid() || v.Kind() != reflect.Pointer || v.IsNil() {
		return false
	}

	s := v.Elem()
	if s.Kind() != reflect.Struct {
		return false
	}

	meta := s.FieldByName("EventMeta")
	if !meta.IsValid() || meta.Kind() != reflect.Struct {
		return false
	}

	pws := meta.FieldByName("ParseWarnings")
	if !pws.IsValid() || pws.Kind() != reflect.Slice || !pws.CanSet() {
		return false
	}

	current, ok := pws.Interface().([]events.ParseWarning)
	if !ok {
		return false
	}

	pws.Set(reflect.ValueOf(append(current, extra...)))
	return true
}
