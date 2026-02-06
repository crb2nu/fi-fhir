# Playground Tutorial

The fi-fhir Playground is an interactive web-based environment for learning and experimenting with healthcare integration concepts. No installation required!

## Accessing the Playground

Visit: **https://flexinfer.ai/playground/fi-fhir**

The playground is part of the FlexInfer site and provides three specialized tools:
1. **Source Profile Editor** - Configure parsing rules
2. **Pipeline Visualization** - See the 3-phase transformation
3. **Mapping Visualizer** - Explore HL7v2 → FHIR mappings

## Tool 1: Source Profile Editor

The Source Profile Editor lets you create and validate Source Profile configurations interactively.

### Accessing the Editor

Navigate to: `/playground/fi-fhir/profiles`

### Interface Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│ [Example Selector ▼]                              [Copy] [Export]   │
├───────────────────────────────────┬─────────────────────────────────┤
│                                   │                                 │
│  YAML Editor                      │  Preview Panel                  │
│                                   │                                 │
│  name: epic_adt                   │  ✓ Valid Source Profile        │
│  version: "1.0"                   │                                 │
│  encoding:                        │  Sections Present:              │
│    charset: UTF-8                 │  ☑ Encoding                     │
│    ...                            │  ☑ Syntax                       │
│                                   │  ☑ Semantics                    │
│                                   │  ☐ FHIR Mapping                 │
│                                   │  ☑ Validation                   │
│                                   │                                 │
└───────────────────────────────────┴─────────────────────────────────┘
```

### Getting Started

1. **Select an Example**: Use the dropdown to load a pre-built profile
   - **ADT Messages** - Admit/Discharge/Transfer
   - **ORU Results** - Lab observations
   - **MDM Documents** - Medical documents

2. **Edit the Profile**: Modify the YAML in the left panel

3. **See Real-time Validation**: The right panel shows:
   - Validation status (valid/invalid)
   - Error messages with line numbers
   - Checklist of configured sections

### Example: ADT Profile

```yaml
name: hospital_adt
version: "1.0"
description: "ADT messages from main hospital system"

encoding:
  charset: UTF-8
  lineEnding: auto
  bomHandling: strip

syntax:
  hl7Version: "2.5"
  fieldSeparator: "|"
  encodingChars: "^~\\&"
  escapeSequences:
    enabled: true
  strictMode: false

semantics:
  messageTypes: [ADT]
  eventTypes: [A01, A02, A03, A04, A08]
  patientIdentifiers:
    - source_field: PID.3.1
      identifier_type: MRN
      assigning_authority: HOSPITAL

validation:
  enabled: true
  requiredSegments: [MSH, PID, PV1]
  requiredFields: [MSH.9, PID.3]
