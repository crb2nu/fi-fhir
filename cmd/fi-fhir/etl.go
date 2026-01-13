package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/crb2nu/fi-fhir/pkg/etl"
	"github.com/crb2nu/fi-fhir/pkg/etl/sink"
	"github.com/crb2nu/fi-fhir/pkg/etl/source"
	"github.com/crb2nu/fi-fhir/pkg/storage"
	"github.com/crb2nu/fi-fhir/pkg/terminology/db"

	_ "github.com/lib/pq" // PostgreSQL driver
)

type minioSink interface {
	etl.Sink
	Validate(ctx context.Context) error
	List(ctx context.Context, prefix string) ([]storage.FileInfo, error)
}

var minioSinkFactory = func() (minioSink, error) {
	return createMinIOSink()
}

var sourcesProvider = getConfiguredSources

func runETL(args []string) error {
	if len(args) == 0 {
		printETLUsage()
		return nil
	}

	switch args[0] {
	case "sync":
		return runETLSync(args[1:])
	case "fetch":
		return runETLFetch(args[1:])
	case "fetch-test":
		return runETLFetchTest(args[1:])
	case "load":
		return runETLLoad(args[1:])
	case "status":
		return runETLStatus(args[1:])
	case "validate":
		return runETLValidate(args[1:])
	case "sources":
		return runETLSources(args[1:])
	case "-h", "--help", "help":
		printETLUsage()
		return nil
	default:
		return fmt.Errorf("unknown etl subcommand: %s", args[0])
	}
}

func printETLUsage() {
	fmt.Println(`fi-fhir etl - ETL Pipeline for Terminology Data

Usage:
  fi-fhir etl <subcommand> [options]

Subcommands:
  sync       Download terminology data from sources to MinIO
  fetch      Download a specific source/version
  fetch-test Download test data to local filesystem (no MinIO required)
  load       Load data from MinIO into PostgreSQL database
  status     Show sync and load status for all sources
  validate   Validate ETL configuration and connectivity
  sources    List configured data sources

Environment Variables:
  MINIO_ENDPOINT     MinIO/S3 endpoint (default: localhost:9000)
  MINIO_ACCESS_KEY   Access key ID
  MINIO_SECRET_KEY   Secret access key
  MINIO_USE_SSL      Use HTTPS (default: false)
  MINIO_BUCKET       Default bucket name (default: terminology)

Data Sources:
  The ETL pipeline supports downloading from:
  - HTTP/HTTPS URLs (CMS ICD-10, public datasets)
  - UMLS Terminology Services (requires API key)
  - FDA OpenFDA (NDC drug codes)
  - Synthea synthetic patient data
  - RxNorm API (drug names)
  - Local filesystem (for testing)

Test Data Sources (no auth required):
  ndc            - FDA National Drug Code database
  synthea-sample - Synthea 100-patient FHIR R4 bundle
  rxnorm-api     - RxNorm API lookups for common drugs

Examples:
  # Validate connectivity
  fi-fhir etl validate

  # List configured sources
  fi-fhir etl sources

  # Download ICD-10-CM from CMS
  fi-fhir etl fetch icd10cm --version FY2024

  # Download test data to local directory (no MinIO needed)
  fi-fhir etl fetch-test ndc --output ./testdata/downloads
  fi-fhir etl fetch-test synthea-sample --output ./testdata/downloads

  # Load data from MinIO into PostgreSQL
  fi-fhir etl load umls --version 2024AB
  fi-fhir etl load rxnorm --version 2024-01
  fi-fhir etl load loinc --version 2.77

  # Check sync status
  fi-fhir etl status

  # Sync all sources
  fi-fhir etl sync`)
}

