# RALPH Iteration Plan: Gate 0B Truthful Security Gates

**Status**: complete
**Date**: 2026-07-12

## Review

- Roadmap milestone: Completion Gate 0B
- Spec sections: truthful verification; security and release evidence
- Prior decision: security findings must be remediated or explicitly bounded
  before advisory jobs become required gates.
- Evidence from main pipeline 18498:
  - all seven security jobs were allowed to fail;
  - the UI audit job exited successfully with 2 critical and 10 high findings;
  - the TypeScript SDK was not audited and contained 1 critical and 3 high
    findings;
  - the UI runtime image contained 4 fixed HIGH Alpine findings;
  - govulncheck found no reachable vulnerabilities, gosec found no findings,
    and the backend runtime image contained no HIGH or CRITICAL findings;
  - Trivy filesystem scanning found no CRITICAL findings and three HIGH findings
    in the test-only Docker dependency, two of which have no published fix.

## Align

### Scope in

- Refresh the UI lock within declared ranges and pin the patched same-major
  Lodash override required by the current GraphQL Codegen toolchain.
- Move the UI to the compatible Vite 7/Svelte plugin 6 pair because Vite 6's
  declarations do not accept the first patched Rollup type contract under the
  repository's strict optional-property checking.
- Upgrade SDK Vitest to 4.1.10 and refresh its npm lock.
- Replace the full UI runtime base with the verified digest-pinned nginx Alpine
  slim image, removing the vulnerable unused packages from production.
- Split UI and SDK audits into required jobs that preserve JSON reports and fail
  on HIGH or CRITICAL findings.
- Make govulncheck, gosec, Trivy filesystem CRITICAL enforcement, and both image
  scans required.
- Run backend and UI image builds/scans on merge requests and fail runtime image
  scans on every CRITICAL finding plus every fixed HIGH finding.
- Make main deploys wait for image scans and make tagged releases retag the
  already-scanned artifacts instead of rebuilding mutable inputs.
- Keep UI dependencies, generated binaries, coverage, and local tool scratch
  outside the backend Docker build context.
- Pin go-licenses and preserve its real policy-check exit status.
- Align the local Makefile npm security target with both required CI audits.

### Scope out

- GraphQL Codegen major migration; its isolated proof rewrote the generated
  client and introduced 130 type errors, so it needs its own compatibility slice.
- Direct-push policy changes on protected main; this changes maintainer workflow
  and is not required to make merge-request evidence truthful.
- A major testcontainers/Docker dependency migration for test-only, unreachable,
  partly-unfixed HIGH findings.
- Full auth/RBAC, PHI policy, tenant isolation, and runtime ingress hardening.

### Riskiest assumption and kill-test

The riskiest load-bearing assumption is that jobs marked required actually
propagate scanner failures instead of merely emitting reports. Before landing,
temporarily audit the preserved vulnerable lockfiles and prove both UI and SDK
commands exit non-zero; then prove the remediated locks exit zero. CI lint and
the MR job metadata must also show every promoted job as non-optional and not
allowed to fail.

### Acceptance criteria

- Fresh npm 10.9.3 installs for UI and SDK have zero HIGH or CRITICAL audit
  findings and pass codegen/typecheck/tests/build.
- The UI runtime image has zero HIGH or CRITICAL findings after a fresh build;
  both runtime image scanners fail on every CRITICAL and every fixed HIGH
  finding.
- govulncheck, gosec HIGH/HIGH, Trivy filesystem CRITICAL, both npm audits,
  go-licenses, and both image scans are required MR jobs.
- Full scanner reports remain available as artifacts even when enforcement
  commands fail.
- Backend and UI image artifacts cannot be absent when their scanner jobs run.
- Main deploy and tagged release jobs cannot publish before their exact image
  artifact passes its required scan.
- The required MR pipeline reaches terminal green before merge.

## Land

### Exact intended files

- `.gitlab-ci.yml`
- `.dockerignore`
- `Makefile`
- `ui/package.json`
- `ui/package-lock.json`
- `ui/Dockerfile`
- `sdk/typescript/package.json`
- `sdk/typescript/package-lock.json`
- `CHANGELOG.md`
- `.loom/iteration-plan-gate-0b-runtime-verification.md`
- `.loom/30-implementation-plan-integration-engine-ide-completion.md`
- this iteration plan

### Implementation sequence

1. Apply dependency and runtime-image remediations.
2. Prove clean frozen installs, generated-code stability, SDK/UI quality, and
   runtime-image scans.
3. Promote scanner exits and align MR build/scan rules.
4. Run negative kill-tests against preserved vulnerable locks.
5. Self-review, then drive the MR and main pipelines to terminal green.

## Prove

### Targeted

    npx --yes npm@10.9.3 --prefix ui ci --no-audit --no-fund
    npx --yes npm@10.9.3 --prefix ui audit --audit-level=high
    npx --yes npm@10.9.3 --prefix ui run codegen:check
    npx --yes npm@10.9.3 --prefix sdk/typescript ci --no-audit --no-fund
    npx --yes npm@10.9.3 --prefix sdk/typescript audit --audit-level=high
    npx --yes npm@10.9.3 --prefix sdk/typescript test
    npx --yes npm@10.9.3 --prefix sdk/typescript run typecheck
    npx --yes npm@10.9.3 --prefix sdk/typescript run build

### Broad

    npx --yes npm@10.9.3 --prefix ui run lint
    npx --yes npm@10.9.3 --prefix ui run lint:css
    npx --yes npm@10.9.3 --prefix ui run check
    npx --yes npm@10.9.3 --prefix ui run typecheck
    npx --yes npm@10.9.3 --prefix ui run test:run
    npx --yes npm@10.9.3 --prefix ui run build
    make security-vulncheck security-gosec security-npm-audit

### Negative kill-tests

- Run HIGH audit enforcement against the preserved pre-remediation UI and SDK
  locks; both commands must exit non-zero.
- Scan a deliberately stale pre-remediation UI image and confirm the HIGH image
  enforcement command exits non-zero.
- Validate the expanded CI and inspect MR job metadata: promoted jobs must not
  be `allow_failure` and image scans must have required build artifacts.

## Handoff

- MR !92 required pipeline 18520 passed and merged as
  `367bab796fa73f1eb45f529ad2d4a9fba9172eba`.
- Post-merge main pipeline 18521 passed all 31 jobs: 9 lint, 9 test, 6 security,
  3 build, 2 image scan, and 2 deploy. Every promoted security/image job was
  required (`allow_failure=false`). Both deploy jobs started only after all
  required upstream stages were green and then published successfully.
- This evidence closes Gate 0B.
- Next: Phase 1 Slice 1.0 foundation contracts and the minimal immutable
  IntegrationDefinitionRevision, followed by the shared MessageProcessor.
