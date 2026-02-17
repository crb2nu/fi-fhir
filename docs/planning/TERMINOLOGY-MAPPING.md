# Terminology Mapping: Upload, Autorouting & Telemetry

This document specifies the enhanced terminology mapping system with CSV upload, LLM-powered autorouting, and traceable decision telemetry.

## Quick Reference

| Feature | Status | Implementation |
|---------|--------|----------------|
| **CSV mapping upload** | ✅ Shipped | `pkg/terminology/upload/parser.go`, `internal/api/graphql/resolvers/schema.resolvers.go` |
| **Persistent mapping store** | ✅ Shipped | `pkg/terminology/db/mappings.go` |
| **Autoroute engine** | ✅ Shipped | `internal/terminology/autoroute/` |
| **Semantic search** | ✅ Exists | `pkg/terminology/semantic/searcher.go` |
| **LLM ranking/reasoning** | ✅ Shipped | `internal/terminology/autoroute/ranker.go` |
| **Decision telemetry** | 🟡 Partial | `internal/terminology/workflow/activities.go`, `pkg/terminology/db/mappings.go` |
| **Mapping review UI** | ✅ Shipped | `ui/src/lib/features/terminology/` |
| **Approval workflow** | ✅ Shipped (GraphQL/UI) | GraphQL pending-autoroute mutations + `PendingReviewList.svelte` |

## Concepts

### Mapping Resolution Modes

| Mode | Description | Latency | Confidence | Auditability |
|------|-------------|---------|------------|--------------|
| **Persistent** | Pre-approved mappings from uploads or manual curation | < 5ms | High (human-reviewed) | Full |
| **Autoroute** | Real-time LLM + semantic search suggestions | 100-500ms | Variable (0.0-1.0) | Full decision tree |
| **Hybrid** | Persistent first, autoroute fallback | 5-500ms | Layered | Full |

### Decision Types

```
PERSISTENT_HIT        - Found in uploaded/approved mappings
AUTOROUTE_HIGH_CONF   - LLM suggestion with confidence ≥ 0.90
AUTOROUTE_MED_CONF    - LLM suggestion with confidence ≥ 0.70
AUTOROUTE_LOW_CONF    - LLM suggestion with confidence < 0.70
NO_MATCH              - No mapping found
MANUAL_REQUIRED       - Flagged for human review
```

## Architecture

### Resolution Flow

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Mapping Request                              │
│  {                                                                   │
│    sourceCode: "LAB001",                                            │
│    sourceSystem: "epic_custom_labs",                                │
│    sourceDisplay: "Glucose Fasting",                                │
│    targetSystem: "http://loinc.org"                                 │
│  }                                                                   │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Step 1: Persistent Lookup                         │
│                                                                      │
│   ┌──────────────────────────────────────────────────────────────┐  │
│   │ 1a. Check profile terminology.mappings (YAML-embedded)       │  │
│   │ 1b. Check terminology.custom_mappings table (uploaded CSVs)  │  │
│   │ 1c. Check terminology.approved_autoroutes (promoted)         │  │
│   └──────────────────────────────────────────────────────────────┘  │
│                                                                      │
│   Result: PERSISTENT_HIT → return immediately                        │
│           PERSISTENT_MISS → continue to Step 2                       │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                          (if miss) ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Step 2: Autorouting Engine                        │
│                                                                      │
│   ┌────────────────────┐    ┌────────────────────┐                  │
│   │ 2a. Semantic Search│    │ 2b. LLM Ranking    │                  │
│   │                    │    │                    │                  │
│   │ • Embed source     │───▶│ • Evaluate top-K   │                  │
│   │ • Query Qdrant     │    │ • Consider context │                  │
│   │ • Get top-K (5-10) │    │ • Pick best match  │                  │
│   │ • Filter by vocab  │    │ • Explain reasoning│                  │
│   └────────────────────┘    └────────────────────┘                  │
│                                                                      │
│   Result: Candidates + confidence + reasoning                        │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Step 3: Confidence Evaluation                     │
│                                                                      │
│   if confidence >= 0.90:                                            │
│       decision = AUTOROUTE_HIGH_CONF                                │
│       action = AUTO_APPLY (optional, configurable)                  │
│   elif confidence >= 0.70:                                          │
│       decision = AUTOROUTE_MED_CONF                                 │
│       action = SUGGEST (requires review)                            │
│   elif confidence >= 0.50:                                          │
│       decision = AUTOROUTE_LOW_CONF                                 │
│       action = SUGGEST_WITH_WARNING                                 │
│   else:                                                             │
│       decision = NO_MATCH                                           │
│       action = FLAG_FOR_MANUAL                                      │
└─────────────────────────────────────────────────────────────────────┘
                                    │
                                    ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    Step 4: Decision Telemetry                        │
