# MCP Inventory

## Why

Capture runtime MCP capabilities and constraints so planning and implementation can rely on verified tool surfaces.

## Runtime Mode Detection

- `functions.list_mcp_resources` returned no generic resources.
- `functions.list_mcp_resource_templates` returned loom-mode templates (`loom://config`, `loom://servers`, `loom://tools/index`, paged tools URIs).
- Conclusion: this session is loom-mode-capable even though top-level resource listing is empty.

## Inventory Snapshot

### Loom Config

- Active profile: `full`
- Managed servers: `44`
- Aggregated tools: `456`
- Daemon status indicates running local target with active proxy sessions.

### Loom Servers and Health

- `loom://servers` confirms broad server catalog (ops, code, search, observability, AI, storage, SCM).
- `loom://health` indicates generally healthy local targets, with monitor-side transient init/read errors noted for `substack` and `neo4j` while still marked healthy.

### Tool Distribution (CLI fallback summary)

Top tool counts by server from `loom tools list --json` (grouped by server prefix):

- `agent_context`: 80
- `jobsearch`: 66
- `gitlab`: 30
- `flexinfer`: 19
- `codebase_memory`: 17
- `github`: 11
- `git`: 11
- `devbox`: 11

## Codebase Index Readiness

- `codebase_memory__codebase_stats(repo_id="fi-fhir")` reports `total_chunks: 0`.
- Index attempt (`job_id: 4f93c59a0acaa0a1`) remained `running` with `files_total: 0` and `files_done: 0`; canceled.
- Planning/research therefore relies on shell + source-file evidence (`rg`, `nl`, `git log/show`) rather than semantic codebase search.

## Constraints

- Loom resources are available through explicit `read_mcp_resource` URIs, not through generic resource listing.
- Codebase-memory indexing is currently non-functional for this repo in this environment.

## Best Tool For Job Notes

- Commit/feature analysis: `git log`, `git show --name-only`, `awk` summaries.
- Precise implementation evidence: `nl -ba` + `rg -n` in target files.
- Cross-repo contract mapping: direct file inspection in sibling service repos.
- MCP/agent inventory checks: `read_mcp_resource(loom://...)` + `loom tools list --json`.

## Sources

- [S1] Tool output: `functions.list_mcp_resources` → `{\"resources\":[]}`
- [S2] Tool output: `functions.list_mcp_resource_templates` → loom URI templates present
- [S3] Tool output: `functions.read_mcp_resource(server=\"loom\", uri=\"loom://config\")` (44 servers, 456 tools, active profile `full`)
- [S4] Tool output: `functions.read_mcp_resource(server=\"loom\", uri=\"loom://servers\")`
- [S5] Tool output: `functions.read_mcp_resource(server=\"loom\", uri=\"loom://tools/index\")` (`totalTools=456`, `totalPages=5`)
- [S6] Tool output: `functions.read_mcp_resource(server=\"loom\", uri=\"loom://health\")`
- [S7] Command: `loom tools list --json | jq -r '.tools[]?.name' | awk -F'__' '{print $1}' | sort | uniq -c | sort -nr`
- [S8] Tool output: `functions.mcp__loom__codebase_memory__codebase_stats(repo_id=\"fi-fhir\")` (`total_chunks=0`)
- [S9] Tool outputs: `codebase_index_start/poll/cancel` for `job_id=4f93c59a0acaa0a1` (stuck at 0 files)
