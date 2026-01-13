# Architecture Diagrams

This directory contains **generated** diagrams to help new contributors understand how `fi-fhir` is organized.

## Diagrams

- `parser-modules.svg` — Go package dependencies within `internal/parser/` (HL7v2, CSV, EDI, CDA)
- `pkg-modules.svg` — Go package dependencies within `pkg/` (public API surface)
- `cli-call-graph.svg` — Call graph starting from `cmd/fi-fhir` `main` (high-level CLI flow)

## Regenerating

If you have `diagram-gen` installed:

```bash
diagram-gen modules ./internal/parser -f svg -o docs/diagrams/parser-modules.svg
diagram-gen modules ./pkg -f svg -o docs/diagrams/pkg-modules.svg
diagram-gen calls ./cmd/fi-fhir --lang go --entry main --depth 3 -f svg -o docs/diagrams/cli-call-graph.svg
```

FlexInfer workspace shortcut (when `../py-diagram-gen` exists):

```bash
../py-diagram-gen/.venv/bin/diagram-gen modules ./internal/parser -f svg -o docs/diagrams/parser-modules.svg
../py-diagram-gen/.venv/bin/diagram-gen modules ./pkg -f svg -o docs/diagrams/pkg-modules.svg
../py-diagram-gen/.venv/bin/diagram-gen calls ./cmd/fi-fhir --lang go --entry main --depth 3 -f svg -o docs/diagrams/cli-call-graph.svg
```

