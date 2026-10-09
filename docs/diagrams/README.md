# Architecture diagrams

All seven SVGs use the workspace's `py-diagram-gen`, `py-sprite-kit`, and
`py-visual-kit` libraries. They share a solid background, native SVG text,
consistent typography, and an accessible title and description.

| Diagram | Source |
| --- | --- |
| [Parser packages](parser-modules.svg) | Actual imports in `internal/parser/` and its shared repository packages |
| [Public packages](pkg-modules.svg) | Actual imports in `pkg/`, packed by connected component |
| [CLI command dispatch](cli-call-graph.svg) | The command switch in `cmd/fi-fhir/main.go`, resolved against functions across the CLI package |

Package arrows mean imports. Standard-library and third-party dependencies are
omitted; packages with no connections in the displayed graph have a separate
inventory. The renderer does not add edges to arrange the layout.

The CLI figure shows top-level command dispatch, not a complete call graph or a
runtime trace. It intentionally omits help/version and handler internals. A new
command fails generation until its presentation group is updated, so cross-file
handlers cannot silently disappear from the figure.

## Regenerate all diagrams

From the repository root:

```bash
make docs-diagrams
```

This regenerates the three code views above and all four
[conceptual diagrams](../mermaid/README.md), including the README illustrations.

- [`narratives.yaml`](narratives.yaml) owns conceptual copy, rows, bands, and links.
- [`generate-diagrams.py`](../../scripts/generate-diagrams.py) runs shared Go analysis
  and validates the generated SVGs.
- [`diagram_artwork.py`](../../scripts/diagram_artwork.py) composes the shared vector
  primitives and design tokens. Graphviz routes actual package-import edges.
- Mermaid files are generated fallbacks, not a second editable source.

Generation rejects empty SVGs, duplicate IDs, non-static content, overflowing
card heights, and ungrouped CLI commands. Review the rendered output at README
width before committing; these checks do not replace a visual review.

Prerequisites:

- `uv` and Python 3.11–3.13; `uv` resolves the script's declared dependencies.
- Workspace checkouts of `py-diagram-gen`, `py-sprite-kit`, and `py-visual-kit`
  under `~/workspace/libs`, or set `DIAGRAM_LIBS=/path/to/workspace/libs`.
- Graphviz (`dot`) for the package diagrams.

The SVG generator no longer requires Node.js, Mermaid CLI, or Chrome.

```bash
make docs-mermaid                         # Conceptual SVGs and Mermaid fallbacks
uv run scripts/generate-diagrams.py --only code
```

Commit the editable sources and generated artifacts together. Keep native text
and the solid background so the diagrams remain crisp and readable in both
GitHub and GitLab themes.
