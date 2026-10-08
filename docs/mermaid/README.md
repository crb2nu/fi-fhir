# Conceptual diagrams

Mermaid sources (`.mmd`) describe the system and user journeys. Their generated
SVGs are committed so GitLab and GitHub render the same illustrations.

| Source / output | Shows |
| --- | --- |
| `overview-flow.mmd` / `.svg` | Authoring, immutable release, durable admission, delivery, and verification |
| `parsing-phases.mmd` / `.svg` | Profile-driven HL7v2 byte, syntactic, and semantic phases; other adapters use format-specific warning phases |
| `cli-flow.mmd` / `.svg` | Parse, workflow run/dry-run/simulate, and serve |
| `ui-mapping-flow.mmd` / `.svg` | Saved sessions, five-stage workspace, connection/definition authoring, Operator, and capability-aware API access |

From the repository root:

```bash
make docs-mermaid    # These four diagrams
make docs-diagrams   # These plus all generated Go architecture diagrams
```

See [tooling prerequisites](../diagrams/README.md#regenerate-all-diagrams).
The shared library's `Diagram.to_svg` renders through the pinned Mermaid CLI;
`config.json` sets a shared palette, native SVG text, and deterministic IDs.
Update the `.mmd` source first, then regenerate and inspect the SVG.
