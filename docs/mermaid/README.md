# Conceptual diagrams

The SVGs are composed with the shared FlexInfer vector tooling from
[`narratives.yaml`](../diagrams/narratives.yaml). Edit that file to change the
copy or layout. The `.mmd` files are generated, portable Mermaid fallbacks with
the same nodes and connections; do not edit them separately.

| Figure | Shows |
| --- | --- |
| [Overview](overview-flow.svg) | Authoring and release, durable admission and delivery, then verification |
| [HL7v2 parsing](parsing-phases.svg) | Source Profile control of byte normalization, syntactic parsing, and semantic extraction |
| [CLI flows](cli-flow.svg) | Parsing followed by run/dry-run/simulate; the server is a separate runtime |
| [Mapping Studio](ui-mapping-flow.svg) | Five-stage journey, connection/definition/Operator work, and shared workspace tools |

```bash
make docs-mermaid    # These four SVGs plus their Mermaid fallbacks
make docs-diagrams   # These plus the three generated Go architecture views
```

See [tooling and prerequisites](../diagrams/README.md#regenerate-all-diagrams).
The renderer uses shared light-theme tokens, fixed diagram geometry, native SVG
text, and explicit backgrounds. Mermaid auto-layout does not determine the
committed SVG layout.
