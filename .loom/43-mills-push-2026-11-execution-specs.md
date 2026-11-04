# Mills push, November 2026: execution specs (rows M-1..M-15)

> **Posted 2026-11-03.** Intake is direct `bl-fifhir-*` backlog rows with
> declared files (the GitLab-issue path and the roadmap carder are not used;
> decision 2026-11-03 "Mills push intake is direct rows"). Each row's full
> spec (Goal, Founding evidence, Scope) lives in the Mills store under its id;
> this file is the program view: order, dependencies, budget, the kill-test
> for the lane, and what stays with a person. Row state:
> `mills_backlog_list id_contains 202611`; the merge-train proof for every row
> is the `api`-source pipeline on its `mills-mq/train-libs-fi-fhir-*` ref
> (ROADMAP "Make the merge-train proof real", observed 2026-11-03).

> Brief (Cody, 2026-11-03): "continue building our roadmaps and plans to make
> a solid feature and improvement push for both projects through mills."
> Coordinator = this session; Mills implements; the coordinator reads the
> merge requests the lane holds for review (protected touches) and keeps the
> ROADMAP and this file true.

## What changed since `.loom/42`

The lane is real. On 2026-11-03 the `workflow:` never-rule from
[MR !288](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/288)
was proven: the first row since the rule,
[MR !290](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/290),
got no push pipeline on its train ref and the queue minted API pipeline
37258 (64 jobs, the gating set), where the four October rows had one
`lint:helm` job each. Protected paths now include `ci/**` (platform/gitops
!1049), so a row that edits a CI include pre-declares it and the merge
request waits for a person. The operator half of the proof fix is itself a
Mills row in loom-core (`bl-mills-mergequeue-train-proof-api-source-only-202611`).
Issues #22–#24 are closed; the Now list is the backlog.

## Design

### Shared contracts (every row)

- **One merge request per row**, files declared on the row; a changed file
  outside the declared set is admitted only under a declared directory.
  Root-level files (`Makefile`, `.env.example`, `.gitlab-ci.yml`) widen the
  envelope to the whole tree and serialize the row behind every other row
  that holds the root (the intake warns; accepted for M-3, M-7).
- **Protected touches are declared, not avoided**: `ci/test-structured-logging.yml`
  (M-3, M-8), `.gitlab-ci.yml` + `ci/job-inventory.txt` (M-4),
  `ci/lint-edi-fixtures.yml` (M-13). Those merge requests are held for human
  review by policy; the coordinator reads them.
- **Honesty rule** (`.loom/42` E-4/E-5): no surface offers an action the
  deployed API refuses; a missing composition is reported as "not configured
  on this deployment", never as an error toast.
- **Tests are the deliverable**: every row's Success.tests run in the Mills
  devbox before the merge request opens; UI rows run vitest and
  `svelte-check`, Go rows run the package tests, CI rows run the inventory
  check.
- **No row touches** `ROADMAP.md`, `CHANGELOG.md` (fragments only), `.loom/`,
  `deploy/**`, platform/gitops or `ci/_shared.yml`.

### Rows

