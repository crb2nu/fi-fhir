### 2026-09-07 - Sprint 6 planning: the 1.0 critical path opens

- What changed:
  - `.loom/34-sprint6-execution-specs.md` — the Sprint 6 plan, written against
    `main` @ `13bf9f4e9` (pipeline 25819 green). Five lanes: S6-0 merge
    surface, S6-A Slice 4.1c-c FHIR destination class, S6-B budgets 1–3
    certification on the pinned runner, S6-C legacy e2e tree repair, S6-D
    Slice 5.1b package pinning and the structural validator.
  - No product code. This entry and the spec are the whole MR.
- Why:
  - Sprint 5 closed on 2026-09-05 with every August MR merged; the board is
    empty (0 open, 0 draft). The Sprint 5 coordinator ruled that 4.1c-c would
    get "a dedicated spec pass targeting Sprint 6" (`.loom/33` ruling 1) and
    that pass never happened. Every 1.0 document points at it: `.loom/28`'s
    kill-test answer, `FHIR-CONFORMANCE-MATRIX.md` §5 row 1, the 5.1a
    handoff's "what's next", and the second day-1 gate 5.1a executed
    (`TestFHIRConformance_DurableEngineProducesNoFHIRResource`).
- Four premises inverted from code, one decisively:
  - **The pinned runner is not missing.** Runner id 8 `fi-fhir-perf` has been
    registered and online since 2026-08-09 (`platform/gitops` `a159dad74`),
    its manager pod is Running in namespace `ci`, and its job history holds
    exactly one canceled smoke job. The repo's `test:performance-profile` is
    `when: never` because the project variable `FI_FHIR_PERF_RUNNER` was never
    set. `SUPPORTED-1.0.md` rows 1–3 still say "needs a pinned runner".
  - **The "which mapper" question is already answered.**
    `pkg/integration`'s `canonicalEventRegistry` and
    `decodeCanonicalEventPayload` reverse the durable `payload_json` into the
    exact `pkg/events` types `pkg/fhir.USCoreMapper` consumes; the redaction
    zeroes eight raw-source keys and no clinical field.
  - **`CreateTransactionBundle` is not replay-safe.** Every entry is a `POST`;
    the outbox is at-least-once; a lease reclaim would create a second Patient.
    The sprint's riskiest assumption and its day-1 kill-test are built on
    this.
  - **The 5.1a gate's fixture is not the wire shape** (`event_type:
    "patient.admitted"` vs the real `patient_admit` inside a `pkg/events`
    struct). The inverted gate must be driven by `NewProcessedEvent`.
- Evidence:
  - `GET /projects/19/merge_requests?state=opened` → 0;
    `GET /projects/19/pipelines?ref=main` → 25819 `success`.
  - `GET /runners/8` → online, `tag_list=[fi-fhir-perf]`;
    `GET /runners/8/jobs` → one job (226438, canceled, 2026-08-09);
    `GET /projects/19/variables` → no `FI_FHIR_PERF_RUNNER`.
  - `kubectl -n ci get pods` → `gitlab-runner-perf-*` Running (18 restarts /
    14 d); `kubectl get nodes` → `cblevins-5930k` Ready.
  - Code citations are in the spec's Sources section, all at `13bf9f4e9`.
  - `bash scripts/validate-docs.sh`, `scripts/worklog.sh check`,
    `scripts/decisions.sh check` — all pass with the new files.
- What's next:
  - Two decisions before lanes launch: 4.1c-c shape (recommendation: a new
    `fhir` `TransportKind` with its own policy) and the operator setting
    `FI_FHIR_PERF_RUNNER = "1"` on project 19.
  - Day 1: S6-0 merge-surface MR; S6-A's three test-only gates; S6-B's
    negative-control run; S6-C's failing-set run; S6-D's pinned packages.
- Sources:
  - [S1] `.loom/34-sprint6-execution-specs.md`
  - [S2] `.loom/33-sprint5-execution-specs.md` — Wave 3, decision 1, ruling 1
  - [S3] `docs/planning/FHIR-CONFORMANCE-MATRIX.md` §5–§6
  - [S4] `docs/operations/SUPPORTED-1.0.md` budgets table
  - [S5] `.loom/worklog/2026-08-09-slice-5-1a-reconciliation-the-mapper-validates.md`
