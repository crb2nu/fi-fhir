# Architecture diagrams

The SVGs in this directory are generated from the current Go source with the
workspace's `py-diagram-gen`, `py-sprite-kit`, and `py-visual-kit` libraries.

| Diagram | Source |
| --- | --- |
| [Parser packages](parser-modules.svg) | `internal/parser/`, including HL7v2, CSV, EDI, CDA, and FHIR |
| [Public packages](pkg-modules.svg) | `pkg/` and its repository imports |
| [CLI call graph](cli-call-graph.svg) | Static calls from `cmd/fi-fhir`'s `main`, to depth 3 |

These are static-analysis views, not runtime traces. The call graph only shows
calls the Go analyzer can resolve; dynamic dispatch and calls across files may
be absent. Package diagrams resolve imports to their repository package paths and omit
standard-library and third-party dependencies.

## Regenerate all diagrams

From the repository root:

```bash
make docs-diagrams
```

This regenerates these three diagrams and all four conceptual diagrams in
[`docs/mermaid/`](../mermaid/README.md), including the README illustrations.
The source of the command is [`scripts/generate-diagrams.py`](../../scripts/generate-diagrams.py).
It validates that each SVG is nonempty and well-formed. The shared Go analyzer
and Graphviz renderer produce compact package and call graphs; the Mermaid
renderer produces the conceptual diagrams.

Prerequisites:

- `uv` and Python 3.11–3.13; `uv` resolves the script's declared dependencies.
- Node.js and npm; the Makefile pins Mermaid CLI to 11.12.0.
- Workspace checkouts of `py-diagram-gen`, `py-sprite-kit`, and `py-visual-kit`
  under `~/workspace/libs`, or set `DIAGRAM_LIBS=/path/to/workspace/libs`.
- Graphviz (`dot`) for the code diagrams.
- Chrome for Mermaid rendering. The CLI normally installs its browser. To use
  an existing browser, set `PUPPETEER_SKIP_DOWNLOAD=true` and
  `PUPPETEER_EXECUTABLE_PATH` to its executable before running the command.

To regenerate only the code diagrams:

```bash
uv run scripts/generate-diagrams.py --only code
```

Review the rendered SVGs and commit them with their source changes. The
conceptual diagrams are curated descriptions; the package and call graphs are
rebuilt from code, so refresh both when the architecture changes.
