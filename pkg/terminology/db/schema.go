// Package db provides PostgreSQL-backed terminology storage for healthcare code systems.
// This replaces file + memory + API approaches with local database storage for faster
// lookups, cross-walk queries, and SNOMED hierarchy traversal.
package db

// SchemaVersion tracks the current schema version for migrations.
const SchemaVersion = 2

// Schema contains all SQL DDL statements for the terminology database.
// Tables are organized in the 'terminology' schema to isolate from event sourcing tables.
const Schema = `
-- =============================================================================
-- TERMINOLOGY DATABASE SCHEMA v1
-- =============================================================================
-- Supports: UMLS Metathesaurus, RxNorm, SNOMED CT, LOINC, ICD-10-CM/PCS
-- Estimated size: ~83 GB for full UMLS + all vocabularies

CREATE SCHEMA IF NOT EXISTS terminology;

-- =============================================================================
-- RELEASE TRACKING
-- =============================================================================

-- Tracks loaded terminology versions and their status
CREATE TABLE IF NOT EXISTS terminology.releases (
    id              SERIAL PRIMARY KEY,
    vocabulary      VARCHAR(50) NOT NULL,         -- UMLS, RXNORM, SNOMEDCT_US, LOINC, ICD10CM, ICD10PCS
    version         VARCHAR(50) NOT NULL,         -- 2024AB, 2024-01, 20240301, 2.77
    release_date    DATE,
    loaded_at       TIMESTAMPTZ DEFAULT NOW(),
    is_active       BOOLEAN DEFAULT TRUE,         -- Active version for queries
    row_count       BIGINT DEFAULT 0,             -- Stats for monitoring
    source_files    JSONB DEFAULT '[]',           -- List of loaded files
    metadata        JSONB DEFAULT '{}',           -- Additional release info
    UNIQUE (vocabulary, version)
);

CREATE INDEX IF NOT EXISTS idx_releases_active ON terminology.releases (vocabulary) WHERE is_active = TRUE;

-- =============================================================================
-- UMLS METATHESAURUS
-- =============================================================================

-- MRCONSO: Concepts and their string representations (atoms)
-- This is the core table mapping source codes to UMLS concepts (CUIs)
CREATE TABLE IF NOT EXISTS terminology.umls_concepts (
    id              BIGSERIAL PRIMARY KEY,
    cui             CHAR(8) NOT NULL,             -- Concept Unique Identifier (C0000001)
    lat             CHAR(3) NOT NULL DEFAULT 'ENG', -- Language
    ts              CHAR(1) NOT NULL,             -- Term Status (P=Preferred, S=Synonym)
    lui             VARCHAR(10) NOT NULL,         -- Lexical Unique Identifier
    stt             VARCHAR(3) NOT NULL,          -- String Type
    sui             VARCHAR(10) NOT NULL,         -- String Unique Identifier
    ispref          CHAR(1) NOT NULL,             -- Is preferred in source
    aui             VARCHAR(12) NOT NULL,         -- Atom Unique Identifier (primary key in UMLS)
    saui            VARCHAR(50),                  -- Source Atom Unique Identifier
    scui            VARCHAR(100),                 -- Source Concept Unique Identifier
    sdui            VARCHAR(100),                 -- Source Descriptor Unique Identifier
    sab             VARCHAR(40) NOT NULL,         -- Source Abbreviation (SNOMEDCT_US, ICD10CM, etc.)
    tty             VARCHAR(20) NOT NULL,         -- Term Type (PT, SY, FN, etc.)
    code            VARCHAR(100) NOT NULL,        -- Source code (the actual code from source vocabulary)
    str             TEXT NOT NULL,                -- String (the term/name)
    srl             SMALLINT NOT NULL DEFAULT 0,  -- Source Restriction Level
    suppress        CHAR(1) NOT NULL DEFAULT 'N', -- Suppressible flag
    cvf             INTEGER,                      -- Content View Flag
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

-- Primary lookup: code within a vocabulary
CREATE INDEX IF NOT EXISTS idx_umls_concepts_sab_code ON terminology.umls_concepts (sab, code);
-- Lookup by CUI (for cross-walks)
CREATE INDEX IF NOT EXISTS idx_umls_concepts_cui ON terminology.umls_concepts (cui);
-- AUI uniqueness per release
CREATE UNIQUE INDEX IF NOT EXISTS idx_umls_concepts_aui ON terminology.umls_concepts (aui, release_id);
-- Preferred terms only
CREATE INDEX IF NOT EXISTS idx_umls_concepts_preferred ON terminology.umls_concepts (sab, code)
    WHERE ts = 'P' AND ispref = 'Y' AND suppress = 'N';
-- Filter by source vocabulary
CREATE INDEX IF NOT EXISTS idx_umls_concepts_sab ON terminology.umls_concepts (sab);
-- Active/non-suppressed concepts
CREATE INDEX IF NOT EXISTS idx_umls_concepts_active ON terminology.umls_concepts (cui, sab) WHERE suppress = 'N';

-- MRREL: Relationships between concepts
CREATE TABLE IF NOT EXISTS terminology.umls_relations (
    id              BIGSERIAL PRIMARY KEY,
    cui1            CHAR(8) NOT NULL,             -- Source CUI
    aui1            VARCHAR(12),                  -- Source AUI
    stype1          VARCHAR(50) NOT NULL,         -- Source type (CUI, AUI, etc.)
    rel             VARCHAR(4) NOT NULL,          -- Relationship type (RN, RB, RO, PAR, CHD, etc.)
    cui2            CHAR(8) NOT NULL,             -- Target CUI
    aui2            VARCHAR(12),                  -- Target AUI
    stype2          VARCHAR(50) NOT NULL,         -- Target type
    rela            VARCHAR(100),                 -- Relationship attribute (is_a, part_of, etc.)
    rui             VARCHAR(15),                  -- Relationship Unique Identifier
    srui            VARCHAR(50),                  -- Source RUI
    sab             VARCHAR(40) NOT NULL,         -- Source abbreviation
    sl              VARCHAR(40) NOT NULL,         -- Source of relationship label
    rg              VARCHAR(10),                  -- Relationship group
    dir             VARCHAR(1),                   -- Direction (Y/N/blank)
    suppress        CHAR(1) NOT NULL DEFAULT 'N', -- Suppressible flag
    cvf             INTEGER,                      -- Content View Flag
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

-- Cross-walk: find related concepts
CREATE INDEX IF NOT EXISTS idx_umls_relations_cui1 ON terminology.umls_relations (cui1, rel);
CREATE INDEX IF NOT EXISTS idx_umls_relations_cui2 ON terminology.umls_relations (cui2, rel);
-- Hierarchical queries (is-a relationships)
CREATE INDEX IF NOT EXISTS idx_umls_relations_hier ON terminology.umls_relations (sab, cui1, cui2)
    WHERE rel = 'CHD' OR rela = 'isa';
-- Source-specific relationships
CREATE INDEX IF NOT EXISTS idx_umls_relations_sab ON terminology.umls_relations (sab, rel);
-- RUI uniqueness per release
CREATE INDEX IF NOT EXISTS idx_umls_relations_rui ON terminology.umls_relations (rui, release_id) WHERE rui IS NOT NULL;

-- Semantic Types (MRSTY.RRF) - for filtering by semantic category
CREATE TABLE IF NOT EXISTS terminology.umls_semantic_types (
    id              BIGSERIAL PRIMARY KEY,
    cui             CHAR(8) NOT NULL,
    tui             VARCHAR(4) NOT NULL,          -- Semantic Type Unique Identifier
    stn             VARCHAR(100),                 -- Semantic Type Tree Number
    sty             VARCHAR(100) NOT NULL,        -- Semantic Type Name
    atui            VARCHAR(12),                  -- Attribute Unique Identifier
    cvf             INTEGER,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_umls_stypes_cui ON terminology.umls_semantic_types (cui);
CREATE INDEX IF NOT EXISTS idx_umls_stypes_tui ON terminology.umls_semantic_types (tui);
CREATE INDEX IF NOT EXISTS idx_umls_stypes_sty ON terminology.umls_semantic_types (sty);

-- =============================================================================
-- RXNORM
-- =============================================================================

-- RxNorm concepts (from RXNCONSO.RRF subset)
CREATE TABLE IF NOT EXISTS terminology.rxnorm_concepts (
    id              BIGSERIAL PRIMARY KEY,
    rxcui           VARCHAR(8) NOT NULL,          -- RxNorm Concept Unique Identifier
    lat             CHAR(3) DEFAULT 'ENG',
    ts              CHAR(1),
    lui             VARCHAR(10),
    stt             VARCHAR(3),
    sui             VARCHAR(10),
    rxaui           VARCHAR(12) NOT NULL,         -- RxNorm Atom Unique Identifier
    saui            VARCHAR(50),
    scui            VARCHAR(100),
    sdui            VARCHAR(100),
    sab             VARCHAR(40) NOT NULL,         -- Source (RXNORM, MTHSPL, VANDF, etc.)
    tty             VARCHAR(20) NOT NULL,         -- Term Type (IN, PIN, BN, SBD, SCD, etc.)
    code            VARCHAR(100) NOT NULL,
    str             TEXT NOT NULL,
    suppress        CHAR(1) DEFAULT 'N',
    cvf             INTEGER,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

-- Primary lookup by RXCUI
CREATE INDEX IF NOT EXISTS idx_rxnorm_rxcui ON terminology.rxnorm_concepts (rxcui);
-- RXAUI uniqueness per release
CREATE UNIQUE INDEX IF NOT EXISTS idx_rxnorm_rxaui ON terminology.rxnorm_concepts (rxaui, release_id);
-- NDC lookups (source = MTHSPL for NDC codes)
CREATE INDEX IF NOT EXISTS idx_rxnorm_ndc ON terminology.rxnorm_concepts (code) WHERE sab = 'MTHSPL';
-- Brand name lookups
CREATE INDEX IF NOT EXISTS idx_rxnorm_bn ON terminology.rxnorm_concepts (rxcui) WHERE tty = 'BN';
-- Generic name lookups
CREATE INDEX IF NOT EXISTS idx_rxnorm_in ON terminology.rxnorm_concepts (rxcui) WHERE tty = 'IN';
-- Semantic clinical drug (SCD) and brand drug (SBD)
CREATE INDEX IF NOT EXISTS idx_rxnorm_drugs ON terminology.rxnorm_concepts (rxcui, tty)
    WHERE tty IN ('SCD', 'SBD', 'GPCK', 'BPCK');

-- RxNorm Relationships (from RXNREL.RRF)
CREATE TABLE IF NOT EXISTS terminology.rxnorm_relations (
    id              BIGSERIAL PRIMARY KEY,
    rxcui1          VARCHAR(8) NOT NULL,
    rxaui1          VARCHAR(12),
    stype1          VARCHAR(50),
    rel             VARCHAR(4) NOT NULL,
    rxcui2          VARCHAR(8) NOT NULL,
    rxaui2          VARCHAR(12),
    stype2          VARCHAR(50),
    rela            VARCHAR(100),                 -- tradename_of, has_ingredient, has_dose_form, etc.
    rui             VARCHAR(15),
    srui            VARCHAR(50),
    sab             VARCHAR(40) NOT NULL,
    sl              VARCHAR(40),
    rg              VARCHAR(10),
    dir             VARCHAR(1),
    suppress        CHAR(1) DEFAULT 'N',
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_rxnorm_rel_rxcui1 ON terminology.rxnorm_relations (rxcui1, rela);
CREATE INDEX IF NOT EXISTS idx_rxnorm_rel_rxcui2 ON terminology.rxnorm_relations (rxcui2, rela);
-- Ingredient lookups
CREATE INDEX IF NOT EXISTS idx_rxnorm_rel_ingredient ON terminology.rxnorm_relations (rxcui1, rxcui2)
    WHERE rela = 'has_ingredient';
-- Brand-generic mapping
CREATE INDEX IF NOT EXISTS idx_rxnorm_rel_tradename ON terminology.rxnorm_relations (rxcui1, rxcui2)
    WHERE rela = 'tradename_of';

-- NDC-RXCUI Cross-reference (denormalized for fast lookups)
CREATE TABLE IF NOT EXISTS terminology.rxnorm_ndc_xref (
    id              BIGSERIAL PRIMARY KEY,
    ndc             VARCHAR(11) NOT NULL,         -- 11-digit NDC (no dashes)
    ndc_formatted   VARCHAR(13),                  -- 5-4-2 formatted NDC
    rxcui           VARCHAR(8) NOT NULL,
    start_date      DATE,
    end_date        DATE,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (ndc, rxcui, release_id)
);

CREATE INDEX IF NOT EXISTS idx_ndc_xref_ndc ON terminology.rxnorm_ndc_xref (ndc);
CREATE INDEX IF NOT EXISTS idx_ndc_xref_rxcui ON terminology.rxnorm_ndc_xref (rxcui);
-- Active NDCs (no end date)
CREATE INDEX IF NOT EXISTS idx_ndc_xref_active ON terminology.rxnorm_ndc_xref (ndc) WHERE end_date IS NULL;

-- =============================================================================
-- SNOMED CT
-- =============================================================================

-- SNOMED CT Concepts (from Concept snapshot file)
CREATE TABLE IF NOT EXISTS terminology.snomed_concepts (
    id              BIGSERIAL PRIMARY KEY,
    sctid           BIGINT NOT NULL,              -- SNOMED CT Identifier (18-digit)
    effective_time  DATE NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    module_id       BIGINT NOT NULL,
    definition_status_id BIGINT NOT NULL,         -- Primitive or Fully Defined
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (sctid, effective_time, release_id)
);

-- Primary lookup by SCTID
CREATE INDEX IF NOT EXISTS idx_snomed_concepts_sctid ON terminology.snomed_concepts (sctid) WHERE active = TRUE;
CREATE INDEX IF NOT EXISTS idx_snomed_concepts_module ON terminology.snomed_concepts (module_id) WHERE active = TRUE;

-- SNOMED CT Descriptions (terms/names for concepts)
CREATE TABLE IF NOT EXISTS terminology.snomed_descriptions (
    id              BIGSERIAL PRIMARY KEY,
    desc_id         BIGINT NOT NULL,              -- Description ID
    effective_time  DATE NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    module_id       BIGINT NOT NULL,
    concept_id      BIGINT NOT NULL,              -- References snomed_concepts.sctid
    language_code   CHAR(2) NOT NULL DEFAULT 'en',
    type_id         BIGINT NOT NULL,              -- FSN (900000000000003001) or Synonym (900000000000013009)
    term            TEXT NOT NULL,
    case_significance_id BIGINT NOT NULL,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (desc_id, effective_time, release_id)
);

-- Lookup by concept ID
CREATE INDEX IF NOT EXISTS idx_snomed_desc_concept ON terminology.snomed_descriptions (concept_id) WHERE active = TRUE;
-- FSN (Fully Specified Name) lookups
CREATE INDEX IF NOT EXISTS idx_snomed_desc_fsn ON terminology.snomed_descriptions (concept_id)
    WHERE active = TRUE AND type_id = 900000000000003001;
-- Synonym lookups
CREATE INDEX IF NOT EXISTS idx_snomed_desc_syn ON terminology.snomed_descriptions (concept_id)
    WHERE active = TRUE AND type_id = 900000000000013009;

-- SNOMED CT Relationships (hierarchies and attributes)
CREATE TABLE IF NOT EXISTS terminology.snomed_relationships (
    id              BIGSERIAL PRIMARY KEY,
    rel_id          BIGINT NOT NULL,              -- Relationship ID
    effective_time  DATE NOT NULL,
    active          BOOLEAN NOT NULL DEFAULT TRUE,
    module_id       BIGINT NOT NULL,
    source_id       BIGINT NOT NULL,              -- Source concept SCTID
    destination_id  BIGINT NOT NULL,              -- Destination concept SCTID
    relationship_group INTEGER NOT NULL DEFAULT 0,
    type_id         BIGINT NOT NULL,              -- Relationship type (116680003 = is_a)
    characteristic_type_id BIGINT NOT NULL,
    modifier_id     BIGINT NOT NULL,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (rel_id, effective_time, release_id)
);

-- Is-A hierarchy traversal (most common query)
-- type_id 116680003 = "Is a" relationship
CREATE INDEX IF NOT EXISTS idx_snomed_rel_isa_parent ON terminology.snomed_relationships (destination_id, source_id)
    WHERE active = TRUE AND type_id = 116680003;
CREATE INDEX IF NOT EXISTS idx_snomed_rel_isa_child ON terminology.snomed_relationships (source_id, destination_id)
    WHERE active = TRUE AND type_id = 116680003;
-- General relationship lookups
CREATE INDEX IF NOT EXISTS idx_snomed_rel_source ON terminology.snomed_relationships (source_id, type_id) WHERE active = TRUE;
CREATE INDEX IF NOT EXISTS idx_snomed_rel_dest ON terminology.snomed_relationships (destination_id, type_id) WHERE active = TRUE;
CREATE INDEX IF NOT EXISTS idx_snomed_rel_type ON terminology.snomed_relationships (type_id) WHERE active = TRUE;

-- Transitive Closure for Is-A (pre-computed hierarchy)
-- This dramatically speeds up "find all ancestors/descendants" queries
CREATE TABLE IF NOT EXISTS terminology.snomed_transitive_closure (
    id              BIGSERIAL PRIMARY KEY,
    ancestor_id     BIGINT NOT NULL,              -- Parent/ancestor concept
    descendant_id   BIGINT NOT NULL,              -- Child/descendant concept
    depth           SMALLINT NOT NULL,            -- Distance in hierarchy
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (ancestor_id, descendant_id, release_id)
);

-- Find all descendants of a concept (e.g., all types of diabetes)
CREATE INDEX IF NOT EXISTS idx_snomed_tc_ancestor ON terminology.snomed_transitive_closure (ancestor_id, depth);
-- Find all ancestors of a concept (e.g., what categories does this belong to)
CREATE INDEX IF NOT EXISTS idx_snomed_tc_descendant ON terminology.snomed_transitive_closure (descendant_id, depth);
-- Direct children/parents only
CREATE INDEX IF NOT EXISTS idx_snomed_tc_direct ON terminology.snomed_transitive_closure (ancestor_id, descendant_id)
    WHERE depth = 1;

-- US Extension preferred terms
CREATE TABLE IF NOT EXISTS terminology.snomed_us_preferred (
    id              BIGSERIAL PRIMARY KEY,
    concept_id      BIGINT NOT NULL,
    preferred_term  TEXT NOT NULL,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (concept_id, release_id)
);

CREATE INDEX IF NOT EXISTS idx_snomed_us_pref_concept ON terminology.snomed_us_preferred (concept_id);

-- =============================================================================
-- LOINC
-- =============================================================================

-- LOINC Codes (from LoincTable.csv)
CREATE TABLE IF NOT EXISTS terminology.loinc_codes (
    id              BIGSERIAL PRIMARY KEY,
    loinc_num       VARCHAR(20) NOT NULL,         -- LOINC code (e.g., "12345-6")
    component       TEXT,                          -- What is measured
    property        VARCHAR(50),                   -- Kind of property (Mass, Num, etc.)
    time_aspct      VARCHAR(50),                   -- Point in time vs over time
    system          TEXT,                          -- Specimen type
    scale_typ       VARCHAR(50),                   -- Quantitative, Ordinal, Nominal
    method_typ      VARCHAR(100),                  -- Method of measurement
    class           VARCHAR(100),                  -- Classification
    classtype       CHAR(1),                       -- 1=Lab, 2=Clinical, 3=Claims, 4=Surveys
    long_common_name TEXT NOT NULL,
    shortname       VARCHAR(200),
    consumer_name   VARCHAR(500),
    status          VARCHAR(20) NOT NULL DEFAULT 'ACTIVE', -- ACTIVE, TRIAL, DISCOURAGED, DEPRECATED
    version_first_released VARCHAR(20),
    version_last_changed VARCHAR(20),
    order_obs       VARCHAR(20),                   -- Order, Observation, Both
    example_units   VARCHAR(500),
    units_required  CHAR(1),
    external_copyright_notice TEXT,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (loinc_num, release_id)
);

-- Primary lookup by LOINC code
CREATE INDEX IF NOT EXISTS idx_loinc_num ON terminology.loinc_codes (loinc_num) WHERE status = 'ACTIVE';
-- Class-based queries (e.g., all Chemistry tests)
CREATE INDEX IF NOT EXISTS idx_loinc_class ON terminology.loinc_codes (class) WHERE status = 'ACTIVE';
-- Lab vs Clinical vs Surveys
CREATE INDEX IF NOT EXISTS idx_loinc_classtype ON terminology.loinc_codes (classtype) WHERE status = 'ACTIVE';
-- Component-based lookup
CREATE INDEX IF NOT EXISTS idx_loinc_component ON terminology.loinc_codes (component) WHERE status = 'ACTIVE';

-- LOINC Panel Hierarchy (from PanelHierarchy.csv)
CREATE TABLE IF NOT EXISTS terminology.loinc_panels (
    id              BIGSERIAL PRIMARY KEY,
    parent_loinc    VARCHAR(20) NOT NULL,         -- Panel code
    loinc           VARCHAR(20) NOT NULL,         -- Member code
    sequence        INTEGER,
    cardinality     CHAR(1),                       -- R=Required, O=Optional
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (parent_loinc, loinc, release_id)
);

-- Expand panel to members
CREATE INDEX IF NOT EXISTS idx_loinc_panels_parent ON terminology.loinc_panels (parent_loinc);
-- Find panels containing a code
CREATE INDEX IF NOT EXISTS idx_loinc_panels_member ON terminology.loinc_panels (loinc);

-- LOINC Hierarchy (from ComponentHierarchyBySystem or LOINC hierarchy files)
CREATE TABLE IF NOT EXISTS terminology.loinc_hierarchy (
    id              BIGSERIAL PRIMARY KEY,
    path_to_root    TEXT NOT NULL,                 -- Dotted path (e.g., "LP29693-6.LP7787-7.LP343-4")
    code            VARCHAR(50) NOT NULL,          -- LOINC Part code (LP...)
    code_text       TEXT NOT NULL,
    parent_code     VARCHAR(50),
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (code, release_id)
);

CREATE INDEX IF NOT EXISTS idx_loinc_hier_code ON terminology.loinc_hierarchy (code);
CREATE INDEX IF NOT EXISTS idx_loinc_hier_parent ON terminology.loinc_hierarchy (parent_code);

-- LOINC Answers (for survey instruments with answer lists)
CREATE TABLE IF NOT EXISTS terminology.loinc_answers (
    id              BIGSERIAL PRIMARY KEY,
    answer_list_id  VARCHAR(20) NOT NULL,
    answer_list_name TEXT,
    loinc_num       VARCHAR(20),                   -- The question this answer belongs to
    answer_code     VARCHAR(20),
    answer_string   TEXT NOT NULL,
    sequence        INTEGER,
    display_text    TEXT,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_loinc_answers_list ON terminology.loinc_answers (answer_list_id);
CREATE INDEX IF NOT EXISTS idx_loinc_answers_loinc ON terminology.loinc_answers (loinc_num);

-- =============================================================================
-- ICD-10-CM (Clinical Modification - Diagnoses)
-- =============================================================================

CREATE TABLE IF NOT EXISTS terminology.icd10cm_codes (
    id              BIGSERIAL PRIMARY KEY,
    code            VARCHAR(10) NOT NULL,         -- ICD-10-CM code (no dots, e.g., "E119")
    code_formatted  VARCHAR(12),                   -- With dot (e.g., "E11.9")
    description     TEXT NOT NULL,                 -- Long description
    short_desc      VARCHAR(200),                  -- Short description (for billing)
    is_header       BOOLEAN DEFAULT FALSE,        -- Category header vs billable code
    chapter         VARCHAR(10),                   -- Chapter (01-22)
    chapter_desc    TEXT,
    block_first     VARCHAR(10),                   -- Block range start
    block_last      VARCHAR(10),                   -- Block range end
    parent_code     VARCHAR(10),                   -- Parent category code
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (code, release_id)
);

-- Primary lookup by code
CREATE INDEX IF NOT EXISTS idx_icd10cm_code ON terminology.icd10cm_codes (code);
-- Billable codes only
CREATE INDEX IF NOT EXISTS idx_icd10cm_billable ON terminology.icd10cm_codes (code) WHERE is_header = FALSE;
-- Chapter browsing
CREATE INDEX IF NOT EXISTS idx_icd10cm_chapter ON terminology.icd10cm_codes (chapter);
-- Hierarchy traversal
CREATE INDEX IF NOT EXISTS idx_icd10cm_parent ON terminology.icd10cm_codes (parent_code);

-- =============================================================================
-- ICD-10-PCS (Procedure Coding System)
-- =============================================================================

CREATE TABLE IF NOT EXISTS terminology.icd10pcs_codes (
    id              BIGSERIAL PRIMARY KEY,
    code            CHAR(7) NOT NULL,             -- 7-character code
    description     TEXT NOT NULL,
    section         CHAR(1) NOT NULL,             -- Position 1 (Medical/Surgical, etc.)
    section_title   TEXT,
    body_system     CHAR(1),                       -- Position 2
    body_system_title TEXT,
    root_operation  CHAR(1),                       -- Position 3
    root_operation_title TEXT,
    body_part       CHAR(1),                       -- Position 4
    body_part_title TEXT,
    approach        CHAR(1),                       -- Position 5
    approach_title  TEXT,
    device          CHAR(1),                       -- Position 6
    device_title    TEXT,
    qualifier       CHAR(1),                       -- Position 7
    qualifier_title TEXT,
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (code, release_id)
);

-- Primary lookup
CREATE INDEX IF NOT EXISTS idx_icd10pcs_code ON terminology.icd10pcs_codes (code);
-- Section browsing (Medical/Surgical, Imaging, etc.)
CREATE INDEX IF NOT EXISTS idx_icd10pcs_section ON terminology.icd10pcs_codes (section);
-- Root operation lookup (Excision, Repair, Replacement, etc.)
CREATE INDEX IF NOT EXISTS idx_icd10pcs_root_op ON terminology.icd10pcs_codes (root_operation);
-- Body system queries
CREATE INDEX IF NOT EXISTS idx_icd10pcs_body ON terminology.icd10pcs_codes (section, body_system);

-- ICD-10 Cross-walk table (GEMs - General Equivalence Mappings)
CREATE TABLE IF NOT EXISTS terminology.icd_crosswalk (
    id              BIGSERIAL PRIMARY KEY,
    source_system   VARCHAR(20) NOT NULL,         -- ICD9CM, ICD10CM, ICD10PCS
    source_code     VARCHAR(10) NOT NULL,
    target_system   VARCHAR(20) NOT NULL,
    target_code     VARCHAR(10) NOT NULL,
    flags           VARCHAR(10),                   -- Mapping flags (approximate, etc.)
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS idx_icd_xwalk_source ON terminology.icd_crosswalk (source_system, source_code);
CREATE INDEX IF NOT EXISTS idx_icd_xwalk_target ON terminology.icd_crosswalk (target_system, target_code);

-- =============================================================================
-- CROSS-SYSTEM MAPPINGS (for FHIR ConceptMap resources)
-- =============================================================================

-- General cross-walk table (beyond what UMLS provides)
CREATE TABLE IF NOT EXISTS terminology.code_mappings (
    id              BIGSERIAL PRIMARY KEY,
    source_system   VARCHAR(255) NOT NULL,        -- FHIR URI (http://snomed.info/sct)
    source_code     VARCHAR(100) NOT NULL,
    source_display  TEXT,
    target_system   VARCHAR(255) NOT NULL,
    target_code     VARCHAR(100) NOT NULL,
    target_display  TEXT,
    equivalence     VARCHAR(20) NOT NULL,         -- equivalent, wider, narrower, inexact, unmatched
    comment         TEXT,
    mapping_source  VARCHAR(50),                   -- UMLS, NLM, CUSTOM, etc.
    release_id      INTEGER NOT NULL REFERENCES terminology.releases(id) ON DELETE CASCADE,
    UNIQUE (source_system, source_code, target_system, target_code, release_id)
);

-- Primary cross-walk lookup
CREATE INDEX IF NOT EXISTS idx_code_mappings_source ON terminology.code_mappings (source_system, source_code);
CREATE INDEX IF NOT EXISTS idx_code_mappings_target ON terminology.code_mappings (target_system, target_code);
-- Filter by equivalence
CREATE INDEX IF NOT EXISTS idx_code_mappings_equiv ON terminology.code_mappings (source_system, source_code, equivalence);

-- =============================================================================
-- SCHEMA VERSION TRACKING
-- =============================================================================

CREATE TABLE IF NOT EXISTS terminology.schema_version (
    version         INTEGER PRIMARY KEY,
    applied_at      TIMESTAMPTZ DEFAULT NOW(),
    description     TEXT
);

-- Record initial schema version
INSERT INTO terminology.schema_version (version, description)
VALUES (1, 'Initial terminology schema with UMLS, RxNorm, SNOMED CT, LOINC, ICD-10')
ON CONFLICT (version) DO NOTHING;
`