```

### Validation Feedback

The editor provides immediate feedback:

- **Parse Errors**: YAML syntax issues
- **Schema Errors**: Missing required fields, invalid values
- **Warnings**: Best practice suggestions

Example error:
```
Line 8: "encoding.charset" must be one of: UTF-8, ISO-8859-1, Windows-1252, US-ASCII
```

## Tool 2: Pipeline Visualization

The Pipeline Visualization tool demonstrates how messages flow through fi-fhir's three-phase parsing pipeline.

### Accessing the Visualizer

Navigate to: `/playground/fi-fhir/pipeline`

### Interface Overview

The page displays three connected phase cards:

```
┌─────────────────┐    ┌─────────────────┐    ┌─────────────────┐
│ Phase 1         │───>│ Phase 2         │───>│ Phase 3         │
│ Byte            │    │ Syntactic       │    │ Semantic        │
│ Normalization   │    │ Parsing         │    │ Extraction      │
└─────────────────┘    └─────────────────┘    └─────────────────┘
```

### Interactive Features

1. **Click a Phase**: Highlights and shows detailed information
2. **Animate Pipeline**: Auto-cycles through phases to show data flow
3. **Sample Message**: Shows a real HL7 message being transformed

### Phase Details

When you select a phase, you see:

**Phase 1: Byte Normalization**
- Input: Raw bytes (UTF-8 with CRLF)
- Output: Normalized UTF-8 string with LF
- Steps:
  - Detect BOM markers
  - Convert charset to UTF-8
  - Normalize line endings to LF
  - Strip trailing whitespace

**Phase 2: Syntactic Parsing**
- Input: Normalized string
- Output: Parsed segments and fields
- Steps:
  - Extract field separator from MSH.1
  - Parse encoding characters from MSH.2
  - Split message into segments
  - Handle escape sequences

**Phase 3: Semantic Extraction**
- Input: Parsed message structure
- Output: Extracted identifiers and data
- Steps:
  - Identify message type (ADT^A01)
  - Extract patient identifier (MRN12345)
  - Map event type to processing rules

### Configuration Reference

Each phase shows which YAML fields control its behavior:

| Phase | Configuration Fields |
|-------|---------------------|
| 1 | `encoding.charset`, `encoding.lineEnding`, `encoding.bomHandling` |
| 2 | `syntax.hl7Version`, `syntax.fieldSeparator`, `syntax.encodingChars` |
| 3 | `semantics.messageTypes`, `semantics.patientIdentifiers` |

## Tool 3: Mapping Visualizer

The Mapping Visualizer shows how HL7v2 segments map to FHIR R4 resources.

### Accessing the Visualizer

Navigate to: `/playground/fi-fhir/mapper`

### Interface Overview

```
┌─────────────────────────────────────────────────────────────────────┐
│ Mapping: [PID → Patient ▼]                                          │
├─────────────────────────────────────────────────────────────────────┤
│                                                                     │
│  PID Segment                          Patient Resource              │
│  ┌─────────────────────────────────────────────────────────────┐   │
│  │ PID.3.1  ──────────────────────────>  identifier[0].value   │   │
│  │ PID.5.1  ──────────────────────────>  name[0].family        │   │
│  │ PID.5.2  ──────────────────────────>  name[0].given[0]      │   │
│  │ PID.7    ─── dateTime ────────────>  birthDate              │   │
│  │ PID.8    ─── gender ──────────────>  gender                 │   │
│  │ PID.11   ─── address ─────────────>  address[0]             │   │
│  └─────────────────────────────────────────────────────────────┘   │
│                                                                     │
└─────────────────────────────────────────────────────────────────────┘
```

### Available Mappings

1. **PID → Patient**
   - Patient demographics
   - Identifiers, name, DOB, gender, address

2. **PV1 → Encounter**
   - Visit information
   - Class, location, period, identifiers

3. **OBX → Observation**
   - Lab results and clinical data
   - Codes, values, units, status

### Understanding Transform Functions

Some mappings include transform functions:

| Function | Description | Example |
|----------|-------------|---------|
| `dateTime` | HL7 datetime → FHIR format | `20240115` → `2024-01-15` |
| `gender` | M/F → male/female | `M` → `male` |
| `address` | XAD → FHIR Address | Structured address |
| `coding` | CE/CWE → FHIR Coding | Code with system |
| `reference` | Create FHIR reference | `Patient/123` |

### HL7v2 Segment Reference

The visualizer includes a reference showing supported segments:

| Segment | FHIR Resource | Description |
|---------|---------------|-------------|
| MSH | MessageHeader | Message metadata |
| PID | Patient | Patient demographics |
| PV1 | Encounter | Patient visit |
| OBR | DiagnosticReport | Test order |
| OBX | Observation | Test result |
| TXA | DocumentReference | Document metadata |
| AL1 | AllergyIntolerance | Allergies |
| DG1 | Condition | Diagnoses |
| PR1 | Procedure | Procedures |
| IN1 | Coverage | Insurance |

## Workflow: Learn by Doing

Here's a recommended learning path using the playground:

### Step 1: Understand the Pipeline

1. Go to `/playground/fi-fhir/pipeline`
2. Click "Animate Pipeline" to watch data flow
3. Click each phase to understand its role

### Step 2: Explore Mappings

1. Go to `/playground/fi-fhir/mapper`
2. Select "PID → Patient" to see field mappings
3. Select "OBX → Observation" for lab results
4. Note the transform functions used

### Step 3: Create a Profile

1. Go to `/playground/fi-fhir/profiles`
2. Start with the "ADT Messages" example
3. Modify the configuration:
   - Change `encoding.charset`
   - Add a custom identifier configuration
4. Watch validation update in real-time
5. Copy the valid profile for use in your project

### Step 4: Apply to CLI

Once you've designed your profile in the playground:

```bash
# Save profile from playground to file
cat > my_profile.yaml << 'EOF'
# Paste your profile here
EOF

# Validate with CLI
fi-fhir validate profile my_profile.yaml

# Parse with profile
fi-fhir parse --format hl7v2 --profile my_profile.yaml message.hl7
```

## Integration with CLI

The playground and CLI work together seamlessly. Use the playground for visual exploration and the CLI for automation and production.

### Playground to CLI Mapping

| Playground Tool | CLI Command | Description |
|-----------------|-------------|-------------|
| **Source Profile Editor** | `fi-fhir profile infer` | Generate profile from samples |
| | `fi-fhir profile lint` | Validate profile best practices |
| | `fi-fhir validate profile` | Schema validation |
| **Pipeline Visualization** | `fi-fhir parse --verbose` | See parsing phases |
| | `fi-fhir parse --explain-warnings` | Get LLM explanations |
| **Mapping Visualizer** | `fi-fhir fhir validate` | Validate FHIR output |
| | `fi-fhir fhir generate` | Generate FHIR from events |

### CLI Commands for Common Tasks

```bash
# After designing a profile in the playground
fi-fhir profile lint my_profile.yaml         # Check best practices
fi-fhir validate profile my_profile.yaml     # Validate schema

# Generate profile from sample messages
fi-fhir profile infer --output inferred.yaml samples/*.hl7

# Parse with verbose output (like Pipeline Visualization)
fi-fhir parse --format hl7v2 --verbose message.hl7

# Get LLM explanations for warnings
fi-fhir parse --format hl7v2 --explain-warnings message.hl7

# Validate FHIR output (like Mapping Visualizer)
fi-fhir fhir validate --profile us-core patient.json
```

### Export and Import

**Export from Playground:**
1. Create/edit profile in playground
2. Click "Copy" to copy YAML
3. Save to file locally

**Import to Playground:**
1. Copy your profile YAML
2. Paste into the editor
3. Validate and iterate

## Tips and Best Practices

### Profile Design

- Start with an example close to your use case
- Enable only the sections you need
- Test with real sample messages

### Validation

- Watch for schema errors (missing required fields)
- Address warnings for production use
- Validate both profile and sample messages

### Learning Path

1. Pipeline → Understand the flow
2. Mapper → Learn field relationships
3. Profiles → Configure your own

## See Also

- [Source Profiles](source-profiles.md) - Detailed profile reference
- [FHIR Output](fhir-output.md) - FHIR mapping details
- [Getting Started](getting-started.md) - CLI usage
