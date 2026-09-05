### 2026-08-09 - Slice 4.4b tasks 5-7 CI jobs report schema and the honest budget table

- What changed:
  - `ci/test-performance-profile.yml`: two jobs, deliberately different in kind.
    `test:benchmark-durable` is blocking, runs in the ordinary shared pool, and
    gates `allocs/op` only. `test:performance-profile` is manual,
    `allow_failure: true`, carries `tags: [fi-fhir-perf]` and a 4 CPU / 8 GiB
    quota, and archives `performance-report.json` for 12 months.
  - `scripts/performance-report.sh` plus its schema, documented in the script's
    own header (schema version 1).
  - `docs/operations/SUPPORTED-1.0.md`: a budget-by-budget certification table,
    and the per-pod/total ambiguity resolved.
  - `deploy/helm/fi-fhir/values-reference-profile.yaml` and `.loom/30` brought
    into line.
- Why:
  - Tasks 5 and 7, plus task 1's second half (the profile ambiguity).
- Decisions and evidence:
  - **The reference profile is per replica.** `SUPPORTED-1.0.md:38` said 4 vCPU
    / 8 GiB "available to the fi-fhir workload" — a deployment-total reading —
    while `values-reference-profile.yaml` has always applied that envelope per
    container across `replicaCount: 2`. Resolved in favour of per replica, and
    both documents now say so, including the 8 vCPU / 16 GiB total. Kubernetes
    `resources` are per container, so a total cannot be expressed in the file
    that carries the profile; and halving an envelope nothing has measured would
    be the capacity-change-dressed-as-a-chart-edit slice 4.4a refused.
  - **`extends:` not `<<: *anchor`, and this is correction 57 biting early.**
    YAML anchors do not cross an `include:` boundary, so `*go-image-debian`,
    `*go-cache` and `*go-changes` — which every job in the root file uses — are
    unavailable in an included file. The jobs use `extends: [.go-image-debian,
    .go-cache]` against the hidden jobs S5-0's `ci/_shared.yml` will publish, and
    spell out their `changes:` list locally. Had this been copy-pasted from the
    root file it would have failed to parse the moment it was included.
  - **The `certified` field is the report's whole point.** It is false unless the
    run happened on the pinned runner, and the budgets array is deliberately not
    derived from the benchmarks array: a benchmark measures a function, a budget
    is a claim about a product, and keeping them separate is what stops a green
    benchmark being silently promoted into a met budget. Verified both paths —
    default emits `certified=false`, `FI_FHIR_PERF_RUNNER=1` emits
    `certified=true` with `runner_tag: fi-fhir-perf`.
  - The report records `replicas: 1` honestly. The harness is in-process; it
    drives `ingress.Service.Submit` directly, not a deployed replica set. That
    single field is what keeps budget 2 from ever looking met by accident.
  - The budget table states the detection floor rather than implying precision:
    the allocation gate catches a regression of roughly 40 allocations per
    message, not one, and it is a regression detector for the thing that causes
    latency rather than a measurement of latency.
- Deliberately NOT done, with reasons:
  - **No `include:` line in `.gitlab-ci.yml`.** S5-0's MR 0a has not merged —
    there is no `ci/` directory on `main` — and not appending before it does is
    the sprint's one hard sequencing rule. The file is inert until this branch's
    final rebase adds the single line. Lane S5-A merges last, so 0a will have
    landed by then.
  - **`test:benchmark`'s package list is NOT narrowed** (task 8). The condition
    was met — the durable benchmarks live in their own job — but narrowing is a
    CI-minutes optimization that touches a line naming `./pkg/validate/...`,
    which is lane S5-E's territory and which S5-E's merged 5.1a did not change.
    Trading a cross-lane conflict for a few CI minutes is a bad trade this late
    in the sprint. Flagged to the coordinator as available and unclaimed.
- Escalation owed to the operator (unchanged, restated because it is the only
  thing standing between "harnessed" and "certified"):
  - Register a GitLab runner tagged `fi-fhir-perf` in `platform/gitops`, with at
    least 4 CPU and 8 GiB available to a job, then set the project variable
    `FI_FHIR_PERF_RUNNER=1`. Until both exist the profile job is invisible and
    every archived report reads `certified: false`.
- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` Lane S5-A tasks 5, 7, 8;
    correction 57 (anchors do not cross include boundaries); correction 10 (the
    per-pod/total ambiguity)
  - [S2] `.gitlab-ci.yml:56-68` — the `.go-changes` list this file mirrors
  - [S3] `.gitlab-ci.yml:815-847` — `test:mllp-runtime`, the Postgres-service
    job shape both new jobs follow
