# Toast Budget Policy (UX redesign §5.6)

**Status**: **Approved 2026-06-04** (decisions D1–D3 resolved, §6) — ready for implementation (Slice 6b-β2)
**Owner**: UI
**Scope**: notification policy for the fi-fhir IDE; classifies the **103** live `toasts.*`
call-sites and defines where each *kind* of message belongs.

> Source census (2026-06-04): `grep -rE "toasts\.(success|error|info|warning|add)" ui/src`
> → 103 sites: **71 error · 32 success · 14 add · 3 info · 1 warning** (the 14 `add` are the
> low-level form behind the variant helpers; counted once via their helper above).

---

## 1. The budget (rules)

A toast is a **transient, self-dismissing acknowledgement of a user-initiated action whose
result the user is actively awaiting.** That is the *whole* budget. Concretely:

| A toast is allowed when… | Mechanism |
|---|---|
| **R1 — Async result of an explicit action** the user just triggered and is waiting on (saved, published, exported, deleted, uploaded, rolled back, approved, mapping resolved), **including its failure** (network/server error from that same call). | `toasts.success` / `toasts.error` in the action's `then`/`catch` |
| **R2 — Transient confirmation** of a one-shot action with no other visible feedback (copied to clipboard, requested cancellation). | `toasts.success` |

A toast is **banned** for:

| Anti-pattern | Belongs instead in… |
|---|---|
| **B1 — Persistent validation state** ("name is required", "invalid JSON", "select two versions") — true until the user fixes it, so a 4-second toast is the wrong lifetime. | **inline** at the field, and/or the **Problems panel** (workflow drafts already have one) |
| **B2 — Precondition guards fired *after* a click** ("create or open a definition first", "start a debug session first"). | **disable the control** + tooltip; never let the dead click happen |
| **B3 — Background/system status** (connection, long-running progress). | **StatusBar** / inline progress |
| **B4 — Duplicate of a message another layer already shows** (global GraphQL error toast *plus* a component error toast for the same failure). | keep **one** layer only |

**North-Star tie-in** (§2 of the spec): "the default screen is quiet; signal earns attention."
Every banned case is a toast competing for attention that a calmer surface should own.

---

## 2. Riskiest assumption + kill-test

**Load-bearing assumption**: the messages we move *out* of toasts (B1 validation, B2
preconditions) have a real destination that already surfaces them — specifically, the
**Problems panel renders workflow-draft validation diagnostics**, so moving workflow
validation errors there (instead of toasting) does not lose them.

**Kill test** (≤15 min): open a managed workflow, introduce an invalid draft (e.g. empty
name), and confirm the existing `ProblemsPanel` lists the diagnostic without any toast.
`ProblemsPanel.svelte` + its tests already assert "renders structured diagnostics when the
workflow draft is invalid" — run that path live.

**Failure mode if wrong**: we delete validation toasts and the error becomes invisible
(silent failure) — strictly worse than a noisy toast. If the Problems panel does *not*
already receive a given validation error, that error stays inline at the field (B1's first
home) and is **not** removed until the inline home exists.