// DropSchema removes all terminology tables (use with caution).
const DropSchema = `
DROP SCHEMA IF EXISTS terminology CASCADE;
`

// SchemaV2Migration adds custom mapping tables for CSV upload, autorouting, and telemetry.
// Applied when upgrading from v1 to v2.
const SchemaV2Migration = `
-- =============================================================================
-- SCHEMA v2: Custom Mapping Tables
-- =============================================================================
-- Adds support for:
-- - CSV mapping uploads (custom_mappings, upload_batches)
-- - Autoroute suggestions (pending_autoroutes)
-- - Decision telemetry (mapping_decisions)

-- =============================================================================
-- Upload Batch Tracking
-- =============================================================================
CREATE TABLE IF NOT EXISTS terminology.upload_batches (
    id              UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    filename        VARCHAR(255) NOT NULL,
    source_system   VARCHAR(100),                       -- Default source system for batch
    target_system   VARCHAR(255),                       -- Default target system for batch
    profile_id      VARCHAR(100),                       -- Optional: scope to profile

    -- Stats
    total_rows      INT NOT NULL DEFAULT 0,
    valid_rows      INT NOT NULL DEFAULT 0,
    duplicate_rows  INT DEFAULT 0,
    error_rows      INT DEFAULT 0,

    -- Audit
    uploaded_at     TIMESTAMPTZ DEFAULT NOW(),
    uploaded_by     VARCHAR(100),

    -- Validation results
    validation_errors JSONB DEFAULT '[]'                -- Array of {row, column, error}
);

CREATE INDEX IF NOT EXISTS idx_upload_batches_uploaded_at
    ON terminology.upload_batches(uploaded_at DESC);

-- =============================================================================
-- Custom Uploaded Mappings
-- =============================================================================
CREATE TABLE IF NOT EXISTS terminology.custom_mappings (
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
    origin          VARCHAR(30) NOT NULL DEFAULT 'csv_upload', -- 'csv_upload', 'approved_autoroute', 'manual'
    upload_batch_id UUID REFERENCES terminology.upload_batches(id) ON DELETE SET NULL,
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

CREATE INDEX IF NOT EXISTS idx_custom_mappings_lookup
    ON terminology.custom_mappings(source_system, source_code, target_system);
CREATE INDEX IF NOT EXISTS idx_custom_mappings_profile
    ON terminology.custom_mappings(profile_id) WHERE profile_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_custom_mappings_batch
    ON terminology.custom_mappings(upload_batch_id) WHERE upload_batch_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_custom_mappings_origin
    ON terminology.custom_mappings(origin);

-- =============================================================================
-- Pending Autoroute Suggestions (for review workflow)
-- =============================================================================
CREATE TABLE IF NOT EXISTS terminology.pending_autoroutes (
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
    decision_trace  JSONB NOT NULL DEFAULT '{}',

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

CREATE INDEX IF NOT EXISTS idx_pending_autoroutes_status
    ON terminology.pending_autoroutes(status) WHERE status = 'pending';
CREATE INDEX IF NOT EXISTS idx_pending_autoroutes_confidence
    ON terminology.pending_autoroutes(confidence DESC);
CREATE INDEX IF NOT EXISTS idx_pending_autoroutes_created
    ON terminology.pending_autoroutes(created_at DESC);

-- =============================================================================
-- Decision Audit Log (telemetry persistence)
-- =============================================================================
-- Note: For production, consider partitioning by month for efficient retention.
-- This simple version works for small-medium deployments.
CREATE TABLE IF NOT EXISTS terminology.mapping_decisions (
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
    decision_tree   JSONB NOT NULL DEFAULT '{}',

    -- Context
    profile_id      VARCHAR(100),
    request_source  VARCHAR(50),                        -- 'graphql', 'cli', 'workflow', 'batch'

    -- Timing
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    duration_ms     INT
);

CREATE INDEX IF NOT EXISTS idx_mapping_decisions_trace
    ON terminology.mapping_decisions(trace_id);
CREATE INDEX IF NOT EXISTS idx_mapping_decisions_source
    ON terminology.mapping_decisions(source_system, source_code);
CREATE INDEX IF NOT EXISTS idx_mapping_decisions_created
    ON terminology.mapping_decisions(created_at DESC);
CREATE INDEX IF NOT EXISTS idx_mapping_decisions_type
    ON terminology.mapping_decisions(decision_type);

-- Record v2 migration
INSERT INTO terminology.schema_version (version, description)
VALUES (2, 'Custom mapping tables: upload_batches, custom_mappings, pending_autoroutes, mapping_decisions')
ON CONFLICT (version) DO NOTHING;
`