func runETLSync(args []string) error {
	// Parse flags
	var (
		sourceName string
		version    string
		dryRun     bool
		overwrite  bool
	)

	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--source", "-s":
			if i+1 < len(args) {
				sourceName = args[i+1]
				i++
			}
		case "--version", "-v":
			if i+1 < len(args) {
				version = args[i+1]
				i++
			}
		case "--dry-run":
			dryRun = true
		case "--overwrite":
			overwrite = true
		}
	}

	// Create MinIO sink
	minioSink, err := minioSinkFactory()
	if err != nil {
		return err
	}

	// Get configured sources
	sources := sourcesProvider()

	if sourceName != "" {
		// Sync specific source
		src, ok := sources[sourceName]
		if !ok {
			return fmt.Errorf("unknown source: %s (use 'fi-fhir etl sources' to list available)", sourceName)
		}

		return syncSource(src, minioSink, version, dryRun, overwrite)
	}

	// Sync all sources
	fmt.Println("Syncing all configured sources...")
	for name, src := range sources {
		fmt.Printf("\n=== %s ===\n", name)
		if err := syncSource(src, minioSink, version, dryRun, overwrite); err != nil {
			fmt.Printf("Error: %v\n", err)
			// Continue with other sources
		}
	}

	return nil
}

func runETLFetch(args []string) error {
	if len(args) == 0 {
		return fmt.Errorf("usage: fi-fhir etl fetch <source> [--version <version>] [--dry-run] [--overwrite]")
	}

	sourceName := args[0]
	var (
		version   string
		dryRun    bool
		overwrite bool
	)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--version", "-v":
			if i+1 < len(args) {
				version = args[i+1]
				i++
			}
		case "--dry-run":
			dryRun = true
		case "--overwrite":
			overwrite = true
		}
	}

	// Create MinIO sink
	minioSink, err := minioSinkFactory()
	if err != nil {
		return err
	}

	// Get source
	sources := sourcesProvider()
	src, ok := sources[sourceName]
	if !ok {
		return fmt.Errorf("unknown source: %s", sourceName)
	}

	return syncSource(src, minioSink, version, dryRun, overwrite)
}

func runETLFetchTest(args []string) error {
	if len(args) == 0 {
		fmt.Println(`fi-fhir etl fetch-test - Download test data to local filesystem

Usage:
  fi-fhir etl fetch-test <source> [options]

Available Test Data Sources:
  ndc            - FDA National Drug Code database (~100MB JSON)
  synthea-sample - Synthea 100-patient FHIR R4 bundle (~50MB)
  rxnorm-api     - RxNorm API response for common drugs (~1KB each)
  icd10cm        - CMS ICD-10-CM code tables (~50MB ZIP)
  icd10pcs       - CMS ICD-10-PCS code tables (~50MB ZIP)

Options:
  --output, -o   Output directory (default: ./testdata/downloads)
  --version, -v  Specific version to download

Examples:
  fi-fhir etl fetch-test ndc --output ./testdata/downloads
  fi-fhir etl fetch-test synthea-sample
  fi-fhir etl fetch-test icd10cm --version FY2025`)
		return nil
	}

	sourceName := args[0]
	outputDir := "./testdata/downloads"
	version := ""

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--output", "-o":
			if i+1 < len(args) {
				outputDir = args[i+1]
				i++
			}
		case "--version", "-v":
			if i+1 < len(args) {
				version = args[i+1]
				i++
			}
		}
	}

	// Get source
	sources := sourcesProvider()
	src, ok := sources[sourceName]
	if !ok {
		return fmt.Errorf("unknown source: %s (available: ndc, synthea-sample, rxnorm-api, icd10cm, icd10pcs)", sourceName)
	}

	// Create local sink
	localSink := sink.NewLocalSink("local-test", outputDir)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	// Validate the sink (creates directory)
	if err := localSink.Validate(ctx); err != nil {
		return fmt.Errorf("failed to validate output directory: %w", err)
	}

	opts := etl.DefaultPipelineOptions()
	opts.OverwriteExisting = true
	opts.OnProgress = func(run *etl.PipelineRun) {
		if run.Status == etl.RunStatusRunning {
			fmt.Printf("  Downloading %s...\n", run.Version)
		}
	}

	p := etl.NewPipeline(src.Name(), src, localSink, opts)

	fmt.Printf("Fetching test data: %s\n", src.Name())
	if version != "" {
		fmt.Printf("  Version: %s\n", version)
	}
	fmt.Printf("  Output: %s\n", outputDir)

	run, err := p.Run(ctx, version)
	if err != nil {
		return fmt.Errorf("fetch failed: %w", err)
	}

	fmt.Printf("\nResult:\n")
	fmt.Printf("  Status: %s\n", run.Status)
	fmt.Printf("  Version: %s\n", run.Version)
	if run.BytesDownloaded > 0 {
		fmt.Printf("  Downloaded: %s\n", humanizeSize(run.BytesDownloaded))
	}
	fmt.Printf("  Duration: %s\n", run.Duration.Round(time.Second))
	fmt.Printf("  Saved to: %s/%s\n", outputDir, run.DestinationPath)

	return nil
}