│                                                                      │
│   {                                                                  │
│     "traceId": "mapping-abc123",                                    │
│     "timestamp": "2026-01-27T22:45:00Z",                            │
│     "request": { sourceCode, sourceSystem, targetSystem },          │
│     "decision": "AUTOROUTE_HIGH_CONF",                              │
│     "decisionTree": [                                               │
│       {                                                             │
│         "step": "persistent_lookup",                                │
│         "substeps": [                                               │
│           { "source": "profile_yaml", "result": "miss", "ms": 1 }, │
│           { "source": "custom_mappings", "result": "miss", "ms": 2 }│
│         ],                                                          │
│         "result": "miss",                                           │
│         "durationMs": 3                                             │
│       },                                                            │
│       {                                                             │
│         "step": "semantic_search",                                  │
│         "candidatesFound": 8,                                       │
│         "topScore": 0.89,                                           │
│         "vocabulary": "LOINC",                                      │
│         "durationMs": 45                                            │
│       },                                                            │
│       {                                                             │
│         "step": "llm_ranking",                                      │
│         "model": "gpt-4o-mini",                                     │
│         "inputTokens": 420,                                         │
│         "outputTokens": 85,                                         │
│         "selectedCode": "2345-7",                                   │
│         "confidence": 0.94,                                         │
│         "reasoning": "Exact semantic match for fasting glucose...", │
│         "durationMs": 320                                           │
│       },                                                            │
│       {                                                             │
│         "step": "confidence_check",                                 │
│         "threshold": 0.90,                                          │
│         "actual": 0.94,                                             │
│         "pass": true                                                │
│       }                                                             │
│     ],                                                              │
│     "result": {                                                     │
│       "code": "2345-7",                                             │
│       "display": "Glucose [Mass/volume] in Serum or Plasma",       │
│       "system": "http://loinc.org",                                 │
│       "confidence": 0.94,                                           │
│       "equivalence": "equivalent"                                   │
│     },                                                              │
│     "alternates": [                                                 │
│       { "code": "1558-6", "confidence": 0.71, "reason": "..." },   │
│       { "code": "2339-0", "confidence": 0.65, "reason": "..." }    │
│     ],                                                              │
│     "totalDurationMs": 368                                          │
│   }                                                                  │
└─────────────────────────────────────────────────────────────────────┘
```

## Scalability & Performance

### Time Complexity Analysis

| Operation | Best Case | Typical | Worst Case | Notes |
|-----------|-----------|---------|------------|-------|
| **Persistent lookup** | O(1) | O(1) | O(log n) | B-tree index; n = mappings count |
| **Profile YAML lookup** | O(1) | O(1) | O(m) | m = mappings in profile (small) |
| **Embedding generation** | O(1) | O(1) | O(1) | Fixed model latency ~20-50ms |
| **Semantic search (Qdrant)** | O(log n) | O(log n) | O(n) | ANN with HNSW; n = vocabulary size |
| **LLM ranking** | O(k) | O(k) | O(k) | k = candidates; ~100-400ms per call |
| **Telemetry write** | O(1) | O(1) | O(1) | Async batch insert |
| **Batch resolve (N items)** | O(N) | O(N) | O(N×k) | Parallelized with worker pool |

### Latency Targets

| Scenario | P50 Target | P99 Target | Strategy |
|----------|------------|------------|----------|
| **Persistent hit** | < 2ms | < 10ms | In-memory cache + DB index |
| **Autoroute (cached)** | < 5ms | < 20ms | Result cache with TTL |
| **Autoroute (uncached)** | < 400ms | < 800ms | Parallel embed + search |
| **Batch (1000 items)** | < 2s | < 5s | Worker pool + batch LLM |

### Multi-Tier Caching Strategy

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Request                                      │
└─────────────────────────────────────────────────────────────────────┘
                                │
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    L1: In-Process Cache                              │
│   • Ristretto (LRU, 100K entries, ~50MB)                            │
│   • TTL: 5 minutes                                                   │
│   • Hit rate target: 60-80%                                          │
│   • Key: hash(source_system:source_code:target_system:profile_id)   │
└─────────────────────────────────────────────────────────────────────┘
                                │ miss
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    L2: Redis/Valkey Cache                            │
│   • Shared across instances                                          │
│   • TTL: 1 hour (persistent hits), 15 min (autoroute results)       │
│   • Hit rate target: 20-30% of L1 misses                            │
│   • Stores: mapping result + compressed decision trace              │
└─────────────────────────────────────────────────────────────────────┘
                                │ miss
                                ▼
┌─────────────────────────────────────────────────────────────────────┐
│                    L3: PostgreSQL + Qdrant                           │
│   • Persistent mappings: PostgreSQL with read replicas              │
│   • Semantic search: Qdrant with sharding                           │
│   • Connection pooling: PgBouncer (transaction mode)                │
└─────────────────────────────────────────────────────────────────────┘
```

### Cache Key Design

```go
// Deterministic cache key for consistent hashing
func CacheKey(req MappingRequest) string {
    h := xxhash.New64()
    h.WriteString(req.SourceSystem)
    h.WriteString("\x00")
    h.WriteString(req.SourceCode)
    h.WriteString("\x00")
    h.WriteString(req.TargetSystem)
    h.WriteString("\x00")
    h.WriteString(req.ProfileID) // Empty string if no profile
    return fmt.Sprintf("map:%x", h.Sum64())
}
```

### Batch Processing Optimization

For ETL pipelines processing thousands of mappings:

```
┌─────────────────────────────────────────────────────────────────────┐
│                    Batch Resolve Pipeline                            │
│                                                                      │
│   Input: []MappingRequest (N items)                                 │
│                                                                      │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 1: Deduplicate                                         │   │
│   │ • Group identical requests                                   │   │
│   │ • Reduces N to M unique requests (M ≤ N)                    │   │
│   │ • O(N) time, O(M) space                                     │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 2: Cache Lookup (Parallel)                             │   │
│   │ • Check L1 + L2 for all M requests                         │   │
│   │ • Partition into: cached[], uncached[]                      │   │
│   │ • O(M) time with parallel lookups                          │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 3: Batch Persistent Lookup                             │   │
│   │ • Single query: WHERE (source_system, source_code) IN (...)│   │
│   │ • Returns all persistent hits at once                       │   │
│   │ • O(log n + |uncached|) with index                         │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 4: Batch Embedding Generation                          │   │
│   │ • Group remaining uncached into batches of 100              │   │
│   │ • Single API call per batch (vs N calls)                    │   │
│   │ • 10x-50x reduction in embedding latency                    │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 5: Parallel Semantic Search                            │   │
│   │ • Worker pool (default: 10 workers)                         │   │
│   │ • Each worker handles one Qdrant query                      │   │
│   │ • Bounded concurrency prevents overload                     │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 6: Batch LLM Ranking (Critical Path)                   │   │
│   │ • Group candidates by similarity for batch prompts          │   │
│   │ • Use smaller model (gpt-4o-mini) for throughput            │   │
│   │ • Parallel requests with rate limiting                      │   │
│   │ • Circuit breaker on LLM failures                           │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                              │                                       │
│                              ▼                                       │
│   ┌─────────────────────────────────────────────────────────────┐   │
│   │ Step 7: Async Telemetry Write                               │   │
│   │ • Buffer decisions in memory (1000 items or 5s)             │   │
│   │ • Batch INSERT into partitioned table                       │   │
│   │ • Non-blocking - doesn't affect response latency            │   │
│   └─────────────────────────────────────────────────────────────┘   │
│                                                                      │
│   Output: []MappingResult (N items, same order as input)            │
└─────────────────────────────────────────────────────────────────────┘
```

### LLM Cost Optimization

