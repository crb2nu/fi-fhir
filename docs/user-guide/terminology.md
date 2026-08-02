# Terminology Management

fi-fhir includes a comprehensive terminology subsystem for managing healthcare code systems, custom mappings, and semantic search. This guide covers database setup, vocabulary loading, and code mapping operations.

## Overview

Healthcare integration requires translating local codes (lab codes, procedure codes, diagnosis codes) to standard terminologies like LOINC, SNOMED CT, RxNorm, and ICD-10-CM. fi-fhir's terminology subsystem provides:

- **Vocabulary Loading**: Import standard terminologies (RxNorm, LOINC, UMLS, ICD-10-CM)
- **Custom Mappings**: Upload and manage local-to-standard code mappings
- **Semantic Search**: Find codes by meaning using LLM embeddings
- **Autoroute Resolution**: LLM-powered mapping suggestions when exact matches aren't found
- **Version Management**: Track and switch between terminology versions

### Supported Vocabularies

| Vocabulary | Description | Use Case |
|------------|-------------|----------|
| **RxNorm** | Drug terminology | Medication normalization |
| **LOINC** | Laboratory codes | Lab result standardization |
| **SNOMED CT** | Clinical terms | Clinical concept mapping |
| **ICD-10-CM** | Diagnosis codes | Diagnosis classification |
| **UMLS** | Unified Medical Language System | Cross-vocabulary mapping |

---

## Getting Started

### Database Setup

The terminology subsystem requires a PostgreSQL database. Initialize it with:

```bash
# Initialize terminology database
fi-fhir terminology init --db "$DATABASE_URL"

# Check initialization status
fi-fhir terminology status --db "$DATABASE_URL"
```

The `init` command creates the necessary tables for storing vocabulary data, mappings, and metadata.

### Environment Variables

Configure the terminology subsystem using environment variables:

```bash
# Required: PostgreSQL connection
export FI_FHIR_TERMINOLOGY_DB_URL="postgres://user:pass@localhost:5432/terminology"

# Optional: Vector database for semantic search
export QDRANT_URL="http://localhost:6333"

# Optional: Embedding service for semantic search
export LLM_EMBEDDING_BASE_URL="http://localhost:8000/v1"
export LLM_EMBEDDING_MODEL="text-embedding-3-small"
```

| Variable | Required | Description |
|----------|----------|-------------|
| `FI_FHIR_TERMINOLOGY_DB_URL` | Yes | PostgreSQL connection string |
| `QDRANT_URL` | No | Qdrant vector database URL for semantic search |
| `LLM_EMBEDDING_BASE_URL` | No | Embedding API endpoint |
| `LLM_EMBEDDING_MODEL` | No | Model for generating embeddings |
| `LLM_EMBEDDING_TIMEOUT` | No | Timeout for embedding requests (default: 30s) |

---

## Loading Vocabularies

Load standard vocabularies from their official distribution files.

### RxNorm

RxNorm files are distributed in RRF (Rich Release Format) from the NLM UMLS portal.

```bash
# Load RxNorm from RRF directory
fi-fhir terminology load rxnorm /path/to/rrf/ --version 2024-01

# Specify database connection
fi-fhir terminology load rxnorm /data/rxnorm/rrf/ \
  --version 2024-01 \
  --db "$DATABASE_URL"
```

The loader imports:
- Concepts (RXNCONSO.RRF)
- Relationships (RXNREL.RRF)
- Attributes (RXNSAT.RRF)

### LOINC

LOINC is distributed as CSV files from loinc.org.

```bash
# Load LOINC from CSV
fi-fhir terminology load loinc /data/loinc/LoincTable.csv --version 2.77

# With full path
fi-fhir terminology load loinc /data/loinc/Loinc_2.77/LoincTable/Loinc.csv \
  --version 2.77 \
  --db "$DATABASE_URL"
```

### UMLS

UMLS Metathesaurus files from the NLM portal.

```bash
# Load UMLS META directory
fi-fhir terminology load umls /data/umls/META/ --version 2024AB

# Load specific tables
fi-fhir terminology load umls /data/umls/2024AB/META/ \
  --version 2024AB \
  --tables MRCONSO,MRREL,MRSTY
```

### ICD-10-CM

ICD-10-CM codes from CMS distribution files.

```bash
# Load ICD-10-CM
fi-fhir terminology load icd10cm /data/icd10cm/codes.csv --version FY2024

# From CMS table format
fi-fhir terminology load icd10cm /data/icd10cm/icd10cm_tabular_2024.xml \
  --version FY2024 \
  --format xml
```

### Version Management

Manage multiple vocabulary versions and set the active version:

```bash
# View loaded vocabularies and versions
fi-fhir terminology status --db "$DATABASE_URL"

# Set active version for a vocabulary
fi-fhir terminology use rxnorm 2024-01 --db "$DATABASE_URL"
fi-fhir terminology use loinc 2.77 --db "$DATABASE_URL"

# Drop all terminology data (use with caution)
fi-fhir terminology drop --force --db "$DATABASE_URL"
```

