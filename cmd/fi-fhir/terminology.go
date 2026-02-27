package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/autoroute"
	termworkflow "gitlab.flexinfer.ai/libs/fi-fhir/internal/terminology/workflow"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/semantic"
	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/upload"
)

func runTerminology(args []string) error {
	if len(args) == 0 {
		printTerminologyUsage()
		return nil
	}

	switch args[0] {
	case "init":
		return runTerminologyInit(args[1:])
	case "status":
		return runTerminologyStatus(args[1:])
	case "use":
		return runTerminologyUse(args[1:])
	case "drop":
		return runTerminologyDrop(args[1:])
	case "load":
		return runTerminologyLoad(args[1:])
	case "crosswalk":
		return runTerminologyCrosswalk(args[1:])
	case "search":
		return runTerminologySearch(args[1:])
	case "mapping":
		return runTerminologyMapping(args[1:])
	case "autoroute":
		return runTerminologyAutoroute(args[1:])
	case "-h", "--help", "help":
		printTerminologyUsage()
		return nil
	default:
		return fmt.Errorf("unknown terminology subcommand: %s", args[0])
	}
}

func printTerminologyUsage() {
	fmt.Println(`fi-fhir terminology - Terminology Database Management

Usage:
  fi-fhir terminology <subcommand> [options]

Subcommands:
  init      Initialize terminology schema in PostgreSQL
  status    Show terminology database status and loaded releases
  use       Set active version for a vocabulary
  drop      Drop the terminology schema (WARNING: deletes all data)
  load      Load terminology data from files (LOINC, UMLS, SNOMED, etc.)
  crosswalk Translate codes between vocabularies
  search    Semantic search for terminology codes (LLM embeddings)
  mapping   Manage custom code mappings (upload, list, delete)
  autoroute Run autoroute engine for a source code (optionally via Temporal workflow)

Options:
  --db      PostgreSQL connection string (or FI_FHIR_TERMINOLOGY_DB_URL env)

Examples:
  # Initialize terminology schema
  fi-fhir terminology init --db "$DATABASE_URL"

  # Check status of loaded terminologies
  fi-fhir terminology status --db "$DATABASE_URL"

  # Switch the active version for a vocabulary
  fi-fhir terminology use loinc 2.77 --db "$DATABASE_URL"

  # Load LOINC codes
  fi-fhir terminology load loinc /path/to/LoincTable.csv --version 2.77

  # Load UMLS Metathesaurus
  fi-fhir terminology load umls /path/to/META/ --version 2024AB

  # Load RxNorm
  fi-fhir terminology load rxnorm /path/to/rrf/ --version 2024-01

  # Load SNOMED CT US Edition
  fi-fhir terminology load snomed /path/to/RF2/ --version 2024-03

  # Cross-walk a code between vocabularies
  fi-fhir terminology crosswalk E11.9 --from ICD10CM --to SNOMEDCT_US

  # Semantic search for terminology codes (requires Qdrant)
  fi-fhir terminology search --query "blood glucose" --vocabulary loinc --limit 10`)
}

func getTerminologyDBURL(args []string) string {
	// Check for --db flag
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--db" {
			return args[i+1]
		}
	}
	// Fall back to environment variables (terminology-specific first, then legacy FI_FHIR_DATABASE_URL)
	if v := os.Getenv("FI_FHIR_TERMINOLOGY_DB_URL"); v != "" {
		return v
	}
	return os.Getenv("FI_FHIR_DATABASE_URL")
}

func runTerminologyInit(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	migrator := db.NewMigrator(conn)

	created, err := migrator.Initialize(ctx)
	if err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	if created {
		fmt.Println("Terminology schema created successfully")
	} else {
		fmt.Println("Terminology schema already exists (up to date)")
	}

	// Get current version
	version, err := migrator.CurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("failed to get schema version: %w", err)
	}
	fmt.Printf("Schema version: %d\n", version)

	return nil
}

func runTerminologyStatus(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrator := db.NewMigrator(conn)
	stats, err := migrator.Stats(ctx)
	if err != nil {
		return fmt.Errorf("failed to get stats: %w", err)
	}

	fmt.Println("Terminology Database Status")
	fmt.Println("---------------------------")
	fmt.Printf("Schema Version: %d\n", stats.SchemaVersion)

	if stats.SchemaVersion == 0 {
		fmt.Println("\nSchema not initialized. Run 'fi-fhir terminology init' first.")
		return nil
	}

	fmt.Printf("Total Rows:     %d\n", stats.TotalRows)

	if len(stats.Releases) > 0 {
		fmt.Println("\nLoaded Releases:")
		fmt.Printf("%-15s %-15s %-8s %12s  %s\n", "VOCABULARY", "VERSION", "ACTIVE", "ROWS", "LOADED")
		fmt.Println("----------------------------------------------------------------")
		for _, r := range stats.Releases {
			active := " "
			if r.IsActive {
				active = "*"
			}
			fmt.Printf("%-15s %-15s %-8s %12d  %s\n",
				r.Vocabulary, r.Version, active, r.RowCount, r.LoadedAt.Format("2006-01-02"))
		}
	} else {
		fmt.Println("\nNo terminology data loaded yet.")
		fmt.Println("Use 'fi-fhir terminology load' to import LOINC, UMLS, SNOMED, etc.")
	}

	if len(stats.Tables) > 0 {
		fmt.Println("\nTable Statistics:")
		for table, ts := range stats.Tables {
			if ts.RowCount > 0 {
				fmt.Printf("  %-35s %12d rows\n", table, ts.RowCount)
			}
		}
	}

	// Optional: compare pinned versions (from env/config) to active releases
	_, pins, policy := loadTerminologyPinConfigFromEnv()
	if len(pins) > 0 {
		pinStatuses, err := migrator.CheckPinnedReleases(ctx, pins)
		if err != nil {
			return fmt.Errorf("failed to check terminology pins: %w", err)
		}

		var mismatches int
		fmt.Println("\nPinned Versions:")
		fmt.Printf("%-15s %-15s %-15s %-10s\n", "VOCABULARY", "PINNED", "ACTIVE", "STATUS")
		fmt.Println("---------------------------------------------------------------")
		for _, ps := range pinStatuses {
			status := "MISMATCH"
			active := ps.ActiveVersion
			if !ps.ActiveReleaseSet {
				active = "(none)"
				status = "NOT_LOADED"
				mismatches++
			} else if ps.Match {
				status = "OK"
			} else {
				mismatches++
			}
			fmt.Printf("%-15s %-15s %-15s %-10s\n", ps.Vocabulary, ps.ExpectedVersion, active, status)
		}

		policy = strings.ToLower(strings.TrimSpace(policy))
		if mismatches > 0 && policy == "error" {
			return fmt.Errorf("terminology pins do not match active releases (%d issue(s))", mismatches)
		}
	}

	return nil
}