| Row | Id | Theme | Declared files (abridged) | Accept |
|---|---|---|---|---|
| M-1 | `bl-fifhir-engine-tab-tracing-delivery-keys-202611` | telemetry | `cmd/fi-fhir/main.go`, `engine_runtime.go`, `engine_runtime_test.go`, `serve_tracing.go`, `docs/operations/CONNECTION-CATALOG.md` | the six keys reach `serveProperties()` through the help-text contract |
| M-2 | `bl-fifhir-telemetry-docs-drift-202611` | telemetry | `docs/operations/README.md`, `docs/user-guide/cli-reference.md`, `docs/operations/SECURITY.md` | no doc says SHA-256 where the digest is keyed; the key is in the CLI reference; SECURITY.md has a tracing section |
| M-3 | `bl-fifhir-tracing-negative-control-asserts-sentinel-202611` | telemetry | `Makefile`, `ci/test-structured-logging.yml`, the gate test, the two leak twins | both controls assert the sentinel failure; a wrong-reason failure FAILS the control |
| M-4 | `bl-fifhir-check-runtime-config-ci-job-202611` | CI | new `ci/lint-runtime-config.yml`, `.gitlab-ci.yml` (one line), `ci/job-inventory.txt` (78) | `lint:runtime-config` blocks on `check_tracing_defaults` |
| M-5 | `bl-fifhir-correlation-key-file-variant-and-format-202611` | telemetry | `cmd/fi-fhir/serve_tracing.go`, its test, `main.go`, `.env.example` | `_FILE` variant via the shared helper; key must decode (hex/base64) to ≥ 32 bytes |
| M-6 | `bl-fifhir-exec-action-test-flake-202611` | tests | `internal/workflow/exec_action_test.go` | `-count=20` green, no wall-clock floor |
| M-7 | `bl-fifhir-edi-fixtures-from-edilint-gen-202611` | fixtures | `testdata/edi/` (+ `defects/`), `internal/parser/edi/fixtures_test.go`, `parser_test.go`, `ci/lint-edi-fixtures.yml` | generated 837P/835 corpus, table-driven tests record the parser's real behavior on each defect; `Closes #15`; reports back to edilint |
| M-8 | `bl-fifhir-trace-successful-publish-webhook-sink-202611` | telemetry | the gate test, `tracing_gate_integration_test.go`, `serve_observability.go`, `ci/test-structured-logging.yml` (comment) | one successful `integration.delivery.dispatch` span exported beside the retry path |
| M-9 | `bl-fifhir-stale-capabilities-reprobe-202611` | IDE | `accessCapabilities.ts`, `AccessChip.svelte`, `intakeState.ts`, `connectionsAccess.ts`, `connectionsErrors.ts`, `docs/operations/INTEGRATION-SESSIONS.md` | capabilities re-probed after credential entry and on the legacy-execution refusal; the reload workaround is gone from the guide |
| M-10 | `bl-fifhir-terminology-store-capability-202611` | IDE | `internal/api/graphql/capabilities.go` (+test), `server.go`, `accessCapabilities.ts`, `TerminologyPage.svelte`, `MappingBrowser.svelte`, `docs/operations/CONNECTION-CATALOG.md` | `/api/auth/status` reports the terminology store; the page renders "not configured", no toast |
| M-11 | `bl-fifhir-workflows-run-event-refused-mutation-202611` | IDE | `WorkflowList.svelte`, `workflowProblemsStore.ts` (+test), `BottomPanel.test.ts`, `EventLineagePanel.svelte` (+new test) | no component calls `triggerWorkflow`; `markWorkflowBuilderOpened` gone; lineage panel tested |
| M-12 | `bl-fifhir-operator-bundle-e2e-stream-helper-202611` | e2e | `ui/e2e/operator-bundle.spec.ts`, `ui/e2e/support.ts` | check 3 on the two helpers; `test:ui-e2e` green |
| M-13 | `bl-fifhir-connection-rows-replica-counts-202611` | IDE | `ConnectionDetails.svelte`, the connections `.graphql` document, `gen/graphql.ts` | rows say "mounted on n of m" from `observedReplicas`/`totalReplicas` |
| M-14 | `bl-fifhir-shell-a11y-nits-e4-review-202611` | IDE | `Dialog.svelte` (+test), `Tabs.svelte` (+test), `IDEShell.svelte`, `BottomPanel.svelte` | focus fallback, Escape order, close buttons out of the tablist, dimmed Copilot says why |
| M-15 | `bl-fifhir-operator-deep-link-polish-202611` | IDE | `OperatorPage.svelte`, `operatorLinks.ts`, `DeliveryConsole.svelte`, `MessageTrace.svelte`, `operator.graphql`, `gen/graphql.ts` | deep link applied after the pre-flight; child links on resubmitted attempts |

M-1, M-2, M-3, M-4 were posted at 16:57 UTC; M-1 and M-3 were implementing
by 17:30. M-5..M-15 were posted between 18:10 and 18:40 UTC.

### Outcomes (as of 2026-11-04 14:35 UTC)

| Row | Merge request | Merge | Train | Note |
|---|---|---|---|---|
| M-1 | [MR !294](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/294) | `e52d39482` | 37464 | rescued: branch pushed, no MR after an `infra` escalation at `tests` |
| M-2 | [MR !301](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/301) | `a5aaa5141` | 37559 | Mills end to end |
| M-3 | [MR !292](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/292) | `1b812590c` | 37525 | rescued; evicted twice by infrastructure (`build:docker` go-mod flake, then the 120-minute clock) |
| M-4 | — | — | — | queued; re-grounded after a scope blocker (the new `ci/lint-runtime-config.yml` was not in the slice) |
| M-5 | [MR !295](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/295) | `d1e23bebf` | 37572 | rescued; the proof key in `tracing_gate_integration_test.go` had to become hex (see worklog 2026-11-04) |
| M-6 | [MR !300](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/300) | `ecc842c35` | 37566 | spec-touching hold reviewed and released |
| M-7 | [MR !299](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/299) | `252777436` | 37581 | Mills end to end; `Closes #15`, reports back to edilint's 834 decision |
| M-8 | [MR !296](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/296) | `4f9c71e51` | 37577 | spec-touching hold reviewed and released; one infrastructure eviction |
| M-9 | [MR !293](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/293) | `f9e3e1b9f` | 37418 | the kill-test row |
| M-10 | — | — | — | queued; requeued after a devbox checkout fault at `tests` |
| M-11 | [MR !297](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/297) | `2b57aa6bf` | 37547 | Mills end to end |
| M-12 | [MR !302](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/302) | `8e999185a` | 37562 | Mills end to end |
| M-13 | — | — | — | queued (P3) |
| M-14 | — | — | — | queued (P3) |
| M-15 | — | — | — | queued (P3) |

