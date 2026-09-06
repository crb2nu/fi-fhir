### 2026-08-09: Gate the durable path on allocations, not on wall-clock, and delete the loadtest subcommand

- Decision:
  - **Option A of `.loom/33`'s pinned-runner table, ratified by coordinator ruling 2
    (2026-08-09).** Slice 4.4b ships:
    - **blocking `allocs/op` ceilings** for new durable-path benchmarks, running in
      the ordinary shared CI pool with `allow_failure: false`;
    - an **inert, tagged, manual, `allow_failure: true` wall-clock job**,
      `ci/test-performance-profile.yml`, carrying `tags: [fi-fhir-perf]`,
      `KUBERNETES_CPU_LIMIT: "4"`, `KUBERNETES_MEMORY_LIMIT: "8Gi"`, gated behind a
      project variable so it does not appear in a normal pipeline until a runner is
      registered. It archives `performance-report.json` for 12 months against a
      documented schema.
    - **No calibrated wall-clock or throughput threshold in the shared pool, in any
      form.** This is a prohibition, not a deferral.
  - **Budgets 1, 2, and 3 are stated in `docs/operations/SUPPORTED-1.0.md` as
    *harnessed but uncertified*** until a `fi-fhir-perf` runner exists. Runner
    registration is a `platform/gitops` task, escalated to the operator; it is
    outside this repository and outside this slice.
  - **`fi-fhir workflow loadtest` is deleted**, along with `internal/workflow/loadtest.go`
    and its tests — not renamed, not banner-labelled.
  - **`configs/adt-workflow.yaml` is repaired**, not deleted: its templates move to the
    engine's real form, and its `emit` actions are replaced, so the one first-party
    workflow config in the repository runs cleanly.
  - **The reference profile is per replica.** `SUPPORTED-1.0.md` says so explicitly and
    states the deployment total, so `deploy/helm/fi-fhir/values-reference-profile.yaml`
    and the prose stop disagreeing.