func runTerminologyUse(args []string) error {
	if len(args) < 2 {
		fmt.Println(`Usage: fi-fhir terminology use <vocabulary> <version> [options]

Options:
  --db      PostgreSQL connection string (or FI_FHIR_TERMINOLOGY_DB_URL env)

Examples:
  fi-fhir terminology use loinc 2.77 --db "$DATABASE_URL"`)
		return nil
	}

	vocab := strings.TrimSpace(args[0])
	version := strings.TrimSpace(args[1])
	if vocab == "" || version == "" {
		return fmt.Errorf("vocabulary and version are required")
	}

	extraArgs := []string{}
	if len(args) > 2 {
		extraArgs = args[2:]
	}
	dbURL := getTerminologyDBURL(extraArgs)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	migrator := db.NewMigrator(conn)
	if err := migrator.SetActiveRelease(ctx, strings.ToUpper(vocab), version); err != nil {
		return fmt.Errorf("failed to set active release: %w", err)
	}

	fmt.Printf("Set active release: %s %s\n", strings.ToUpper(vocab), version)
	return nil
}

func runTerminologyDrop(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Check for --force flag
	force := false
	for _, arg := range args {
		if arg == "--force" || arg == "-f" {
			force = true
		}
	}

	if !force {
		fmt.Println("WARNING: This will delete ALL terminology data!")
		fmt.Println("Use --force to confirm.")
		return nil
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	migrator := db.NewMigrator(conn)
	if err := migrator.Drop(ctx); err != nil {
		return fmt.Errorf("failed to drop schema: %w", err)
	}

	fmt.Println("Terminology schema dropped successfully")
	return nil
}

func runTerminologyLoad(args []string) error {
	if len(args) < 2 {
		fmt.Println(`Usage: fi-fhir terminology load <vocabulary> <path> [options]

Vocabularies:
  loinc    Load from LoincTable.csv
  umls     Load from UMLS META directory (MRCONSO.RRF, MRREL.RRF)
  rxnorm   Load from RxNorm RRF files
  snomed   Load from SNOMED CT RF2 release
  icd10cm  Load from ICD-10-CM XML or tabular file
  icd10pcs Load from ICD-10-PCS XML or tabular file

Options:
  --db       Database connection URL
  --version  Release version (e.g., "2.77", "2024AB", "FY2024")
  --date     Release date (YYYY-MM-DD)
  --dry-run  Validate inputs and print what would run (no DB required)

Examples:
  fi-fhir terminology load loinc /data/loinc/LoincTable.csv --version 2.77
  fi-fhir terminology load umls /data/umls/META/ --version 2024AB
  fi-fhir terminology load snomed /data/snomed/RF2/ --version 2024-03-01`)
		return nil
	}

	vocab := args[0]
	path := args[1]

	// Parse remaining args
	var version string
	var dateStr string
	var dryRun bool

	for i := 2; i < len(args); {
		switch args[i] {
		case "--version":
			if i+1 < len(args) {
				version = args[i+1]
				i += 2
				continue
			}
		case "--date":
			if i+1 < len(args) {
				dateStr = args[i+1]
				i += 2
				continue
			}
		case "--dry-run":
			dryRun = true
			i++
			continue
		}
		i++
	}

	if version == "" {
		return fmt.Errorf("--version is required")
	}

	// Parse release date if provided
	var releaseDate *time.Time
	if dateStr != "" {
		t, err := time.Parse("2006-01-02", dateStr)
		if err != nil {
			return fmt.Errorf("invalid date format (use YYYY-MM-DD): %w", err)
		}
		releaseDate = &t
	}

	if dryRun {
		switch vocab {
		case "loinc":
			return loadLOINC(context.Background(), nil, nil, path, version, releaseDate, true)
		case "umls":
			return loadUMLS(context.Background(), nil, nil, path, version, releaseDate, true)
		case "rxnorm":
			return loadRxNorm(context.Background(), nil, nil, path, version, releaseDate, true)
		case "snomed":
			return loadSNOMED(context.Background(), nil, nil, path, version, releaseDate, true)
		case "icd10cm":
			return loadICD10CM(context.Background(), nil, nil, path, version, releaseDate, true)
		case "icd10pcs":
			return loadICD10PCS(context.Background(), nil, nil, path, version, releaseDate, true)
		default:
			return fmt.Errorf("unknown vocabulary: %s", vocab)
		}
	}

	extraArgs := []string{}
	if len(args) > 2 {
		extraArgs = args[2:]
	}
	dbURL := getTerminologyDBURL(extraArgs)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Hour) // Long timeout for bulk loads
	defer cancel()

	// Ensure schema exists
	migrator := db.NewMigrator(conn)
	if _, err := migrator.Initialize(ctx); err != nil {
		return fmt.Errorf("failed to initialize schema: %w", err)
	}

	switch vocab {
	case "loinc":
		return loadLOINC(ctx, conn, migrator, path, version, releaseDate, false)
	case "umls":
		return loadUMLS(ctx, conn, migrator, path, version, releaseDate, false)
	case "rxnorm":
		return loadRxNorm(ctx, conn, migrator, path, version, releaseDate, false)
	case "snomed":
		return loadSNOMED(ctx, conn, migrator, path, version, releaseDate, false)
	case "icd10cm":
		return loadICD10CM(ctx, conn, migrator, path, version, releaseDate, false)
	case "icd10pcs":
		return loadICD10PCS(ctx, conn, migrator, path, version, releaseDate, false)
	default:
		return fmt.Errorf("unknown vocabulary: %s", vocab)
	}
}

// loadLOINC loads LOINC codes from LoincTable.csv into the database.
func loadLOINC(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading LOINC version %s from %s...\n", version, path)

	if dryRun {
		fi, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("invalid LOINC file: %w", err)
		}
		if fi.IsDir() {
			return fmt.Errorf("invalid LOINC file: %s is a directory", path)
		}

		panelPath := strings.TrimSuffix(path, "LoincTable.csv") + "PanelHierarchy.csv"
		if _, err := os.Stat(panelPath); err == nil {
			fmt.Printf("DRY RUN: would load LOINC (%s) and PanelHierarchy.csv (%s)\n", path, panelPath)
		} else {
			fmt.Printf("DRY RUN: would load LOINC (%s)\n", path)
		}
		return nil
	}

	loader := db.NewLOINCLoader(conn)

	// Progress reporter that prints to stdout
	progress := db.DefaultProgressReporter(os.Stdout)

	result, err := loader.LoadLoincTable(ctx, path, version, releaseDate, progress)
	if err != nil {
		return fmt.Errorf("failed to load LOINC table: %w", err)
	}

	fmt.Println() // Newline after progress output
	fmt.Printf("LOINC load complete:\n")
	fmt.Printf("  Release ID: %d\n", result.ReleaseID)
	fmt.Printf("  Codes loaded: %d\n", result.CodesLoaded)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))

	// Check if PanelHierarchy.csv exists in the same directory
	panelPath := strings.TrimSuffix(path, "LoincTable.csv") + "PanelHierarchy.csv"
	if _, err := os.Stat(panelPath); err == nil {
		fmt.Printf("\nFound PanelHierarchy.csv, loading panels...\n")
		panelsLoaded, err := loader.LoadPanelHierarchy(ctx, panelPath, result.ReleaseID, progress, version)
		if err != nil {
			fmt.Printf("Warning: failed to load panel hierarchy: %v\n", err)
		} else {
			fmt.Println() // Newline after progress
			fmt.Printf("  Panels loaded: %d\n", panelsLoaded)
		}
	}

	return nil
}

