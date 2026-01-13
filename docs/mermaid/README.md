# Mermaid Diagrams (Pre-rendered)

GitLab's Mermaid renderer is stricter than upstream Mermaid and can reject otherwise-valid diagrams.
To keep READMEs reliable, `fi-fhir` stores Mermaid sources (`.mmd`) and commits pre-rendered SVGs.

## Files

- `overview-flow.mmd` → `overview-flow.svg`: end-to-end dataflow (inputs → parsing → events → workflow → outputs)
- `parsing-phases.mmd` → `parsing-phases.svg`: the 3-phase parsing pipeline
- `cli-flow.mmd` → `cli-flow.svg`: CLI pipe (parse → workflow)

## Regenerate

Requires Node.js.

```bash
make docs-mermaid
```

