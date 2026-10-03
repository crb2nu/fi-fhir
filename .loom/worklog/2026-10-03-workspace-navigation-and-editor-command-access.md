### 2026-10-03 - Workspace navigation and editor command access

- What changed: editor tabs retain the latest supported session/record selectors;
  activity-bar and Go to commands resume the same location, and closing a tab
  returns to its neighbour's record. Arbitrary query parameters are not saved.
  Cmd/Ctrl+K works from inputs and CodeMirror, preserves the query on repeated
  use, and leaves other modals alone. At 390 px the current stage uses its number
  so Commands and Theme stay in the viewport. The user guide documents scope.
- Why: the user prioritized overall navigation and daily workflow after the IDE
  unlock. Returning to a tab previously lost its deep link, and editing blocked
  the global command shortcut.
- Evidence: 1,200 UI tests passed, three existing tests skipped; 264 focused
  shell/primitive tests passed; Svelte check (zero errors/warnings), production UI
  build, changed-file ESLint, StageControl stylelint, e2e type-check and docs
  validation passed. Browser verification exercised editor focus restoration,
  unchanged synthetic content, session-link retention and a 390 px header.
  The PostgreSQL-backed shell browser suite is recorded with the merge request.
- What's next: connection-link resolution, connection edit-buffer protection,
  remaining dialog/tab accessibility findings, and bounded journey reads remain
  separate items in ROADMAP. Form state and filters are not retained by this fix.
- Sources:
  - [S1] `.loom/42-ide-unlock-execution-specs.md`
  - [S2] `ui/src/lib/ui/ide/IDEShell.svelte`, `ideStore.ts`, `StageControl.svelte`
  - [S3] `ui/e2e/shell.spec.ts` (E4-3 and E4-4)