func loadUMLS(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading UMLS version %s from %s...\n", version, path)

	// Validate directory
	if err := db.ValidateUMLSDirectory(path); err != nil {
		return fmt.Errorf("invalid UMLS META directory: %w", err)
	}

	if dryRun {
		fmt.Printf("DRY RUN: would load UMLS META directory (%s)\n", path)
		return nil
	}

	loader := db.NewUMLSLoader(conn)

	// Configure load options
	opts := &db.UMLSLoadOptions{
		EnglishOnly:    true, // Only English terms
		SkipSuppressed: true, // Skip obsolete/suppressed
		// FilterSources: []string{"SNOMEDCT_US", "ICD10CM", "RXNORM"}, // Uncomment to filter
	}

	// Progress reporter
	progress := db.DefaultProgressReporter(os.Stdout)

	result, err := loader.LoadMETA(ctx, path, version, releaseDate, opts, progress)
	if err != nil {
		return fmt.Errorf("failed to load UMLS: %w", err)
	}

	fmt.Println() // Newline after progress
	fmt.Println("UMLS load complete:")
	fmt.Printf("  Release ID: %d\n", result.ReleaseID)
	fmt.Printf("  Concepts (MRCONSO): %d\n", result.ConceptsLoaded)
	fmt.Printf("  Relations (MRREL): %d\n", result.RelationsLoaded)
	fmt.Printf("  Semantic Types (MRSTY): %d\n", result.SemTypesLoaded)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))

	if len(result.SourcesFiltered) > 0 {
		fmt.Printf("  Filtered to: %v\n", result.SourcesFiltered)
	}

	return nil
}

func loadRxNorm(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading RxNorm version %s from %s...\n", version, path)

	if err := db.ValidateRxNormDirectory(path); err != nil {
		return fmt.Errorf("invalid RxNorm directory: %w", err)
	}

	if dryRun {
		fmt.Printf("DRY RUN: would load RxNorm RRF directory (%s)\n", path)
		return nil
	}

	loader := db.NewRxNormLoader(conn)

	// Configure load options
	opts := &db.RxNormLoadOptions{
		SkipSuppressed: true,
		LoadNDC:        true, // Extract NDC cross-references
	}

	// Progress reporter
	progress := db.DefaultProgressReporter(os.Stdout)

	result, err := loader.LoadRRF(ctx, path, version, releaseDate, opts, progress)
	if err != nil {
		return fmt.Errorf("failed to load RxNorm: %w", err)
	}

	fmt.Println() // Newline after progress
	fmt.Println("RxNorm load complete:")
	fmt.Printf("  Release ID: %d\n", result.ReleaseID)
	fmt.Printf("  Concepts (RXNCONSO): %d\n", result.ConceptsLoaded)
	fmt.Printf("  Relations (RXNREL): %d\n", result.RelationsLoaded)
	fmt.Printf("  NDC Cross-refs: %d\n", result.NDCLoaded)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))

	return nil
}

func loadSNOMED(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading SNOMED CT version %s from %s...\n", version, path)
	if dryRun {
		fmt.Printf("DRY RUN: would load SNOMED CT (not yet implemented)\n")
		return nil
	}
	fmt.Println("SNOMED loader not yet implemented. Coming in Phase 4.")
	return nil
}

func loadICD10CM(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading ICD-10-CM version %s from %s...\n", version, path)

	if dryRun {
		fi, err := os.Stat(path)
		if err != nil {
			return fmt.Errorf("invalid ICD-10-CM input: %w", err)
		}
		if fi.IsDir() {
			return fmt.Errorf("invalid ICD-10-CM input: %s is a directory", path)
		}
		fmt.Printf("DRY RUN: would load ICD-10-CM (%s)\n", path)
		return nil
	}

	loader := db.NewICD10Loader(conn)

	// Load options - include category headers
	opts := &db.ICD10LoadOptions{
		IncludeHeaders: true,
	}

	// Progress reporter that prints to stdout
	progress := db.DefaultProgressReporter(os.Stdout)

	result, err := loader.LoadICD10CMCSV(ctx, path, version, releaseDate, progress, opts)
	if err != nil {
		return fmt.Errorf("failed to load ICD-10-CM: %w", err)
	}

	fmt.Println() // Newline after progress output
	fmt.Printf("ICD-10-CM load complete:\n")
	fmt.Printf("  Release ID: %d\n", result.ReleaseID)
	fmt.Printf("  Billable codes: %d\n", result.CodesLoaded)
	fmt.Printf("  Category headers: %d\n", result.HeadersLoaded)
	fmt.Printf("  Total: %d\n", result.CodesLoaded+result.HeadersLoaded)
	fmt.Printf("  Duration: %s\n", result.Duration.Round(time.Millisecond))

	return nil
}

func loadICD10PCS(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time, dryRun bool) error {
	fmt.Printf("Loading ICD-10-PCS version %s from %s...\n", version, path)
	if dryRun {
		fmt.Printf("DRY RUN: would load ICD-10-PCS (not yet implemented)\n")
		return nil
	}
	fmt.Println("ICD-10-PCS loader not yet implemented. Coming in Phase 3.")
	return nil
}

func runTerminologyCrosswalk(args []string) error {
	if len(args) < 1 {
		fmt.Println(`Usage: fi-fhir terminology crosswalk <code> [options]

Options:
  --from     Source vocabulary (ICD10CM, SNOMEDCT_US, RXNORM, LOINC)
  --to       Target vocabulary
  --db       Database connection URL

Examples:
  fi-fhir terminology crosswalk E11.9 --from ICD10CM --to SNOMEDCT_US
  fi-fhir terminology crosswalk 73211009 --from SNOMEDCT_US --to ICD10CM
  fi-fhir terminology crosswalk 316076 --from RXNORM --to NDC`)
		return nil
	}

	code := args[0]

	// Parse flags
	var fromVocab, toVocab string
	dbURL := getTerminologyDBURL(args[1:])

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--from":
			if i+1 < len(args) {
				fromVocab = args[i+1]
				i++
			}
		case "--to":
			if i+1 < len(args) {
				toVocab = args[i+1]
				i++
			}
		}
	}

	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	if fromVocab == "" || toVocab == "" {
		return fmt.Errorf("--from and --to vocabularies are required")
	}

	fmt.Printf("Cross-walk: %s (%s) -> %s\n", code, fromVocab, toVocab)
	fmt.Println("Cross-walk queries not yet implemented. Coming in Phase 3.")
	return nil
}

// ============================================================================
// Mapping Subcommands
// ============================================================================

func runTerminologyMapping(args []string) error {
	if len(args) == 0 {
		printTerminologyMappingUsage()
		return nil
	}

	switch args[0] {
	case "upload":
		return runTerminologyMappingUpload(args[1:])
	case "list":
		return runTerminologyMappingList(args[1:])
	case "delete":
		return runTerminologyMappingDelete(args[1:])
	case "get":
		return runTerminologyMappingGet(args[1:])
	case "resolve":
		return runTerminologyMappingResolve(args[1:])
	case "pending":
		return runTerminologyMappingPending(args[1:])
	case "approve":
		return runTerminologyMappingApprove(args[1:])
	case "reject":
		return runTerminologyMappingReject(args[1:])
	case "-h", "--help", "help":
		printTerminologyMappingUsage()
		return nil
	default:
		return fmt.Errorf("unknown mapping subcommand: %s", args[0])
	}
}

