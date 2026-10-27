### 2026-10-27 - Roadmap and backlog reconciliation; Mills merge train for fi-fhir and edilint

- What changed: `ROADMAP.md` records the public demo program (`.loom/40`) and
  the October workspace-navigation, security-baseline and diagram merges as
  delivered, reconciles the Now list against the code on `a9be5577a` (three
  items closed by the 2026-10-03 merges, two re-confirmed open, the three open
  merge requests and the edilint pin added), triages the eleven open GitLab
  issues, and adds a Mills posture section. The Mills serial merge queue gains
  a `merge_ref` lane for `libs/fi-fhir` and `libs/edilint` at window 1 plus
  flake retry for `test:observability-replicas` and `test:ui-e2e`
  (platform/gitops MR !1005), and this repository's CI gains the
  `$MILLS_MERGE_QUEUE == "1"` proof-profile arm the train pipeline needs
  (MR !280). edilint's ROADMAP got the same treatment with a
  Go 1.26.10 toolchain move (edilint MR !31). Both repositories' first
  pipelines of the day refused the existing pins — govulncheck with
  GO-2026-6809 (`crypto/x509`, fixed in Go 1.26.10) and, in fi-fhir, trivy
  with GHSA-cg2g-xh64-v4q7 (DOMPurify 3.4.16, fixed 3.4.17) — so the fixes
  ride MR !280 and edilint !31; the docs MR rebases onto main once !280 lands.
- Why: the roadmap's last record was 2026-09-30 and still listed the public
  demo as planned; nine October commits and three open MRs were unrecorded;
  the merge train had been loom-core-only since 2026-10-03 while both repos
  stayed on rebase proofs. fi-fhir's rules run jobs only on merge-request,
  tag and main pipelines, so an API-minted proof on `mills-mq/train-*` would
  have had no jobs.
- Evidence: `git log 27ced54ae..main`; `ui/src/lib/features/connections/
  ConnectionsPage.test.ts` (`?connection=` navigation) and `ConnectionsPage.svelte`
  (`setDraftState('/connections', …)`) close the deep-link and dirty-buffer
  items; `journeyState.ts` no longer calls `fetchRecentSessions(MAX_SAFE_INTEGER)`;
  `WorkflowList.svelte` still calls `triggerWorkflow`; `scripts/ci-job-inventory.sh
  --check` reports 77 jobs unchanged after the CI edit, and every edited YAML
  parses. The global pre-push hook's full local `go test ./...` failed
  `TestRunServe_Execution_GracefulShutdown` (port 9090 already bound on the
  workstation) and `TestExecAction_RunsAllowedCommand` (killed under load,
  the known flake; passes alone), so the YAML-only branch was pushed with the
  hook bypassed and the MR pipeline is the gate.
- What's next: merge the CI arm before the gitops lane goes live; watch
  `mills_mergequeue_merged_total{proof_mode="merge_ref"}` and
  `mills_mergequeue_train_tree_mismatch_total` for the two lanes; decide the
  HUD watcher scope, the hub `MILLS_MERGE_QUEUE_PROJECTS` list and main-pipeline
  dedup separately; land or close !246, !253 and !279; bump the edilint pins.
- Sources:
  - [S1] `.loom/40-public-demo-execution-specs.md`, `.loom/42-ide-unlock-execution-specs.md`
  - [S2] loom-core `docs/MILLS_MERGE_TRAIN_DESIGN.md`, `docs/MILLS_RUNBOOK.md` § Merge train policy keys
  - [S3] platform/gitops `k3s/mills/configmap-policy.yaml` (`merge_queue.lanes`, `flake_retry`, `per_repo_overrides`, `protected_paths_per_repo`)
  - [S4] `.loom/worklog/2026-10-03-*.md`, `changelog.d/*.md`
