package index

import (
	"context"
	"encoding/csv"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"gitlab.flexinfer.ai/libs/fi-fhir/pkg/llm"
)

// Builder constructs terminology embedding indexes.
type Builder struct {
	config          IndexConfig
	qdrant          *QdrantClient
	embeddingClient llm.EmbeddingClient
	progressChan    chan BuildProgress
}

// NewBuilder creates a new index builder.
func NewBuilder(config IndexConfig) (*Builder, error) {
	// Create embedding client
	embCfg := llm.EmbeddingConfig{
		BaseURL:    config.EmbeddingBaseURL,
		APIKey:     config.EmbeddingAPIKey,
		Model:      config.EmbeddingModel,
		Dimensions: config.EmbeddingDimensions,
		Timeout:    config.Timeout,
		MaxRetries: 3,
		BatchSize:  config.BatchSize,
	}
	embClient, err := llm.NewEmbeddingClient(embCfg)
	if err != nil {
		return nil, fmt.Errorf("create embedding client: %w", err)
	}

	// Create Qdrant client
	qdrant := NewQdrantClient(config.QdrantURL, config.QdrantAPIKey, config.Timeout)

	return &Builder{
		config:          config,
		qdrant:          qdrant,
		embeddingClient: embClient,
		progressChan:    make(chan BuildProgress, 100),
	}, nil
}

// BuildOptions configures a build operation.
type BuildOptions struct {
	// SourcePath is the path to the vocabulary source file.
	SourcePath string

	// Vocabulary is the vocabulary type to build.
	Vocabulary Vocabulary

	// Version is the vocabulary version.
	Version string

	// DropExisting drops the existing collection before building.
	DropExisting bool

	// OnProgress is called with progress updates.
	OnProgress func(BuildProgress)
}

// Build builds an embedding index for a vocabulary.
func (b *Builder) Build(ctx context.Context, opts BuildOptions) error {
	progress := BuildProgress{
		Vocabulary: opts.Vocabulary,
		Status:     BuildStatusRunning,
		StartedAt:  time.Now(),
		UpdatedAt:  time.Now(),
	}

	// Send progress updates
	sendProgress := func() {
		progress.UpdatedAt = time.Now()
		if opts.OnProgress != nil {
			opts.OnProgress(progress)
		}
	}

	// Create or recreate collection
	collectionName := opts.Vocabulary.CollectionName()
	if opts.DropExisting {
		if err := b.qdrant.DeleteCollection(ctx, collectionName); err != nil {
			// Ignore not found errors
			if !strings.Contains(err.Error(), "404") && !strings.Contains(err.Error(), "not found") {
				progress.Status = BuildStatusFailed
				progress.Errors = append(progress.Errors, err.Error())
				sendProgress()
				return fmt.Errorf("delete existing collection: %w", err)
			}
		}
	}

	exists, err := b.qdrant.CollectionExists(ctx, collectionName)
	if err != nil {
		progress.Status = BuildStatusFailed
		progress.Errors = append(progress.Errors, err.Error())
		sendProgress()
		return fmt.Errorf("check collection exists: %w", err)
	}

	if !exists {
		if err := b.qdrant.CreateCollection(ctx, collectionName, b.config.EmbeddingDimensions); err != nil {
			progress.Status = BuildStatusFailed
			progress.Errors = append(progress.Errors, err.Error())
			sendProgress()
			return fmt.Errorf("create collection: %w", err)
		}
	}

	// Load entries from source file
	entries, err := b.loadEntries(opts.SourcePath, opts.Vocabulary)
	if err != nil {
		progress.Status = BuildStatusFailed
		progress.Errors = append(progress.Errors, err.Error())
		sendProgress()
		return fmt.Errorf("load entries: %w", err)
	}

	progress.TotalItems = len(entries)
	sendProgress()

	// Process in batches
	batchSize := b.config.BatchSize
	if batchSize <= 0 {
		batchSize = 32
	}

	for i := 0; i < len(entries); i += batchSize {
		select {
		case <-ctx.Done():
			progress.Status = BuildStatusCanceled
			sendProgress()
			return ctx.Err()
		default:
		}

		end := i + batchSize
		if end > len(entries) {
			end = len(entries)
		}

		batch := entries[i:end]
		progress.ProcessedItems = end

		// Generate embeddings
		texts := make([]string, len(batch))
		for j, entry := range batch {
			texts[j] = entry.EmbeddingText
		}

		embeddings, err := b.embeddingClient.Embed(ctx, texts)
		if err != nil {
			progress.Errors = append(progress.Errors, fmt.Sprintf("batch %d: %v", i/batchSize, err))
			sendProgress()
			continue // Continue with next batch
		}

		progress.EmbeddedItems = end

		// Create points
		points := make([]Point, len(batch))
		for j, entry := range batch {
			payload := map[string]interface{}{
				"code":       entry.Code,
				"system":     entry.System,
				"display":    entry.Display,
				"vocabulary": string(entry.Vocabulary),
			}
			for k, v := range entry.Metadata {
				payload[k] = v
			}

			points[j] = Point{
				ID:      entry.ID,
				Vector:  embeddings[j],
				Payload: payload,
			}
		}

		// Upsert to Qdrant
		if err := b.qdrant.UpsertPoints(ctx, collectionName, points); err != nil {
			progress.Errors = append(progress.Errors, fmt.Sprintf("upsert batch %d: %v", i/batchSize, err))
			sendProgress()
			continue
		}

		progress.IndexedItems = end
		sendProgress()
	}

	progress.Status = BuildStatusCompleted
	sendProgress()

	return nil
}

