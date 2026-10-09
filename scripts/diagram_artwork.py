"""Documentation artwork built with the shared FlexInfer vector and token APIs."""

import html
import json
import math
import tempfile
import xml.etree.ElementTree as ET
from pathlib import Path

import networkx as nx
from diagram_gen.models import Diagram, DiagramType, OutputFormat
from sprite_kit import Color, SpriteBuilder, SVGExporter
from visual_kit import tokens

SVG = "http://www.w3.org/2000/svg"
ET.register_namespace("", SVG)
THEME = tokens.diagram["light"]
BRAND = tokens.palettes["brand"]
WIDTH, MARGIN, GAP = 1120, 48, 28


def mix(color, background, amount):
    a, b = (
        tuple(int(c[i : i + 2], 16) for i in (1, 3, 5)) for c in (color, background)
    )
    return "#" + "".join(
        f"{round(x * amount + y * (1 - amount)):02x}" for x, y in zip(a, b)
    )


ACCENTS = {
    "blue": BRAND["primary"],
    "purple": BRAND["secondary"],
    "teal": mix(BRAND["green"], THEME.fg, 0.48),
    "slate": THEME.muted,
}


class Sheet:
    """A static, accessible SVG with a shared typographic and spacing system."""

    def __init__(self, name, title, subtitle, height, footer):
        self.name, self.title, self.subtitle = name, title, subtitle
        self.height, self.footer = height, footer
        self.builder = SpriteBuilder(name, WIDTH, height, "icon")
        self.builder.define_arrow_marker("arrow", THEME.muted, size=5)
        self.builder.layer("paper").rect(0, 0, WIDTH, height, fill=THEME.bg)
        self.rect(0.5, 0.5, WIDTH - 1, height - 1, stroke=THEME.border, rx=16)
        self.rect(MARGIN, 28, 26, 4, fill=BRAND["primary"], rx=2)
        self.text(84, 34, "FI-FHIR  /  ENGINEERING", 11, THEME.muted, bold=True)
        self.text(MARGIN, 75, title, 29, bold=True)
        self.text(MARGIN, 103, subtitle, 16, THEME.muted)
        self.builder.layer("connections")
        self.embeds = []

    def text(self, x, y, value, size=16, color=None, bold=False, anchor="start"):
        self.builder.text(
            x,
            y,
            value,
            font_size=size,
            fill=color or THEME.fg,
            font_family="Arial, Helvetica, sans-serif",
            font_weight="bold" if bold else "normal",
            text_anchor=anchor,
        )

    def rect(self, *args, **kwargs):
        # SpriteBuilder supplies ry=0 by default; SVG then squares off rx corners.
        kwargs["ry"] = kwargs.get("rx", 0)
        self.builder.rect(*args, **kwargs)

    def line(self, points, dashed=False, arrow=True):
        d = "M " + " L ".join(f"{x:g} {y:g}" for x, y in points)
        self.builder.path(
            d,
            stroke=Color.from_hex(THEME.muted if arrow else THEME.border),
            stroke_width=1.5 if arrow else 1,
            marker_end="arrow" if arrow else None,
            dash="5 5" if dashed else None,
        )

    def card(self, node):
        x, y, w, h = (node[k] for k in ("x", "y", "w", "h"))
        accent = ACCENTS[node.get("tone", "blue")]
        self.rect(
            x,
            y,
            w,
            h,
            fill=mix(accent, THEME.bg, 0.055),
            stroke=mix(accent, THEME.bg, 0.25),
            rx=9,
        )
        self.rect(x + 17, y + 16, 22, 3, fill=accent, rx=1.5)
        titles = node["title"].split("\n")
        for i, title in enumerate(titles):
            self.text(x + 17, y + 43 + i * 22, title, 18, bold=True)
        body_y = y + 68 + (len(titles) - 1) * 22
        for i, line in enumerate(node.get("body", [])):
            self.text(x + 17, body_y + i * 21, line, 15, THEME.muted)
        if node.get("body") and body_y + (len(node["body"]) - 1) * 21 > y + h - 12:
            raise ValueError(f"Text exceeds card height: {self.name}/{node['id']}")

    def band(self, band):
        accent = ACCENTS[band.get("tone", "slate")]
        self.rect(
            MARGIN,
            band["y"],
            WIDTH - 2 * MARGIN,
            68,
            fill=mix(accent, THEME.bg, 0.045),
            stroke=mix(accent, THEME.bg, 0.22),
            rx=9,
        )
        self.rect(MARGIN, band["y"] + 17, 3, 34, fill=accent, rx=1.5)
        self.text(MARGIN + 20, band["y"] + 28, band["title"], 17, bold=True)
        self.text(MARGIN + 20, band["y"] + 51, band["body"], 15, THEME.muted)

    def save(self, output):
        self.builder.layer("footnote")
        self.text(MARGIN, self.height - 24, self.footer, 14, THEME.muted)
        root = ET.fromstring(SVGExporter().export(self.builder.build()))
        root.set("role", "img")
        root.set("aria-labelledby", f"{self.name}-title {self.name}-desc")
        root.set("width", str(WIDTH))
        root.set("height", str(self.height))
        root.set("viewBox", f"0 0 {WIDTH} {self.height}")
        root.set("style", "max-width:100%;height:auto;background:#fff")
        title = ET.Element(f"{{{SVG}}}title", id=f"{self.name}-title")
        title.text = self.title
        desc = ET.Element(f"{{{SVG}}}desc", id=f"{self.name}-desc")
        desc.text = self.subtitle + " " + self.footer
        root.insert(0, desc)
        root.insert(0, title)
        for embed in self.embeds:
            root.append(embed)
        ET.indent(root, space="  ")
        Diagram(
            content=ET.tostring(root, encoding="unicode") + "\n",
            format=OutputFormat.SVG,
            diagram_type=DiagramType.ARCHITECTURE,
        ).to_svg(output)


