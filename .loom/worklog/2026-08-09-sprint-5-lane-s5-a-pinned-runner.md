### 2026-08-09 - Sprint 5 Lane S5-A pinned-runner decision and loadtest disposition

- What changed:
  - Recorded Lane S5-A's gating decision in `.loom/40-decisions.md`: option A of
    `.loom/33`'s pinned-runner table, per coordinator ruling 2. Blocking
    `allocs/op` in the shared pool; an inert, tagged (`fi-fhir-perf`), manual,
    `allow_failure: true` wall-clock job with an archived report schema; **no**
    calibrated wall-clock or throughput gate in the shared pool, ever. Budgets
    1-3 stated as *harnessed but uncertified* until a runner is registered.
  - Recorded the two lane-local dispositions the same entry owed: delete
    `fi-fhir workflow loadtest` (not relabel), and repair rather than delete
    `configs/adt-workflow.yaml`. Also resolved the reference profile's
    per-pod-versus-total ambiguity in favour of **per replica**.
  - Corrected two claims in `.loom/33-sprint5-execution-specs.md` before writing
    any production code, per the sprint's coordination rule.
- Why:
  - `.loom/33` makes the decision the lane's gate and forbids writing a harness
    before it is recorded. The `.loom/33` corrections are required by the rule
    that a lane which finds a false premise fixes the spec first.
- Evidence:
  - **Correction to `.loom/33` correction 8.** The spec attributed the loadtest's
    100% error rate to `NewHealthcareEventGenerator` emitting events that fall
    through to the catch-all. It does not: it emits exactly the four `patient_*`
    types the named routes match. Substituting `type: log` for `type: emit` in a
    copy of `configs/adt-workflow.yaml`, changing nothing else, drops the error
    rate from `100.0000%` to `0.0000%` at the same rate — the `emit` action fails
    because the loadtest path configures no event store. A log-only probe using
    the engine's real template form prints `ok patient_admit mrn=12345`, so the
    templating defect is confined to `configs/adt-workflow.yaml`.
    The conclusion is *strengthened*: `Achievement: 100.1% of target` is an event
    emission rate, structurally blind to whether any action succeeded. That is
    what settles delete over relabel — a banner cannot make that number honest.
  - **Correction to `.loom/33` Lane S5-A task 4.** Its premise, that `Check` must
    be taught to treat an unset threshold as "not gated rather than zero", is
    false — `benchmark_util.go:569,581,591` already skip absent keys. The real
    hazard is one scope up: `Check`'s `required` set (`:547-566`) unions all three
    maps and fails any name with no result, and `ResolveWorkflowThresholds`
    (`:481-500`) copies `workflowAllocCeilings` into every profile. So a durable
    name added to the shared map would break `test:benchmark`, whose package list
    excludes `internal/integration`. The sibling map is mandatory, not the "or"
    branch of a choice.
  - Reproduced `.loom/33` correction 8's headline against the shipped config:
    `Events/sec: 500.49`, `Achievement: 100.1% of target`, `P99: 212.209µs`,
    `Errors: Count: 1001, Rate: 100.0000%`, with 3500 lines of
    `function "event" not defined` on stderr.
  - Reproduced correction 10: `SUPPORTED-1.0.md:38` says 4 vCPU / 8 GiB "available
    to the fi-fhir workload" while `values-reference-profile.yaml:25-31` applies
    4/8Gi per container across `replicaCount: 2` — silently double.
- What's next:
  - Task 2 (delete the subcommand, repair the config), then tasks 3-7. Budget-2
    measurement is sequenced after S5-D merges.
  - **Escalation owed to the operator**: register a `fi-fhir-perf` GitLab runner
    in `platform/gitops`. Until it exists, no wall-clock number this repository
    produces is evidence for any budget, and `performance-report.json` carries
    `certified: false`.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Lane S5-A, coordinator ruling 2
  - [S2] `internal/workflow/benchmark_util.go:290-301,321-335,481-500,538-602`
  - [S3] `cmd/fi-fhir/main.go:2931`; `internal/workflow/loadtest.go:586-626`
  - [S4] `5d07101c4` — fixed `examples/workflows/*.yaml`, missed `configs/`
