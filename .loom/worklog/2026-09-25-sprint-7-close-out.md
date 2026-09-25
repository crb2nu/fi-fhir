### 2026-09-25 - Sprint 7 close-out

- What changed:
  - Sprint 7 (`.loom/35-sprint7-execution-specs.md`, MR !214) delivered all
    three roadmap "Now" items that were code, plus two coordinator fixes:
    Slice 5.1c-α mapper cardinality gaps (MR !216), Slice 5.1c-β the official
    HL7 validator as an offline blocking gate (MR !219), Slice 4.2c the operator
    trace shows the FHIR delivery (MR !217), the performance-report
    certification hole (MR !215, harness only), and the MinIO service-image
    mirror after quay.io and Docker Hub withdrew `minio/minio` (MR !218).
  - This entry's MR flips `ROADMAP.md` Now → Delivered, records the 5.1c-γ
    remainder (157 findings, 28 errors, zero cardinality) in `.loom/30`, writes
    the single Sprint 7 `CHANGELOG.md` block, and sets `USCoreMapper.Source`
    in `fhirout.mapEvent` so the legacy workflow `fhir` action's raw Encounter
    identifier carries the same `urn:fi-fhir:source:<source>` system the
    durable transport already added through `ensureIdentifier`.
- Why:
  - Lanes were barred from `ROADMAP.md`, `.loom/30`, and `CHANGELOG.md` so
    that four concurrent MRs never conflicted on shared prose; the
    coordinator writes each once at close.
  - The `Source` one-liner was Lane S7-A's banked finding: `fhirout` never set
    the field the lane introduced, and `fhirout` belonged to Lane S7-B until
    !219 merged.
- Evidence:
  - Merges on `main`: 3164ea326 (!214), 57ceba92e (!218), 650df2f0e (!215),
    c59257971 (!216, pipeline 28851), 4f3fb2aec (!217, pipeline 28852),
    7ae14604c (!219, pipeline 28874).
  - MinIO break: job 306102 (`ErrImagePull … 401 UNAUTHORIZED`); only
    k3s-w-9 / k3s-w-11 cached the 14cea493 digest, no node cached 4c4a4876;
    both images pushed from the 7900xtx docker host; Harbor token-flow HEAD 200
    on both new digests; pipeline 28837 green including both MinIO jobs.
  - S6-B: jobs 305765 / 305981 `runner_system_failure` after 915 s; node
    `cblevins-5930k` 27.7 GiB allocatable, 24,278 Mi requested, 16 GiB held by
    the 27B inference workhorse; CI pods at PriorityClass `ci-low`.
  - Official validator: hermetic only with 23 archives (2 pinned + 21 more);
    online empty-cache run produced the identical package summary and
    findings; ledger 163 → 157 after 5.1c-α (cardinality 14 → 0).
- What's next:
  - S6-B certification resumes on `docs/perf-budgets-certified` once
    platform/gitops frees ≥ 7 GiB on `cblevins-5930k` or moves runner 8; the
    "is a GPU-inference node a reference host" question needs a ruling.
  - Slice 5.1c-γ: close the 28 official-validator errors (invariants,
    datatypes, local code membership, in-Bundle references, two slices).
  - Budget 3 needs a real 1-GiB batch-import workload reading container RSS.
  - `EligibilityResponseEvent` should carry X12 INS02 so a dependent's
    `Coverage.relationship` stops being `unknown`.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md`
  - [S2] `.loom/worklog/2026-09-24-slice-5-1c-a-the-mapper-s.md`
  - [S3] `.loom/worklog/2026-09-24-slice-5-1c-b-the-official-validator.md`
  - [S4] `.loom/decisions/2026-09-24-run-the-hl7-validator-offline-as-a.md`
  - [S5] GitLab MRs !214–!219 and the job/pipeline ids above