def port(node, side, x=None):
    return (
        x if x is not None else node["x"] + node["w"] / 2,
        node["y"] if side == "top" else node["y"] + node["h"],
    )


def narrative(name, spec, directory):
    sheet = Sheet(name, spec["title"], spec["subtitle"], spec["height"], spec["footer"])
    nodes, edges = {}, []
    for row in spec["rows"]:
        weights = row.get("weights", [1] * len(row["nodes"]))
        available = WIDTH - 2 * MARGIN - GAP * (len(weights) - 1)
        x = MARGIN
        for node, weight in zip(row["nodes"], weights, strict=True):
            w = available * weight / sum(weights)
            nodes[node["id"]] = dict(
                node,
                x=x,
                y=row["y"],
                w=w,
                h=row["height"],
                tone=node.get("tone", row.get("tone", "blue")),
            )
            x += w + GAP
        if row.get("connect"):
            for a, b in zip(row["nodes"], row["nodes"][1:]):
                source, target = nodes[a["id"]], nodes[b["id"]]
                y = row["y"] + row["height"] / 2
                sheet.line([(source["x"] + source["w"], y), (target["x"] - 2, y)])
                edges.append((a["id"], b["id"], False, ""))
    for band in spec.get("bands", []):
        nodes[band["id"]] = dict(band, x=MARGIN, w=WIDTH - 2 * MARGIN, h=68)
    for link in spec.get("links", []):
        start = port(
            nodes[link["from"]], link.get("from_side", "bottom"), link.get("from_x")
        )
        end = port(nodes[link["to"]], link.get("to_side", "top"), link.get("to_x"))
        mid = link.get("via_y", (start[1] + end[1]) / 2)
        sheet.line(
            [start, (start[0], mid), (end[0], mid), end], link.get("dashed", False)
        )
        if link.get("label"):
            sheet.text(
                link["label_x"],
                mid - 9,
                link["label"],
                13,
                THEME.muted,
                anchor="middle",
            )
        edges.append(
            (link["from"], link["to"], link.get("dashed", False), link.get("label", ""))
        )
    sheet.builder.layer("content")
    for row in spec["rows"]:
        if row.get("label"):
            sheet.text(
                MARGIN,
                row.get("label_y", row["y"] - 19),
                row["label"],
                12,
                THEME.muted,
                bold=True,
            )
        if row.get("note"):
            sheet.text(
                WIDTH - MARGIN,
                row.get("note_y", row["y"] - 19),
                row["note"],
                13,
                THEME.muted,
                anchor="end",
            )
        for node in row["nodes"]:
            sheet.card(nodes[node["id"]])
    for band in spec.get("bands", []):
        sheet.band(band)
    output = directory / f"{name}.svg"
    sheet.save(output)
    # Keep a portable, generated Mermaid representation alongside the curated SVG.
    lines = [
        "%% Generated from docs/diagrams/narratives.yaml; do not edit.",
        "flowchart LR",
        f"  accTitle: {spec['title']}",
        f"  accDescr: {spec['subtitle']}",
    ]
    for key, node in nodes.items():
        body = node.get("body", [])
        parts = [node["title"].replace("\n", " ")] + (
            body if isinstance(body, list) else [body]
        )
        label = "<br/>".join(html.escape(p, quote=True) for p in parts)
        lines.append(f'  {key}["{label}"]')
    for source, target, dashed, label in edges:
        arrow = "-.->" if dashed else "-->"
        lines.append(f"  {source} {arrow}{'|' + label + '|' if label else ''} {target}")
    output.with_suffix(".mmd").write_text("\n".join(lines) + "\n")
    return output