- Rationale:
  - **The allocation signal is the only one this repository has evidence for.**
    `internal/workflow/benchmark_util.go:321-327` records that across 78-87 CI
    artifacts spanning all three CPU classes, every gated benchmark reported a
    *bit-identical* allocation count. That is not an argument from principle; it is a
    measurement already in the tree. Allocations are a property of the code, and
    gating them extends a mechanism this repo has already proved rather than
    inventing one.
  - **A wall-clock gate in the shared pool cannot be both truthful and useful.**
    `LatencyMarginFactor = 1.6` (`benchmark_util.go:290-301`) was backtested against
    *micro*-benchmarks: hardware spread up to 5.3x handled by profile selection, then
    whole-run contention p99 1.37x and single-benchmark jitter up to 1.55x on top.
    A durable-accept benchmark runs against PostgreSQL and Kafka **service containers
    sharing the same pod**, under a global `KUBERNETES_CPU_LIMIT: "1"`
    (`.gitlab-ci.yml:41-44`). Its variance is nowhere near a micro-benchmark's, so a
    p95 gate calibrated for that pool is either permanently red or so wide it cannot
    detect a real regression. The repository already says this to itself in the
    `test:benchmark` comment at `.gitlab-ci.yml:1257-1261`.
  - **The repo-side half of the pin is genuinely cheap, so shipping it inert costs
    almost nothing and removes the excuse.** There is not one `tags:` block in
    `.gitlab-ci.yml`'s 2738 lines — runner selection is 100% default-pool. The pin is
    one `tags:` line plus a quota bump. Writing the job, the tag value, and the report
    schema now turns the infrastructure request into one sentence with a concrete
    artifact behind it, instead of a design task someone has to redo later.
  - **On deleting the loadtest rather than relabelling it** — a banner fixes the
    subject, and the subject is not the main problem. Executed against the repository's
    own `configs/adt-workflow.yaml` (2s, 500 rps): `Events/sec: 500.49`,
    `Achievement: 100.1% of target`, `P99: 212.209µs`, and simultaneously
    `Errors: Count: 1001, Rate: 100.0000%`. The headline achievement figure is an
    event **emission** rate. It reads ~100% whether every action succeeded or every
    action failed, so it is not a metric that can be made honest by explaining what it
    measures — it has to be replaced. Making it truthful would mean fixing the
    achievement metric, the hardcoded pass constants at `main.go:2931`
    (`result.Passed(0.90, 0.01, 10*time.Millisecond)`, unrelated to any product
    budget), and the event generator, in order to load-test an engine **no durable path
    calls at runtime** — the durable path reaches `internal/workflow` only for parse and
    plan (`internal/integration/processor/workflow_plan.go:41,45`). That is building a
    second harness for the wrong subject, in the same slice that builds the real one.
    Two harnesses, one of which flatters, is worse than one.
  - **The error rate is not the generator's fault, and the real cause was worth
    finding.** `.loom/33` correction 8 attributed it to `NewHealthcareEventGenerator`
    emitting events that fall through to the catch-all. Reproduced and corrected in the
    same document: the generator emits exactly the four `patient_*` types the named
    routes match. Substituting `type: log` for `type: emit` in a copy of
    `configs/adt-workflow.yaml` and changing nothing else drops the error rate from
    100.0000% to 0.0000%. And `emit` is not a registered action type at all
    (`engine.go:125-134`), so no event store would have helped — every action in the
    file failed with `unknown action type: emit`, under `workflow run` as much as
    under the loadtest. That makes the affordance's dishonesty structural rather than
    incidental, which is what settles delete over relabel: a harness that reports
    100% achievement while every single action is failing is not mislabelled, it is
    measuring the wrong thing.
  - **On repairing `configs/adt-workflow.yaml` rather than deleting it** — its
    `{{ event.type }}` / `{{ event.data.patient.mrn }}` form was never implemented; the
    engine renders Go `text/template` against the event's JSON form, so the working
    form is `{{.type}}` / `{{.patient.mrn}}`. Verified: a log-only probe prints
    `ok patient_admit mrn=12345`. Commit `5d07101c4` corrected precisely this across
    `examples/workflows/*.yaml` and the user guide and missed `configs/` — the single
    directory the loadtest usage text pointed users at. It is the only first-party
    workflow config left in the repository, so repairing it finishes a pass that was
    already 90% done rather than deleting the evidence that it was incomplete.
    **Repairing it also surfaced a third defect nobody had named: its routing never
    worked.** `condition:` is a field of `Filter` (`types.go:19-31`) and the file put
    it at the route's top level, where it is an unrecognized key, silently dropped,
    leaving a zero-value `Filter` that matches every event — 4 events produced 20
    route matches across 5 routes, including a `lab_result` event the file has no
    route for. `docker-compose.yaml:80` mounts this file as
    `FI_FHIR_WORKFLOW_CONFIG_PATH`, so that behaviour was live in the default
    development stack. `internal/workflow/shipped_config_test.go` now asserts the
    routing, because the signal that would have caught this — the route-match count —
    was asserted nowhere.
  - **On per replica** — Kubernetes `resources` are per container; "4 vCPU across the
    deployment" has no expression in a values file, so a total reading could never be
    encoded in the artifact that is supposed to carry the profile. Making the file match
    a total-reading sentence would also halve a resource envelope nothing has measured,
    which is the capacity-claim-dressed-as-a-chart-edit that slice 4.4a explicitly
    refused (`SUPPORTED-1.0.md:52-53`). The budgets that matter are per-process
    properties: accept latency and a 1-GiB batch import's peak RSS are measured in one
    process, not summed across a Deployment.

