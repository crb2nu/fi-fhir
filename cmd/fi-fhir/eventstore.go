package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/cblevins/fi-fhir/pkg/eventsourcing"
	"github.com/cblevins/fi-fhir/pkg/eventsourcing/projections"
	_ "github.com/lib/pq"
)

func runEventStore(args []string) error {
	if len(args) < 1 {
		return printEventStoreUsage()
	}

	switch args[0] {
	case "init":
		return runEventStoreInit(args[1:])
	case "stats":
		return runEventStoreStats(args[1:])
	case "streams":
		return runEventStoreStreams(args[1:])
	case "read":
		return runEventStoreRead(args[1:])
	case "append":
		return runEventStoreAppend(args[1:])
	case "help", "--help", "-h":
		return printEventStoreUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown eventstore subcommand: %s\n", args[0])
		return printEventStoreUsage()
	}
}

func printEventStoreUsage() error {
	fmt.Println(`fi-fhir eventstore - Event Store Management

Usage:
  fi-fhir eventstore <subcommand> [options]

Subcommands:
  init      Initialize event store schema (PostgreSQL)
  stats     Show event store statistics
  streams   List event streams
  read      Read events from a stream or all events
  append    Append events to a stream

Options:
  --db       PostgreSQL connection string (or FI_FHIR_DATABASE_URL env)
  --table    Events table name (default: events)

Examples:
  # Initialize schema
  fi-fhir eventstore init --db "postgres://localhost/fhir?sslmode=disable"

  # Show statistics
  fi-fhir eventstore stats --db "$DATABASE_URL"

  # List all streams
  fi-fhir eventstore streams --db "$DATABASE_URL"

  # Read events from a stream
  fi-fhir eventstore read --stream "patient:MRN001" --db "$DATABASE_URL"

  # Read all events
  fi-fhir eventstore read --all --db "$DATABASE_URL"`)

	return nil
}

func getEventStoreDB(args []string) (string, string, error) {
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")
	tableName := "events"

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--db":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--db requires a value")
			}
			i++
			dbURL = args[i]
		case "--table":
			if i+1 >= len(args) {
				return "", "", fmt.Errorf("--table requires a value")
			}
			i++
			tableName = args[i]
		}
	}

	if dbURL == "" {
		return "", "", fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
	}

	return dbURL, tableName, nil
}

func runEventStoreInit(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	if err := store.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	// Also initialize checkpoint table
	checkpointStore := eventsourcing.NewPostgresCheckpointStore(db, tableName+"_checkpoints")
	if err := checkpointStore.InitSchema(ctx); err != nil {
		return fmt.Errorf("failed to initialize checkpoint schema: %w", err)
	}

	fmt.Printf("Event store schema initialized successfully\n")
	fmt.Printf("  Events table: %s\n", tableName)
	fmt.Printf("  Checkpoints table: %s_checkpoints\n", tableName)

	return nil
}

func runEventStoreStats(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	stats, err := store.GetStats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	lastPos, err := store.GetLastPosition(ctx)
	if err != nil {
		return fmt.Errorf("failed to get last position: %w", err)
	}

	fmt.Println("Event Store Statistics")
	fmt.Println("----------------------")
	fmt.Printf("Total Events:  %d\n", stats.TotalEvents)
	fmt.Printf("Total Streams: %d\n", stats.StreamCount)
	fmt.Printf("Last Position: %d\n", lastPos)
	fmt.Println()

	if len(stats.EventTypes) > 0 {
		fmt.Println("Events by Type:")
		for eventType, count := range stats.EventTypes {
			fmt.Printf("  %-30s %d\n", eventType, count)
		}
	}

	return nil
}

