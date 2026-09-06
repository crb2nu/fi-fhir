### 2026-07-13: Roll Matching Runtime Images Behind an Automation Barrier

- Decision:
  - Publish API and UI from one successful default-branch pipeline under one
    immutable release tag, then verify both registry manifest digests before
    changing deployment state.
  - Suspend image automation before landing runtime prerequisites. Roll both
    image tags plus their deployment hardening in one reviewed GitOps change.
  - Keep automation suspended until both deployments are Ready on the verified
    image IDs and the public ingress passes auth, origin, containment,
    provenance, suppressed-delivery, and PHI-leakage probes.
  - Resume automation in a separate reviewed GitOps change and verify the
    controller is Ready, the repository is up to date, and the running images
    have not drifted.
- Rationale:
  - The IDE and API are one compatibility boundary. Independent image updates
    can expose a client/server contract mismatch even when both images are
    individually healthy.
  - Suspending automation makes the rollout and rollback set explicit while
    live security probes run against a stable pair of artifacts.
  - A separate resume change preserves evidence that the safety barrier was not
    removed before acceptance passed.
- Alternatives considered:
  - Let Flux update API and UI independently (rejected because policy polling
    and reconciliation do not guarantee an atomic compatible pair).
  - Resume automation in the rollout MR (rejected because the live gate cannot
    run until that MR is applied).
  - Pin digests permanently (rejected because verified immutable tags plus
    observed image IDs retain normal automated release operation).
- Consequences:
  - Coordinated runtime releases require two small GitOps MRs around the live
    gate. The rollback tag is recorded per release rather than becoming policy.
  - A failed probe leaves automation suspended and both workloads on the known
    pair until an explicit rollback or corrected rollout is reviewed.
- Evidence:
  - App MR `!96`; main pipeline `18621`; release tag `v0.1.18621`.
  - GitOps prerequisite MRs `!359`/`!360`, rollout MR `!368`, and resume MR
    `!369`; exact digests and live assertions are in the Slice 1.1c iteration
    plan.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] `.loom/iteration-plan-phase-1-slice-1-1c-authenticated-preview-adapters.md`
  - [S3] `deploy/kubernetes/`
  - [S4] `ui/nginx/default.conf.template`