func printTerminologyMappingUsage() {
	fmt.Println(`fi-fhir terminology mapping - Custom Code Mapping Management

Usage:
  fi-fhir terminology mapping <subcommand> [options]

Subcommands:
  upload    Upload mappings from a CSV file
  list      List existing mappings with optional filters
  get       Get details of a specific mapping or batch
  delete    Delete mappings by ID or batch
  resolve   Find mapping using persistent lookup + LLM autorouting
  pending   List pending autoroute suggestions awaiting review
  approve   Approve a pending autoroute suggestion
  reject    Reject a pending autoroute suggestion

Options:
  --db      PostgreSQL connection string (or FI_FHIR_TERMINOLOGY_DB_URL env)

Upload Command:
  fi-fhir terminology mapping upload <file.csv> [options]
    --source-system   Default source system if not in CSV
    --target-system   Default target system if not in CSV
    --profile         Profile ID to associate mappings with
    --dry-run         Validate without persisting (preview mode)

  CSV Format (standard):
    source_system,source_code,source_display,target_system,target_code,target_display,equivalence,comment
    epic_labs,GLU,Glucose,http://loinc.org,2345-7,Glucose [Mass/volume] in Serum,equivalent,Standard mapping

  CSV Format (simple - requires --source-system and --target-system):
    source_code,target_code
    GLU,2345-7
    HGB,718-7

List Command:
  fi-fhir terminology mapping list [options]
    --source-system   Filter by source system
    --target-system   Filter by target system
    --profile         Filter by profile ID
    --batch           Filter by upload batch ID
    --limit           Maximum rows to return (default: 100)
    --offset          Offset for pagination

Get Command:
  fi-fhir terminology mapping get <id>
  fi-fhir terminology mapping get --batch <batch-id>

Delete Command:
  fi-fhir terminology mapping delete <id>
  fi-fhir terminology mapping delete --batch <batch-id>
    --force           Skip confirmation prompt

Resolve Command:
  fi-fhir terminology mapping resolve <code> [options]
    --source-system   Source system URI (required)
    --target-system   Target system URI (required)
    --display         Human-readable description (improves accuracy)
    --profile         Profile ID for scoped lookups
    --no-autoroute    Disable LLM autorouting (persistent lookup only)
    --json            Output as JSON

Examples:
  # Upload a CSV file with mappings
  fi-fhir terminology mapping upload mappings.csv --db "$DATABASE_URL"

  # Upload with defaults for simple format
  fi-fhir terminology mapping upload simple.csv \
    --source-system epic_labs \
    --target-system http://loinc.org

  # Preview upload without persisting
  fi-fhir terminology mapping upload mappings.csv --dry-run

  # List all mappings for a profile
  fi-fhir terminology mapping list --profile my-profile

  # Delete all mappings from a batch
  fi-fhir terminology mapping delete --batch abc123 --force

  # Resolve a mapping (checks persistent first, then uses LLM)
  fi-fhir terminology mapping resolve GLU001 \
    --source-system epic_labs \
    --target-system http://loinc.org \
    --display "Glucose Fasting"

  # Persistent lookup only (no LLM)
  fi-fhir terminology mapping resolve GLU001 \
    --source-system epic_labs \
    --target-system http://loinc.org \
    --no-autoroute`)
}

func runTerminologyMappingUpload(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("CSV file path required")
	}

	filePath := args[0]
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	var sourceSystem, targetSystem, profileID string
	var dryRun bool

	for i := 1; i < len(args); {
		switch args[i] {
		case "--source-system":
			if i+1 < len(args) {
				sourceSystem = args[i+1]
				i += 2
				continue
			}
		case "--target-system":
			if i+1 < len(args) {
				targetSystem = args[i+1]
				i += 2
				continue
			}
		case "--profile":
			if i+1 < len(args) {
				profileID = args[i+1]
				i += 2
				continue
			}
		case "--dry-run":
			dryRun = true
		}
		i++
	}

	// Open file
	file, err := os.Open(filePath)
	if err != nil {
		return fmt.Errorf("failed to open file: %w", err)
	}
	defer func() { _ = file.Close() }()

	// Parse CSV
	parser := upload.NewParser(upload.ParseOptions{
		DefaultSourceSystem: sourceSystem,
		DefaultTargetSystem: targetSystem,
		MaxRows:             50000,
	})

	parseResult, err := parser.Parse(file)
	if err != nil {
		return fmt.Errorf("failed to parse CSV: %w", err)
	}

	// Print parse summary
	fmt.Printf("CSV Parse Results:\n")
	fmt.Printf("  Format:     %s\n", parseResult.DetectedFormat)
	fmt.Printf("  Total Rows: %d\n", parseResult.TotalRows)
	fmt.Printf("  Valid Rows: %d\n", parseResult.ValidRows)
	fmt.Printf("  Error Rows: %d\n", parseResult.ErrorRows)

	if len(parseResult.Errors) > 0 {
		fmt.Printf("\nValidation Errors (first 10):\n")
		limit := 10
		if len(parseResult.Errors) < limit {
			limit = len(parseResult.Errors)
		}
		for i := 0; i < limit; i++ {
			e := parseResult.Errors[i]
			if e.Column != "" {
				fmt.Printf("  Row %d, %s: %s\n", e.Row, e.Column, e.Message)
			} else {
				fmt.Printf("  Row %d: %s\n", e.Row, e.Message)
			}
		}
		if len(parseResult.Errors) > 10 {
			fmt.Printf("  ... and %d more errors\n", len(parseResult.Errors)-10)
		}
	}

	if dryRun {
		fmt.Println("\n[DRY RUN] No changes made to database.")
		if len(parseResult.Rows) > 0 {
			fmt.Printf("\nPreview (first 5 mappings):\n")
			limit := 5
			if len(parseResult.Rows) < limit {
				limit = len(parseResult.Rows)
			}
			for i := 0; i < limit; i++ {
				row := parseResult.Rows[i]
				fmt.Printf("  %s:%s -> %s:%s (%s)\n",
					row.SourceSystem, row.SourceCode,
					row.TargetSystem, row.TargetCode,
					row.Equivalence)
			}
		}
		return nil
	}

	if parseResult.ValidRows == 0 {
		return fmt.Errorf("no valid rows to upload")
	}

	// Connect to database
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()

	store := db.NewMappingStore(conn)

	// Create batch record
	batch := &db.UploadBatch{
		Filename:     filePath,
		SourceSystem: sourceSystem,
		TargetSystem: targetSystem,
		ProfileID:    profileID,
		TotalRows:    parseResult.TotalRows,
		ValidRows:    parseResult.ValidRows,
		ErrorRows:    parseResult.ErrorRows,
	}
	for _, e := range parseResult.Errors {
		batch.ValidationErrors = append(batch.ValidationErrors, db.ValidationError{
			Row:     e.Row,
			Column:  e.Column,
			Message: e.Message,
		})
	}

	if err := store.CreateBatch(ctx, batch); err != nil {
		return fmt.Errorf("failed to create batch: %w", err)
	}

	fmt.Printf("\nUpload Batch ID: %s\n", batch.ID)

	// Create mappings
	var mappings []*db.CustomMapping
	for _, row := range parseResult.Rows {
		m := &db.CustomMapping{
			SourceSystem:  row.SourceSystem,
			SourceCode:    row.SourceCode,
			SourceDisplay: row.SourceDisplay,
			TargetSystem:  row.TargetSystem,
			TargetCode:    row.TargetCode,
			TargetDisplay: row.TargetDisplay,
			Equivalence:   row.Equivalence,
			Confidence:    row.Confidence,
			Comment:       row.Comment,
			Origin:        db.OriginCSVUpload,
			UploadBatchID: &batch.ID,
			ProfileID:     profileID,
		}
		mappings = append(mappings, m)
	}

	created, duplicates, err := store.CreateMappingsBatch(ctx, mappings)
	if err != nil {
		return fmt.Errorf("failed to create mappings: %w", err)
	}

	fmt.Printf("Mappings Created:   %d\n", created)
	fmt.Printf("Duplicates Skipped: %d\n", duplicates)

	return nil
}

