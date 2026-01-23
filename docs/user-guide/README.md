# fi-fhir User Guide

Welcome to fi-fhir, a format-agnostic healthcare integration platform that transforms legacy formats (HL7v2, CSV, EDI X12, CDA/CCDA) into semantic events and routes them through configurable workflows.

## Table of Contents

1. [Getting Started](getting-started.md)
2. [Core Concepts](core-concepts.md)
3. [CLI Reference](cli-reference.md)
4. [Source Profiles](source-profiles.md)
5. [Workflow Configuration](workflows.md)
6. [FHIR Output](fhir-output.md)
7. [Playground Tutorial](playground-tutorial.md)

## Quick Links

| I want to... | Read... |
|--------------|---------|
| Parse my first HL7v2 message | [Getting Started](getting-started.md#parse-your-first-message) |
| Understand the 3-phase pipeline | [Core Concepts](core-concepts.md#parsing-pipeline) |
| Configure a source profile | [Source Profiles](source-profiles.md) |
| Route events to multiple destinations | [Workflow Configuration](workflows.md) |
| Generate FHIR resources | [FHIR Output](fhir-output.md) |
| Try it in the browser | [Playground Tutorial](playground-tutorial.md) |

## What is fi-fhir?

fi-fhir addresses a core problem in healthcare integration: **users think in workflow terms, but tools require format-specific knowledge**.

Instead of writing code that references `PID.3.1` or `OBX.5`, you work with semantic events like `patient_admit` and `lab_result`. The library handles format parsing, field mapping, validation, and routing automatically.

### Key Features

- **Multi-format parsing**: HL7v2, CSV/flatfiles, EDI X12, CDA/CCDA
- **Profile-driven**: Each data feed gets its own configuration
- **Semantic events**: Work with business concepts, not format details
- **Workflow DSL**: YAML-based routing with CEL expression filters
- **FHIR R4 output**: US Core profile mapper with 24+ resources
- **Production-ready**: Retry, circuit breaker, observability built-in

### Architecture Overview

```
Input Formats          Semantic Layer           Output/Actions
─────────────         ────────────────         ──────────────
HL7v2    ──┐          ┌─────────────┐          ┌─> FHIR API
Flatfile ──┼──────────┤ Canonical   ├──────────┼─> REST Webhook
EDI X12  ──┤          │ Event Model │          ├─> Database
CDA/CCDA ──┤          └─────────────┘          └─> Message Queue
FHIR     ──┘
```

## Prerequisites

- **For CLI usage**: Go 1.21+ or Docker
- **For TypeScript SDK**: Node.js 18+
- **For Playground**: Just a web browser!

## Next Steps

If you're new to fi-fhir, start with [Getting Started](getting-started.md) for a hands-on tutorial.

If you prefer to learn by exploring, try the [Playground Tutorial](playground-tutorial.md) to experiment in your browser.

## See Also

- [Developer Guide](../developer-guide/README.md) - For contributors and advanced users
- [Planning Documents](../planning/README.md) - Technical specifications
- [Operations Guide](../operations/README.md) - Production deployment
