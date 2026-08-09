### 2026-06-18

- What changed:
  - Diagnosed MR `!80` after auto-merge stalled and found `lint:go` failed with `job_execution_timeout`.
  - Switched `lint:go` from source-building golangci-lint to the pinned `golangci/golangci-lint:${GOLANGCI_LINT_VERSION}-alpine` image available through Harbor.
  - Increased `lint:go` to 2 CPU / 4 GiB and a 30-minute golangci-lint timeout after the image-based job exposed cold-cache package loading as the next bottleneck.
  - Added a RALPH iteration plan for the CI repair slice.
- Why:
  - The previous source-profile template slice could not satisfy RALPH exit criteria while CI was red, and the failure was a CI bootstrap problem rather than a product-code lint finding.
- What’s next:
  - Push the CI repair commit to MR `!80`, re-arm auto-merge, and monitor the replacement pipeline.
  - Resume Wave 2 Slice 0 LLM reachability kill-test after the MR is green/queued.
- Sources:
  - [S1] `.gitlab-ci.yml`
  - [S2] `.loom/iteration-plan-ci-golangci-lint-image.md`
  - [S3] GitLab pipeline `14326`, job `142164`