func runTerminologyMappingList(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	filter := db.ListMappingsFilter{
		Limit:  100,
		Offset: 0,
	}

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source-system":
			if i+1 < len(args) {
				filter.SourceSystem = args[i+1]
				i++
			}
		case "--target-system":
			if i+1 < len(args) {
				filter.TargetSystem = args[i+1]
				i++
			}
		case "--profile":
			if i+1 < len(args) {
				filter.ProfileID = args[i+1]
				i++
			}
		case "--batch":
			if i+1 < len(args) {
				batchID, err := parseUUID(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid batch ID: %w", err)
				}
				filter.UploadBatchID = &batchID
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				limit, err := parseInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid limit: %w", err)
				}
				filter.Limit = limit
				i++
			}
		case "--offset":
			if i+1 < len(args) {
				offset, err := parseInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid offset: %w", err)
				}
				filter.Offset = offset
				i++
			}
		}
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)
	mappings, total, err := store.ListMappings(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list mappings: %w", err)
	}

	fmt.Printf("Custom Mappings (%d total, showing %d-%d)\n", total, filter.Offset+1, filter.Offset+len(mappings))
	fmt.Println(strings.Repeat("-", 100))
	fmt.Printf("%-8s %-15s %-12s %-30s %-12s %-12s\n", "ID", "SOURCE_SYS", "SOURCE_CODE", "TARGET_SYS", "TARGET_CODE", "EQUIV")
	fmt.Println(strings.Repeat("-", 100))

	for _, m := range mappings {
		targetSys := m.TargetSystem
		if len(targetSys) > 28 {
			targetSys = targetSys[:25] + "..."
		}
		fmt.Printf("%-8d %-15s %-12s %-30s %-12s %-12s\n",
			m.ID,
			truncate(m.SourceSystem, 15),
			truncate(m.SourceCode, 12),
			targetSys,
			truncate(m.TargetCode, 12),
			m.Equivalence)
	}

	if total > filter.Offset+len(mappings) {
		fmt.Printf("\nShowing %d of %d. Use --offset %d for next page.\n",
			len(mappings), total, filter.Offset+filter.Limit)
	}

	return nil
}

func runTerminologyMappingGet(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	var batchID string
	var mappingID int64

	for i := 0; i < len(args); {
		if args[i] == "--batch" {
			if i+1 < len(args) {
				batchID = args[i+1]
				i += 2
				continue
			}
		} else if !strings.HasPrefix(args[i], "--") && !strings.HasPrefix(args[i], "-") {
			id, err := parseInt(args[i])
			if err == nil {
				mappingID = int64(id)
			}
		}
		i++
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)

	if batchID != "" {
		batchUUID, err := parseUUID(batchID)
		if err != nil {
			return fmt.Errorf("invalid batch ID: %w", err)
		}
		batch, err := store.GetBatch(ctx, batchUUID)
		if err != nil {
			return fmt.Errorf("failed to get batch: %w", err)
		}
		if batch == nil {
			return fmt.Errorf("batch not found: %s", batchID)
		}

		fmt.Printf("Upload Batch: %s\n", batch.ID)
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Filename:      %s\n", batch.Filename)
		fmt.Printf("Source System: %s\n", batch.SourceSystem)
		fmt.Printf("Target System: %s\n", batch.TargetSystem)
		fmt.Printf("Profile ID:    %s\n", batch.ProfileID)
		fmt.Printf("Total Rows:    %d\n", batch.TotalRows)
		fmt.Printf("Valid Rows:    %d\n", batch.ValidRows)
		fmt.Printf("Duplicates:    %d\n", batch.DuplicateRows)
		fmt.Printf("Error Rows:    %d\n", batch.ErrorRows)
		fmt.Printf("Uploaded At:   %s\n", batch.UploadedAt.Format(time.RFC3339))
		fmt.Printf("Uploaded By:   %s\n", batch.UploadedBy)

		if len(batch.ValidationErrors) > 0 {
			fmt.Printf("\nValidation Errors (%d):\n", len(batch.ValidationErrors))
			limit := 10
			if len(batch.ValidationErrors) < limit {
				limit = len(batch.ValidationErrors)
			}
			for i := 0; i < limit; i++ {
				e := batch.ValidationErrors[i]
				if e.Column != "" {
					fmt.Printf("  Row %d, %s: %s\n", e.Row, e.Column, e.Message)
				} else {
					fmt.Printf("  Row %d: %s\n", e.Row, e.Message)
				}
			}
		}
		return nil
	}

	if mappingID > 0 {
		mapping, err := store.GetMapping(ctx, mappingID)
		if err != nil {
			return fmt.Errorf("failed to get mapping: %w", err)
		}
		if mapping == nil {
			return fmt.Errorf("mapping not found: %d", mappingID)
		}

		fmt.Printf("Mapping ID: %d\n", mapping.ID)
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Source System:  %s\n", mapping.SourceSystem)
		fmt.Printf("Source Code:    %s\n", mapping.SourceCode)
		fmt.Printf("Source Display: %s\n", mapping.SourceDisplay)
		fmt.Printf("Target System:  %s\n", mapping.TargetSystem)
		fmt.Printf("Target Code:    %s\n", mapping.TargetCode)
		fmt.Printf("Target Display: %s\n", mapping.TargetDisplay)
		fmt.Printf("Equivalence:    %s\n", mapping.Equivalence)
		if mapping.Confidence != nil {
			fmt.Printf("Confidence:     %.2f\n", *mapping.Confidence)
		}
		fmt.Printf("Comment:        %s\n", mapping.Comment)
		fmt.Printf("Origin:         %s\n", mapping.Origin)
		if mapping.UploadBatchID != nil {
			fmt.Printf("Batch ID:       %s\n", mapping.UploadBatchID)
		}
		fmt.Printf("Profile ID:     %s\n", mapping.ProfileID)
		fmt.Printf("Created At:     %s\n", mapping.CreatedAt.Format(time.RFC3339))
		fmt.Printf("Created By:     %s\n", mapping.CreatedBy)
		return nil
	}

	return fmt.Errorf("mapping ID or --batch required")
}