---

## Custom Code Mappings

Healthcare systems use local codes that must be mapped to standard terminologies. fi-fhir supports uploading and managing these mappings.

### CSV Upload Format

Prepare mappings in CSV format:

```csv
source_code,source_display,target_code,target_system,target_display,confidence
GLU001,Glucose Serum,2345-7,http://loinc.org,Glucose [Mass/volume] in Serum or Plasma,1.0
HGB001,Hemoglobin,718-7,http://loinc.org,Hemoglobin [Mass/volume] in Blood,1.0
WBC001,White Blood Count,6690-2,http://loinc.org,Leukocytes [#/volume] in Blood,1.0
```

| Column | Required | Description |
|--------|----------|-------------|
| `source_code` | Yes | Local code identifier |
| `source_display` | No | Human-readable name for source code |
| `target_code` | Yes | Standard code |
| `target_system` | Yes | Target code system URI |
| `target_display` | No | Standard display name |
| `confidence` | No | Mapping confidence (0.0-1.0) |

### Upload Mappings

```bash
# Upload mappings from CSV
fi-fhir terminology mapping upload mappings.csv \
  --source-system epic_labs \
  --target-system http://loinc.org

# With description
fi-fhir terminology mapping upload lab_mappings.csv \
  --source-system hospital_lis \
  --target-system http://loinc.org \
  --description "Main hospital LIS to LOINC mappings"
```

### List and Manage Mappings

```bash
# List all mapping sets
fi-fhir terminology mapping list

# List mappings for a specific source system
fi-fhir terminology mapping list --source-system epic_labs

# Get details of a specific mapping
fi-fhir terminology mapping get <mapping-id>

# Delete a mapping set
fi-fhir terminology mapping delete <mapping-id> --force
```

### Resolve Mappings

Look up the target code for a source code:

```bash
# Basic resolution
fi-fhir terminology mapping resolve GLU001 \
  --source-system epic_labs \
  --target-system http://loinc.org

# Output
{
  "source_code": "GLU001",
  "source_system": "epic_labs",
  "target_code": "2345-7",
  "target_system": "http://loinc.org",
  "target_display": "Glucose [Mass/volume] in Serum or Plasma",
  "confidence": 1.0,
  "method": "exact"
}
```

---

## LLM-Powered Autoroute

When an exact mapping isn't found, fi-fhir can use LLM embeddings to suggest the most likely target code.

### How Autoroute Works

1. **Exact Match**: First, attempts direct lookup in custom mappings
2. **Fuzzy Match**: Falls back to string similarity if enabled
3. **Semantic Match**: Uses embeddings to find semantically similar codes

### Usage

```bash
# Resolve with autoroute enabled
fi-fhir terminology mapping resolve UNKNOWN_LAB_CODE \
  --source-system hospital_lis \
  --target-system http://loinc.org \
  --autoroute

# Output with autoroute
{
  "source_code": "UNKNOWN_LAB_CODE",
  "source_system": "hospital_lis",
  "target_code": "2345-7",
  "target_system": "http://loinc.org",
  "target_display": "Glucose [Mass/volume] in Serum or Plasma",
  "confidence": 0.87,
  "method": "semantic",
  "alternatives": [
    {
      "code": "2339-0",
      "display": "Glucose [Mass/volume] in Blood",
      "score": 0.82
    }
  ]
}
```

### Configuration

Autoroute requires Qdrant and an embedding service:

```yaml
terminology:
  autoroute:
    enabled: true
    min_confidence: 0.7        # Minimum score to return a match
    max_alternatives: 3        # Number of alternatives to return
    embedding_model: text-embedding-3-small
```

---

## Semantic Search

Find terminology codes by meaning rather than exact string matching. "Blood sugar" finds glucose codes even though the strings don't match.

### CLI Usage

```bash
# Search LOINC for glucose-related tests
fi-fhir terminology search --query "blood sugar" --vocabulary loinc --limit 10

# Search SNOMED for heart conditions
fi-fhir terminology search --query "chest pain" --vocabulary snomed --limit 5

# Search across all loaded vocabularies
fi-fhir terminology search --query "diabetes medication" --limit 20
```

### Output

```json
{
  "results": [
    {
      "code": "2345-7",
      "system": "http://loinc.org",
      "display": "Glucose [Mass/volume] in Serum or Plasma",
      "score": 0.94
    },
    {
      "code": "2339-0",
      "system": "http://loinc.org",
      "display": "Glucose [Mass/volume] in Blood",
      "score": 0.91
    },
    {
      "code": "41653-7",
      "system": "http://loinc.org",
      "display": "Glucose [Mass/volume] in Capillary blood by Glucometer",
      "score": 0.87
    }
  ],
  "query": "blood sugar",
  "vocabulary": "loinc",
  "total_results": 3
}
```