def component_svg(graph, ids, name):
    lines = [
        "digraph G {",
        'graph [rankdir=LR, bgcolor="transparent", pad=0.04, nodesep=0.24, ranksep=0.42];',
        'node [shape=box, style="rounded,filled", fontname="Arial", fontsize=15, margin="0.18,0.14", penwidth=1];',
        f'edge [color="{THEME.muted}", arrowsize=0.55, penwidth=1.2];',
    ]
    for node_id in sorted(ids):
        label = node_id.removeprefix("pkg/").removeprefix("internal/parser/")
        accent = ACCENTS["teal" if node_id.startswith("pkg/") else "blue"]
        lines.append(
            f'{json.dumps(node_id)} [label={json.dumps(label)}, fontcolor="{THEME.fg}", '
            f'fillcolor="{mix(accent, THEME.bg, 0.055)}", color="{mix(accent, THEME.bg, 0.3)}"];'
        )
    for edge in graph.edges:
        if edge.source_id in ids and edge.target_id in ids:
            lines.append(
                f"{json.dumps(edge.source_id)} -> {json.dumps(edge.target_id)};"
            )
    lines.append("}")
    with tempfile.TemporaryDirectory() as temp:
        output = Path(temp) / "component.svg"
        Diagram(
            content="\n".join(lines),
            format=OutputFormat.DOT,
            diagram_type=DiagramType.MODULE,
        ).to_svg(output)
        svg = ET.parse(output).getroot()
    for element in svg.iter():
        if element.get("id"):
            element.set("id", f"{name}-{element.get('id')}")
    return svg


