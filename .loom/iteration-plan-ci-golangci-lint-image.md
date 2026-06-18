# RALPH Iteration Plan

## Review

- Roadmap milestone: Functionality gap delivery program / CI prove gate.
- Spec section(s): `.loom/23-functionality-gaps-plan.md` CI gotchas; RALPH exit criteria.
- Prior decisions to preserve: keep blocking Go lint as a merge gate; avoid marking a slice complete while CI is red or unqueued.

## Align

- Slice name: CI golangci-lint image bootstrap.
- Scope in:
  - Repair MR `!80` after `lint:go` timed out compiling golangci-lint from source.
  - Keep `GOLANGCI_LINT_VERSION` pinned and continue running `golangci-lint run --timeout=15m ./...`.
  - Record why the CI shape changed.
- Scope out:
  - Broad GitLab cache redesign.
  - Changing lint rules or allowing `lint:go` to fail.
  - Any new product roadmap feature until the previous MR is green/queued.
- Acceptance criteria:
  - `lint:go` uses the pinned official golangci-lint image instead of `go install` from source.
  - Local focused validation passes.
  - MR pipeline is re-triggered and auto-merge is re-armed.
- Dependencies/blockers:
  - Harbor Docker Hub cache must serve `golangci/golangci-lint:v2.8.0-alpine`.
  - GitLab runner must be able to run the image with the repo's Go module cache settings.
- Riskiest assumption:
  - The pinned golangci-lint image is built with a Go toolchain compatible with this repo's `go` directive.
- Kill-test:
  - Verify the image manifest exists before patching, then let the MR pipeline execute `lint:go`; failure would require reverting to source build with a longer timeout or a CI-warmed custom lint image.

## Land

- Planned file areas:
  - `.gitlab-ci.yml`
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/40-decisions.md`
  - `.loom/50-worklog.md`
- Implementation steps:
  1. Switch `lint:go` to the pinned golangci-lint image.
  2. Preserve GOPATH/GOCACHE setup and current lint command.
  3. Document the CI failure and decision.

## Prove

- Tests to run:
  - `go test ./pkg/profile`
- Lint/static checks:
  - Validate YAML shape by inspecting `.gitlab-ci.yml` and relying on GitLab pipeline parse on push.
- CI checks:
  - Re-run MR `!80` pipeline after pushing the fix commit.

## Handoff/Harvest

- Docs to update:
  - `.loom/23-functionality-gaps-plan.md`
  - `.loom/40-decisions.md`
  - `.loom/50-worklog.md`
- Agent-context entries to add:
  - Decision: use pinned prebuilt golangci-lint image for MR lint bootstrap.
  - Finding: source-building golangci-lint can consume the full 1h CI job on cold cache.
- Next-slice candidates:
  - Resume Wave 2 Slice 0 LLM reachability kill-test once MR `!80` is green/queued.