// loadEntries loads entries from a source file based on vocabulary type.
func (b *Builder) loadEntries(path string, vocabulary Vocabulary) ([]IndexEntry, error) {
	switch vocabulary {
	case VocabularyLOINC:
		return b.loadLOINCEntries(path)
	case VocabularySNOMED:
		return b.loadSNOMEDEntries(path)
	case VocabularyICD10CM:
		return b.loadICD10Entries(path)
	default:
		return nil, fmt.Errorf("unsupported vocabulary: %s", vocabulary)
	}
}

// loadLOINCEntries loads LOINC entries from a CSV file.
func (b *Builder) loadLOINCEntries(path string) ([]IndexEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close() //nolint:errcheck // Best-effort close on read-only file

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1 // Variable fields

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	// Map column names to indices
	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Validate required columns
	required := []string{"loinc_num"} // At minimum need the code
	for _, col := range required {
		if _, ok := colIdx[col]; !ok {
			return nil, fmt.Errorf("missing required column: %s", col)
		}
	}

	var entries []IndexEntry
	lineNum := 1
	for {
		lineNum++
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("read line %d: %w", lineNum, err)
		}

		entry := LOINCEntry{
			Code:         getCSVCol(record, colIdx, "loinc_num"),
			Component:    getCSVCol(record, colIdx, "component"),
			Property:     getCSVCol(record, colIdx, "property"),
			TimeAspect:   getCSVCol(record, colIdx, "time_aspct"),
			System:       getCSVCol(record, colIdx, "system"),
			Scale:        getCSVCol(record, colIdx, "scale_typ"),
			Method:       getCSVCol(record, colIdx, "method_typ"),
			ShortName:    getCSVCol(record, colIdx, "shortname"),
			LongName:     getCSVCol(record, colIdx, "long_common_name"),
			Consumer:     getCSVCol(record, colIdx, "consumer_name"),
			RelatedNames: getCSVCol(record, colIdx, "relatednames2"),
			Status:       getCSVCol(record, colIdx, "status"),
		}

		// Skip deprecated/inactive codes
		if entry.Status == "DEPRECATED" || entry.Status == "DISCOURAGED" {
			continue
		}

		if entry.Code == "" {
			continue
		}

		entries = append(entries, entry.ToIndexEntry())
	}

	return entries, nil
}

// loadSNOMEDEntries loads SNOMED CT entries from a file.
// Supports two formats:
//   - RF2 descriptions (auto-detected by "effectivetime" header column):
//     columns conceptId, term, typeId, active
//   - Simplified TSV: columns id, fsn, term, synonyms, semantictag, active
func (b *Builder) loadSNOMEDEntries(path string) ([]IndexEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close() //nolint:errcheck // Best-effort close on read-only file

	reader := csv.NewReader(f)
	reader.Comma = '\t' // SNOMED files are typically TSV
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	// Auto-detect RF2 format by checking for the effectivetime column.
	_, isRF2 := colIdx["effectivetime"]
	if isRF2 {
		return b.loadSNOMEDRF2(reader, colIdx)
	}
	return b.loadSNOMEDSimplified(reader, colIdx)
}

// SNOMED CT RF2 typeId constants.
const (
	snomedTypeIDFSN     = "900000000000003001" // Fully Specified Name
	snomedTypeIDSynonym = "900000000000013009" // Synonym (preferred/acceptable)
)