func runETLLoad(args []string) error {
	if len(args) == 0 {
		fmt.Println(`fi-fhir etl load - Load terminology data from MinIO into PostgreSQL

Usage:
  fi-fhir etl load <source> [options]

Supported Sources:
  umls     - UMLS Metathesaurus (MRCONSO, MRREL, MRSTY)
  rxnorm   - RxNorm drug terminology
  loinc    - LOINC lab codes
  icd10cm  - ICD-10-CM diagnosis codes

Options:
  --version, -v   Version to load (required)
  --sabs          Comma-separated SABs to filter (UMLS only)
  --dry-run       Validate without loading
  --progress      Show detailed progress

Environment Variables:
  FI_FHIR_DATABASE_URL  PostgreSQL connection string
  MINIO_ENDPOINT        MinIO server endpoint
  MINIO_ACCESS_KEY      MinIO access key
  MINIO_SECRET_KEY      MinIO secret key
  MINIO_BUCKET          Bucket name (default: terminology)

Examples:
  # Load UMLS 2024AB from MinIO
  fi-fhir etl load umls --version 2024AB

  # Load RxNorm with progress
  fi-fhir etl load rxnorm --version 2024-01 --progress

  # Load ICD-10-CM
  fi-fhir etl load icd10cm --version FY2024

  # Load UMLS with SAB filter
  fi-fhir etl load umls --version 2024AB --sabs SNOMEDCT_US,ICD10CM`)
		return nil
	}

	sourceName := args[0]
	var (
		version  string
		sabs     string
		dryRun   bool
		progress bool
	)

	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--version", "-v":
			if i+1 < len(args) {
				version = args[i+1]
				i++
			}
		case "--sabs":
			if i+1 < len(args) {
				sabs = args[i+1]
				i++
			}
		case "--dry-run":
			dryRun = true
		case "--progress":
			progress = true
		}
	}

	if version == "" {
		return fmt.Errorf("--version is required")
	}

	// Connect to PostgreSQL
	dbURL := os.Getenv("FI_FHIR_DATABASE_URL")
	if dbURL == "" {
		dbURL = os.Getenv("DATABASE_URL")
	}
	if dbURL == "" {
		return fmt.Errorf("FI_FHIR_DATABASE_URL or DATABASE_URL environment variable is required")
	}

	database, err := sql.Open("postgres", dbURL)
	if err != nil {
		return fmt.Errorf("failed to connect to database: %w", err)
	}
	defer func() { _ = database.Close() }()

	// Create MinIO provider for downloading
	minioCfg := storage.MinIOConfig{
		Endpoint:        getEnvOrDefault("MINIO_ENDPOINT", "localhost:9000"),
		AccessKeyID:     os.Getenv("MINIO_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("MINIO_SECRET_KEY"),
		UseSSL:          os.Getenv("MINIO_USE_SSL") == "true",
		DefaultBucket:   getEnvOrDefault("MINIO_BUCKET", "terminology"),
	}

	if minioCfg.AccessKeyID == "" || minioCfg.SecretAccessKey == "" {
		return fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY environment variables are required")
	}

	minioProvider, err := storage.NewMinIOProvider(minioCfg)
	if err != nil {
		return fmt.Errorf("failed to create MinIO provider: %w", err)
	}

	// Create PostgreSQL sink
	pgSink := sink.NewPostgresSink(sink.PostgresSinkConfig{
		Name:     "postgres",
		DB:       database,
		Provider: minioProvider,
	})

	// Validate the sink
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Minute)
	defer cancel()

	if err := pgSink.Validate(ctx); err != nil {
		return fmt.Errorf("database validation failed: %w", err)
	}

	if dryRun {
		fmt.Printf("[DRY RUN] Would load %s version %s from MinIO to PostgreSQL\n", sourceName, version)
		return nil
	}

	// Create progress reporter if requested
	var progressReporter db.ProgressReporter
	if progress {
		progressReporter = cliProgressReporter()
	}

	// Construct MinIO path
	storagePath := fmt.Sprintf("%s/%s", sourceName, version)

	fmt.Printf("Loading %s version %s from MinIO to PostgreSQL...\n", sourceName, version)
	start := time.Now()

	var result *sink.LoadResult
	switch sourceName {
	case "umls":
		var opts *db.UMLSLoadOptions
		if sabs != "" {
			sabList := strings.Split(sabs, ",")
			opts = &db.UMLSLoadOptions{FilterSources: sabList}
		}
		result, err = pgSink.LoadUMLS(ctx, storagePath, version, opts, progressReporter)
	case "rxnorm":
		result, err = pgSink.LoadRxNorm(ctx, storagePath, version, nil, progressReporter)
	case "loinc":
		result, err = pgSink.LoadLOINC(ctx, storagePath, version, progressReporter)
	case "icd10cm":
		result, err = pgSink.LoadICD10CM(ctx, storagePath, version, nil, progressReporter)
	default:
		return fmt.Errorf("unknown source: %s (supported: umls, rxnorm, loinc, icd10cm)", sourceName)
	}

	if err != nil {
		return fmt.Errorf("load failed: %w", err)
	}

	// Print results
	fmt.Printf("\nLoad completed:\n")
	fmt.Printf("  Source:     %s\n", result.Source)
	fmt.Printf("  Version:    %s\n", result.Version)
	fmt.Printf("  Release ID: %d\n", result.ReleaseID)
	fmt.Printf("  Rows:       %d\n", result.RowsLoaded)
	fmt.Printf("  Duration:   %s\n", time.Since(start).Round(time.Second))

	if len(result.Details) > 0 {
		fmt.Printf("  Details:\n")
		for table, count := range result.Details {
			fmt.Printf("    - %s: %d\n", table, count)
		}
	}

	return nil
}