func runTerminologyMappingDelete(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	var batchID string
	var mappingID int64
	var force bool

	for i := 0; i < len(args); {
		switch args[i] {
		case "--batch":
			if i+1 < len(args) {
				batchID = args[i+1]
				i += 2
				continue
			}
		case "--force", "-f":
			force = true
		default:
			if !strings.HasPrefix(args[i], "--") && !strings.HasPrefix(args[i], "-") {
				id, err := parseInt(args[i])
				if err == nil {
					mappingID = int64(id)
				}
			}
		}
		i++
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)

	if batchID != "" {
		batchUUID, err := parseUUID(batchID)
		if err != nil {
			return fmt.Errorf("invalid batch ID: %w", err)
		}

		if !force {
			fmt.Printf("Delete all mappings from batch %s? [y/N]: ", batchID)
			var response string
			if _, err := fmt.Scanln(&response); err != nil || strings.ToLower(response) != "y" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		count, err := store.DeleteMappingsByBatch(ctx, batchUUID)
		if err != nil {
			return fmt.Errorf("failed to delete batch mappings: %w", err)
		}
		fmt.Printf("Deleted %d mappings from batch %s\n", count, batchID)
		return nil
	}

	if mappingID > 0 {
		if !force {
			fmt.Printf("Delete mapping %d? [y/N]: ", mappingID)
			var response string
			if _, err := fmt.Scanln(&response); err != nil || strings.ToLower(response) != "y" {
				fmt.Println("Aborted.")
				return nil
			}
		}

		err := store.DeleteMapping(ctx, mappingID)
		if err != nil {
			return fmt.Errorf("failed to delete mapping: %w", err)
		}
		fmt.Printf("Deleted mapping %d\n", mappingID)
		return nil
	}

	return fmt.Errorf("mapping ID or --batch required")
}

// Helper functions for parsing
func parseInt(s string) (int, error) {
	var n int
	_, err := fmt.Sscanf(s, "%d", &n)
	return n, err
}

func parseUUID(s string) (uuid.UUID, error) {
	return uuid.Parse(s)
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func runTerminologyMappingResolve(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("source code required")
	}

	code := args[0]
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	var sourceSystem, targetSystem, display, profileID string
	var noAutoroute, jsonOutput bool

	for i := 1; i < len(args); {
		switch args[i] {
		case "--source-system":
			if i+1 < len(args) {
				sourceSystem = args[i+1]
				i += 2
				continue
			}
		case "--target-system":
			if i+1 < len(args) {
				targetSystem = args[i+1]
				i += 2
				continue
			}
		case "--display":
			if i+1 < len(args) {
				display = args[i+1]
				i += 2
				continue
			}
		case "--profile":
			if i+1 < len(args) {
				profileID = args[i+1]
				i += 2
				continue
			}
		case "--no-autoroute":
			noAutoroute = true
		case "--json":
			jsonOutput = true
		}
		i++
	}

	if sourceSystem == "" {
		return fmt.Errorf("--source-system is required")
	}
	if targetSystem == "" {
		return fmt.Errorf("--target-system is required")
	}

	// Connect to database
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)

	// Step 1: Check persistent mappings first
	existing, err := store.LookupMapping(ctx, sourceSystem, code, targetSystem, profileID)
	if err != nil {
		return fmt.Errorf("persistent lookup failed: %w", err)
	}

	if existing != nil {
		// Found in persistent storage
		if jsonOutput {
			return printResolveResultJSON(existing, nil, "PERSISTENT_HIT")
		}
		fmt.Println("✓ Found in persistent mappings")
		fmt.Println(strings.Repeat("-", 50))
		fmt.Printf("Source:      %s:%s\n", existing.SourceSystem, existing.SourceCode)
		fmt.Printf("Target:      %s:%s\n", existing.TargetSystem, existing.TargetCode)
		fmt.Printf("Display:     %s\n", existing.TargetDisplay)
		fmt.Printf("Equivalence: %s\n", existing.Equivalence)
		fmt.Printf("Origin:      %s\n", existing.Origin)
		if existing.Confidence != nil {
			fmt.Printf("Confidence:  %.2f\n", *existing.Confidence)
		}
		return nil
	}

	if noAutoroute {
		if jsonOutput {
			return printResolveResultJSON(nil, nil, "NO_MATCH")
		}
		fmt.Println("✗ No mapping found (autoroute disabled)")
		return nil
	}

	// Step 2: Use autoroute engine
	fmt.Println("No persistent mapping found, trying autoroute...")

	// Initialize semantic searcher
	searchCfg := semantic.DefaultSearchConfig().WithEnv()
	if err := searchCfg.Validate(); err != nil {
		return fmt.Errorf("semantic search not configured: %w (set QDRANT_URL and LLM_EMBEDDING_BASE_URL)", err)
	}

	searcher, err := semantic.NewSearcher(searchCfg)
	if err != nil {
		return fmt.Errorf("failed to create searcher: %w", err)
	}

	// Initialize LLM client
	llmCfg := llm.DefaultConfig().WithEnv()
	llmClient, err := llm.New(llmCfg)
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}

	// Create autoroute engine
	engine := autoroute.NewEngine(searcher, llmClient, autoroute.DefaultConfig())

	// Request suggestion
	result, err := engine.Suggest(ctx, autoroute.SuggestRequest{
		SourceCode:    code,
		SourceSystem:  sourceSystem,
		SourceDisplay: display,
		TargetSystem:  targetSystem,
		ProfileID:     profileID,
		MaxCandidates: 5,
	})
	if err != nil {
		return fmt.Errorf("autoroute failed: %w", err)
	}

	// Classify decision
	decision := result.Classify(0.90, 0.70)

	if jsonOutput {
		return printResolveResultJSON(nil, result, string(decision))
	}

	// Print results
	if result.BestMatch == nil {
		fmt.Println("✗ No mapping suggestions found")
		return nil
	}

	fmt.Printf("✓ Autoroute suggestion (%s)\n", decision)
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Source:      %s:%s\n", sourceSystem, code)
	fmt.Printf("Target:      %s:%s\n", result.BestMatch.System, result.BestMatch.Code)
	fmt.Printf("Display:     %s\n", result.BestMatch.Display)
	fmt.Printf("Confidence:  %.2f\n", result.Confidence)
	fmt.Printf("Equivalence: %s\n", result.BestMatch.Equivalence)
	fmt.Printf("Reasoning:   %s\n", result.BestMatch.Reasoning)
	fmt.Printf("Duration:    %s\n", result.TotalDuration.Round(time.Millisecond))

	if len(result.Alternates) > 0 {
		fmt.Printf("\nAlternate candidates:\n")
		for i, alt := range result.Alternates {
			fmt.Printf("  %d. %s (%s) - confidence: %.2f\n",
				i+1, alt.Code, alt.Display, alt.Confidence)
		}
	}

	return nil
}

