#!/usr/bin/env -S uv run --script
# /// script
# requires-python = ">=3.11,<3.14"
# dependencies = [
#   "pydantic>=2,<3", "networkx>=3,<4", "pillow>=11,<13", "pyyaml>=6,<7",
#   "tree-sitter>=0.25,<0.26", "tree-sitter-go>=0.25,<0.26",
#   "tree-sitter-python>=0.25,<0.26", "tree-sitter-javascript>=0.25,<0.26",
#   "tree-sitter-typescript>=0.23,<0.24", "tree-sitter-rust>=0.24,<0.25",
#   "tree-sitter-dockerfile>=0.2,<0.3", "tree-sitter-yaml>=0.7,<0.8",
# ]
# ///
"""Regenerate documentation SVGs with the FlexInfer workspace libraries."""

import argparse
import json
import os
import re
import sys
import xml.etree.ElementTree as ET
from pathlib import Path


def repository_packages(graph):
    """Resolve repository imports to package nodes and omit external dependencies."""
    from diagram_gen.models import EdgeType, Graph, NodeType

    module = next(
        line.split()[1]
        for line in Path("go.mod").read_text().splitlines()
        if line.startswith("module ")
    )
    nodes = {}
    aliases = {}
    for node_id, node in sorted(graph.nodes.items()):
        if node.type != NodeType.PACKAGE:
            continue
        if node.file_path:
            name = node.file_path.as_posix()
        elif node_id.startswith(f"package:external:{module}/"):
            name = node_id.removeprefix(f"package:external:{module}/")
        else:
            continue
        aliases[node_id] = name
        nodes[name] = node.model_copy(update={"id": name, "name": name})
    edges = {}
    for edge in graph.edges:
        source, target = aliases.get(edge.source_id), aliases.get(edge.target_id)
        if edge.type == EdgeType.IMPORTS and source and target and source != target:
            edges[source, target] = edge.model_copy(
                update={"source_id": source, "target_id": target}
            )
    return Graph(
        nodes=dict(sorted(nodes.items())), edges=[edges[key] for key in sorted(edges)]
    )


def save_code_diagram(diagram, output):
    # Graphviz packs large code graphs more tightly than the native card layout.
    diagram.content = diagram.content.replace(
        "digraph G {",
        """digraph G {
    graph [bgcolor="white", pad=0.3, nodesep=0.18, ranksep=0.5];
    node [style="rounded,filled", fillcolor="#e8f1fc", color="#5790be", fontcolor="#152f4b", fontsize=12];
    edge [color="#56718b", arrowsize=0.65];""",
    ).replace("style=rounded,", 'style="rounded,filled",')
    diagram.to_svg(output)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--libs", type=Path, default=Path.home() / "workspace/libs")
    parser.add_argument("--only", choices=["all", "mermaid", "code"], default="all")
    args = parser.parse_args()
    for library in ("py-diagram-gen", "py-sprite-kit", "py-visual-kit"):
        source = args.libs / library / "src"
        if not source.is_dir():
            parser.error(
                f"Missing {source}; set --libs to the workspace library directory"
            )
        sys.path.insert(0, str(source))

    from diagram_gen import DiagramGenerator
    from diagram_gen.models import (
        Diagram,
        DiagramConfig,
        DiagramType,
        EdgeType,
        NodeType,
        OutputFormat,
    )

    root = Path(__file__).resolve().parents[1]
    os.chdir(root)
    outputs = []
    if args.only in ("all", "mermaid"):
        config = json.loads(Path("docs/mermaid/config.json").read_text())
        for source in sorted(Path("docs/mermaid").glob("*.mmd")):
            config["deterministicIDSeed"] = source.stem
            diagram = Diagram(
                content="%%{init: " + json.dumps(config) + "}%%\n" + source.read_text(),
                format=OutputFormat.MERMAID,
                diagram_type=DiagramType.ARCHITECTURE,
            )
            output = source.with_suffix(".svg")
            diagram.to_svg(output)
            outputs.append(output)

    if args.only in ("all", "code"):
        config = DiagramConfig(format=OutputFormat.DOT, direction="LR")
        for source, name in (
            ("internal/parser", "parser-modules"),
            ("pkg", "pkg-modules"),
        ):
            generator = DiagramGenerator(source, config=config, language="go")
            diagram = generator.module_diagram(repository_packages(generator.analyze()))
            output = Path("docs/diagrams") / f"{name}.svg"
            save_code_diagram(diagram, output)
            outputs.append(output)
        generator = DiagramGenerator("cmd/fi-fhir", config=config, language="go")
        output = Path("docs/diagrams/cli-call-graph.svg")
        calls = generator.analyze().filter(
            node_types=[NodeType.FUNCTION, NodeType.METHOD], edge_types=[EdgeType.CALLS]
        )
        save_code_diagram(
            generator.call_graph(entry="main", depth=3, graph=calls), output
        )
        outputs.append(output)

    for output in outputs:
        # Repository hosts embed static images; omit the native renderer's pan/zoom script.
        content = re.sub(
            r"<script\b[^>]*>.*?</script>", "", output.read_text(), flags=re.S
        )
        # Mermaid splits words across tspans; preserve their separating spaces in SVG readers.
        content = content.replace("<svg ", '<svg xml:space="preserve" ', 1)
        svg = ET.fromstring(content)
        if not svg.get("viewBox") or len(svg) == 0:
            raise ValueError(f"Empty diagram: {output}")
        output.write_text(
            "\n".join(line.rstrip() for line in content.splitlines()) + "\n"
        )
        print(f"Generated {output}")


if __name__ == "__main__":
    main()