// cliProgressReporter returns a db.ProgressReporter that writes to stdout.
func cliProgressReporter() db.ProgressReporter {
	var lastUpdate time.Time
	return func(p db.LoadProgress) {
		// Throttle updates to avoid spamming the terminal
		if time.Since(lastUpdate) < 100*time.Millisecond {
			return
		}
		lastUpdate = time.Now()

		if p.RowsTotal > 0 {
			pct := float64(p.RowsLoaded) / float64(p.RowsTotal) * 100
			fmt.Printf("\r  [%5.1f%%] %s %s: %d / %d rows",
				pct, p.Vocabulary, p.Phase, p.RowsLoaded, p.RowsTotal)
		} else {
			fmt.Printf("\r  %s %s: %d rows", p.Vocabulary, p.Phase, p.RowsLoaded)
		}
	}
}

func runETLStatus(args []string) error {
	// Create MinIO sink to check what's stored
	minioSink, err := minioSinkFactory()
	if err != nil {
		return err
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sources := sourcesProvider()

	fmt.Printf("%-15s %-15s %-20s %s\n", "SOURCE", "VERSION", "LAST SYNC", "STATUS")
	fmt.Println(strings.Repeat("-", 70))

	for name := range sources {
		// Check what versions exist in MinIO
		prefix := name + "/"
		files, err := minioSink.List(ctx, prefix)

		var latestVersion string
		var lastMod time.Time

		if err == nil && len(files) > 0 {
			// Find the most recent version
			for _, f := range files {
				if f.LastModified.After(lastMod) {
					lastMod = f.LastModified
					// Extract version from path
					parts := strings.Split(f.Path, "/")
					if len(parts) >= 2 {
						latestVersion = parts[1]
					}
				}
			}
		}

		status := "not synced"
		if latestVersion != "" {
			status = "synced"
		}

		lastSyncStr := "-"
		if !lastMod.IsZero() {
			lastSyncStr = lastMod.Format("2006-01-02 15:04")
		}

		fmt.Printf("%-15s %-15s %-20s %s\n", name, latestVersion, lastSyncStr, status)
	}

	return nil
}

func runETLValidate(args []string) error {
	fmt.Println("Validating ETL configuration...")

	// Check MinIO connectivity
	fmt.Print("  MinIO connectivity: ")
	minioSink, err := minioSinkFactory()
	if err != nil {
		fmt.Printf("FAILED (%v)\n", err)
	} else {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		if err := minioSink.Validate(ctx); err != nil {
			fmt.Printf("FAILED (%v)\n", err)
		} else {
			fmt.Println("OK")
		}
	}

	// Check sources
	fmt.Println("\nSource availability:")
	sources := getConfiguredSources()
	for name, src := range sources {
		fmt.Printf("  %s: ", name)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		if err := src.Validate(ctx); err != nil {
			fmt.Printf("UNAVAILABLE (%v)\n", err)
		} else {
			fmt.Println("OK")
		}
		cancel()
	}

	return nil
}

func runETLSources(args []string) error {
	sources := getConfiguredSources()

	fmt.Printf("%-15s %-10s %s\n", "NAME", "TYPE", "DESCRIPTION")
	fmt.Println(strings.Repeat("-", 60))

	for name, src := range sources {
		srcType := "unknown"
		desc := ""

		switch s := src.(type) {
		case *source.HTTPSource:
			srcType = "http"
			desc = "HTTP download"
			_ = s // Use variable
		case *source.LocalSource:
			srcType = "local"
			desc = "Local filesystem"
			_ = s
		}

		fmt.Printf("%-15s %-10s %s\n", name, srcType, desc)
	}

	return nil
}

func syncSource(src etl.Source, snk etl.Sink, version string, dryRun, overwrite bool) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Minute)
	defer cancel()

	opts := etl.DefaultPipelineOptions()
	opts.DryRun = dryRun
	opts.OverwriteExisting = overwrite
	opts.OnProgress = func(run *etl.PipelineRun) {
		if run.Status == etl.RunStatusRunning {
			fmt.Printf("  Downloading %s...\n", run.Version)
		}
	}

	p := etl.NewPipeline(src.Name(), src, snk, opts)

	fmt.Printf("Syncing %s", src.Name())
	if version != "" {
		fmt.Printf(" (version: %s)", version)
	}
	if dryRun {
		fmt.Printf(" [DRY RUN]")
	}
	fmt.Println()

	run, err := p.Run(ctx, version)
	if err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	fmt.Printf("  Status: %s\n", run.Status)
	fmt.Printf("  Version: %s\n", run.Version)
	if run.BytesDownloaded > 0 {
		fmt.Printf("  Downloaded: %s\n", humanizeSize(run.BytesDownloaded))
	}
	fmt.Printf("  Duration: %s\n", run.Duration.Round(time.Second))
	if run.DestinationPath != "" {
		fmt.Printf("  Destination: %s\n", run.DestinationPath)
	}

	return nil
}

