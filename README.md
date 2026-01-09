# fi-fhir

A format-agnostic healthcare integration library that transforms legacy formats (HL7v2, flatfiles, EDI) into semantic events.

## Overview

fi-fhir addresses a core problem in healthcare integration: **users think in workflow terms, but tools require format-specific knowledge**.

Instead of writing code that references `PID.3.1` or `OBX.5`, you work with semantic events like `patient_admit` and `lab_result`. The library handles format parsing, field mapping, and validation automatically.

```
HL7v2 ADT^A01  ──┐
CSV patient file ──┼──► Semantic Event ──► Workflow Actions
FHIR Bundle    ──┘     (patient_admit)
```

## Installation

```bash
# CLI tool
go install github.com/cblevins/fi-fhir/cmd/fi-fhir@latest

# As a library
go get github.com/cblevins/fi-fhir
```

## Quick Start

### CLI Usage

```bash
# Parse an HL7v2 message to semantic event JSON
fi-fhir parse --format hl7v2 --source "lab_system" message.hl7

# Pretty-print output
fi-fhir parse --pretty message.hl7

# Pipe from stdin
cat message.hl7 | fi-fhir parse -f hl7v2 -
```

### Library Usage

```go
package main

import (
    "fmt"
    "github.com/cblevins/fi-fhir/internal/parser/hl7v2"
    "github.com/cblevins/fi-fhir/pkg/events"
)

func main() {
    parser := hl7v2.NewParser("hospital_adt", hl7v2.ParserConfig{})

    msg := `MSH|^~\&|APP|FAC|APP|FAC|20240115||ADT^A01|123|P|2.5
PID|1||MRN123||DOE^JOHN||19800315|M`

    result, err := parser.Parse(msg)
    if err != nil {
        panic(err)
    }

    event := result.(*events.PatientAdmitEvent)
    fmt.Printf("Patient %s %s admitted\n",
        event.Patient.GivenName,
        event.Patient.FamilyName)
}
```

## Supported Formats

### HL7 v2.x

| Message Type | Description | Semantic Event |
|--------------|-------------|----------------|
| ADT^A01 | Admit | `patient_admit` |
| ADT^A02 | Transfer | `patient_transfer` |
| ADT^A03 | Discharge | `patient_discharge` |
| ADT^A04 | Register (outpatient) | `patient_admit` |
| ADT^A08 | Update patient info | `patient_update` |
| ORU^R01 | Lab result | `lab_result` |
| SIU^S12 | New appointment | `appointment_scheduled` |

### Coming Soon
- CSV/Flatfile with schema inference
- EDI X12 (835, 837)
- FHIR R4

## Semantic Event Model

Events are format-agnostic representations of healthcare data:

```go
// All events have common metadata
type EventMeta struct {
    ID            string       `json:"id"`
    Type          EventType    `json:"type"`
    Timestamp     time.Time    `json:"timestamp"`
    Source        string       `json:"source"`
    SourceFormat  SourceFormat `json:"source_format"`
}

// Patient data is normalized regardless of source
type Patient struct {
    MRN        string    `json:"mrn"`
    FamilyName string    `json:"family_name"`
    GivenName  string    `json:"given_name"`
    // ... other fields
}
```

## Project Structure

```
fi-fhir/
├── cmd/fi-fhir/          # CLI entry point
├── internal/
│   ├── parser/
│   │   └── hl7v2/        # HL7v2 parser implementation
│   ├── semantic/         # Event processing
│   └── workflow/         # Workflow engine (planned)
├── pkg/
│   ├── events/           # Public semantic event types
│   └── config/           # Configuration types
└── testdata/             # Sample messages
```

## Development

```bash
# Build
make build
# or
go build -o bin/fi-fhir ./cmd/fi-fhir

# Test
make test
# or
go test ./...

# Run
./bin/fi-fhir parse testdata/adt_a01_sample.hl7
```

## Roadmap

- [x] HL7v2 ADT parsing (A01, A02, A03, A04, A08)
- [x] HL7v2 ORU (lab results) parsing
- [ ] HL7v2 SIU (scheduling) parsing
- [ ] CSV/flatfile adapter with schema inference
- [ ] EDI 835/837 adapter
- [ ] YAML-based workflow DSL
- [ ] Workflow engine execution
- [ ] TypeScript SDK (npm package)

## License

Apache 2.0
