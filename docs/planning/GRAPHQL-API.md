# GraphQL API Runtime Contract

This document describes the currently supported GraphQL runtime boundary. The
generated schema in `internal/api/graphql/schema.graphql` remains the complete
type reference.

## Current status

Slice 1.1c exposes one authenticated preview capability. Phase 4 Slice 4.1a
adds per-request OIDC human identity to the same bounded transport:

```text
Mapping Studio or GraphQL client
  -> authenticated bounded GraphQL POST
  -> previewIntegrationMessage
  -> server-owned integration binding
  -> canonical MessageProcessor
  -> raw-free ProcessResult projection
```

The preview mutation remains side-effect-free and cannot persist a receipt or
invoke a destination. Durable production submission and delivery use the
separate authenticated ingress and shared processor paths; Integration Session
operations retain their own PostgreSQL-backed authoring lifecycle.

The response omits the source bytes, secrets, and executable configuration. Its
canonical event payload can still contain PHI and must be handled accordingly.

## Startup security boundary

`fi-fhir serve` fails startup unless its common values and exactly one
authentication mode are valid. Artifact and tenant boundaries come from
deployment configuration, never GraphQL input.

| Variable | Requirement |
| --- | --- |
| `FI_FHIR_DEPLOYMENT_TENANT_ID` | Canonical deployment security-domain ID |
| `FI_FHIR_GRAPHQL_AUTH_MODE` | `static` (default compatibility path) or `oidc` |
| `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS` | Comma-separated exact HTTP(S) origins; no wildcard |
| `FI_FHIR_INTEGRATION_REGISTRY_PATH` | Strict immutable registry JSON for the same tenant |

Static mode additionally requires:

| Variable | Requirement |
| --- | --- |
| `FI_FHIR_GRAPHQL_PRINCIPAL_ID` | Server-owned compatibility principal ID |
| `FI_FHIR_GRAPHQL_ROLES` | Comma-separated roles, including `integration:preview` or `clinical:read` where appropriate |
| `FI_FHIR_GRAPHQL_BEARER_TOKEN` | Direct secret of at least 24 canonical bytes |
| `FI_FHIR_GRAPHQL_BEARER_TOKEN_FILE` | Preferred production secret-file path |

Set exactly one token source. The registry is bounded, rejects unknown fields,
and verifies the definition, profile, and workflow digests before startup.

OIDC mode instead requires:

| Variable | Requirement |
| --- | --- |
| `FI_FHIR_GRAPHQL_OIDC_ISSUER_URL` | Exact HTTPS issuer discovery URL |
| `FI_FHIR_GRAPHQL_OIDC_AUDIENCE` | Exact accepted token audience |
| `FI_FHIR_GRAPHQL_OIDC_TENANT_CLAIM` | Optional claim name; defaults to `tenant_id` |
| `FI_FHIR_GRAPHQL_OIDC_ROLES_CLAIM` | Optional claim name; defaults to `roles` |
| `FI_FHIR_GRAPHQL_OIDC_SIGNING_ALGS` | Optional allowlist; defaults to `RS256` |

The verifier accepts signed JWT access tokens with the protected header
`typ=at+jwt`; generic JWT/typeless tokens are rejected to prevent ID-token
substitution. It validates the issuer, one exact audience, signature, allowed
algorithm, expiry, not-before, and subject before accepting exact
deployment-tenant and strict role claims. It maps `sub` to a human request
principal. This token-class check is not a claim of complete RFC 9068
conformance.

The discovered `jwks_uri` and all outbound requests must remain HTTPS; redirects
are rejected.
The client has a 10-second maximum request timeout, a 1 MiB response cap for
discovery and JWKS documents, and a 30-second default floor between outbound
unknown-key refreshes. Providers
should publish new keys before issuing tokens that use them; a token for a newly
introduced key can otherwise fail until the refresh floor elapses. OIDC mode
rejects all static principal, roles, bearer-token, token-file, and trusted-CIDR
settings so no compatibility bypass is ambiguous.

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

### Per-root-field roles

Since Sprint 4 (Lane S4-E) the transport gate is an enumeration, not a boolean.
Every one of the 131 root fields — 64 `Query`, 60 `Mutation`, 7 `Subscription` —
names the roles a caller must hold, and a root field with no entry is refused.
`TestTransportGateRoleMapIsExhaustive` compares the map against the schema the
server actually executes, so a new root field cannot ship without a role
decision.

A field's requirement is an AND-set, matching what the service behind it demands
(`internal/integration/operator/service.go`), so the gate can never be more
permissive than the service. Twenty-six fields have fine-grained requirements:

| Requirement | Root fields |
|---|---|
| `integration.operator` | `operatorReceipts`, `operatorMessageTrace`, `operatorDeliveryAttempts`, `operatorDeliveryAttempt`, `operatorDeadLetters`, `operatorCircuits`, `operatorAttemptAudit`, `operatorDeployments`, `operatorDeploymentEvents` |
| `integration.operator` + `integration.delivery.operator` | `replayDelivery`, `resubmitMessage`, `discardDeadLetter` |
| `integration.operator` + `integration.deployment.operator` | `pauseIntegrationDeployment`, `resumeIntegrationDeployment`, `retireIntegrationDeployment`, `deployIntegrationRelease` |
| `clinical:read` | `event`, `events`, `patient`, `patients`, `patientTimeline`, `eventStatistics`, `activeEncounters`, `activeEncounter`, `activeEncounterByPatient`, `projectionStatus` |

`integration.phi.export` is not in the table on purpose. It gates the
`includeRawPayload` argument of `exportIntegrationBundle`, not any field, so a
token holding only that grant reaches nothing at the transport gate. The
colon-form `integration:submit` / `integration:mllp` / `integration:batch`
grants are minted by the ingress, MLLP, and batch transports for their own
principals and are never carried by a GraphQL token, so the submit mutations are
not mapped to them either.

### The compatibility grant

`graphql:operator` is a **named compatibility grant** that expands to all 131
root fields. A token holding it behaves exactly as it did before the narrowing.
It is deprecated, not removed: the remaining 105 root fields — the legacy
workflow catalog, FHIR subscriptions, the integration session workspace,
profiles, LLM, terminology and autoroute review, Temporal, the debugger, and
all seven subscriptions — have no shipped
fine-grained role and are reachable only through it. Each one carries an
explicit entry and a `TODO` naming its follow-up slice, so the ungoverned
surface is enumerable rather than implicit.

Do not assign `graphql:operator` to a normal IDE caller, and do not swap it out
of an existing operator token for the fine-grained roles — that keeps the
control plane and loses the whole IDE. `serve` prints the mapping's shape at
startup so a deployment still relying on the grant says so in its own log.

Implementation: `internal/api/graphql/operation_authorization.go` and
`internal/api/graphql/operation_authorization_roles.go`. The narrowing is
defence in depth: every request that clears the gate is authorized again by the
service that answers it.

## Transport policy

### HTTP

`GET /metadata` is an unauthenticated, PHI-free FHIR R4 metadata route. It
returns `application/fhir+json` with the resources emitted by the US Core
mapper. The CapabilityStatement deliberately declares no read or search
interactions because fi-fhir delivers mapped resources to configured
destinations rather than serving them through a FHIR REST repository.

- Path: `/graphql`
- Method: `POST` only; `OPTIONS` is accepted only for CORS preflight
- Content type: `application/json`
- Maximum complete request body: 1 MiB
- Authentication: `Authorization: Bearer <token>` verified by the selected
  static or OIDC authenticator
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
