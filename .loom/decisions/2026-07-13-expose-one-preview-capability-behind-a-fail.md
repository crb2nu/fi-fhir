### 2026-07-13: Expose One Preview Capability Behind a Fail-Closed Deployment Boundary

- Decision:
  - Expose one typed `previewIntegrationMessage` mutation. Both the Mapping
    Studio direct preview and its former Integration Session client call it.
  - Load tenant, principal, roles, exact browser origins, one [REDACTED],
    and an immutable integration registry at startup. Missing, ambiguous, or
    inconsistent values prevent `serve` from starting.
  - Grant the transitional `integration:preview` role only `health` and
    `previewIntegrationMessage`. Keep `graphql:operator` as an explicit legacy
    escape hatch, not an IDE credential.
  - Accept GraphQL HTTP only as bounded `application/json` POST, require an
    exact allowed browser origin, require canonical duplicate-free JSON, return
    catalog-safe errors, and do not mount GraphQL WebSocket transport.
  - Keep [REDACTED], imported raw clinical samples, and
    filename-derived source labels in tab memory only. Reloading discards all
    three; **Clear access** discards the bearer. Purge the two legacy PHI-bearing
    localStorage keys during layout startup.
  - Stream bounded GraphQL request bodies through nginx/Ingress without proxy
    temp-file buffering. Compile the non-secret preview registry alias with a
    Vite-prefixed build variable; credentials remain runtime-only.
  - Leave submit, batch, workflow-trigger, parse-preview, session run/sample,
    export, and live-parse operations unavailable by default. Do not mount the
    profile-YAML or generic ingest HTTP bypasses.
- Rationale:
  - A single adapter makes parity with `MessageProcessor` directly testable and
    avoids preserving a second session-specific semantic path.
  - Server-owned identity and registry data prevent callers from selecting a
    tenant, source, profile, workflow, or executable revision.
  - Operation authorization is required in addition to [REDACTED]
    because the legacy schema still includes PHI and execution-capable fields.
  - Browser persistence is not an approved raw-PHI or secret store.
  - Proxy buffering and transport decoder errors are part of the PHI boundary,
    not merely implementation details behind the resolver.
- Alternatives considered:
  - Add a second Integration Session preview mutation (rejected because it
    duplicates the adapter contract and creates drift risk).
  - Make a [REDACTED] the complete legacy schema (rejected because those
    stores are not yet tenant-scoped and several mutations can execute actions).
  - Put the token in a `PUBLIC_*`, localStorage, or sessionStorage value
    (rejected because builds and browser persistence are not secret stores).
- Consequences:
  - Operators must supply the complete preview configuration and a random
    [REDACTED] at least 24 canonical bytes before `serve` starts.
  - This static [REDACTED] a transitional single-security-domain control. OIDC,
    fine-grained RBAC, audited token administration, and durable user sessions
    remain Phase 4 work.
  - Durable receipts, production submit, and delivery remain blocked on Slice
    1.2 even when preview is available.
- Sources:
  - [S1] `cmd/fi-fhir/preview_runtime.go`
  - [S2] `internal/api/requestsecurity/auth.go`
  - [S3] `internal/api/graphql/operation_authorization.go`
  - [S4] `internal/api/graphql/server.go`
  - [S5] `internal/integration/preview/service.go`
  - [S6] `internal/integration/registry/static.go`
  - [S7] `ui/src/lib/graphql/GraphQLCredentialGate.svelte`
  - [S8] `ui/src/lib/features/hl7/samples/sampleStore.ts`
  - [S9] `ui/src/lib/features/hl7/samples/legacyStorage.ts`
  - [S10] `ui/nginx/default.conf.template`
