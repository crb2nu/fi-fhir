# MCP Inventory

## Why

Capture the available MCP servers/resources/templates so planning and implementation can use the right tools without guesswork.

## Checklist

- [x] List MCP servers
- [x] List resource templates per server
- [x] List resources per server (if available)
- [x] Record any auth/permission constraints
- [x] Record “best tool for job” notes

## Inventory

### Servers

- `functions.list_mcp_resources` returned:
  - `{"resources":[]}`

### Resource Templates

- `functions.list_mcp_resource_templates` returned:
  - `{"resourceTemplates":[]}`

### Resources

- No MCP resources available in this session.

## Constraints

- No discoverable MCP resources/templates were available to seed context docs.
- Planning relied on repository-local evidence (files + commands).

## Best Tool For Job Notes

- For this planning pass, `functions.exec_command` + `rg`/`nl` provided the needed evidence.
- If MCP resources are later configured, refresh this file first and source plans from those resources where applicable.

## Sources

- [S1] Tool output: `functions.list_mcp_resources` → `{"resources":[]}`
- [S2] Tool output: `functions.list_mcp_resource_templates` → `{"resourceTemplates":[]}`