| Strategy | Impact | Implementation |
|----------|--------|----------------|
| **Result caching** | 60-80% cost reduction | Cache autoroute results for 1hr |
| **Batch prompts** | 30-50% token reduction | Group similar codes in one prompt |
| **Smaller model first** | 70% cost reduction | gpt-4o-mini → gpt-4o fallback |
| **Confidence threshold** | Variable | Skip LLM if semantic score > 0.95 |
| **Pre-filtering** | 50% fewer LLM calls | Only rank if semantic search finds candidates |

```go
// Cost-aware ranking configuration
type RankingConfig struct {
    // Skip LLM if top semantic match exceeds this threshold
    SemanticAutoAcceptThreshold float64 // Default: 0.95

    // Use quality model only for low-confidence results
    UseQualityModelThreshold float64 // Default: 0.75

    // Model selection
    FastModel    string // "gpt-4o-mini"
    QualityModel string // "gpt-4o"

    // Rate limiting
    MaxRequestsPerSecond float64 // Default: 10
    MaxConcurrent        int     // Default: 5
}
```

### Database Scaling

#### Read Replicas for Mapping Lookups

```yaml
# config.yaml
terminology:
  db:
    primary: postgresql://primary:5432/terminology
    replicas:
      - postgresql://replica1:5432/terminology
      - postgresql://replica2:5432/terminology

    # Read routing
    read_from_replica: true
    replica_lag_tolerance: 100ms
```

#### Partitioning Strategy

```sql
-- Partition mapping_decisions by month for efficient retention
-- Auto-create partitions via pg_partman or custom function

-- Retention policy: 90 days default, configurable per tenant
CREATE OR REPLACE FUNCTION terminology.cleanup_old_decisions()
RETURNS void AS $$
BEGIN
    -- Drop partitions older than retention period
    -- This is O(1) - just drops the partition, no row-by-row delete
    EXECUTE format(
        'DROP TABLE IF EXISTS terminology.mapping_decisions_%s',
        to_char(NOW() - INTERVAL '90 days', 'YYYY_MM')
    );
END;
$$ LANGUAGE plpgsql;
```

#### Connection Pooling

```yaml
# PgBouncer config
[databases]
terminology = host=pg-primary port=5432 dbname=terminology

[pgbouncer]
pool_mode = transaction
max_client_conn = 1000
default_pool_size = 20
reserve_pool_size = 5
```

### Qdrant Scaling

#### Sharding for Large Vocabularies

```yaml
# Qdrant collection config
collections:
  loinc_embeddings:
    shard_number: 4          # Distribute across nodes
    replication_factor: 2    # HA
    vectors:
      size: 1536             # text-embedding-3-small
      distance: Cosine
    optimizers:
      indexing_threshold: 20000  # When to build HNSW index
    hnsw:
      m: 16                  # Max connections per node
      ef_construct: 100      # Index build quality
```

#### Query Optimization

```go
// Optimized search with early termination
func (s *Searcher) SearchOptimized(ctx context.Context, embedding []float64, opts SearchOptions) ([]SemanticMatch, error) {
    return s.qdrant.Search(ctx, &qdrant.SearchRequest{
        Vector:      embedding,
        Limit:       opts.TopK,
        ScoreThreshold: 0.5,  // Don't return low-quality matches
        WithPayload: true,
        Params: &qdrant.SearchParams{
            HnswEf: 64,       // Lower ef for faster search (vs 128 default)
            Exact:  false,    // Use ANN, not exact search
        },
    })
}
```

### Throughput Benchmarks

Target performance at scale:

| Workload | Volume | Throughput | Latency P99 |
|----------|--------|------------|-------------|
| **Real-time HL7** | 100 msg/sec | 100 resolve/sec | < 50ms (cached) |
| **Batch ETL** | 100K records | 500 resolve/sec | < 5s total |
| **Bulk upload** | 50K mappings | 1000 insert/sec | < 60s total |
| **Review queue** | 1K pending | 100 approve/sec | < 100ms |

### Graceful Degradation

```go
// Circuit breaker for LLM failures
type AutorouteEngine struct {
    // ...
    circuitBreaker *gobreaker.CircuitBreaker
}

func NewAutorouteEngine(cfg Config) *AutorouteEngine {
    cb := gobreaker.NewCircuitBreaker(gobreaker.Settings{
        Name:        "llm-ranking",
        MaxRequests: 5,                    // Half-open state requests
        Interval:    10 * time.Second,     // Closed state window
        Timeout:     30 * time.Second,     // Open state duration
        ReadyToTrip: func(counts gobreaker.Counts) bool {
            return counts.ConsecutiveFailures > 3
        },
    })
    // ...
}

// Fallback behavior when LLM is unavailable
func (e *Engine) SuggestWithFallback(ctx context.Context, req SuggestRequest) (*SuggestResult, error) {
    result, err := e.circuitBreaker.Execute(func() (interface{}, error) {
        return e.Suggest(ctx, req)
    })

    if err != nil {
        // Fallback: return semantic search results without LLM ranking
        return e.semanticOnlyFallback(ctx, req)
    }
    return result.(*SuggestResult), nil
}
```

### Capacity Planning

| Component | Small (< 1M mappings) | Medium (1-10M) | Large (> 10M) |
|-----------|----------------------|----------------|---------------|
| **PostgreSQL** | Single node, 8GB | Primary + replica, 32GB | Citus sharding |
| **Qdrant** | Single node, 16GB | 3-node cluster | Sharded cluster |
| **Redis** | Single node, 4GB | Sentinel HA, 8GB | Cluster mode |
| **LLM API** | 10 req/s | 50 req/s | 100+ req/s (dedicated) |
| **Workers** | 2 pods | 5 pods | 10+ pods, HPA |

## Database Schema

### New Tables