// ============================================================================
// Pending / Approve / Reject Subcommands
// ============================================================================

func runTerminologyMappingPending(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	filter := db.ListPendingAutoroutesFilter{
		Status: db.StatusPending,
		Limit:  100,
		Offset: 0,
	}
	var jsonOutput bool

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--status":
			if i+1 < len(args) {
				filter.Status = db.PendingStatus(args[i+1])
				i++
			}
		case "--min-confidence":
			if i+1 < len(args) {
				var val float64
				if _, err := fmt.Sscanf(args[i+1], "%f", &val); err == nil {
					filter.MinConfidence = &val
				}
				i++
			}
		case "--source-system":
			if i+1 < len(args) {
				filter.SourceSystem = args[i+1]
				i++
			}
		case "--target-system":
			if i+1 < len(args) {
				filter.TargetSystem = args[i+1]
				i++
			}
		case "--limit":
			if i+1 < len(args) {
				limit, err := parseInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid limit: %w", err)
				}
				filter.Limit = limit
				i++
			}
		case "--offset":
			if i+1 < len(args) {
				offset, err := parseInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid offset: %w", err)
				}
				filter.Offset = offset
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)
	pending, total, err := store.ListPendingAutoroutes(ctx, filter)
	if err != nil {
		return fmt.Errorf("failed to list pending autoroutes: %w", err)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(map[string]interface{}{
			"total":   total,
			"offset":  filter.Offset,
			"limit":   filter.Limit,
			"pending": pending,
		}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("Pending Autoroutes (%d total, showing %d-%d)\n", total, filter.Offset+1, filter.Offset+len(pending))
	fmt.Println(strings.Repeat("-", 120))
	fmt.Printf("%-6s %-15s %-12s %-15s %-12s %-10s %-10s %s\n",
		"ID", "SOURCE_SYS", "SOURCE_CODE", "TARGET_SYS", "SUGGESTED", "CONF", "STATUS", "CREATED")
	fmt.Println(strings.Repeat("-", 120))

	for _, p := range pending {
		fmt.Printf("%-6d %-15s %-12s %-15s %-12s %-10.2f %-10s %s\n",
			p.ID,
			truncate(p.SourceSystem, 15),
			truncate(p.SourceCode, 12),
			truncate(p.TargetSystem, 15),
			truncate(p.SuggestedCode, 12),
			p.Confidence,
			string(p.Status),
			p.CreatedAt.Format("2006-01-02"))
	}

	if total > filter.Offset+len(pending) {
		fmt.Printf("\nShowing %d of %d. Use --offset %d for next page.\n",
			len(pending), total, filter.Offset+filter.Limit)
	}

	return nil
}

func runTerminologyMappingApprove(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("pending autoroute ID required")
	}

	pendingID, err := parseInt(args[0])
	if err != nil {
		return fmt.Errorf("invalid pending ID: %w", err)
	}

	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	approvedBy := fmt.Sprintf("cli:%s", os.Getenv("USER"))
	var equivalence, comment string
	var jsonOutput bool

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--by":
			if i+1 < len(args) {
				approvedBy = args[i+1]
				i++
			}
		case "--equivalence":
			if i+1 < len(args) {
				equivalence = args[i+1]
				i++
			}
		case "--comment":
			if i+1 < len(args) {
				comment = args[i+1]
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)
	mapping, err := store.ApprovePendingAutoroute(ctx, int64(pendingID), approvedBy, equivalence, comment)
	if err != nil {
		return fmt.Errorf("failed to approve pending autoroute: %w", err)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(mapping, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✓ Approved pending autoroute %d\n", pendingID)
	fmt.Println(strings.Repeat("-", 50))
	fmt.Printf("Created Mapping ID: %d\n", mapping.ID)
	fmt.Printf("Source:  %s:%s\n", mapping.SourceSystem, mapping.SourceCode)
	fmt.Printf("Target:  %s:%s\n", mapping.TargetSystem, mapping.TargetCode)
	fmt.Printf("Approved By: %s\n", approvedBy)

	return nil
}

func runTerminologyMappingReject(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("pending autoroute ID required")
	}

	pendingID, err := parseInt(args[0])
	if err != nil {
		return fmt.Errorf("invalid pending ID: %w", err)
	}

	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	rejectedBy := fmt.Sprintf("cli:%s", os.Getenv("USER"))
	var reason string
	var jsonOutput bool

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--by":
			if i+1 < len(args) {
				rejectedBy = args[i+1]
				i++
			}
		case "--reason":
			if i+1 < len(args) {
				reason = args[i+1]
				i++
			}
		case "--json":
			jsonOutput = true
		}
	}

	if reason == "" {
		return fmt.Errorf("--reason is required for rejection")
	}

	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	store := db.NewMappingStore(conn)
	if err := store.RejectPendingAutoroute(ctx, int64(pendingID), rejectedBy, reason); err != nil {
		return fmt.Errorf("failed to reject pending autoroute: %w", err)
	}

	if jsonOutput {
		data, err := json.MarshalIndent(map[string]interface{}{
			"status":      "rejected",
			"pending_id":  pendingID,
			"rejected_by": rejectedBy,
			"reason":      reason,
		}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
		return nil
	}

	fmt.Printf("✗ Rejected pending autoroute %d\n", pendingID)
	fmt.Printf("Reason: %s\n", reason)

	return nil
}

// ============================================================================
// Autoroute Subcommand
// ============================================================================

func runTerminologyAutoroute(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("source code required")
	}

	code := args[0]
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_TERMINOLOGY_DB_URL env var")
	}

	// Parse flags
	var sourceSystem, targetSystem, display string
	var temporalAddr, temporalNamespace string
	autoApproveThreshold := 0.95
	reviewTimeoutDays := 7
	var waitForResult, jsonOutput bool

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--source-system":
			if i+1 < len(args) {
				sourceSystem = args[i+1]
				i++
			}
		case "--target-system":
			if i+1 < len(args) {
				targetSystem = args[i+1]
				i++
			}
		case "--display":
			if i+1 < len(args) {
				display = args[i+1]
				i++
			}
		case "--temporal":
			if i+1 < len(args) {
				temporalAddr = args[i+1]
				i++
			}
		case "--temporal-namespace":
			if i+1 < len(args) {
				temporalNamespace = args[i+1]
				i++
			}
		case "--auto-approve-threshold":
			if i+1 < len(args) {
				if _, err := fmt.Sscanf(args[i+1], "%f", &autoApproveThreshold); err != nil {
					return fmt.Errorf("invalid threshold: %s", args[i+1])
				}
				i++
			}
		case "--review-timeout-days":
			if i+1 < len(args) {
				n, err := parseInt(args[i+1])
				if err != nil {
					return fmt.Errorf("invalid timeout days: %w", err)
				}
				reviewTimeoutDays = n
				i++
			}
		case "--wait":
			waitForResult = true
		case "--json":
			jsonOutput = true
		}
	}

	if sourceSystem == "" {
		return fmt.Errorf("--source-system is required")
	}
	if targetSystem == "" {
		return fmt.Errorf("--target-system is required")
	}

	// Connect to database
	conn, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = conn.Close() }()

	mappingStore := db.NewMappingStore(conn)

	if temporalAddr != "" {
		// Use Temporal workflow path
		return runAutorouteViaWorkflow(code, sourceSystem, targetSystem, display,
			temporalAddr, temporalNamespace, autoApproveThreshold, reviewTimeoutDays,
			waitForResult, jsonOutput, mappingStore)
	}

	// Direct autoroute engine call (no Temporal)
	return runAutoroute(code, sourceSystem, targetSystem, display, jsonOutput)
}

