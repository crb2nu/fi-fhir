### 2026-09-19 - CDA Source Profile event selection

- What changed: `source_profile.cda` now selects document and section event
  emission in both `parse --format cda` and `ccda`. A nonempty section list
  enables only entries with `emit_events: true`; global section suppression
  takes precedence. Missing settings retain the existing CLI output.
- Why: `.loom/25-spec-cda-section-expansion.md` requires profile-controlled
  emission. The three section extractors had shipped (#13), but the profile
  type lacked CDA settings and the mapper ignored its two emission flags.
  Current FHIR and workflow work is already covered by open MRs !204–!208.
- Evidence: `TestParseCDAProfileSelection` first failed on unmodified product
  code: a medication-only profile emitted all four events. The completed
  implementation passes `go test -race ./internal/parser/cda ./pkg/profile
  ./cmd/fi-fhir -count=1`; affected-package golangci-lint reports zero issues.
  Profile tests cover load/lint errors, nested unknown keys, explicit false
  versus omission, JSON/YAML round trips, and the custom YAML renderer.
  Mapper tests prove disabled custom mappers are not invoked, configuration
  is copied, and parsed patient/document data remains available.
- Scope: selection is not redaction; full document/raw XML still returns.
  `NewMapper(nil)` emits everything; explicit `MapperConfig` flags are now
  honored, including zero values. Document-type restrictions and code-system
  overrides remain unimplemented and are marked as such in the CDA guide.
- Tooling: devbox returned `Transport closed`, so local race tests and GitLab
  CI provide verification. Agent-context session `18a007776b2c028f` started,
  but persistence returned a Qdrant vector-dimension mismatch; this entry
  preserves the decision and test evidence. Worktree allocation returned a
  home-level path despite the repo-relative request; it was moved under
  the repo's `.worktrees/codex/cda-profile-section-selection` before edits.
- What's next: run the branch through required CI and merge under the
  workspace auto-ship policy. Narrative fallback and partial-data warnings
  remain a separate CDA follow-up.
- CI prerequisite: existing image-scan job 283441 on MR !208 rejects gRPC
  1.83.1 for CVE-2026-84445. Updated to 1.83.2 and its required `x/net` 0.58.0;
  `go mod tidy` changes only these two module versions/checksums. Upstream:
  https://github.com/grpc/grpc-go/releases/tag/v1.83.2. The full local
  `go test -race ./...` suite passes (59 tested packages), and `govulncheck
  ./...` reports no reachable vulnerabilities.
- Sources: `.loom/25-spec-cda-section-expansion.md`,
  `docs/planning/CDA-CCDA.md`, `pkg/profile/cda.go`,
  `internal/parser/cda/mapper.go`, `cmd/fi-fhir/parse_cda_profile_test.go`.