```sql
-- ============================================================================
-- Custom Uploaded Mappings
-- ============================================================================
CREATE TABLE terminology.custom_mappings (
    id              BIGSERIAL PRIMARY KEY,

    -- Source identification
    source_system   VARCHAR(100) NOT NULL,
    source_code     VARCHAR(100) NOT NULL,
    source_display  TEXT,

    -- Target mapping
    target_system   VARCHAR(255) NOT NULL,
    target_code     VARCHAR(100) NOT NULL,
    target_display  TEXT,

    -- Mapping metadata
    equivalence     VARCHAR(20) DEFAULT 'equivalent',  -- equivalent, wider, narrower, inexact
    confidence      FLOAT,                              -- NULL for manual uploads
    comment         TEXT,

    -- Provenance
    origin          VARCHAR(30) NOT NULL,               -- 'csv_upload', 'approved_autoroute', 'manual'
    upload_batch_id UUID,                               -- Links to upload_batches table
    profile_id      VARCHAR(100),                       -- Optional: profile-scoped mapping

    -- Audit
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    created_by      VARCHAR(100),
    approved_at     TIMESTAMPTZ,
    approved_by     VARCHAR(100),

    -- For approved autoroutes: full decision context
    decision_trace  JSONB,

    UNIQUE(source_system, source_code, target_system, COALESCE(profile_id, ''))
);

CREATE INDEX idx_custom_mappings_lookup
    ON terminology.custom_mappings(source_system, source_code, target_system);
CREATE INDEX idx_custom_mappings_profile
    ON terminology.custom_mappings(profile_id) WHERE profile_id IS NOT NULL;
CREATE INDEX idx_custom_mappings_batch
    ON terminology.custom_mappings(upload_batch_id);

-- ============================================================================
-- Upload Batch Tracking
-- ============================================================================
CREATE TABLE terminology.upload_batches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename        VARCHAR(255) NOT NULL,
    source_system   VARCHAR(100),                       -- Default source system for batch
    target_system   VARCHAR(255),                       -- Default target system for batch
    profile_id      VARCHAR(100),                       -- Optional: scope to profile

    -- Stats
    total_rows      INT NOT NULL,
    valid_rows      INT NOT NULL,
    duplicate_rows  INT DEFAULT 0,
    error_rows      INT DEFAULT 0,

    -- Audit
    uploaded_at     TIMESTAMPTZ DEFAULT NOW(),
    uploaded_by     VARCHAR(100),

    -- Validation results
    validation_errors JSONB                             -- Array of {row, column, error}
);

-- ============================================================================
-- Pending Autoroute Suggestions (for review workflow)
-- ============================================================================
CREATE TABLE terminology.pending_autoroutes (
    id              BIGSERIAL PRIMARY KEY,

    -- Request context
    source_system   VARCHAR(100) NOT NULL,
    source_code     VARCHAR(100) NOT NULL,
    source_display  TEXT,
    target_system   VARCHAR(255) NOT NULL,

    -- Suggestion
    suggested_code  VARCHAR(100) NOT NULL,
    suggested_display TEXT,
    confidence      FLOAT NOT NULL,
    equivalence     VARCHAR(20),
    reasoning       TEXT,

    -- Full decision tree for auditability
    decision_trace  JSONB NOT NULL,

    -- Alternatives considered
    alternates      JSONB,                              -- Array of {code, confidence, reason}

    -- Workflow state
    status          VARCHAR(20) DEFAULT 'pending',      -- pending, approved, rejected, expired
    reviewed_at     TIMESTAMPTZ,
    reviewed_by     VARCHAR(100),
    rejection_reason TEXT,

    -- Timing
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    expires_at      TIMESTAMPTZ,                        -- Auto-expire old suggestions

    UNIQUE(source_system, source_code, target_system, suggested_code)
);

CREATE INDEX idx_pending_autoroutes_status
    ON terminology.pending_autoroutes(status) WHERE status = 'pending';
CREATE INDEX idx_pending_autoroutes_confidence
    ON terminology.pending_autoroutes(confidence DESC);

-- ============================================================================
-- Decision Audit Log (telemetry persistence)
-- ============================================================================
CREATE TABLE terminology.mapping_decisions (
    id              BIGSERIAL PRIMARY KEY,
    trace_id        VARCHAR(64) NOT NULL,               -- OpenTelemetry trace ID

    -- Request
    source_system   VARCHAR(100),
    source_code     VARCHAR(100),
    source_display  TEXT,
    target_system   VARCHAR(255),

    -- Decision
    decision_type   VARCHAR(30) NOT NULL,               -- PERSISTENT_HIT, AUTOROUTE_*, NO_MATCH
    confidence      FLOAT,

    -- Result
    selected_code   VARCHAR(100),
    selected_display TEXT,

    -- Full decision tree
    decision_tree   JSONB NOT NULL,

    -- Context
    profile_id      VARCHAR(100),
    request_source  VARCHAR(50),                        -- 'graphql', 'cli', 'workflow', 'batch'

    -- Timing
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    duration_ms     INT,

    -- Partition by month for efficient retention
    CONSTRAINT mapping_decisions_created_at_check CHECK (created_at IS NOT NULL)
) PARTITION BY RANGE (created_at);

-- Create monthly partitions (example for 2026)
CREATE TABLE terminology.mapping_decisions_2026_01
    PARTITION OF terminology.mapping_decisions
    FOR VALUES FROM ('2026-01-01') TO ('2026-02-01');
CREATE TABLE terminology.mapping_decisions_2026_02
    PARTITION OF terminology.mapping_decisions
    FOR VALUES FROM ('2026-02-01') TO ('2026-03-01');
-- ... etc

CREATE INDEX idx_mapping_decisions_trace
    ON terminology.mapping_decisions(trace_id);
CREATE INDEX idx_mapping_decisions_source
    ON terminology.mapping_decisions(source_system, source_code);
```

## GraphQL Schema

### Types