- Alternatives considered:
  - **Option B — calibrate a wall-clock p95 gate for the shared pool** (rejected: the
    1.6x margin's backtest does not transfer from micro-benchmarks to a database-backed
    accept path on a 1-CPU pod, and the failure mode is a gate that looks like evidence
    and is not. Explicitly prohibited by coordinator ruling 2.)
  - **Option C — wait for the runner and ship nothing** (rejected: it leaves the
    CPU-independent regression signal unbuilt for the whole RC window, and the
    allocation gate does not depend on the runner in any way.)
  - **Option D — measure on a developer machine and archive the report** (rejected on
    sight: unreproducible, unattributable, and not the shipped artifact.)
  - **Relabelling `workflow loadtest` with a legacy-engine banner** (rejected: see
    above — the achievement metric is blind to success, so the banner would sit above a
    number that is still false.)
  - **Deleting `configs/adt-workflow.yaml`** (rejected: it is the only first-party
    workflow config in the tree, and the fix is four template expressions and an action
    type.)
  - **Making `values-reference-profile.yaml` total 4 vCPU / 8 GiB across two replicas**
    (rejected: halves an unmeasured envelope, and the per-container reading is the only
    one the chart can express.)

- Consequences:
  - `SUPPORTED-1.0.md` gains a budget-by-budget certification table: budget 6 certified
    by 4.4a; budgets 1, 2, 3 **harnessed but uncertified pending a pinned runner**, with
    budget 2 additionally blocked on 4.4e's durable token bucket (a 250 msg/s run on two
    replicas against a revision declaring 250 admits up to 500 and certifies nothing);
    budgets 4, 5, 7 owned by 4.4c. `.loom/30`'s slice 4.4 section takes the same
    correction.
  - **A `fi-fhir-perf` runner must be registered in `platform/gitops`** before any
    wall-clock number in this repository is evidence for anything. Until then
    `performance-report.json` carries `certified: false`, and only a job that ran on
    that tag may set it true.
  - The durable alloc ceilings need a **sibling threshold map**, not an extension of
    `workflowAllocCeilings`: `Check`'s `required` set (`benchmark_util.go:547-566`)
    unions all three maps and fails any name with no result, so a durable name in the
    shared map would break the existing `test:benchmark` job, whose package list does
    not include `internal/integration`. Recorded as a correction to `.loom/33` task 4,
    whose stated premise — that `Check` treats an unset threshold as zero — is false;
    it already skips absent keys.
  - Deleting `workflow loadtest` removes a CLI subcommand. It is pre-1.0, the
    subcommand has no non-test caller besides its own dispatch, and
    `internal/workflow/loadtest.go`'s only consumers are that dispatch and
    `loadtest_test.go`.
  - `test:benchmark`'s package list may narrow. `./pkg/validate/...` sits in it and is
    lane S5-E's file territory, so any narrowing is a one-line coordination with S5-E,
    not an edit to S5-E's files.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-A pinned-runner table,
    coordinator ruling 2, corrections 3-11
  - [S2] `internal/workflow/benchmark_util.go:290-301,321-335,481-500,538-602`
  - [S3] `.gitlab-ci.yml:41-44,1238-1293,1250,1257-1261`
  - [S4] `docs/operations/SUPPORTED-1.0.md:33-75`;
    `deploy/helm/fi-fhir/values-reference-profile.yaml:22-31`
  - [S5] `cmd/fi-fhir/main.go:1955,2758-2940,2931`; `internal/workflow/loadtest.go:586-626`
  - [S6] Executed 2026-08-09: `fi-fhir workflow loadtest -c configs/adt-workflow.yaml
    -d 2s -r 500` → 100.1% achievement with a 100.0000% error rate; the same config with
    `type: emit` replaced by `type: log` → 0.0000%; a log-only probe using `{{.type}}`
    and `{{.patient.mrn}}` → `ok patient_admit mrn=12345`
  - [S7] `5d07101c4` — the schema-truth pass that fixed `examples/workflows/*.yaml` and
    missed `configs/`
