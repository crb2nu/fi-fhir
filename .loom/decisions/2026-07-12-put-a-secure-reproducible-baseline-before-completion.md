### 2026-07-12: Put a Secure, Reproducible Baseline Before Completion Features

- Decision:
  - Gate all engine/IDE completion work behind Go 1.26.5, a Go-1.26-compatible
    pinned linter, pinned scanners, and strict validation/quoting for
    configuration-controlled SQL identifiers.
- Rationale:
  - A deployed-build pipeline contained reachable standard-library
    vulnerabilities and a HIGH/HIGH SQL injection while advisory jobs still
    allowed a green pipeline. Additional review found the same identifier class
    in IDE-authored workflow actions.
- Alternatives considered:
  - Defer security to release-candidate hardening (rejected; it would build new
    schemas and ingress on a known unsafe/unreproducible baseline).
- Consequences:
  - Gate 0A precedes the shared runtime spine. Gate 0B promotes proven security
    jobs to required merge gates and reconciles remaining CI truthfulness.
- Sources:
  - [S1] GitLab pipeline `15878`
  - [S2] `cmd/fi-fhir/eventstore.go`
  - [S3] `internal/workflow/event_store.go`
  - [S4] `internal/workflow/database.go`
  - [S5] https://go.dev/doc/devel/release
