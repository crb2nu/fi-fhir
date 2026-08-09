### 2026-03-04

- What changed:
  - Promoted `lint:contracts` from `allow_failure: true` to blocking merge gate (M0 exit criterion).
  - Added `scripts/smoke-test.sh` covering `/health`, `/graphql`, `/graphql/ws` endpoint reachability.
  - Added `smoke-test` and `smoke-test-local` Makefile targets.
  - Moved 15 root-level `ROADMAP_RECONCILIATION_*.md` files into `docs/` for consistency.
- Why:
  - M0 milestone requires enforced contract governance and endpoint smoke baseline.
  - Reconciliation files were cluttering the repo root and inconsistent with the `docs/` convention.
- What's next:
  - Validate full CI pipeline passes with blocking contract gate on `main`.
  - Begin M1 execution: backend↔frontend integration parity (issue #9).
  - Open implementation tasks for M2/M3 adapters.
- Sources:
  - [S1] Contract check: `make contract-check-strict` → 36/36 events, zero drift.
  - [S2] `.gitlab-ci.yml:324-325` — removed `allow_failure: true`.
  - [S3] New file: `scripts/smoke-test.sh`.
  - [S4] `Makefile:218-225` — added `smoke-test` / `smoke-test-local`.
