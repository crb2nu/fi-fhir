### 2026-07-12: Fix Tenancy, Identity, Secret, and PHI Contracts Before Schemas

- Decision:
  - Slice 1.0 defines a single security domain per 1.0 deployment, required
    logical tenant/actor propagation, secret references, PHI classification,
    raw-retention policy, encryption/TTL/audit fields, and production/preview
    modes before receipt and trace migrations are written.
- Rationale:
  - These fields define primary keys, access predicates, audit semantics, and
    retention behavior. Retrofitting them after durable ingestion would risk PHI
    leakage, destructive migrations, and incompatible receipts.
- Alternatives considered:
  - Add auth/RBAC/PHI only in the operations phase (rejected; enforcement UI can
    come later, but persistence boundaries cannot).
  - Claim shared multi-tenant hosting in 1.0 (rejected until isolation tests and
    operational ownership are proven).
- Consequences:
  - Every durable runtime/artifact type is tenant- and actor-aware from creation.
    Fine-grained UI RBAC and shared-hosting certification remain later slices.
- Sources:
  - [S1] `.loom/20-product-spec-integration-engine-ide-completion.md`
  - [S2] `.loom/30-implementation-plan-integration-engine-ide-completion.md`
