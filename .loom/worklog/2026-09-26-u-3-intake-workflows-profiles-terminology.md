### 2026-09-26 - U-3: intake, workflows, profiles, terminology

- What changed:
  - **HL7 intake (`/hl7`)**: Toolbar "HL7 intake" (redaction Select · Load
    file · Preview · Process; keyboard hints in button titles), one row of
    24 px pipeline chips (source · profile · workflow · destination, a state
    dot each, click opens the owning Results tab), and an editor-first
    resizable split (shell `SplitPane`, unchanged): CodeEditor fills the left
    pane, Results on the right. Deleted the hero, the 01/02/03 stepper and the
    four-card flow diagram. The chips show only what the last preview or
    process actually used (preview artifact revisions and planned deliveries);
    the diagram they replace showed invented defaults ("Default (Strict)",
    "Auto-Route", "FHIR Storage", a hard-coded "Standard ADT").
    `SessionRunProgress` is a one-line status (Badge + stages) that keeps the
    "Server preview progression" region, `state-*` class and "Preview
    complete"; `SessionStreamNotice` still renders `StreamingUnavailable`
    unchanged. Results tabs (Samples, Warnings, Events, Extraction,
    Inspector, Profile draft, Process, Live events) are dense tables; the
    quality and extraction panels lost their gradients.
  - **Workflows**: Toolbar tabs Inventory · Design · Verification. Inventory
    is a table + details pane (publish, rollback, run an event); Design is a
    Definition panel on a 2-column Field grid + routes + an aside (managed
    version, readiness, version history table, compare); Verification is
    Live stream (honest `workflowEvents` state) + run diagnostics table +
    approval queue table. Dry run uses the Tabs primitive; the Session source
    still appears only with the session engine on.
  - **Profiles**: Toolbar (Builder · YAML · Revisions, New, Duplicate,
    Delete, "Review & publish") over a profile table and a details workspace.
    `ProfileSelector` gains `layout="table"`; the default select layout still
    serves HL7 intake's Profile draft tab. `profileStore` maps `updatedAt`
    and `createdBy`.
  - **Terminology**: Toolbar tabs Browse · Upload · Review (count) · Resolver ·
    Workflows; Browse and Review are filters + table + details pane.
  - Deleted `AuthoringFlowRail`, `authoringFlow.ts` and `LifecycleTrace`
    (no importers left).
- Why: `.loom/37` U-3 — the four authoring routes read as a toy (hero +
  stepper + explainer cards, editor ~100 px tall on `/hl7` at 900 px).
- Evidence:
  - `npx vitest run`: 874 passed / 3 skipped (main after U-0: 864 passed);
    `npm run check` 0 errors (5 pre-existing unused-selector warnings in
    `WarningList.svelte`); eslint, stylelint, tsc clean; production build OK.
  - e2e check 3 and 6b green against local dev stacks (sessions on / off);
    `make ui-e2e` result and before/after PNGs (1440×900) in MR !234.
- What's next: coordinator review of !234.
- Findings for other lanes:
  - Legacy-syntax (`export let`) components do not re-run a template
    expression when state read *inside a called function* changes, nor when
    a `SvelteSet` is mutated from `$:`. Found and fixed four instances
    (workflow readiness, inventory env summary, terminology review bulk
    selection, sample inbox bulk selection); look for the same pattern when
    restyling other legacy components.
  - `/terminology` needs `FI_FHIR_TERMINOLOGY_DB_URL` plus
    `fi-fhir terminology init` on the local stack, or Browse shows an
    honest "GraphQL request failed".
  - `make ui-e2e` names its containers after `id -un`, so two lanes on the
    same docker host collide; a PATH shim for `id` gives a distinct name.
- Sources:
  - [S1] `.loom/37-ide-design-uplift-execution-specs.md` (U-3)
  - [S2] `ui/docs/DESIGN.md`
