# Implementation Plan: IDE Connectivity & Feedback Loop

## Objective
To close the loop between production telemetry and IDE authoring, enabling users to fix terminology gaps and debug workflows with live production signals.

## Proposed Slices

### Slice A: Visual Workflow Debugging (MVP 3)
*Goal: Visualize matched/skipped routes directly in the builder.*
- **Task A1**: Update `DryRunPanel.svelte` to bubble up results to the parent `WorkflowBuilder`.
- **Task A2**: Enhance `WorkflowBuilder.svelte` to apply conditional styling (e.g., `is-matched`, `is-skipped`) to routes based on dry-run output.
- **Task A3**: Add "Evaluation Tooltips" to show exactly why a CEL rule matched or failed.

### Slice B: Terminology Coverage Dashboard (MVP 4)
*Goal: Surface unmapped codes seen in production.*
- **Task B1**: Create `UnmappedCodesWidget.svelte` in `lib/features/dashboard`.
- **Task B2**: Bridge `eventStatistics` with the mapping status to identify high-volume gaps.
- **Task B3**: Add "Quick Resolve" actions that jump the user to the `AutorouteResolver`.

### Slice C: Profile Lifecycle Management (MVP 2)
*Goal: Safety rails for profile changes.*
- **Task C1**: Implement "Draft" detection in `profileStore`.
- **Task C2**: Create a `ProfileDiffModal.svelte` to show changes between the local draft and the published version.
- **Task C3**: Add "Publish" workflow with mandatory change summaries.

### Slice D: Inline AI Fixes (UX)
*Goal: Reduce friction for common triage tasks.*
- **Task D1**: Update `WarningList.svelte` to support specialized action buttons per warning code.
- **Task D2**: Implement "Resolve with AI" inline button for terminology warnings (W042, E099).
- **Task D3**: Trigger `resolveMapping` and show a confirmation toast/popover.

## Key Files
- `ui/src/lib/features/workflows/components/WorkflowBuilder.svelte`
- `ui/src/lib/features/dashboard/DashboardStats.svelte`
- `ui/src/lib/features/profiles/ProfilesPage.svelte`
- `ui/src/lib/ui/WarningList.svelte`

## Verification Plan
- **Unit Tests**: Test the logic for matching dry-run results to route IDs.
- **Visual Check**: Verify highlighting in the Workflow list.
- **Manual Triage**: Simulate a terminology warning and fix it using the inline AI action.