// loadSNOMEDRF2 reads SNOMED CT RF2 description files.
// RF2 columns: id, effectiveTime, active, moduleId, conceptId, languageCode, typeId, term, caseSignificanceId
func (b *Builder) loadSNOMEDRF2(reader *csv.Reader, colIdx map[string]int) ([]IndexEntry, error) {
	// Collect descriptions grouped by conceptId so we can merge FSN + preferred term.
	type conceptData struct {
		FSN      string
		Synonyms []string
		Active   bool
	}
	concepts := make(map[string]*conceptData)

	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed lines
		}

		activeStr := getCSVCol(record, colIdx, "active")
		if activeStr != "1" {
			continue
		}

		conceptID := getCSVCol(record, colIdx, "conceptid")
		if conceptID == "" {
			continue
		}

		term := getCSVCol(record, colIdx, "term")
		typeID := getCSVCol(record, colIdx, "typeid")

		data, ok := concepts[conceptID]
		if !ok {
			data = &conceptData{Active: true}
			concepts[conceptID] = data
		}

		switch typeID {
		case snomedTypeIDFSN:
			data.FSN = term
		default:
			// Treat all non-FSN descriptions as synonyms.
			data.Synonyms = append(data.Synonyms, term)
		}
	}

	entries := make([]IndexEntry, 0, len(concepts))
	for conceptID, data := range concepts {
		// Use first synonym as Description (preferred term), or fall back to FSN.
		description := data.FSN
		if len(data.Synonyms) > 0 {
			description = data.Synonyms[0]
		}

		// Extract semantic tag from FSN (e.g., "Aspirin (substance)" → "substance").
		semanticTag := ""
		if data.FSN != "" {
			if idx := strings.LastIndex(data.FSN, "("); idx > 0 {
				semanticTag = strings.TrimSuffix(data.FSN[idx+1:], ")")
			}
		}

		entry := SNOMEDEntry{
			ConceptID:   conceptID,
			FSN:         data.FSN,
			Description: description,
			Synonyms:    strings.Join(data.Synonyms, "; "),
			Semantic:    semanticTag,
			Active:      true,
		}
		entries = append(entries, entry.ToIndexEntry())
	}

	return entries, nil
}

// loadSNOMEDSimplified reads SNOMED CT simplified TSV files.
// Expected columns: id, fsn, term, synonyms, semantictag, active
func (b *Builder) loadSNOMEDSimplified(reader *csv.Reader, colIdx map[string]int) ([]IndexEntry, error) {
	var entries []IndexEntry
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue // Skip malformed lines
		}

		// Parse active status
		activeStr := getCSVCol(record, colIdx, "active")
		active := activeStr == "1" || strings.ToLower(activeStr) == "true"

		entry := SNOMEDEntry{
			ConceptID:   getCSVCol(record, colIdx, "id"),
			FSN:         getCSVCol(record, colIdx, "fsn"),
			Description: getCSVCol(record, colIdx, "term"),
			Synonyms:    getCSVCol(record, colIdx, "synonyms"),
			Semantic:    getCSVCol(record, colIdx, "semantictag"),
			Active:      active,
		}

		if entry.ConceptID == "" {
			continue
		}

		// Skip inactive concepts
		if !entry.Active {
			continue
		}

		entries = append(entries, entry.ToIndexEntry())
	}

	return entries, nil
}

// loadICD10Entries loads ICD-10-CM entries from a file.
func (b *Builder) loadICD10Entries(path string) ([]IndexEntry, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open file: %w", err)
	}
	defer f.Close() //nolint:errcheck // Best-effort close on read-only file

	reader := csv.NewReader(f)
	reader.TrimLeadingSpace = true
	reader.FieldsPerRecord = -1

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("read header: %w", err)
	}

	colIdx := make(map[string]int)
	for i, col := range header {
		colIdx[strings.ToLower(strings.TrimSpace(col))] = i
	}

	var entries []IndexEntry
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			continue
		}

		// Parse valid/billable status
		validStr := getCSVCol(record, colIdx, "valid")
		valid := validStr == "1" || strings.ToLower(validStr) == "true" || validStr == ""

		entry := ICD10Entry{
			Code:        getCSVCol(record, colIdx, "code"),
			Description: getCSVCol(record, colIdx, "short_description"),
			LongDesc:    getCSVCol(record, colIdx, "long_description"),
			Category:    getCSVCol(record, colIdx, "category"),
			Valid:       valid,
		}

		// Try alternate column names
		if entry.Code == "" {
			entry.Code = getCSVCol(record, colIdx, "icd10cm")
		}
		if entry.Description == "" {
			entry.Description = getCSVCol(record, colIdx, "description")
		}

		if entry.Code == "" {
			continue
		}

		entries = append(entries, entry.ToIndexEntry())
	}

	return entries, nil
}

// getCSVCol safely gets a column value from a CSV record.
func getCSVCol(record []string, colIdx map[string]int, colName string) string {
	idx, ok := colIdx[colName]
	if !ok || idx >= len(record) {
		return ""
	}
	return strings.TrimSpace(record[idx])
}

// Progress returns a channel for receiving progress updates.
func (b *Builder) Progress() <-chan BuildProgress {
	return b.progressChan
}