```graphql
# ============================================================================
# Core Types
# ============================================================================

enum MappingDecisionType {
  PERSISTENT_HIT
  AUTOROUTE_HIGH_CONF
  AUTOROUTE_MED_CONF
  AUTOROUTE_LOW_CONF
  NO_MATCH
  MANUAL_REQUIRED
}

enum MappingEquivalence {
  EQUIVALENT
  WIDER
  NARROWER
  INEXACT
  UNMATCHED
}

enum MappingOrigin {
  CSV_UPLOAD
  APPROVED_AUTOROUTE
  MANUAL
  PROFILE_YAML
}

enum PendingStatus {
  PENDING
  APPROVED
  REJECTED
  EXPIRED
}

# ============================================================================
# Mapping Results
# ============================================================================

type CodeMapping {
  id: ID!
  sourceSystem: String!
  sourceCode: String!
  sourceDisplay: String
  targetSystem: String!
  targetCode: String!
  targetDisplay: String
  equivalence: MappingEquivalence!
  confidence: Float
  origin: MappingOrigin!
  profileId: String
  createdAt: DateTime!
  createdBy: String
  approvedAt: DateTime
  approvedBy: String
}

type MappingCandidate {
  code: String!
  display: String!
  system: String!
  confidence: Float!
  equivalence: MappingEquivalence!
  reasoning: String
}

type MappingResult {
  found: Boolean!
  decision: MappingDecisionType!
  mapping: CodeMapping
  candidates: [MappingCandidate!]!
  decisionTrace: MappingDecisionTrace!
}

# ============================================================================
# Decision Telemetry
# ============================================================================

type DecisionStep {
  step: String!
  result: String!
  durationMs: Int!
  metadata: JSON
}

type MappingDecisionTrace {
  traceId: String!
  steps: [DecisionStep!]!
  totalDurationMs: Int!
}

# ============================================================================
# Upload Types
# ============================================================================

type UploadValidationError {
  row: Int!
  column: String
  message: String!
}

type UploadBatch {
  id: ID!
  filename: String!
  sourceSystem: String
  targetSystem: String
  profileId: String
  totalRows: Int!
  validRows: Int!
  duplicateRows: Int!
  errorRows: Int!
  validationErrors: [UploadValidationError!]!
  uploadedAt: DateTime!
  uploadedBy: String
}

type UploadMappingResult {
  batch: UploadBatch!
  mappingsCreated: Int!
  mappingsUpdated: Int!
  preview: [CodeMapping!]!
}

# ============================================================================
# Pending Autoroutes
# ============================================================================

type PendingAutoroute {
  id: ID!
  sourceSystem: String!
  sourceCode: String!
  sourceDisplay: String
  targetSystem: String!
  suggestedCode: String!
  suggestedDisplay: String
  confidence: Float!
  equivalence: MappingEquivalence
  reasoning: String
  decisionTrace: MappingDecisionTrace!
  alternates: [MappingCandidate!]!
  status: PendingStatus!
  createdAt: DateTime!
  reviewedAt: DateTime
  reviewedBy: String
  rejectionReason: String
}

type PendingAutorouteConnection {
  edges: [PendingAutorouteEdge!]!
  pageInfo: PageInfo!
  totalCount: Int!
}

type PendingAutorouteEdge {
  cursor: String!
  node: PendingAutoroute!
}

# ============================================================================
# Input Types
# ============================================================================

input ResolveMappingInput {
  sourceCode: String!
  sourceSystem: String!
  sourceDisplay: String
  targetSystem: String!
  profileId: String
  # Options
  allowAutoroute: Boolean = true
  minConfidence: Float = 0.7
}

input SuggestMappingsInput {
  sourceCode: String!
  sourceSystem: String!
  sourceDisplay: String
  targetSystem: String!
  maxCandidates: Int = 5
}

input UploadMappingInput {
  # CSV content (base64 encoded or raw string)
  csv: String!
  filename: String!
  # Defaults for columns if not in CSV
  defaultSourceSystem: String
  defaultTargetSystem: String
  profileId: String
  # Options
  dryRun: Boolean = false
  updateExisting: Boolean = false
}

input ApproveMappingInput {
  pendingId: ID!
  # Optional overrides
  equivalence: MappingEquivalence
  comment: String
}

input RejectMappingInput {
  pendingId: ID!
  reason: String!
}

input MappingFilterInput {
  sourceSystem: String
  targetSystem: String
  profileId: String
  origin: MappingOrigin
  search: String
}

# ============================================================================
# Queries
# ============================================================================

extend type Query {
  # Resolve a single mapping (persistent or autoroute)
  resolveMapping(input: ResolveMappingInput!): MappingResult!

  # Batch resolve for ETL pipelines
  resolveMappingsBatch(inputs: [ResolveMappingInput!]!): [MappingResult!]!

  # Get autoroute suggestions only (for discovery/review)
  suggestMappings(input: SuggestMappingsInput!): [MappingCandidate!]!

  # Browse persistent mappings
  mappings(
    filter: MappingFilterInput
    first: Int = 50
    after: String
  ): CodeMappingConnection!

  # Get a specific mapping
  mapping(id: ID!): CodeMapping

  # List upload batches
  uploadBatches(first: Int = 20, after: String): UploadBatchConnection!

  # Get upload batch details
  uploadBatch(id: ID!): UploadBatch

  # List pending autoroutes for review
  pendingAutoroutes(
    status: PendingStatus = PENDING
    minConfidence: Float
    first: Int = 50
    after: String
  ): PendingAutorouteConnection!

  # Get pending autoroute details
  pendingAutoroute(id: ID!): PendingAutoroute

  # Mapping statistics
  mappingStats: MappingStats!
}

type MappingStats {
  totalMappings: Int!
  byOrigin: [OriginCount!]!
  bySourceSystem: [SystemCount!]!
  byTargetSystem: [SystemCount!]!
  pendingReviewCount: Int!
  recentDecisions: DecisionSummary!
}

type OriginCount {
  origin: MappingOrigin!
  count: Int!
}

type SystemCount {
  system: String!
  count: Int!
}

type DecisionSummary {
  period: String!
  total: Int!
  persistentHits: Int!
  autorouteHighConf: Int!
  autorouteMedConf: Int!
  autorouteLowConf: Int!
  noMatch: Int!
  avgConfidence: Float
  avgDurationMs: Int
}

# ============================================================================
# Mutations
# ============================================================================

extend type Mutation {
  # Upload CSV mappings
  uploadMappingCSV(input: UploadMappingInput!): UploadMappingResult!

  # Create a single mapping manually
  createMapping(input: CreateMappingInput!): CodeMapping!

  # Update an existing mapping
  updateMapping(id: ID!, input: UpdateMappingInput!): CodeMapping!

  # Delete a mapping
  deleteMapping(id: ID!): Boolean!

  # Approve a pending autoroute (promotes to persistent)
  approvePendingAutoroute(input: ApproveMappingInput!): CodeMapping!

  # Reject a pending autoroute
  rejectPendingAutoroute(input: RejectMappingInput!): Boolean!

  # Bulk approve high-confidence autoroutes
  bulkApprovePendingAutoroutes(
    minConfidence: Float = 0.95
    maxCount: Int = 100
  ): BulkApproveResult!
}

input CreateMappingInput {
  sourceSystem: String!
  sourceCode: String!
  sourceDisplay: String
  targetSystem: String!
  targetCode: String!
  targetDisplay: String
  equivalence: MappingEquivalence = EQUIVALENT
  profileId: String
  comment: String
}

input UpdateMappingInput {
  targetCode: String
  targetDisplay: String
  equivalence: MappingEquivalence
  comment: String
}

type BulkApproveResult {
  approved: Int!
  skipped: Int!
  mappings: [CodeMapping!]!
}
```

