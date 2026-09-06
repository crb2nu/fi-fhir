### 2026-06-18: Use Pinned golangci-lint Image for CI Go Lint

- Decision:
  - Run `lint:go` in the pinned official `golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine` image instead of compiling golangci-lint from source in every MR pipeline, and give that job 2 CPU / 4 GiB plus a 30-minute lint timeout for cold-cache package loading.
- Rationale:
  - MR `!80` first failed because `go install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.8.0` spent the full 1-hour job timeout downloading/building the linter dependency graph before linting began.
  - After moving to the pinned image, the job reached `golangci-lint run` but the 1 CPU / 15-minute configuration still timed out during cold-cache package loading.
  - The image is still version-pinned and is available through the workspace Harbor Docker Hub cache.
- Alternatives considered:
  - Increase the `lint:go` timeout (rejected as slower and still cache-fragile).
  - Make `lint:go` soft-fail (rejected because it weakens a blocking merge gate).
  - Redesign all Go cache keys in this slice (deferred; broader CI platform change).
- Consequences:
  - Fresh branches avoid source-building the linter before every run and have a realistic package-load budget.
  - If the pinned image ever lags the repo Go directive, the rollback is to a prebuilt internal lint image or a longer source-build job.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] GitLab job `142164` trace: `lint:go` timed out after 1h while downloading golangci-lint dependencies.
  - [S3] Command: `docker manifest inspect registry.harbor.lan/dockerhub-cache/golangci/golangci-lint:v2.8.0-alpine`
