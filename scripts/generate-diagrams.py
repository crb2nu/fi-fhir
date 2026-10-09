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
import os
import sys
import xml.etree.ElementTree as ET
from pathlib import Path

sys.dont_write_bytecode = True


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


def cli_commands():
    """Resolve top-level CLI dispatch using a package-wide function symbol table."""
    from diagram_gen.analyzers.go import GoAnalyzer

    analyzer = GoAnalyzer()
    functions = {}
    for path in sorted(Path("cmd/fi-fhir").glob("*.go")):
        if path.name.endswith("_test.go"):
            continue
        tree = analyzer.parser.parse(path.read_bytes())
        for node in tree.root_node.named_children:
            if node.type == "function_declaration":
                name = node.child_by_field_name("name").text.decode()
                if name in functions:
                    raise ValueError(f"Ambiguous CLI function: {name}")
                functions[name] = (node, path)

    def walk(node):
        yield node
        for child in node.named_children:
            yield from walk(child)

    switches = [
        node
        for node in walk(functions["main"][0])
        if node.type == "expression_switch_statement"
        and (value := node.child_by_field_name("value")) is not None
        and value.text == b"os.Args[1]"
    ]
    if len(switches) != 1:
        raise ValueError("Expected one main command switch")
    commands = {}
    for case in switches[0].named_children:
        if case.type != "expression_case":
            continue
        literals = [
            node.text.decode().strip('"')
            for node in case.named_children[0].named_children
        ]
        if "help" in literals or "version" in literals:
            continue
        calls = {
            node.child_by_field_name("function").text.decode()
            for node in walk(case)
            if node.type == "call_expression"
            and node.child_by_field_name("function").type == "identifier"
        }
        handlers = calls & functions.keys()
        if len(handlers) != 1:
            raise ValueError(f"Ambiguous command handler: {literals}: {handlers}")
        handler = handlers.pop()
        declaration, path = functions[handler]
        for command in literals:
            commands[command] = (handler, path, declaration.start_point[0] + 1)
    return commands


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

    import yaml
    from diagram_artwork import commands, narrative, packages
    from diagram_gen import DiagramGenerator

    root = Path(__file__).resolve().parents[1]
    os.chdir(root)
    outputs = []
    if args.only in ("all", "mermaid"):
        specs = yaml.safe_load(Path("docs/diagrams/narratives.yaml").read_text())
        for name, spec in specs.items():
            outputs.append(narrative(name, spec, Path("docs/mermaid")))

    if args.only in ("all", "code"):
        for source, name, title, subtitle in (
            (
                "internal/parser",
                "parser-modules",
                "Format adapters & shared contracts",
                "Blue nodes: internal/parser packages. Green nodes: shared pkg packages.",
            ),
            (
                "pkg",
                "pkg-modules",
                "Public package dependencies",
                "Connected components reveal the actual imports. All paths are relative to pkg/.",
            ),
        ):
            graph = repository_packages(
                DiagramGenerator(source, language="go").analyze()
            )
            output = Path("docs/diagrams") / f"{name}.svg"
            packages(graph, name, title, subtitle, output)
            outputs.append(output)
        output = Path("docs/diagrams/cli-call-graph.svg")
        commands(cli_commands(), output)
        outputs.append(output)

    for output in outputs:
        svg = ET.parse(output).getroot()
        if not svg.get("viewBox") or len(svg) == 0:
            raise ValueError(f"Empty diagram: {output}")
        ids = [node.get("id") for node in svg.iter() if node.get("id")]
        if len(ids) != len(set(ids)):
            raise ValueError(f"Duplicate SVG identifiers: {output}")
        if any(
            node.tag.rsplit("}", 1)[-1] in {"script", "foreignObject"}
            for node in svg.iter()
        ):
            raise ValueError(f"Non-static SVG content: {output}")
        print(f"Generated {output}")


if __name__ == "__main__":
    main()