## Go Implementation

### Package Structure

```
internal/terminology/
├── mapping/
│   ├── service.go          # MappingService - main orchestrator
│   ├── persistent.go       # PersistentStore - DB lookups
│   ├── telemetry.go        # DecisionRecorder - OpenTelemetry
│   └── types.go            # Shared types
├── autoroute/
│   ├── engine.go           # AutorouteEngine - orchestrates autorouting
│   ├── ranker.go           # LLMRanker - LLM-based ranking
│   ├── cache.go            # AutorouteCache - result caching
│   └── prompts.go          # LLM prompt templates
└── upload/
    ├── parser.go           # CSV parser with validation
    ├── validator.go        # Row validation rules
    └── importer.go         # Batch import logic
```

### Core Interfaces

```go
// internal/terminology/mapping/service.go

package mapping

import (
    "context"
    "time"
)

// MappingService orchestrates persistent and autorouted mapping resolution.
type MappingService struct {
    persistent *PersistentStore
    autorouter *autoroute.Engine
    telemetry  *DecisionRecorder
    config     Config
}

type Config struct {
    // Autorouting thresholds
    HighConfidenceThreshold float64 // Default: 0.90
    MedConfidenceThreshold  float64 // Default: 0.70

    // Behavior
    AutoApproveHighConfidence bool   // Auto-promote high-conf to persistent
    CacheTTL                  time.Duration

    // Telemetry
    RecordAllDecisions bool // Record to mapping_decisions table
}

// MappingRequest represents a request to resolve a code mapping.
type MappingRequest struct {
    SourceCode    string
    SourceSystem  string
    SourceDisplay string
    TargetSystem  string
    ProfileID     string // Optional: scope to profile

    // Options
    AllowAutoroute bool
    MinConfidence  float64
}

// MappingResult contains the resolution result and full decision trace.
type MappingResult struct {
    Found      bool
    Decision   DecisionType
    Mapping    *CodeMapping
    Candidates []MappingCandidate
    Trace      *DecisionTrace
}

// Resolve finds a mapping with full decision tracing.
func (s *MappingService) Resolve(ctx context.Context, req MappingRequest) (*MappingResult, error) {
    trace := s.telemetry.StartTrace(ctx, req)
    defer trace.End()

    // Step 1: Persistent lookup
    if mapping, err := s.persistent.Lookup(ctx, req); err == nil && mapping != nil {
        trace.RecordStep("persistent_lookup", "hit", nil)
        return &MappingResult{
            Found:    true,
            Decision: DecisionPersistentHit,
            Mapping:  mapping,
            Trace:    trace.Finalize(),
        }, nil
    }
    trace.RecordStep("persistent_lookup", "miss", nil)

    // Step 2: Autorouting (if enabled)
    if !req.AllowAutoroute {
        return &MappingResult{
            Found:    false,
            Decision: DecisionNoMatch,
            Trace:    trace.Finalize(),
        }, nil
    }

    suggestion, err := s.autorouter.Suggest(ctx, autoroute.SuggestRequest{
        SourceCode:    req.SourceCode,
        SourceSystem:  req.SourceSystem,
        SourceDisplay: req.SourceDisplay,
        TargetSystem:  req.TargetSystem,
    })
    if err != nil {
        trace.RecordStep("autoroute", "error", map[string]interface{}{"error": err.Error()})
        return nil, err
    }

    trace.RecordStep("semantic_search", "complete", map[string]interface{}{
        "candidates": len(suggestion.Candidates),
        "topScore":   suggestion.Candidates[0].Confidence,
    })
    trace.RecordStep("llm_ranking", "complete", map[string]interface{}{
        "model":      suggestion.Model,
        "confidence": suggestion.Confidence,
        "reasoning":  suggestion.Reasoning,
    })

    // Step 3: Evaluate confidence
    decision := s.evaluateConfidence(suggestion.Confidence, req.MinConfidence)
    trace.RecordStep("confidence_check", decision.String(), map[string]interface{}{
        "confidence": suggestion.Confidence,
        "threshold":  req.MinConfidence,
    })

    result := &MappingResult{
        Found:      decision != DecisionNoMatch,
        Decision:   decision,
        Candidates: suggestion.Candidates,
        Trace:      trace.Finalize(),
    }

    if len(suggestion.Candidates) > 0 {
        result.Mapping = suggestion.Candidates[0].ToCodeMapping()
    }

    // Step 4: Record telemetry
    if s.config.RecordAllDecisions {
        s.telemetry.Record(ctx, result)
    }

    // Auto-approve high confidence if configured
    if s.config.AutoApproveHighConfidence && decision == DecisionAutorouteHighConf {
        s.persistent.PromoteAutoroute(ctx, result.Mapping, trace)
    }

    return result, nil
}
```

### Autoroute Engine

