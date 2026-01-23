# Adding a Format Parser

This guide walks through adding support for a new healthcare format to fi-fhir.

## Overview

Parsers live in `internal/parser/<format>/` and transform raw bytes into canonical events from `pkg/events/`.

## Step-by-Step Guide

### 1. Create the Package

```bash
mkdir -p internal/parser/myformat
```

Create the following files:
- `parser.go` - Main parser implementation
- `parser_test.go` - Tests
- `types.go` - Format-specific types (if needed)

### 2. Define the Parser Struct

```go
// internal/parser/myformat/parser.go
package myformat

import (
    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

// Parser handles MyFormat message parsing
type Parser struct {
    profile  *profile.Profile
    warnings []events.ParseWarning
}

// NewParser creates a parser with the given profile
func NewParser(p *profile.Profile) *Parser {
    return &Parser{
        profile:  p,
        warnings: make([]events.ParseWarning, 0),
    }
}
```

### 3. Implement the Parse Method

```go
// Parse transforms raw bytes into canonical events
func (p *Parser) Parse(raw []byte) ([]events.Event, error) {
    // Phase 1: Byte normalization
    normalized, err := p.normalizeBytes(raw)
    if err != nil {
        return nil, fmt.Errorf("byte normalization: %w", err)
    }

    // Phase 2: Syntactic parsing
    parsed, err := p.syntacticParse(normalized)
    if err != nil {
        return nil, fmt.Errorf("syntactic parse: %w", err)
    }

    // Phase 3: Semantic extraction
    events, err := p.extractEvents(parsed)
    if err != nil {
        return nil, fmt.Errorf("semantic extraction: %w", err)
    }

    return events, nil
}
```

### 4. Implement Phase 1: Byte Normalization

Handle character encoding and line endings:

```go
func (p *Parser) normalizeBytes(raw []byte) (string, error) {
    charset := p.profile.Encoding.Charset

    // Detect and handle BOM
    if bytes.HasPrefix(raw, []byte{0xEF, 0xBB, 0xBF}) {
        raw = raw[3:] // Strip UTF-8 BOM
    }

    // Convert charset if needed
    var content string
    switch charset {
    case "UTF-8":
        content = string(raw)
    case "ISO-8859-1":
        content = convertFromLatin1(raw)
    default:
        content = string(raw)
    }

    // Normalize line endings
    content = strings.ReplaceAll(content, "\r\n", "\n")
    content = strings.ReplaceAll(content, "\r", "\n")

    return content, nil
}
```

### 5. Implement Phase 2: Syntactic Parsing

Parse the format-specific structure:

```go
// ParsedMessage represents the syntactically parsed message
type ParsedMessage struct {
    Header  map[string]string
    Records []ParsedRecord
}

func (p *Parser) syntacticParse(content string) (*ParsedMessage, error) {
    msg := &ParsedMessage{
        Header:  make(map[string]string),
        Records: make([]ParsedRecord, 0),
    }

    lines := strings.Split(content, "\n")
    for i, line := range lines {
        if i == 0 {
            // Parse header
            msg.Header = p.parseHeader(line)
        } else {
            // Parse record
            record, err := p.parseRecord(line)
            if err != nil {
                p.addWarning("PARSE_ERROR", fmt.Sprintf("line %d", i), err.Error())
                continue // Record warning, continue parsing
            }
            msg.Records = append(msg.Records, record)
        }
    }

    return msg, nil
}
```

### 6. Implement Phase 3: Semantic Extraction

Transform parsed data into canonical events:

```go
func (p *Parser) extractEvents(msg *ParsedMessage) ([]events.Event, error) {
    var result []events.Event

    for _, record := range msg.Records {
        event, err := p.recordToEvent(record)
        if err != nil {
            p.addWarning("EXTRACTION_ERROR", record.ID, err.Error())
            continue
        }

        // Set metadata
        event.Meta().ParseWarnings = p.warnings

        result = append(result, event)
    }

    return result, nil
}

func (p *Parser) recordToEvent(record ParsedRecord) (events.Event, error) {
    // Determine event type from record data
    eventType := p.classifyEvent(record)

    // Create appropriate event
    meta := events.NewEventMeta(eventType, p.profile.ID, events.FormatMyFormat)

    switch eventType {
    case events.EventPatientAdmit:
        return p.createPatientAdmitEvent(meta, record)
    case events.EventLabResult:
        return p.createLabResultEvent(meta, record)
    default:
        return nil, fmt.Errorf("unknown event type: %s", eventType)
    }
}
```

### 7. Handle Warnings

Use warnings for recoverable issues:

```go
func (p *Parser) addWarning(code, path, message string) {
    p.warnings = append(p.warnings, events.ParseWarning{
        Phase:   "semantic",
        Code:    code,
        Path:    path,
        Message: message,
    })
}

// Check profile tolerance before failing
func (p *Parser) handleMissingField(fieldName, path string) (string, error) {
    if p.profile.IsMissingFieldTolerated(fieldName) {
        p.addWarning("MISSING_FIELD", path, fmt.Sprintf("field %s not found", fieldName))
        return "", nil // Return empty, don't fail
    }
    return "", fmt.Errorf("required field %s not found", fieldName)
}
```

