# GraphQL API Runtime Contract

This document describes the currently supported GraphQL runtime boundary. The
generated schema in `internal/api/graphql/schema.graphql` remains the complete
type reference.

## Current status

Slice 1.1c exposes one authenticated preview capability:

```text
Mapping Studio or GraphQL client
  -> authenticated bounded GraphQL POST
  -> previewIntegrationMessage
  -> server-owned integration binding
  -> canonical MessageProcessor
  -> raw-free ProcessResult projection
```

The preview path is stateless and cannot persist a receipt, sample, or run. It
cannot invoke a destination. Durable submit and delivery remain blocked on
Slice 1.2.

The response omits the source bytes, secrets, and executable configuration. Its
canonical event payload can still contain PHI and must be handled accordingly.

## Startup security boundary

`fi-fhir serve` fails startup unless every value below is present and valid.
Identity and artifact facts come from deployment configuration, never GraphQL
input.

| Variable | Requirement |
| --- | --- |
| `FI_FHIR_DEPLOYMENT_TENANT_ID` | Canonical deployment security-domain ID |
| `FI_FHIR_GRAPHQL_PRINCIPAL_ID` | Server-owned principal ID |
| `FI_FHIR_GRAPHQL_ROLES` | Comma-separated roles containing `integration:preview` |
| `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS` | Comma-separated exact HTTP(S) origins; no wildcard |
| `FI_FHIR_INTEGRATION_REGISTRY_PATH` | Strict immutable registry JSON for the same tenant |
| `FI_FHIR_GRAPHQL_BEARER_TOKEN` | Direct secret of at least 24 canonical bytes |
| `FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE` | Preferred production secret-file path |

Set exactly one token source. The registry is bounded, rejects unknown fields,
and verifies the definition, profile, and workflow digests before startup.

Implementation: `cmd/fi-fhir/preview_runtime.go`,
`internal/api/requestsecurity/auth.go`, and
`internal/integration/registry/static.go`.

## Authorization

The transitional `integration:preview` role permits only:

- `Query.health`
- `Mutation.previewIntegrationMessage`
- `__typename` on either allowed root operation

Aliases and fragments do not bypass this root-field allowlist. Subscriptions
and every legacy query or mutation are forbidden for the preview role.

`graphql:operator` is an explicit temporary escape hatch for authenticated
legacy capabilities. Do not assign it to the IDE bearer. Fine-grained RBAC,
OIDC, and audited user sessions remain Phase 4 work.

Implementation: `internal/api/graphql/operation_authorization.go`.

## Transport policy

### HTTP

- Path: `/graphql`
- Method: `POST` only; `OPTIONS` is accepted only for CORS preflight
- Content type: `application/json`
- Maximum complete request body: 1 MiB
- Authentication: `Authorization: Bearer <token>`
- Origins: exact allowlist match for browser requests
- Rejected: query-string operations, multipart, compressed bodies, wildcard or
  reflected origins, and GET queries
- Query limits: depth 10 and complexity 1000 in the default runtime

Non-browser clients may omit `Origin`, but they still require authentication.
CORS responses never enable credentialed wildcard access.

### WebSocket

- Path: `/graphql/ws` returns `404`
- No WebSocket transport is registered by `fi-fhir serve`
- The UI subscription adapter fails locally without resolving a credential,
  opening a socket, or retrying

This containment is deliberate: gqlgen does not provide a bounded
pre-authentication frame limit, and query or mutation execution over WebSocket
would bypass the POST-only body boundary. Re-enabling subscriptions requires a
separate bounded, authorized transport design.

Implementation: `internal/api/graphql/server.go` and
`ui/src/lib/graphql/subscriptions.ts`.

## Preview mutation

The caller owns only the browser-safe integration key, source bytes,
correlation ID, and human reason:

```graphql
input PreviewIntegrationMessageInput {
  integrationId: ID!
  data: String!
  correlationId: ID!
  reason: String!
}

type Mutation {
  previewIntegrationMessage(
    input: PreviewIntegrationMessageInput!
  ): IntegrationPreviewResult!
}
```

The server-owned registry supplies tenant, source, format, classification,
integration revision, profile revision, and workflow revision. The service
requires `integration:preview`, limits the raw message to 1 MiB, and calls the
same `MessageProcessor` used by direct kernel tests.

The typed result contains:

- exact integration, source, profile, and workflow provenance;
- canonical events and catalog-safe diagnostics;
- matched/skipped routes and planned action IDs;
- suppressed delivery plans; and
- correlation and trace identifiers.

It has no raw-message field, receipt, persisted run, secret, workflow source,
or executable client.

Implementation: `internal/integration/preview/service.go`,
`internal/api/graphql/resolvers/integration_preview.go`, and
`ui/src/lib/graphql/integrationPreview.graphql`.

## Mapping Studio credential and PHI handling

The Mapping Studio requires an operator to paste the deployment bearer before
the IDE renders. It validates the credential through `health` and then holds it
only in the current tab's JavaScript memory. It is not read from a `PUBLIC_*`
build variable and is never written to localStorage or sessionStorage.

Imported raw HL7 samples and filename-derived source labels also remain only in
the current store instance. Clear access or reload the page to discard the
credential; reload the page to discard samples and source labels. Browser
memory is still PHI-bearing while the tab is open, so use only approved
workstations and redact fixtures whenever possible.

On startup, the UI removes the two legacy localStorage keys used by earlier
releases for raw samples and recent source labels. Other browser preferences
are preserved.

Implementation: `ui/src/lib/graphql/GraphQLCredentialGate.svelte`,
`ui/src/lib/graphql/credentials.ts`, and
`ui/src/lib/features/hl7/samples/sampleStore.ts`.

## Legacy containment

The generated schema still describes older authoring and execution operations
for compatibility and future migration. They are not part of the preview
capability.

The default resolver configuration fails these paths closed:

- `submitMessage`, `submitEvent`, and `submitBatch`;
- `triggerWorkflow`;
- `parsePreview` and `parsePreviewWithProfile`;
- session sample, run, retained-raw export, and live-parse paths; and
- the profile-YAML HTTP endpoints and unauthenticated generic ingest webhook,
  which are no longer mounted by `serve`. The UI profile-YAML adapter fails
  locally and the canonical UI proxy exposes no `/api` fallback.

Containment tests require zero persistence and zero workflow-action calls.
Production submit remains unavailable until the durable receipt/idempotency/
outbox boundary ships.

Implementation: `internal/api/graphql/resolvers/schema.resolvers.go`,
`internal/api/graphql/resolvers/legacy_containment_test.go`, and
`cmd/fi-fhir/main.go`.

## Verification

Focused local checks for this boundary:

```bash
go test ./cmd/fi-fhir
go test ./internal/integration/registry
go test ./internal/api/graphql -run 'TestServerRejects|TestGraphQLHTTPBoundary|TestGraphQLWebSocketTransportIsDisabled|TestAuthenticatedPreviewTransport|TestPreviewTransportCatalog' -v
bash scripts/smoke-test_test.sh
```

MR, default-branch pipeline, image, and live rollout evidence are pending until
the Slice 1.1c release candidate ships.

## See also

- [Workflow DSL](WORKFLOW-DSL.md)
- [FHIR Subscriptions](FHIR-SUBSCRIPTIONS.md)
- [Operations Runbook](../operations/RUNBOOK.md)
- [Production Hardening](../operations/PRODUCTION-HARDENING.md)