```go
// internal/terminology/autoroute/engine.go

package autoroute

import (
    "context"

    "fi-fhir/pkg/llm"
    "fi-fhir/pkg/terminology/semantic"
)

// Engine performs LLM-powered autorouting with semantic search.
type Engine struct {
    searcher *semantic.Searcher
    ranker   *LLMRanker
    cache    *Cache
}

type SuggestRequest struct {
    SourceCode    string
    SourceSystem  string
    SourceDisplay string
    TargetSystem  string
    MaxCandidates int
}

type SuggestResult struct {
    Candidates  []MappingCandidate
    Reasoning   string
    Confidence  float64
    Model       string
    SearchSteps []SearchStep
    RankingStep *RankingStep
}

func (e *Engine) Suggest(ctx context.Context, req SuggestRequest) (*SuggestResult, error) {
    // Check cache first
    if cached := e.cache.Get(req); cached != nil {
        return cached, nil
    }

    // Step 1: Semantic search
    searchResults, err := e.searcher.Search(ctx, buildSearchQuery(req), semantic.SearchOptions{
        Vocabularies: []semantic.Vocabulary{vocabularyFromSystem(req.TargetSystem)},
        TopK:         max(req.MaxCandidates*2, 10), // Get more for ranking
    })
    if err != nil {
        return nil, fmt.Errorf("semantic search failed: %w", err)
    }

    if len(searchResults) == 0 {
        return &SuggestResult{Candidates: nil, Confidence: 0}, nil
    }

    // Step 2: LLM ranking
    ranked, err := e.ranker.Rank(ctx, RankRequest{
        SourceCode:    req.SourceCode,
        SourceDisplay: req.SourceDisplay,
        SourceSystem:  req.SourceSystem,
        Candidates:    searchResults,
        MaxResults:    req.MaxCandidates,
    })
    if err != nil {
        return nil, fmt.Errorf("LLM ranking failed: %w", err)
    }

    result := &SuggestResult{
        Candidates:  ranked.Candidates,
        Reasoning:   ranked.Reasoning,
        Confidence:  ranked.TopConfidence,
        Model:       ranked.Model,
    }

    // Cache result
    e.cache.Set(req, result)

    return result, nil
}

func buildSearchQuery(req SuggestRequest) string {
    // Combine code and display for richer semantic matching
    if req.SourceDisplay != "" {
        return fmt.Sprintf("%s %s", req.SourceCode, req.SourceDisplay)
    }
    return req.SourceCode
}
```

### LLM Ranker

```go
// internal/terminology/autoroute/ranker.go

package autoroute

import (
    "context"
    "encoding/json"

    "fi-fhir/pkg/llm"
)

type LLMRanker struct {
    client llm.Client
    model  string
}

type RankRequest struct {
    SourceCode    string
    SourceDisplay string
    SourceSystem  string
    Candidates    []semantic.SemanticMatch
    MaxResults    int
}

type RankResult struct {
    Candidates    []MappingCandidate
    Reasoning     string
    TopConfidence float64
    Model         string
}

// rankingOutput is the structured JSON schema for LLM output
type rankingOutput struct {
    BestMatch struct {
        Code        string  `json:"code"`
        Confidence  float64 `json:"confidence"`
        Equivalence string  `json:"equivalence"`
        Reasoning   string  `json:"reasoning"`
    } `json:"best_match"`
    Alternates []struct {
        Code        string  `json:"code"`
        Confidence  float64 `json:"confidence"`
        Reasoning   string  `json:"reasoning"`
    } `json:"alternates"`
    OverallReasoning string `json:"overall_reasoning"`
}

func (r *LLMRanker) Rank(ctx context.Context, req RankRequest) (*RankResult, error) {
    prompt := buildRankingPrompt(req)

    resp, err := r.client.CompleteStructured(ctx, llm.CompletionRequest{
        Model:       r.model,
        Temperature: 0.1, // Low temp for deterministic ranking
        Messages: []llm.Message{
            llm.SystemMessage(rankingSystemPrompt),
            llm.UserMessage(prompt),
        },
    }, "terminology_ranking", rankingOutputSchema)
    if err != nil {
        return nil, err
    }

    var output rankingOutput
    if err := json.Unmarshal(resp, &output); err != nil {
        return nil, fmt.Errorf("failed to parse ranking output: %w", err)
    }

    candidates := make([]MappingCandidate, 0, len(output.Alternates)+1)

    // Add best match
    candidates = append(candidates, MappingCandidate{
        Code:        output.BestMatch.Code,
        Confidence:  output.BestMatch.Confidence,
        Equivalence: parseEquivalence(output.BestMatch.Equivalence),
        Reasoning:   output.BestMatch.Reasoning,
    })

    // Add alternates
    for _, alt := range output.Alternates {
        if len(candidates) >= req.MaxResults {
            break
        }
        candidates = append(candidates, MappingCandidate{
            Code:       alt.Code,
            Confidence: alt.Confidence,
            Reasoning:  alt.Reasoning,
        })
    }

    return &RankResult{
        Candidates:    candidates,
        Reasoning:     output.OverallReasoning,
        TopConfidence: output.BestMatch.Confidence,
        Model:         r.model,
    }, nil
}

const rankingSystemPrompt = `You are a healthcare terminology expert specializing in code mapping.

Your task is to evaluate candidate mappings between a source code and target vocabulary codes.

Consider:
1. Semantic equivalence - does the meaning match?
2. Specificity - is the match too broad or too narrow?
3. Clinical context - would this mapping be appropriate in clinical workflows?
4. Standard practices - is this a commonly accepted mapping?

Provide confidence scores from 0.0 to 1.0 where:
- 0.95-1.0: Exact semantic match, high certainty
- 0.85-0.94: Strong match, minor differences in specificity
- 0.70-0.84: Good match, some nuance differences
- 0.50-0.69: Partial match, may need review
- Below 0.50: Weak match, likely incorrect`
```

## Frontend Implementation

### Component Structure

```text
ui/src/lib/features/terminology/
├── MappingUploader.svelte        # CSV upload workflow
├── MappingBrowser.svelte         # Search/filter/edit mappings
├── PendingReviewList.svelte      # Review + approve/reject/bulk approve
├── AutorouteResolver.svelte      # Resolve/suggest mapping exploration
├── MappingEditor.svelte          # Manual mapping edits
└── terminologyApi.ts             # GraphQL client helpers for mapping/review flows
```

### Upload Flow

```svelte
<!-- MappingUploader.svelte -->
<script lang="ts">
  import { graphqlFetch } from '$lib/graphql/client';
  import { UploadMappingCsvDocument } from '$lib/gen/graphql';

  let file: File | null = null;
  let preview: UploadMappingResult | null = null;
  let uploading = false;

  async function handleUpload() {
    if (!file) return;
    uploading = true;

    const csv = await file.text();

    try {
      // First do a dry run
      const dryRunResult = await graphqlFetch(UploadMappingCsvDocument, {
        input: {
          csv,
          filename: file.name,
          defaultSourceSystem: sourceSystem,
          defaultTargetSystem: targetSystem,
          dryRun: true
        }
      });

      preview = dryRunResult.uploadMappingCSV;
    } finally {
      uploading = false;
    }
  }

  async function confirmUpload() {
    // ... actual upload
  }
</script>
```