### 8. Add Tests

Create comprehensive tests in `parser_test.go`:

```go
package myformat

import (
    "testing"

    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

func TestParser_Parse(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        profile *profile.Profile
        want    []events.EventType
        wantErr bool
    }{
        {
            name:    "valid message",
            input:   "HEADER\nRECORD1\nRECORD2",
            profile: defaultProfile(),
            want:    []events.EventType{events.EventPatientAdmit, events.EventPatientAdmit},
            wantErr: false,
        },
        {
            name:    "missing required field",
            input:   "HEADER\nINCOMPLETE",
            profile: strictProfile(),
            wantErr: true,
        },
        {
            name:    "missing optional field with tolerance",
            input:   "HEADER\nINCOMPLETE",
            profile: tolerantProfile(),
            want:    []events.EventType{events.EventPatientAdmit},
            wantErr: false, // Tolerates missing field
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            p := NewParser(tt.profile)
            got, err := p.Parse([]byte(tt.input))

            if (err != nil) != tt.wantErr {
                t.Errorf("Parse() error = %v, wantErr %v", err, tt.wantErr)
                return
            }

            if !tt.wantErr {
                if len(got) != len(tt.want) {
                    t.Errorf("Parse() returned %d events, want %d", len(got), len(tt.want))
                }
                for i, event := range got {
                    if event.Type() != tt.want[i] {
                        t.Errorf("event[%d].Type() = %v, want %v", i, event.Type(), tt.want[i])
                    }
                }
            }
        })
    }
}

// Test helpers
func defaultProfile() *profile.Profile {
    return &profile.Profile{
        ID:   "test",
        Name: "Test Profile",
    }
}
```

### 9. Add Test Data

Create sample files in `testdata/`:

```
testdata/
├── myformat_sample.txt
├── myformat_with_errors.txt
└── myformat_edge_cases.txt
```

### 10. Register the Parser

Add CLI support in `cmd/fi-fhir/main.go`:

```go
case "myformat":
    p := myformat.NewParser(profile)
    events, err = p.Parse(raw)
```

### 11. Add Format Constant

In `pkg/events/events.go`:

```go
const (
    // ... existing formats
    FormatMyFormat SourceFormat = "MyFormat"
)
```

## Best Practices

### 1. Follow the Three-Phase Pattern

Keep phases separate for clarity and testability:
- Phase 1: Pure byte manipulation
- Phase 2: Format syntax only
- Phase 3: Business semantics

### 2. Use Warnings, Not Errors

Healthcare data is messy. Prefer warnings for recoverable issues:

```go
// Good
if field == "" {
    p.addWarning("EMPTY_FIELD", path, "field is empty")
    return defaultValue, nil
}

// Bad (too strict)
if field == "" {
    return nil, errors.New("field is required")
}
```

### 3. Respect Profile Configuration

Always check profile settings before making decisions:

```go
// Good
if p.profile.IsMissingSegmentTolerated(segmentID) {
    p.addWarning(...)
    return nil, nil
}

// Bad (ignores profile)
return nil, errors.New("segment missing")
```

### 4. Preserve Raw Data

Store original data for auditing:

```go
event.Meta().RawPayload = json.RawMessage(raw)
```

### 5. Test Edge Cases

Include tests for:
- Empty input
- Malformed input
- Missing fields
- Extra/unknown fields
- Character encoding issues
- Large messages

## Example: Minimal Parser

Here's a complete minimal parser:

```go
package myformat

import (
    "fmt"
    "strings"

    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/events"
    "gitlab.flexinfer.ai/libs/fi-fhir/pkg/profile"
)

type Parser struct {
    profile  *profile.Profile
    warnings []events.ParseWarning
}

func NewParser(p *profile.Profile) *Parser {
    return &Parser{profile: p}
}

func (p *Parser) Parse(raw []byte) ([]events.Event, error) {
    content := string(raw)
    lines := strings.Split(content, "\n")

    var result []events.Event
    for _, line := range lines {
        if line == "" {
            continue
        }

        parts := strings.Split(line, "|")
        if len(parts) < 3 {
            p.warnings = append(p.warnings, events.ParseWarning{
                Code:    "MALFORMED_LINE",
                Message: "expected 3+ fields",
            })
            continue
        }

        meta := events.NewEventMeta(
            events.EventPatientAdmit,
            p.profile.ID,
            "MyFormat",
        )
        meta.ParseWarnings = p.warnings

        event := &events.PatientAdmitEvent{
            EventMeta: meta,
            Patient: events.Patient{
                MRN: parts[0],
                Name: events.Name{
                    Family: parts[1],
                    Given:  []string{parts[2]},
                },
            },
        }

        result = append(result, event)
    }

    return result, nil
}
```

## See Also

- [Architecture Overview](architecture.md) - System architecture
- [Adding Event Types](adding-events.md) - Create new events
- [Testing Guidelines](testing.md) - Testing best practices