func runAutorouteViaWorkflow(code, sourceSystem, targetSystem, display string,
	temporalAddr, temporalNamespace string,
	autoApproveThreshold float64, reviewTimeoutDays int,
	waitForResult, jsonOutput bool,
	mappingStore *db.MappingStore) error {
	if temporalNamespace == "" {
		temporalNamespace = "terminology-mapping"
	}

	// Create a Temporal worker (we only need the client, but Worker provides StartReviewWorkflow)
	workerCfg := termworkflow.WorkerConfig{
		HostPort:  temporalAddr,
		Namespace: temporalNamespace,
	}

	// Initialize autoroute engine for the worker
	searcher, err := semantic.NewSearcher(semantic.DefaultSearchConfig().WithEnv())
	if err != nil {
		return fmt.Errorf("failed to create semantic searcher: %w", err)
	}
	llmClient, err := llm.New(llm.DefaultConfig().WithEnv())
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}
	engine := autoroute.NewEngine(searcher, llmClient, autoroute.DefaultConfig())

	worker, err := termworkflow.NewWorker(context.Background(), workerCfg, engine, mappingStore)
	if err != nil {
		return fmt.Errorf("failed to create Temporal worker: %w", err)
	}
	defer worker.Stop()

	input := termworkflow.TerminologyReviewInput{
		SourceCode:           code,
		SourceSystem:         sourceSystem,
		SourceDisplay:        display,
		TargetSystem:         targetSystem,
		AutoApproveThreshold: autoApproveThreshold,
		ReviewTimeout:        time.Duration(reviewTimeoutDays) * 24 * time.Hour,
	}

	run, err := worker.StartReviewWorkflow(context.Background(), input)
	if err != nil {
		return fmt.Errorf("failed to start workflow: %w", err)
	}

	if !jsonOutput {
		fmt.Printf("Started terminology review workflow: %s\n", run.GetID())
	}

	if waitForResult {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(reviewTimeoutDays+1)*24*time.Hour)
		defer cancel()

		result, err := worker.GetWorkflowResult(ctx, run)
		if err != nil {
			return fmt.Errorf("workflow failed: %w", err)
		}

		if jsonOutput {
			data, err := json.MarshalIndent(result, "", "  ")
			if err != nil {
				return err
			}
			fmt.Println(string(data))
		} else {
			fmt.Printf("Workflow completed: %s\n", result.Status)
			if result.FinalCode != "" {
				fmt.Printf("Result: %s (%s)\n", result.FinalCode, result.FinalDisplay)
			}
			if result.MappingID > 0 {
				fmt.Printf("Mapping ID: %d\n", result.MappingID)
			}
		}
	} else if jsonOutput {
		data, err := json.MarshalIndent(map[string]string{
			"workflow_id": run.GetID(),
			"run_id":      run.GetRunID(),
			"status":      "started",
		}, "", "  ")
		if err != nil {
			return err
		}
		fmt.Println(string(data))
	}

	return nil
}

func runAutoroute(code, sourceSystem, targetSystem, display string, jsonOutput bool) error {
	// Initialize autoroute engine directly
	searcher, err := semantic.NewSearcher(semantic.DefaultSearchConfig().WithEnv())
	if err != nil {
		return fmt.Errorf("failed to create semantic searcher: %w", err)
	}
	llmClient, err := llm.New(llm.DefaultConfig().WithEnv())
	if err != nil {
		return fmt.Errorf("failed to create LLM client: %w", err)
	}
	engine := autoroute.NewEngine(searcher, llmClient, autoroute.DefaultConfig())

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	result, err := engine.Suggest(ctx, autoroute.SuggestRequest{
		SourceCode:    code,
		SourceSystem:  sourceSystem,
		SourceDisplay: display,
		TargetSystem:  targetSystem,
		MaxCandidates: 5,
	})
	if err != nil {
		return fmt.Errorf("autoroute failed: %w", err)
	}

	decision := result.Classify(0.90, 0.70)

	if jsonOutput {
		return printResolveResultJSON(nil, result, string(decision))
	}

	fmt.Printf("Autoroute result (%s)\n", decision)
	fmt.Println(strings.Repeat("-", 50))
	if result.BestMatch != nil {
		fmt.Printf("Best Match: %s:%s (%s)\n", result.BestMatch.System, result.BestMatch.Code, result.BestMatch.Display)
	}
	fmt.Printf("Confidence: %.2f\n", result.Confidence)
	fmt.Printf("Reasoning:  %s\n", result.Reasoning)

	if len(result.Alternates) > 0 {
		fmt.Printf("\nAlternates:\n")
		for i, alt := range result.Alternates {
			fmt.Printf("  %d. %s (%s) - %.2f\n", i+1, alt.Code, alt.Display, alt.Confidence)
		}
	}

	return nil
}

func printResolveResultJSON(persistent *db.CustomMapping, autorouted *autoroute.SuggestResult, decision string) error {
	output := map[string]interface{}{
		"decision": decision,
	}

	if persistent != nil {
		output["mapping"] = map[string]interface{}{
			"sourceSystem":  persistent.SourceSystem,
			"sourceCode":    persistent.SourceCode,
			"targetSystem":  persistent.TargetSystem,
			"targetCode":    persistent.TargetCode,
			"targetDisplay": persistent.TargetDisplay,
			"equivalence":   persistent.Equivalence,
			"origin":        persistent.Origin,
		}
		if persistent.Confidence != nil {
			output["confidence"] = *persistent.Confidence
		}
	}

	if autorouted != nil && autorouted.BestMatch != nil {
		output["mapping"] = map[string]interface{}{
			"sourceSystem":  autorouted.Trace.Request.SourceSystem,
			"sourceCode":    autorouted.Trace.Request.SourceCode,
			"targetSystem":  autorouted.BestMatch.System,
			"targetCode":    autorouted.BestMatch.Code,
			"targetDisplay": autorouted.BestMatch.Display,
			"equivalence":   autorouted.BestMatch.Equivalence,
			"origin":        "autoroute",
		}
		output["confidence"] = autorouted.Confidence
		output["reasoning"] = autorouted.Reasoning
		output["durationMs"] = autorouted.TotalDuration.Milliseconds()

		if len(autorouted.Alternates) > 0 {
			alts := make([]map[string]interface{}, len(autorouted.Alternates))
			for i, alt := range autorouted.Alternates {
				alts[i] = map[string]interface{}{
					"code":       alt.Code,
					"display":    alt.Display,
					"confidence": alt.Confidence,
				}
			}
			output["alternates"] = alts
		}
	}

	data, err := json.MarshalIndent(output, "", "  ")
	if err != nil {
		return err
	}
	fmt.Println(string(data))
	return nil
}