// Vocabulary constants matching UMLS SAB (Source Abbreviation) codes.
const (
	VocabUMLS     = "UMLS"
	VocabSNOMEDCT = "SNOMEDCT_US"
	VocabICD10CM  = "ICD10CM"
	VocabICD10PCS = "ICD10PCS"
	VocabRxNorm   = "RXNORM"
	VocabLOINC    = "LOINC"
	VocabCPT      = "CPT"
	VocabHCPCS    = "HCPCS"
	VocabCVX      = "CVX"
	VocabNDC      = "NDC"
)

// SNOMED CT type IDs for descriptions.
const (
	SNOMEDTypeFSN     int64 = 900000000000003001 // Fully Specified Name
	SNOMEDTypeSynonym int64 = 900000000000013009 // Synonym
)

// SNOMED CT relationship type IDs.
const (
	SNOMEDRelIsA int64 = 116680003 // "Is a" relationship
)

// RxNorm term types (TTY).
const (
	RxTTYIngredient   = "IN"   // Ingredient
	RxTTYBrandName    = "BN"   // Brand Name
	RxTTYSCD          = "SCD"  // Semantic Clinical Drug
	RxTTYSBD          = "SBD"  // Semantic Branded Drug
	RxTTYGPCK         = "GPCK" // Generic Pack
	RxTTYBPCK         = "BPCK" // Brand Pack
	RxTTYPrescribable = "PSN"  // Prescribable Name
)