### Building the Search Index

Semantic search requires pre-built embedding indexes:

```bash
# Build LOINC index
fi-fhir terminology index build --vocabulary loinc --source ./data/LoincTable.csv

# Build SNOMED index
fi-fhir terminology index build --vocabulary snomed --source ./data/sct2_Description.txt

# Build RxNorm index
fi-fhir terminology index build --vocabulary rxnorm --source ./data/rxnorm/rrf/

# Check index status
fi-fhir terminology index status

# Output
{
  "indexes": [
    {
      "vocabulary": "loinc",
      "version": "2.77",
      "document_count": 98543,
      "last_updated": "2024-01-15T10:30:00Z",
      "status": "ready"
    },
    {
      "vocabulary": "snomed",
      "version": "2024-01",
      "document_count": 456789,
      "last_updated": "2024-01-14T15:45:00Z",
      "status": "ready"
    }
  ]
}
```

---

## Crosswalk Between Vocabularies

Map codes between different terminology systems using UMLS relationships.

```bash
# Cross-walk ICD-10-CM to SNOMED CT
fi-fhir terminology crosswalk --from icd10cm --to snomed E11.9

# Output
{
  "source": {
    "code": "E11.9",
    "system": "ICD-10-CM",
    "display": "Type 2 diabetes mellitus without complications"
  },
  "targets": [
    {
      "code": "44054006",
      "system": "SNOMED CT",
      "display": "Type 2 diabetes mellitus",
      "relationship": "exact_match",
      "confidence": 0.95
    }
  ]
}

# Cross-walk LOINC to SNOMED
fi-fhir terminology crosswalk --from loinc --to snomed 2345-7
```

---

## Workflow Integration

Use terminology operations within workflow transforms:

```yaml
workflow:
  name: standardize_codes
  version: "1.0"
  routes:
    - name: map_lab_codes
      filter:
        event_type: lab_result
      transform:
        # Map local codes to LOINC
        - map_terminology:
            field: observation.code
            from: hospital_lis
            to: http://loinc.org
      actions:
        - type: fhir
          endpoint: https://fhir.example.com/r4
          token: my-static-bearer-token
```

Workflow YAML values are literal — `${VAR}` references are **not** expanded by
`fi-fhir`. Render the file with `envsubst` before loading it if you need
environment-specific values.

### Transform Options

`map_terminology` accepts exactly these three keys; any other key is silently
ignored when the workflow loads.

| Option | Description |
|--------|-------------|
| `field` | Event field containing the code to map |
| `from` | Source code system identifier |
| `to` | Target code system identifier |

If no mapping is found, the field is left unchanged and the route continues.
When the matched mapping carries a display name, it is written to a parallel
`<field>_display` key (here, `observation.code_display`).

Autoroute is not a workflow transform option. Enable it in the
`terminology.autoroute` service config (see
[LLM-Powered Autoroute](#llm-powered-autoroute)), or per-invocation with
`fi-fhir terminology mapping resolve --autoroute`.

---

## Troubleshooting

### Database Connection Issues

```bash
# Test database connectivity
fi-fhir terminology status --db "$DATABASE_URL"

# Reinitialize if tables are missing
fi-fhir terminology init --db "$DATABASE_URL" --force
```

### Slow Semantic Search

- Ensure Qdrant is running and accessible
- Verify embedding index is built for the vocabulary
- Check network latency to embedding service
- Consider local embedding deployment for production

### Missing Mappings

```bash
# Check if mappings are loaded
fi-fhir terminology mapping list --source-system your_system

# Verify vocabulary is loaded
fi-fhir terminology status
```

### Index Build Failures

```bash
# Check index status for errors
fi-fhir terminology index status --verbose

# Rebuild index
fi-fhir terminology index build --vocabulary loinc --force
```

---

## Best Practices

### Vocabulary Management

1. **Version tracking**: Always specify versions when loading vocabularies
2. **Regular updates**: Schedule quarterly vocabulary updates
3. **Backup before updates**: Export mappings before vocabulary changes

### Custom Mappings

1. **Start with high-confidence mappings**: Focus on frequently used codes first
2. **Review autoroute suggestions**: Don't blindly accept LLM suggestions
3. **Document mapping decisions**: Add descriptions to mapping uploads
4. **Monitor unmapped codes**: Track codes that fail to resolve

### Performance

1. **Pre-build indexes**: Build embedding indexes during deployment, not runtime
2. **Cache resolutions**: Terminology lookups are cached automatically
3. **Batch operations**: Use batch upload for large mapping sets
4. **Local deployment**: Run embedding service locally for consistent latency

---

## See Also

- [CLI Reference](cli-reference.md) - Terminology command summary
- [LLM-Powered Features](llm-features.md) - Semantic search details
- [Workflow Configuration](workflows.md) - Using terminology in workflows
- [Core Concepts](core-concepts.md) - Architecture overview