def packages(graph, name, title, subtitle, output):
    network = nx.Graph()
    network.add_nodes_from(graph.nodes)
    network.add_edges_from((e.source_id, e.target_id) for e in graph.edges)
    islands = sorted(nx.isolates(network))
    components = sorted(
        (c for c in nx.connected_components(network) if len(c) > 1),
        key=lambda c: (-len(c), sorted(c)),
    )
    rendered = [
        component_svg(graph, c, f"{name}-{i}") for i, c in enumerate(components)
    ]
    # Pack actual connected components without adding semantic or invisible import edges.
    remaining = []
    for svg in rendered:
        _, _, w, h = map(float, svg.get("viewBox").split())
        scale = min(1.16, (WIDTH - 2 * MARGIN - 40) / w)
        remaining.append((svg, max(300, w * scale + 40), h * scale + 40, scale, w, h))
    placements, y = [], 142
    while remaining:
        row = [remaining.pop(0)]
        used = row[0][1]
        for item in list(remaining):
            if used + 18 + item[1] <= WIDTH - 2 * MARGIN:
                row.append(item)
                remaining.remove(item)
                used += 18 + item[1]
        extra = (WIDTH - 2 * MARGIN - used) / len(row)
        row_height = max(item[2] for item in row)
        x = MARGIN
        for svg, cell_w, cell_h, scale, w, h in row:
            cell_w += extra
            placements.append((svg, x, y, cell_w, row_height, scale, w, h))
            x += cell_w + 18
        y += row_height + 18
    y += 6
    island_height = (math.ceil(len(islands) / 4) * 40 + 42) if islands else 0
    sheet = Sheet(
        name,
        title,
        subtitle,
        math.ceil(y + island_height + 52),
        "Arrows mean imports. Standard-library and third-party dependencies are omitted.",
    )
    sheet.builder.layer("components")
    for svg, x, top, w, h, scale, graph_w, graph_h in placements:
        sheet.rect(x, top, w, h, fill=THEME.panel, stroke=THEME.border, rx=11)
        svg.set("x", str(x + (w - graph_w * scale) / 2))
        svg.set("y", str(top + (h - graph_h * scale) / 2))
        svg.set("width", str(graph_w * scale))
        svg.set("height", str(graph_h * scale))
        sheet.embeds.append(svg)
    if islands:
        sheet.text(
            MARGIN,
            y + 16,
            "NO REPOSITORY IMPORTS IN THIS VIEW",
            12,
            THEME.muted,
            bold=True,
        )
        for i, node_id in enumerate(islands):
            xx, yy = MARGIN + (i % 4) * 258, y + 34 + (i // 4) * 40
            sheet.rect(xx, yy, 240, 31, fill=THEME.panel2, rx=5)
            sheet.text(xx + 12, yy + 21, node_id.removeprefix("pkg/"), 15)
    sheet.save(output)


def commands(entries, output):
    groups = [
        (
            "Parse & shape",
            "blue",
            ["parse", "validate", "profile", "companion", "fhir"],
        ),
        (
            "Execute & operate",
            "purple",
            ["workflow", "serve", "delivery", "lifecycle", "subscription"],
        ),
        (
            "Data & support",
            "teal",
            [
                "eventstore",
                "projection",
                "terminology",
                "storage",
                "etl",
                "config",
                "llm",
            ],
        ),
    ]
    if {name for _, _, names in groups for name in names} != set(entries):
        raise ValueError(
            "CLI dispatch changed: update the command groupings before regenerating"
        )
    sheet = Sheet(
        "cli-call-graph",
        "CLI command dispatch",
        "The entry point resolves each command to its Go handler, across the entire CLI package.",
        820,
        "Extracted from main's command switch. Help/version and handler internals are omitted.",
    )
    sheet.builder.layer("dispatch")
    sheet.rect(444, 132, 232, 50, fill=THEME.panel2, stroke=THEME.border, rx=8)
    sheet.text(560, 163, "main · os.Args[1]", 18, bold=True, anchor="middle")
    for i, (title, tone, names) in enumerate(groups):
        x, center = MARGIN + i * 352, MARGIN + i * 352 + 160
        sheet.line([(560, 182), (560, 206), (center, 206), (center, 232)])
        sheet.rect(x, 232, 320, 538, fill=THEME.panel, stroke=THEME.border, rx=10)
        sheet.rect(x, 232, 320, 5, fill=ACCENTS[tone], rx=2)
        sheet.text(x + 18, 268, title, 19, bold=True)
        for j, command in enumerate(names):
            handler, path, line = entries[command]
            y = 302 + j * 64
            sheet.text(x + 18, y, command, 16, bold=True)
            sheet.text(x + 302, y, handler, 15, THEME.muted, anchor="end")
            sheet.text(x + 18, y + 22, f"{path.name}:{line}", 12, THEME.muted)
            if j < len(names) - 1:
                sheet.line([(x + 18, y + 35), (x + 302, y + 35)], arrow=False)
    sheet.save(output)