Beside the rows: [MR !290](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/290) and [MR !291](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/291) (coordinator docs), [MR !298](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/298)
(`1cc0769c4`, the Dockerfile `go mod download` retry loop, from a side
session), gitops !1050 (`build:docker` on the queue's flake-retry list) and
gitops !1051 (`await_pipeline_minutes` 120 → 180).

## Order, dependencies, budget

- The lane dispatches by priority then age; the budget overlay is 3 USD per
  run and 8 runs per day, so the fifteen rows pace over three to four days
  with retries. Nothing here raises the overlay.
- Dependencies: M-5 before Cody wires the production secret (ROADMAP Now,
  "Wire `FI_FHIR_TELEMETRY_CORRELATION_KEY` into production"); M-7 reports
  back to edilint before edilint's 834 generator is scheduled; M-9 and M-10
  both touch `accessCapabilities.ts` and are serialized by the queue, the
  second one rebases (conflict autofix does not cover cross-repo rows); M-13
  and M-15 both regenerate `ui/src/lib/gen/graphql.ts` (same rule).
- Root-envelope rows (M-3 `Makefile`, M-5 `.env.example`, M-4
  `.gitlab-ci.yml`) serialize against every other row while they implement.

## Riskiest assumption + kill-test (`spec-riskiest-assumption`)

**Load-bearing assumption**: a Mills row that changes the Svelte UI lands
through this lane within budget: the devbox runs `npm ci`, vitest and
`svelte-check` for `ui/` inside the 3 USD / 40-turn envelope, and
`test:ui-e2e` (blocking, Playwright against the built UI) passes on the
train proof without a person. Seven of the fifteen rows depend on it; the
September rows that touched the UI (`bl-fifhir-connection-runtime-state-revision-id-202609`,
`bl-fifhir-engine-runtime-undocumented-properties-202609`) merged, but
before the train carried the real gating set.

**Kill test** (M-12 first among the UI rows, by priority order it may not
be; the coordinator watches whichever UI row runs first): the row opens a
merge request whose head pipeline is green including `test:ui-e2e`, its
train proof (API pipeline, ~64 jobs) is green, and it merges with no human
hand other than reading a protected-path hold. Negative control: the
operator's run ledger shows the devbox ran vitest (turn log), not only
`go test`. Disconfirming search: `mills_escalation_diagnose` on the first
UI row that escalates, looking for `infra` (devbox Node) or `code` on
`test:ui-e2e` only.

**Failure mode if wrong**: UI rows escalate on the devbox or on the e2e
gate. Then the UI rows (M-9..M-11, M-13..M-15) are held
(`mills_backlog_update_state` to `retired` with a note, or left queued with
the overlay lowered) and done as a coordinated session in a worktree, as
`.loom/42` was; the Go, docs and CI rows continue through the lane.

**Status: PASSED 2026-11-04 01:48 UTC.** M-9
(`bl-fifhir-stale-capabilities-reprobe-202611`) merged as [MR !293](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/293)
(`f9e3e1b9f`) on train pipeline 37418 — 105 minutes under runner saturation —
with no hand on the row or the merge request: the devbox ran vitest and
`svelte-check`, the head pipeline and the train both passed `test:ui-e2e`.
M-11 ([MR !297](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/297), train 37547) and M-12 ([MR !302](https://gitlab.flexinfer.ai/libs/fi-fhir/-/merge_requests/302), train 37562) followed the
same way, so the UI rows stay on the lane.

## Decisions taken by the coordinator (2026-11-03)

- Intake is direct rows with declared files, not GitLab issues (no Slices,
  five-minute declaration window) and not the carder (Now bullets are prose;
  turning it on would spin briefs nobody approves). Recorded in
  `.loom/decisions/`.
- `ci/**` is protected; rows that need a CI include declare it and wait for
  review rather than route around it.
- Documentation merge requests by the coordinator merge directly on a green
  head pipeline (no train) only when the lane is empty, to spare a 64-job
  proof for a docs-only tree; when the lane is non-empty they ride the train
  like any row, because a direct merge moves `main` under the head's train
  and forces a rebuild (amended 2026-11-03 evening; !291 rode train 37372).
  Every code row goes through the train.
- Not in the push, with the owner: the production correlation secret
  (SOPS + `secretKeyRef`: Cody); budgets 2 and 3 (runner 8 window); Slice
  5.1c-γ official-validator findings (a mapper program, not a row); the
  legacy event GraphQL fields retirement (regenerates `generated.go`,
  beyond a row's diff budget; a session); the configuration-plane spec
  kill-test; Tier B demo; the NetworkPolicy remainder; stream capture of real
  ADT feeds (kernel admission); session preview round-trip cost; E1-1
  STATIC → validated; definition authoring follow-ups (four small findings,
  worth one row once the first UI rows have landed).