## CLI Commands

```bash
# Upload mappings from CSV
fi-fhir terminology mapping upload ./mappings.csv \
  --source-system epic_labs \
  --target-system http://loinc.org \
  --profile epic_adt \
  --dry-run

# Resolve a single mapping
fi-fhir terminology mapping resolve LAB001 \
  --source-system epic_labs \
  --target-system http://loinc.org \
  --allow-autoroute

# List pending autoroutes
fi-fhir terminology mapping pending \
  --min-confidence 0.8 \
  --format table

# Approve pending autoroutes
fi-fhir terminology mapping approve <id>
fi-fhir terminology mapping bulk-approve --min-confidence 0.95

# Export mappings
fi-fhir terminology mapping export \
  --source-system epic_labs \
  --format csv \
  > epic_labs_mappings.csv

# Show mapping statistics
fi-fhir terminology mapping stats
```

## Configuration

### Server Configuration

```yaml
# config.yaml
terminology:
  db_url: ${secret:TERMINOLOGY_DATABASE_URL}

  mapping:
    # Autorouting configuration
    autoroute:
      enabled: true
      high_confidence_threshold: 0.90
      med_confidence_threshold: 0.70
      auto_approve_high_confidence: false  # Require manual approval
      cache_ttl: 1h

      # LLM settings
      llm:
        model: gpt-4o-mini
        temperature: 0.1
        max_candidates: 5

      # Semantic search settings
      semantic:
        qdrant_url: ${secret:QDRANT_URL}
        embedding_model: text-embedding-3-small
        top_k: 10

    # Telemetry
    telemetry:
      record_all_decisions: true
      retention_days: 90

    # Review workflow
    review:
      auto_expire_days: 30
      notification_webhook: ${secret:SLACK_WEBHOOK}
```

### Environment Variables

```bash
FI_FHIR_TERMINOLOGY_DB_URL=postgresql://...
FI_FHIR_MAPPING_AUTOROUTE_ENABLED=true
FI_FHIR_MAPPING_HIGH_CONF_THRESHOLD=0.90
FI_FHIR_MAPPING_AUTO_APPROVE=false
FI_FHIR_MAPPING_LLM_MODEL=gpt-4o-mini
FI_FHIR_MAPPING_QDRANT_URL=http://qdrant:6333
```

## Implementation Phases

### Phase 1: CSV Upload + Persistent Storage (2-3 days) ✅
- [x] Database schema migrations - see `pkg/terminology/db/migrations.go`, `pkg/terminology/db/schema.go`
- [x] `upload/parser.go` - CSV parsing with validation
- [x] Persistent store CRUD - see `pkg/terminology/db/mappings.go` (`MappingStore`)
- [x] GraphQL mutations: `uploadMappingCSV`, `createMapping`, `deleteMapping`
- [x] Basic UI: `MappingUploader.svelte`, `MappingBrowser.svelte`
- [ ] CLI: `fi-fhir terminology mapping upload`

### Phase 2: Autoroute Engine (3-4 days) ✅
- [x] `autoroute/engine.go` - Core orchestration
- [x] `autoroute/ranker.go` - LLM ranking with prompts
- [x] `autoroute/cache.go` - Result caching
- [x] Resolution flow via GraphQL resolver + mapping store fallback (`internal/api/graphql/resolvers/schema.resolvers.go`)
- [x] GraphQL queries: `resolveMapping`, `suggestMappings`
- [ ] CLI: `fi-fhir terminology mapping resolve`

### Phase 3: Decision Telemetry (1-2 days)
- [x] Decision recording path via workflow activities + persistent store (`internal/terminology/workflow/activities.go`, `pkg/terminology/db/mappings.go`)
- [ ] OpenTelemetry span attributes
- [ ] `mapping_decisions` table with partitioning
- [x] GraphQL decision trace included in mapping results (`ResolveMappingResult.trace`, `PendingAutoroute.decisionTrace`)

### Phase 4: Review Workflow + UI (3-4 days) ✅
- [x] `pending_autoroutes` table and logic
- [x] GraphQL: `listPendingAutoroutes`, `approvePendingAutoroute`, `rejectPendingAutoroute` (+ bulk approve)
- [x] UI: `PendingReviewList.svelte` with bulk actions
- [x] Decision trace is reviewable in pending review UI (expandable trace payload)
- [ ] CLI: `fi-fhir terminology mapping pending`, `approve`, `reject`

### Phase 5: Analytics + Polish (2-3 days)
- [ ] `MappingStats` query with aggregations (current: `pendingAutorouteStats`)
- [ ] UI: `MappingStats.svelte` dashboard
- [ ] Notification webhooks for new pending items
- [ ] Performance optimization and load testing
- [ ] Documentation and examples

## Testing Strategy

### Unit Tests
- CSV parser edge cases (encoding, missing columns, invalid values)
- Confidence threshold logic
- Decision type evaluation
- Cache hit/miss scenarios

### Integration Tests
- Full resolution flow with mocked LLM
- Database operations (CRUD, batch operations)
- GraphQL resolver coverage

### E2E Tests
- Upload CSV via UI
- Review and approve autoroutes
- Verify mappings applied in parsing

## Metrics & Monitoring

### Key Metrics
```
# Counter: mapping decisions by type
terminology_mapping_decisions_total{decision="PERSISTENT_HIT|AUTOROUTE_*|NO_MATCH"}

# Histogram: resolution latency
terminology_mapping_resolution_duration_seconds{decision}

# Gauge: pending autoroutes
terminology_mapping_pending_autoroutes{status="pending|approved|rejected"}

# Counter: uploads
terminology_mapping_uploads_total{status="success|error"}
```

### Alerts
- High `NO_MATCH` rate (> 20% of requests)
- Autoroute latency > 2s p95
- Pending queue > 1000 items
- LLM error rate > 5%

## See Also

- [TERMINOLOGY.md](TERMINOLOGY.md) - Code system basics and existing mapper
- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Profile-embedded terminology mappings
- [GRAPHQL-API.md](GRAPHQL-API.md) - API patterns and conventions