func createMinIOSink() (*sink.MinIOSink, error) {
	cfg := storage.MinIOConfig{
		Endpoint:        getEnvOrDefault("MINIO_ENDPOINT", "localhost:9000"),
		AccessKeyID:     os.Getenv("MINIO_ACCESS_KEY"),
		SecretAccessKey: os.Getenv("MINIO_SECRET_KEY"),
		UseSSL:          os.Getenv("MINIO_USE_SSL") == "true",
		DefaultBucket:   getEnvOrDefault("MINIO_BUCKET", "terminology"),
	}

	if cfg.AccessKeyID == "" || cfg.SecretAccessKey == "" {
		return nil, fmt.Errorf("MINIO_ACCESS_KEY and MINIO_SECRET_KEY environment variables are required")
	}

	return sink.NewMinIOSink(sink.MinIOSinkConfig{
		Name:        "minio",
		MinIOConfig: cfg,
		Bucket:      cfg.DefaultBucket,
	})
}

func getConfiguredSources() map[string]etl.Source {
	sources := make(map[string]etl.Source)

	// ICD-10-CM from CMS (public domain, no auth required)
	sources["icd10cm"] = source.NewHTTPSource(source.HTTPSourceConfig{
		Name: "icd10cm",
		URLs: map[string]string{
			"FY2024": "https://www.cms.gov/files/zip/2024-code-tables-tabular-and-index.zip",
			"FY2025": "https://www.cms.gov/files/zip/2025-code-tables-tabular-and-index.zip",
		},
	})

	// ICD-10-PCS from CMS
	sources["icd10pcs"] = source.NewHTTPSource(source.HTTPSourceConfig{
		Name: "icd10pcs",
		URLs: map[string]string{
			"FY2024": "https://www.cms.gov/files/zip/2024-icd-10-pcs-code-tables-and-index.zip",
			"FY2025": "https://www.cms.gov/files/zip/2025-icd-10-pcs-code-tables-and-index.zip",
		},
	})

	// NDC (National Drug Code) from FDA - public domain
	sources["ndc"] = source.NewHTTPSource(source.HTTPSourceConfig{
		Name: "ndc",
		URLs: map[string]string{
			"current": "https://download.open.fda.gov/drug/ndc/drug-ndc-0001-of-0001.json.zip",
		},
	})

	// Synthea synthetic patient data (public domain)
	sources["synthea-sample"] = source.NewHTTPSource(source.HTTPSourceConfig{
		Name: "synthea-sample",
		URLs: map[string]string{
			"100patients": "https://synthetichealth.github.io/synthea-sample-data/downloads/synthea_sample_data_fhir_r4_sep2019.zip",
		},
	})

	// RxNorm from NLM (public, no auth required for RxNorm API)
	sources["rxnorm-api"] = source.NewHTTPSource(source.HTTPSourceConfig{
		Name:      "rxnorm-api",
		UserAgent: "fi-fhir-etl/1.0 (healthcare data ETL)",
		URLs: map[string]string{
			// RxNorm REST API endpoints for specific drug lookups
			"aspirin":      "https://rxnav.nlm.nih.gov/REST/rxcui.json?name=aspirin",
			"metformin":    "https://rxnav.nlm.nih.gov/REST/rxcui.json?name=metformin",
			"lisinopril":   "https://rxnav.nlm.nih.gov/REST/rxcui.json?name=lisinopril",
			"atorvastatin": "https://rxnav.nlm.nih.gov/REST/rxcui.json?name=atorvastatin",
		},
	})

	// LOINC (requires registration but free)
	loincAuth := &source.HTTPAuth{
		Type:     "basic",
		Username: os.Getenv("LOINC_USERNAME"),
		Password: os.Getenv("LOINC_PASSWORD"),
	}
	if loincAuth.Username != "" {
		sources["loinc"] = source.NewHTTPSource(source.HTTPSourceConfig{
			Name: "loinc",
			URLs: map[string]string{
				"2.77": "https://loinc.org/download/loinc-complete/",
				"2.78": "https://loinc.org/download/loinc-complete/",
			},
			Auth: loincAuth,
		})
	}

	// UMLS Metathesaurus (requires API key from NLM)
	umlsAPIKey := os.Getenv("UMLS_API_KEY")
	if umlsAPIKey != "" {
		sources["umls"] = source.NewUMLSSource(source.UMLSSourceConfig{
			APIKey: umlsAPIKey,
		})
	}

	return sources
}

func getEnvOrDefault(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}
