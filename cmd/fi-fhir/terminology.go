package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	_ "github.com/lib/pq"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/terminology/db"
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
	case "drop":
		return runTerminologyDrop(args[1:])
	case "load":
		return runTerminologyLoad(args[1:])
	case "crosswalk":
		return runTerminologyCrosswalk(args[1:])
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
  drop      Drop the terminology schema (WARNING: deletes all data)
  load      Load terminology data from files (LOINC, UMLS, SNOMED, etc.)
  crosswalk Translate codes between vocabularies

Options:
  --db      PostgreSQL connection string (or FI_FHIR_DATABASE_URL env)

Examples:
  # Initialize terminology schema
  fi-fhir terminology init --db "$DATABASE_URL"

  # Check status of loaded terminologies
  fi-fhir terminology status --db "$DATABASE_URL"

  # Load LOINC codes
  fi-fhir terminology load loinc /path/to/LoincTable.csv --version 2.77

  # Load UMLS Metathesaurus
  fi-fhir terminology load umls /path/to/META/ --version 2024AB

  # Load RxNorm
  fi-fhir terminology load rxnorm /path/to/rrf/ --version 2024-01

  # Load SNOMED CT US Edition
  fi-fhir terminology load snomed /path/to/RF2/ --version 2024-03

  # Cross-walk a code between vocabularies
  fi-fhir terminology crosswalk E11.9 --from ICD10CM --to SNOMEDCT_US`)
}

func getTerminologyDBURL(args []string) string {
	// Check for --db flag
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "--db" { //nolint:gosec // guarded by loop bounds; gosec false positive
			return args[i+1]
		}
	}
	// Fall back to environment variable
	return os.Getenv("FI_FHIR_DATABASE_URL")
}

func runTerminologyInit(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
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
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
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

	return nil
}

func runTerminologyDrop(args []string) error {
	dbURL := getTerminologyDBURL(args)
	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
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
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

	for i := 2; i < len(args); {
		switch args[i] { //nolint:gosec // guarded by loop bounds; gosec false positive
		case "--db":
			if i+1 < len(args) {
				dbURL = args[i+1]
				i += 2
				continue
			}
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
		}
		i++
	}

	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
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
		return loadLOINC(ctx, conn, migrator, path, version, releaseDate)
	case "umls":
		return loadUMLS(ctx, conn, migrator, path, version, releaseDate)
	case "rxnorm":
		return loadRxNorm(ctx, conn, migrator, path, version, releaseDate)
	case "snomed":
		return loadSNOMED(ctx, conn, migrator, path, version, releaseDate)
	case "icd10cm":
		return loadICD10CM(ctx, conn, migrator, path, version, releaseDate)
	case "icd10pcs":
		return loadICD10PCS(ctx, conn, migrator, path, version, releaseDate)
	default:
		return fmt.Errorf("unknown vocabulary: %s", vocab)
	}
}

// loadLOINC loads LOINC codes from LoincTable.csv into the database.
func loadLOINC(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading LOINC version %s from %s...\n", version, path)

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

func loadUMLS(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading UMLS version %s from %s...\n", version, path)

	// Validate directory
	if err := db.ValidateUMLSDirectory(path); err != nil {
		return fmt.Errorf("invalid UMLS META directory: %w", err)
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

func loadRxNorm(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading RxNorm version %s from %s...\n", version, path)

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

func loadSNOMED(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading SNOMED CT version %s from %s...\n", version, path)
	fmt.Println("SNOMED loader not yet implemented. Coming in Phase 4.")
	return nil
}

func loadICD10CM(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading ICD-10-CM version %s from %s...\n", version, path)

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

func loadICD10PCS(ctx context.Context, conn *sql.DB, migrator *db.Migrator, path, version string, releaseDate *time.Time) error {
	fmt.Printf("Loading ICD-10-PCS version %s from %s...\n", version, path)
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
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")

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
		case "--db":
			if i+1 < len(args) {
				dbURL = args[i+1]
				i++
			}
		}
	}

	if dbURL == "" {
		return fmt.Errorf("database URL required: use --db flag or FI_FHIR_DATABASE_URL env var")
	}

	if fromVocab == "" || toVocab == "" {
		return fmt.Errorf("--from and --to vocabularies are required")
	}

	fmt.Printf("Cross-walk: %s (%s) -> %s\n", code, fromVocab, toVocab)
	fmt.Println("Cross-walk queries not yet implemented. Coming in Phase 3.")
	return nil
}