**Status**: **passed 2026-06-17** — `ProblemsPanel.svelte` now consumes the
`workflowDiagnostics` derived store and renders structured workflow-draft diagnostics
(invalid → "Workflow draft needs attention" + per-issue rows; valid → "Ready for runtime
verification"). The 2 `ProblemsPanel.test.ts` cases (the kill-test in unit form) load an
invalid draft and assert the diagnostics render **without any toast** — both green. The
destination home for B1 validation messages provably exists, so β2c toast-redirect is
unblocked. (Slice 6b-β2c-1; see §5c.)

---

## 3. Disposition of the 103 call-sites

### KEEP — R1/R2 async results & transient confirmations (~78)

These are the legitimate budget. No change beyond wording consistency.

- **All 32 `success`** — every one is an async result of an explicit action: `Saved workflow
  version v…`, `Published version to …`, `Mappings exported successfully`, `Mapping deleted`,
  `Uploaded N mappings`, `Approved mapping: …`, `Rolled back … in …`, `Workflow cancellation
  requested`, `Resolved mapping for …`, etc. (WorkflowBuilder, WorkflowList, MappingBrowser,
  PendingReviewList, MappingUploader, MappingEditor, TemporalWorkflowList, AutorouteResolver,
  HL7PreviewPage, GenerateFromDescription, WorkflowMonitor, WorkflowDraftLibrary).
- **`error` from a `catch` on a real async call** (network/server failure of the very action
  the user triggered) — e.g. WorkflowBuilder's `error(lifecycleError)` after a failed
  save/publish/approve mutation, WorkflowList publish/rollback `catch`, MappingBrowser
  export/delete `catch`, PendingReviewList approve/reject `catch`, MappingUploader upload
  `catch`, TemporalWorkflowList cancel/load `catch`, AutorouteResolver approve `catch`,
  HL7PreviewPage resolve `catch`. These are R1 failures — keep as error toasts.

### MOVE → inline / Problems panel — B1 persistent validation (~16)

Fired on a click but describe form/draft state that persists until fixed → show **inline at
the field** (preferred) or route workflow-draft issues to the **Problems panel**:

| Site | Message |
|---|---|
| WorkflowBuilder | `Workflow name is required` / `…is required to create a managed definition` (×2) |
| WorkflowBuilder | `Select two versions to compare` · `Choose two different versions` · `Select a workflow template first` (×3) |
| WorkflowBuilder | `Draft name must match managed definition name …` · snapshot/import name-match errors (×3) |
| WorkflowList | `Provide an event JSON payload first` · `Invalid JSON payload` · `Event payload must be a JSON object` (×3) · `Select a version to publish` · `Select a version to roll back to` (×2) |
| WorkflowDraftLibrary | `YAML has N validation issues` · `Failed to parse YAML` (×2) |
| DryRunPanel | `No events available for dry run` · `Invalid JSON for custom events` (×2) |
| AutorouteResolver | `Source code, source system, and target system are required` (×2) |
| MappingUploader | `Please select a CSV file` (×1) |

### MOVE → disabled control + tooltip — B2 preconditions (~6)

The triggering button should be **disabled** when the precondition isn't met, so the click
never happens and no toast is needed:

| Site | Message |
|---|---|
| WorkflowBuilder | `Create or open a managed workflow definition first` (**×4** — recurs across save/publish/snapshot/import) |
| DebugPanel | `Start a debug session before changing breakpoints` · `…before adding breakpoints` (×2, currently `info`) |

### CUT / dedupe — B4 duplicate layers (~3)

| Site | Issue | Action |
|---|---|---|
| `graphql/client.ts` | Global `Request failed: HTTP …` / `Network error: …` / `Unexpected response from server` fire on every failed request, often **alongside** a component-level `catch` toast for the same failure. A `markToasted(error)` dedupe flag already exists. | Keep the global net as the **single** source for *unhandled* errors; ensure component `catch` blocks that already toast call the dedupe (or drop the global one for handled paths). Net effect: no double-toasts. |

---

## 4. Estimated effect

| Metric | Before | After |
|---|---|---|
| Toast call-sites | 103 | ~78 (transient-only) |
| Persistent errors shown as toasts | ~22 | 0 (→ inline/Problems) |
| Precondition dead-click toasts | ~6 | 0 (→ disabled controls) |
| Double-toasts (global + component) | possible | 0 |

Success metric (spec §7): "Toast call-sites: categorized; transient-only." ✅ once applied.

---

## 5. Suggested implementation sub-slices (after approval)

1. **6b-β2a — non-color-state audit** (independent, low-risk; ships first). ✅ **SHIPPED**
2. **6b-β2b — B2 disabled-control preconditions** (WorkflowBuilder ×4, DebugPanel ×2) — small,
   high-value, removes dead clicks. ✅ **SHIPPED**
3. **6b-β2c — B1 validation → inline/Problems** (the ~16) — the largest; do per-feature
   (workflows first, since the Problems panel already exists there).
   - **β2c-1 — Problems-panel wiring (kill-test).** ✅ **SHIPPED** — see §5c. Builds the
     destination *before* removing any toast.
   - **β2c-2+ — toast redirect** (WorkflowBuilder / WorkflowList / WorkflowDraftLibrary /
     DryRunPanel / AutorouteResolver / MappingUploader). Now unblocked.
4. **6b-β2d — B4 graphql dedupe** — verify `markToasted` covers component-handled errors.

Each keeps tests green and ships behind its own MR per the slice discipline.

### 5c. Slice 6b-β2c-1 outcome (Problems-panel wiring = riskiest-assumption kill-test)

**Discovery:** the kill-test (§2) was failing. `ProblemsPanel.svelte` rendered the generic
cross-stage `diagnosticsStore`, which is **dead in production** — no adapter is ever called,
`addDiagnostic`/`addDiagnostics` are never invoked, and `+page.svelte` carries a "no
diagnosticsStore exists yet" placeholder comment. The only live validation signal is
`workflowDiagnostics` (derived from `workflowDraft` via `validateWorkflowDraft`), which had
**zero consumers** — an orphan bridge built speculatively for β2c. The 2 `ProblemsPanel.test.ts`
cases were unrendered future copy (RED), encoding the assumption rather than asserting it.

**Change (single file):** rewired `ProblemsPanel.svelte` to render `workflowDiagnostics`:

| Draft state | Renders |
|---|---|
| invalid | "Workflow draft needs attention" + `{n} error(s)` count + per-issue rows (`location` · `message`), each with a **text severity tag** (non-color, WCAG 1.4.1, consistent with β2a) |
| valid | "Ready for runtime verification" + "No blocking problems detected." + route/action/transform meta chips |

The generic `diagnosticsStore`/`diagnosticAdapters` are left untouched as dormant scaffolding
(still feed `BottomPanel`'s count badge); no toast was removed in this slice — the redirect is
β2c-2+. Validation: 2 target tests RED→GREEN; full UI suite 494 passed / 2 skipped;
svelte-check + eslint clean for the changed file.

**Follow-ups queued:** (a) `BottomPanel` problem-count badge still reads the always-empty
generic store — point it at `workflowDiagnostics` for coherence; (b) decide whether to retire
or wire the dormant cross-stage diagnostics scaffolding.

### 5b. Slice 6b-β2b outcome (disabled-control preconditions, B2/D2)

Verified each of the 6 precondition toasts against its actual trigger control. **Only the
2 DebugPanel sites were live dead-clicks**; the 4 WorkflowBuilder sites were already prevented:

| Site | Trigger control | Before β2b | β2b change |
|---|---|---|---|
| DebugPanel add breakpoint | `BreakpointList` add button (always enabled) | **live dead-click** → toast | `BreakpointList` gets `hasSession` prop; add button disabled + tooltip "Start a debug session to manage breakpoints"; handler also guards |
| DebugPanel toggle breakpoint | `BreakpointList` toggle checkbox (always enabled) | **live dead-click** → toast | checkbox disabled + tooltip when no session (remove stays enabled — it works without a session) |
| WorkflowBuilder save | Save button | already `disabled={!linkedWorkflowId ‖ !valid}` (no reason shown) | added explanatory `title` (D2's missing tooltip half) |
| WorkflowBuilder compare | Compare button | `disabled` on version-select only | added `!linkedWorkflowId` to disabled + explanatory `title` |
| WorkflowBuilder snapshot/import promote | `WorkflowDraftLibrary` push/import controls | already **hidden** via `pushToServerEnabled`/`promoteImportEnabled={!!linkedWorkflowId}` | none — control not rendered, toast unreachable |

`Button` needed no change — it already forwards `title` via `{...$$restProps}`, and
`.btn:disabled` has no `pointer-events:none`, so the tooltip shows on the disabled control.
The 6 toast guards remain as unreachable defensive backstops (cheap; cannot fire from the UI).
Tests: 7 new (BreakpointList session-gating ×6, Button title-forwarding ×1).

### 5a. Slice 6b-β2a outcome (non-color-state, WCAG 1.4.1)

Audited every status/severity/priority indicator under `ui/src` for state conveyed by
**color alone**. Two genuine gaps (state present *only* in a colored marker, no text/glyph):

| Component | Was | Now |
|---|---|---|
| `AlertBadge.svelte` | `aria-hidden` color-only severity **dot**; severity level appeared in no text or glyph | labeled **severity tag** (`Critical`/`Warning`/`Info`) — text + color + border, screen-reader announced |
| `TaskPanel.svelte` | color-only priority **dot**; priority appeared in no text on the card | visible **priority tag** (`Critical`/`High`/`Medium`/`Low`) in the meta row; dot is now decorative (`aria-hidden` + `title`) |

Helpers `severityLabel()` / `priorityLabel()` extracted into the respective stores
(co-located with their types, unit-tested). Regression coverage: 12 tests across
`observabilityStore`, `collaborationStore`, `AlertBadge`, `TaskPanel`.

**Reviewed and left as-is (not color-only):**
- `ProblemsPanel.svelte` — diagnostics are grouped under text section headers
  (`Errors`/`Warnings`/`Info`), so per-row severity is already conveyed by text; the colored
  strip is redundant. (β2c reworks this panel anyway.)
- `TemporalWorkflowList.svelte` — status badges already render `formatStatus()` text.
- `StatusBar.svelte` / `RuntimeOutputPanel.svelte` — connection dots sit beside adjacent
  status text labels.
- `PresenceBar.svelte` — parent carries `aria-label="{name}: {status}"` and non-compact mode
  shows the status word; **residual**: compact-mode dot is color-only for colorblind *sighted*
  users (AT-covered). Tracked as a low-priority follow-up, not blocking.

---

## 6. Decisions (resolved 2026-06-04)

- **D1 — RESOLVED: Problems panel + inline mix.** Draft-level / multi-issue validation →
  Problems panel (already exists for workflows); single-field "required" → inline at the field.
- **D2 — RESOLVED: disable control + tooltip.** When a precondition isn't met, the triggering
  button is disabled with an explanatory tooltip; the dead click never happens.
- **D3 — RESOLVED: global net, components defer.** Keep the `graphql/client.ts` error toast as
  the single safety net; component `catch` blocks rely on it (via `markToasted`) unless they add
  field-specific context.