func runEventStoreStreams(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	limit := 100
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--limit":
			if i+1 >= len(args) {
				return fmt.Errorf("--limit requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		}
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Query distinct streams
	query := fmt.Sprintf(`
		SELECT stream_id, MAX(stream_version) as version, COUNT(*) as event_count
		FROM %s
		GROUP BY stream_id
		ORDER BY MAX(position) DESC
		LIMIT $1
	`, tableName)

	rows, err := db.QueryContext(ctx, query, limit)
	if err != nil {
		return fmt.Errorf("failed to query streams: %w", err)
	}
	defer rows.Close()

	fmt.Printf("%-40s %10s %10s\n", "STREAM ID", "VERSION", "EVENTS")
	fmt.Println(strings.Repeat("-", 64))

	for rows.Next() {
		var streamID string
		var version, eventCount int64
		if err := rows.Scan(&streamID, &version, &eventCount); err != nil {
			return fmt.Errorf("failed to scan row: %w", err)
		}
		fmt.Printf("%-40s %10d %10d\n", streamID, version, eventCount)
	}

	return nil
}

func runEventStoreRead(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	var (
		streamID     = ""
		readAll      = false
		fromPosition = int64(0)
		fromVersion  = int64(0)
		limit        = 50
		pretty       = false
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--stream":
			if i+1 >= len(args) {
				return fmt.Errorf("--stream requires a value")
			}
			i++
			streamID = args[i]
		case "--all":
			readAll = true
		case "--from-position":
			if i+1 >= len(args) {
				return fmt.Errorf("--from-position requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &fromPosition)
		case "--from-version":
			if i+1 >= len(args) {
				return fmt.Errorf("--from-version requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &fromVersion)
		case "--limit":
			if i+1 >= len(args) {
				return fmt.Errorf("--limit requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &limit)
		case "--pretty":
			pretty = true
		}
	}

	if streamID == "" && !readAll {
		return fmt.Errorf("must specify --stream or --all")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	var events []eventsourcing.StoredEvent

	if readAll {
		events, err = store.ReadAll(ctx, fromPosition, limit)
	} else {
		events, err = store.ReadStream(ctx, streamID, fromVersion, limit)
	}

	if err != nil {
		return fmt.Errorf("failed to read events: %w", err)
	}

	if len(events) == 0 {
		fmt.Println("No events found")
		return nil
	}

	encoder := json.NewEncoder(os.Stdout)
	if pretty {
		encoder.SetIndent("", "  ")
	}

	for _, event := range events {
		output := map[string]interface{}{
			"position":       event.Position,
			"stream_id":      event.StreamID,
			"stream_version": event.StreamVersion,
			"event_type":     event.EventType,
			"timestamp":      event.Timestamp.Format(time.RFC3339),
			"metadata":       event.Metadata,
		}

		// Try to parse data as JSON
		var data interface{}
		if err := json.Unmarshal(event.Data, &data); err == nil {
			output["data"] = data
		} else {
			output["data"] = string(event.Data)
		}

		encoder.Encode(output)
	}

	return nil
}

func runEventStoreAppend(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	var (
		streamID        = ""
		expectedVersion = eventsourcing.VersionAny
		eventType       = ""
		dataStr         = ""
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--stream":
			if i+1 >= len(args) {
				return fmt.Errorf("--stream requires a value")
			}
			i++
			streamID = args[i]
		case "--version":
			if i+1 >= len(args) {
				return fmt.Errorf("--version requires a value")
			}
			i++
			fmt.Sscanf(args[i], "%d", &expectedVersion)
		case "--type":
			if i+1 >= len(args) {
				return fmt.Errorf("--type requires a value")
			}
			i++
			eventType = args[i]
		case "--data":
			if i+1 >= len(args) {
				return fmt.Errorf("--data requires a value")
			}
			i++
			dataStr = args[i]
		}
	}

	if streamID == "" {
		return fmt.Errorf("--stream is required")
	}
	if eventType == "" {
		return fmt.Errorf("--type is required")
	}
	if dataStr == "" {
		return fmt.Errorf("--data is required (JSON)")
	}

	// Validate JSON
	var data json.RawMessage
	if err := json.Unmarshal([]byte(dataStr), &data); err != nil {
		return fmt.Errorf("--data must be valid JSON: %w", err)
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	newVersion, err := store.Append(ctx, streamID, expectedVersion, []eventsourcing.EventData{
		{
			EventType: eventType,
			Data:      data,
		},
	})

	if err != nil {
		return fmt.Errorf("failed to append event: %w", err)
	}

	fmt.Printf("Event appended successfully\n")
	fmt.Printf("  Stream:  %s\n", streamID)
	fmt.Printf("  Version: %d\n", newVersion)

	return nil
}

func runProjection(args []string) error {
	if len(args) < 1 {
		return printProjectionUsage()
	}

	switch args[0] {
	case "list":
		return runProjectionList(args[1:])
	case "status":
		return runProjectionStatus(args[1:])
	case "run":
		return runProjectionRun(args[1:])
	case "rebuild":
		return runProjectionRebuild(args[1:])
	case "help", "--help", "-h":
		return printProjectionUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown projection subcommand: %s\n", args[0])
		return printProjectionUsage()
	}
}

func printProjectionUsage() error {
	fmt.Println(`fi-fhir projection - Projection Management

Usage:
  fi-fhir projection <subcommand> [options]

Subcommands:
  list      List available projections
  status    Show projection checkpoint status
  run       Run projections (catch up to latest events)
  rebuild   Rebuild a projection from scratch

Options:
  --db      PostgreSQL connection string (or FI_FHIR_DATABASE_URL env)

Examples:
  # List available projections
  fi-fhir projection list

  # Show projection status
  fi-fhir projection status --db "$DATABASE_URL"

  # Run projections once
  fi-fhir projection run --db "$DATABASE_URL"

  # Rebuild patient timeline projection
  fi-fhir projection rebuild --name patient_timeline --db "$DATABASE_URL"

  # Rebuild from snapshot (faster recovery)
  fi-fhir projection rebuild --name patient_timeline --from-snapshot --db "$DATABASE_URL"

  # Rebuild all projections in parallel
  fi-fhir projection rebuild --all --parallel --db "$DATABASE_URL"

  # Dry run to see how many events would be processed
  fi-fhir projection rebuild --name event_statistics --dry-run --db "$DATABASE_URL"

  # Rebuild from specific position
  fi-fhir projection rebuild --name active_encounters --from-position 1000 --db "$DATABASE_URL"`)

	return nil
}

func runProjectionList(args []string) error {
	fmt.Println("Available Projections")
	fmt.Println("---------------------")
	fmt.Printf("%-25s %s\n", "NAME", "DESCRIPTION")
	fmt.Printf("%-25s %s\n", "patient_timeline", "Chronological patient event history")
	fmt.Printf("%-25s %s\n", "event_statistics", "Aggregate event counts by type/source")
	fmt.Printf("%-25s %s\n", "active_encounters", "Current active patient encounters")

	return nil
}

func runProjectionStatus(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	checkpointStore := eventsourcing.NewPostgresCheckpointStore(db, tableName+"_checkpoints")

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	lastPosition, err := store.GetLastPosition(ctx)
	if err != nil {
		return fmt.Errorf("failed to get last position: %w", err)
	}

	projectionNames := []string{"patient_timeline", "event_statistics", "active_encounters"}

	fmt.Println("Projection Status")
	fmt.Println("-----------------")
	fmt.Printf("Last Event Position: %d\n\n", lastPosition)

	fmt.Printf("%-25s %12s %12s %s\n", "PROJECTION", "CHECKPOINT", "BEHIND", "STATUS")
	fmt.Println(strings.Repeat("-", 60))

	for _, name := range projectionNames {
		checkpoint, err := checkpointStore.GetCheckpoint(ctx, name)
		if err != nil {
			fmt.Printf("%-25s %12s %12s %s\n", name, "error", "-", err.Error())
			continue
		}

		behind := lastPosition - checkpoint
		if checkpoint < 0 {
			behind = lastPosition + 1
		}

		status := "up-to-date"
		if behind > 0 {
			status = "catching up"
		}
		if checkpoint < 0 {
			status = "not started"
			checkpoint = -1
		}

		fmt.Printf("%-25s %12d %12d %s\n", name, checkpoint, behind, status)
	}

	return nil
}

func runProjectionRun(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	var projectionName = "" // empty means all

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			projectionName = args[i]
		}
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	checkpointStore := eventsourcing.NewPostgresCheckpointStore(db, tableName+"_checkpoints")

	runner := eventsourcing.NewProjectionRunner(store, checkpointStore, eventsourcing.DefaultProjectionRunnerConfig())

	// Register projections
	allProjections := []eventsourcing.Projection{
		projections.NewPatientTimelineProjection(),
		projections.NewEventStatisticsProjection(),
		projections.NewActiveEncountersProjection(),
	}

	for _, p := range allProjections {
		if projectionName == "" || p.Name() == projectionName {
			runner.RegisterProjection(p)
		}
	}

	fmt.Println("Running projections...")
	if err := runner.RunOnce(ctx); err != nil {
		return fmt.Errorf("projection run failed: %w", err)
	}

	fmt.Println("Projections updated successfully")

	return nil
}

func runProjectionRebuild(args []string) error {
	dbURL, tableName, err := getEventStoreDB(args)
	if err != nil {
		return err
	}

	var (
		projectionName string
		allProjections bool
		fromSnapshot   bool
		fromPosition   int64
		stopPosition   int64
		dryRun         bool
		parallel       bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--name":
			if i+1 >= len(args) {
				return fmt.Errorf("--name requires a value")
			}
			i++
			projectionName = args[i]
		case "--all":
			allProjections = true
		case "--from-snapshot":
			fromSnapshot = true
		case "--from-position":
			if i+1 >= len(args) {
				return fmt.Errorf("--from-position requires a value")
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &fromPosition); err != nil {
				return fmt.Errorf("invalid --from-position: %s", args[i])
			}
		case "--stop-position":
			if i+1 >= len(args) {
				return fmt.Errorf("--stop-position requires a value")
			}
			i++
			if _, err := fmt.Sscanf(args[i], "%d", &stopPosition); err != nil {
				return fmt.Errorf("invalid --stop-position: %s", args[i])
			}
		case "--dry-run":
			dryRun = true
		case "--parallel":
			parallel = true
		}
	}

	if !allProjections && projectionName == "" {
		return fmt.Errorf("--name or --all is required for rebuild")
	}

	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer db.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	store := eventsourcing.NewPostgresStore(db, eventsourcing.PostgresStoreConfig{
		TableName: tableName,
	})

	checkpointStore := eventsourcing.NewPostgresCheckpointStore(db, tableName+"_checkpoints")
	snapshotStore := eventsourcing.NewPostgresSnapshotStore(db, tableName+"_snapshots")

	// Create rebuilder with all projections
	rebuilder := eventsourcing.NewProjectionRebuilder(store, checkpointStore, snapshotStore)
	rebuilder.RegisterProjection(projections.NewPatientTimelineProjection())
	rebuilder.RegisterProjection(projections.NewEventStatisticsProjection())
	rebuilder.RegisterProjection(projections.NewActiveEncountersProjection())

	config := &eventsourcing.RebuildConfig{
		FromSnapshot: fromSnapshot,
		FromPosition: fromPosition,
		StopPosition: stopPosition,
		DryRun:       dryRun,
		Progress: func(stats *eventsourcing.RebuildProgress) {
			if stats.Complete {
				return // Final stats printed below
			}
			fmt.Printf("\r  [%s] %d events processed (%.1f/sec)",
				stats.ProjectionName,
				stats.EventsProcessed,
				stats.EventsPerSecond)
		},
	}

	if dryRun {
		fmt.Println("DRY RUN - no changes will be made")
	}

	var results []*eventsourcing.RebuildResult

	if allProjections {
		fmt.Println("Rebuilding all projections...")
		if parallel {
			results, err = rebuilder.RebuildAllParallel(ctx, config)
		} else {
			results, err = rebuilder.RebuildAll(ctx, config)
		}
	} else {
		if _, ok := rebuilder.GetProjection(projectionName); !ok {
			return fmt.Errorf("unknown projection: %s", projectionName)
		}
		fmt.Printf("Rebuilding projection '%s'...\n", projectionName)
		result, rerr := rebuilder.Rebuild(ctx, projectionName, config)
		results = []*eventsourcing.RebuildResult{result}
		err = rerr
	}

	fmt.Println() // Clear progress line

	// Print results
	for _, result := range results {
		if result.Error != nil {
			fmt.Printf("✗ %s: FAILED - %v\n", result.ProjectionName, result.Error)
		} else {
			snapshotInfo := ""
			if result.SnapshotRestored {
				snapshotInfo = fmt.Sprintf(" (restored from snapshot at %d)", result.SnapshotPosition)
			}
			fmt.Printf("✓ %s: %d events processed in %v (%.1f/sec)%s\n",
				result.ProjectionName,
				result.EventsProcessed,
				result.Duration.Round(time.Millisecond),
				result.EventsPerSecond,
				snapshotInfo)
		}
	}

	if err != nil {
		return fmt.Errorf("rebuild failed: %w", err)
	}

	return nil
}
