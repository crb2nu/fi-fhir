### 2026-03-01: Treat Contract Drift Gate Promotion as M0 Exit Criterion

- Decision:
  - Promote `lint:contracts` from soft-fail to blocking after a short clean-run burn-in, and make this the formal M0 exit criterion.
- Rationale:
  - Contract tooling is already implemented and wired in Makefile/CI; enforcing it closes a known governance gap.
- Alternatives considered:
  - Keep permanent warning mode (rejected; does not prevent incompatible drift).
- Consequences:
  - Short-term CI friction may increase.
  - Medium-term API stability and client confidence improve.
- Sources:
  - [S1] `scripts/check_event_contracts.go:40`
  - [S2] `Makefile:203`
  - [S3] `.gitlab-ci.yml:320`
  - [S4] `.gitlab-ci.yml:325`
