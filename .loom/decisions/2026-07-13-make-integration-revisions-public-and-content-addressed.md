### 2026-07-13: Make Integration Revisions Public and Content-Addressed

- Decision:
  - Put the dependency-light product boundary in public `pkg/integration` and
    keep the future orchestrating implementation in `internal/integration/processor`.
  - Bind integration definitions to exact source, profile, workflow, and
    destination revisions with SHA-256 digests. Treat destination, secret, and
    role collections as sets for semantic digesting; exclude only the revision
    digest from its own preimage.
  - Keep source bytes in an unexported, defensive-copy envelope field and make
    preview side-effect freedom a validated result invariant. A sandbox may be
    planned in preview, but only an audited production request may execute it.
  - Construct processed events through a package-owned registry of concrete
    canonical event schemas, stripping raw fields and parser warning text before
    serialization. Construct diagnostics from a safe code-to-message catalog;
    reject duplicate or noncanonical JSON keys at every contract decode boundary.
  - Bind production results back to the authenticated request and require exact
    source-message, correlation, route, action, event, and attempt lineage; cap
    encrypted receipt expiry at the revision policy TTL.
  - Pin Kubernetes 1.36 as the 1.0 deployment minor while keeping installation,
    upgrade, recovery, performance, and standards claims behind explicit gates.
- Rationale:
  - Ingress, GraphQL, the IDE, persistence, and SDKs need one stable contract
    without importing internal profile/workflow/session storage models.
  - Content addressing detects mutation and prevents a published integration
    from silently following mutable “current” pointers.
  - Raw bytes and inline secret values are unsafe serialization defaults for PHI
    workloads; preview must be safe by construction, not caller convention.
  - Internal consistency is insufficient for audit: a self-consistent result
    could otherwise substitute an actor, reason, correlation, or explicit
    idempotency key from the request.
  - Creation actor, reason, and time are integrity-bearing audit facts, so the
    revision digest protects them along with the artifact and policy bindings.
  - A pinned deployment target makes later performance and recovery evidence
    comparable while the evidence-status labels prevent premature support claims.
- Alternatives considered:
  - Reuse Integration Session DTOs (rejected because they contain raw samples,
    nondeterministic diagnostics, and prototype retention behavior).
  - Reuse GraphQL profile/workflow store records (rejected because their
    revision identifiers differ and they lack tenant/content-digest contracts).
  - Hide all contracts under `internal` (rejected because adapters and SDK/API
    serializers need a deliberate stable boundary).
  - Pin the current k3s 1.33 cluster (rejected because it is outside the selected
    upstream-supported 1.0 target and has not run the release proof).
- Consequences:
  - Slice 1.1 must adapt existing parser/workflow/session representations into
    these contracts and prove exact artifact resolution after later revisions
    publish.
  - Slice 1.2 can design receipts and outbox schemas without retrofitting tenant,
    PHI, secret, or revision identity.
  - Changing semantic digest rules or the Kubernetes minor is now a versioned,
    dated compatibility decision.
- Sources:
  - [S1] `pkg/integration/revision.go`
  - [S2] `pkg/integration/contracts.go`
  - [S3] `internal/integration/session/types.go`
  - [S4] `internal/api/graphql/store/profile_store.go`
  - [S5] `internal/api/graphql/store/workflow_lifecycle_store.go`
  - [S6] `docs/operations/SUPPORTED-1.0.md`
  - [S7] https://kubernetes.io/releases/1.36/
