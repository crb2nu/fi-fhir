### 2026-07-18: Sign Verified Production Bindings, Not Session Identities

- Decision:
  - Publish a canonical PHI-minimal manifest that records both the exact session
    revisions tested and the exact production artifact references to deploy.
  - Resolve production bytes through the existing artifact resolver and
    recompute production-domain references from session content; never copy a
    session digest or mutable current pointer into deployment evidence.
  - Verify a detached Ed25519 signature against an explicit trust root before
    lifecycle approval and again immediately before deployment, then reuse the
    existing optimistic lifecycle state graph and immutable release record.
- Rationale:
  - Session revisions use opaque IDs and plain SHA-256, while production profile
    and workflow references use different domain-separated digest rules. One
    identity cannot safely stand in for the other even when content matches.
  - A second deployment state machine would create conflicting release truth and
    bypass existing validation freshness, optimistic version, and active-release
    invariants.
- Alternatives considered:
  - Copy session refs directly (rejected because they are invalid production
    identities and digest domains).
  - Promote current profile/workflow pointers (rejected because later edits could
    silently change the approved executable bytes).
  - Mutate GitOps from the authoring flow (deferred as a separately reviewed
    production operation).
- Consequences:
  - Publication requires an already-validated exact definition and configured
    matching PKCS#8/PKIX Ed25519 keys. Partial configuration fails startup.
  - Publication is append-only, rejects retained-raw fixtures, and performs no
    transform, action, destination, network, or GitOps side effect.
- Sources:
  - [S1] `internal/integration/session/publication.go`
  - [S2] `internal/integration/processor/revisions.go`
  - [S3] `internal/integration/lifecycle/transitions.go`
  - [S4] `.loom/iteration-plan-phase-3-slice-3-4-publish-deploy.md`
