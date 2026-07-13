# Iteration Plan: Phase 1 Slice 1.1c Authenticated Preview Adapters

**Status**: complete
**Date**: 2026-07-13

## Outcome

Expose the Slice 1.1b `MessageProcessor` to GraphQL and the Integration Session
IDE through one authenticated, bounded, side-effect-free preview service while
failing every legacy raw-submit or session-execution path closed.

## Scope

- Add a transport-neutral preview service that creates `ProcessRequest` only
  from an authenticated deployment tenant/principal and calls the exact 1.1b
  processor.
- Add one stateless GraphQL `previewIntegrationMessage` mutation returning the
  raw-free `ProcessResult` contract. The Mapping Studio and its former
  Integration Session client both use this operation.
- Require POST on the HTTP GraphQL endpoint, bound request bodies, enforce exact
  HTTP origins, and leave WebSocket transport unmounted.
- Add a fail-closed bearer authenticator suitable for the single-security-domain
  1.0 deployment boundary; full OIDC/RBAC remains Phase 4 work.
- Disable legacy `parsePreview`, direct production submit, raw session sample/run
  preview, retained-raw export, and unauthenticated generic webhook execution by
  default.
- Move the feature-flagged Integration Session UI preview onto the stateless
  kernel-backed mutation, forward the in-memory credential to HTTP, and make
  subscription consumers fail locally without a socket or retry loop.
- Update roadmap/implementation truth and add transport/security regression
  coverage.

## Out of scope

- Durable receipts, idempotency, outbox, or production delivery (Slice 1.2).
- Integration definition publication/deployment lifecycle (Slice 2.1).
- Durable Integration Session storage and retained-PHI policy (Slice 3.1).
- OIDC discovery, fine-grained RBAC, shared multi-tenancy, or audited token
  administration (Phase 4).

## Riskiest assumption

**Assumption:** a deployment-scoped bearer identity plus a server-owned exact
integration revision is a sufficient temporary authentication/configuration
boundary to activate preview without creating a second semantic path or a PHI
persistence/execution bypass.

Why it is load-bearing: if the adapter accepts caller-owned tenant/principal
facts, resolves a different revision, buffers an unbounded body, or leaves a
legacy action path reachable, the preview surface is unsafe even though the
kernel itself is deterministic and side-effect-free.

## Kill test (target: under 30 minutes)

Run one real ADT A01 fixture through the GraphQL preview mutation and directly
through the same `MessageProcessor`, using a fixed clock and exact immutable
definition/profile/workflow revisions. Require byte-equivalent JSON results. In
the same transport test suite require rejection of missing/invalid auth, wrong
tenant, GET, oversized/chunked bodies, disallowed HTTP origin, and WebSocket
upgrade attempts on both GraphQL paths. Assert legacy submit/session preview and retained-raw
sample/export surfaces fail closed and no store/action spy is invoked.

**Stop condition:** do not activate or merge the IDE adapter if the GraphQL
adapter differs from direct kernel output, the UI accepts provenance drift, any
rejection case reaches the processor, or any legacy path persists raw/event/run
state or invokes a workflow action.

## Execution order

1. Add failing service, resolver, HTTP boundary, and WebSocket-containment tests.
2. Implement authenticated context, exact-origin policy, POST/body gates, and
   the shared preview service.
3. Generate GraphQL types and implement the one stateless resolver adapter.
4. Default legacy raw/execution surfaces to unavailable and remove the webhook
   runtime mount.
5. Switch the feature-flagged Integration Session UI client to the stateless
   mutation and add credential forwarding/tests.
6. Run focused tests, full Go/UI checks, race/security gates, and independent
   review; then ship through MR, required CI, merge, main CI, and image evidence.

## Local implementation evidence

The current Slice 1.1c working tree has passed these focused checks:

```bash
go test ./...
go test -race ./cmd/fi-fhir ./internal/api/requestsecurity ./internal/integration/registry ./internal/integration/preview ./internal/api/graphql ./internal/api/graphql/resolvers
go vet ./...
golangci-lint run --timeout=30m ./cmd/... ./internal/... ./pkg/... ./scripts/... ./sdk/...
bash scripts/smoke-test_test.sh
helm lint deploy/helm/fi-fhir
sh scripts/validate-kustomize-preview.sh
make security-vulncheck security-gosec
```

The UI recorded 598 passing tests with three skipped, plus green typecheck,
ESLint, Stylelint, Svelte check, production build, Vite alias bundle proof, and
stable GraphQL/OpenAPI code generation. Startup purges only the two known legacy
localStorage keys that held raw samples or PHI-bearing source labels.

A compiled `fi-fhir serve` process passed real HTTP probes for missing/wrong
authentication (`401`), disallowed origin (`403`), duplicate/malformed JSON
without PHI reflection (`400`), legacy-catalog denial (`FORBIDDEN`), disabled
WebSocket (`404`), removed profile-YAML route (`404`), and one ADT A01 preview
with exactly one event and one suppressed delivery. The focused parity test
proves the typed mutation matches the direct kernel.

The live rollout prerequisites were applied before publishing a new image:
GitOps MRs `!359` and `!360` suspended fi-fhir image automation, mounted the
immutable registry and SOPS-managed token, bounded/streamed request bodies, and
removed the legacy `/api` ingress. Flux applied revision
`2c8855be65a77426702550a9c49c64dc83e23970`; the existing rollback image
`v0.1.18548` remained healthy while automation stayed suspended.

## CI and merge

- Application MR `!96` merged at `2026-07-13T16:54:12Z` as
  `9eac738a12da7d0837d470ed2b1ab8764a2ebb15`.
- MR pipeline `18604` passed all 30 jobs. The required benchmark job `179305`
  passed, and merged coverage was 44.50%.
- Default-branch pipeline `18621` passed all 33 jobs at
  `2026-07-13T17:24:23Z`, including blocking lint, test, benchmark, security,
  build, image-scan, and API/UI publish gates.

## Published images

- API `registry.harbor.lan/library/fi-fhir:v0.1.18621` resolved to
  `sha256:233085b9927063e1811bd9e87104bc5a05bbe231bc8ffaeae28f737d6b591fab`.
- UI `registry.harbor.lan/library/fi-fhir-ui:v0.1.18621` resolved to
  `sha256:9dd708b303e65c75fb7132b2e041636a969dc0afecc95a156649612e6d20deea`.
- Independent manifest inspection matched the digests reported by both Harbor
  publish jobs. The pre-rollout rollback tag remained `v0.1.18548`.

## GitOps rollout

- GitOps MR `!368`, pipeline `18630` (13/13 jobs), merged as
  `ce05b41c368acacd9961241c5f06723e8a449a45`. It pinned the matching images,
  disabled playground/introspection, and corrected UI probes to `/ui/health`.
- Flux applied `main@sha1:ce05b41c368acacd9961241c5f06723e8a449a45`.
  Both deployments rolled out successfully and reported Ready on the exact
  published image IDs while image automation remained suspended.

## Live acceptance probes

The pre-resume public-ingress gate passed while image automation still reported
`suspend=true`, after both release ReplicaSets were created at
`2026-07-13T17:29:10Z` and before the automation-resume pipeline was created at
`2026-07-13T17:33:13Z`. The same gate was reverified from
`2026-07-13T17:39:01Z` through `2026-07-13T17:39:02Z` after the resume:

- missing and wrong bearer credentials returned `401`; a disallowed Origin with
  the valid deployment credential returned `403`;
- authenticated health returned `200`; the legacy event catalog returned a
  GraphQL `FORBIDDEN` error;
- the supported ADT A01 preview returned one `patient_admit` event, one matched
  route, and one `suppressed` delivery with exact tenant, correlation,
  integration, source, profile, workflow, and destination provenance;
- the raw sentinel plus `rawPayload`, `security`, and `receipt` fields were
  absent from the response, and the sentinel was absent from API and ingress
  logs;
- `/graphql/ws`, `/api`, and `/api/v1/parse` returned `404`; the UI root and
  `/ui/health` returned `200`.

No bearer value or raw clinical payload is retained in this evidence.

## Automation resume

- GitOps MR `!369`, pipeline `18634` (13/13 jobs), merged as
  `756faa46725db828fa3f35d83c6d4c129d17dc7f` after the live gate passed.
- Flux applied `main@sha1:756faa46725db828fa3f35d83c6d4c129d17dc7f`.
  `ImageUpdateAutomation/fi-fhir` reported `suspend=false`, Ready `True`,
  reason `Succeeded`, and `repository up-to-date`; both image policies and
  running deployments remained on `v0.1.18621` and the verified digests.

## Evidence harvested and handoff

Slice 1.1c is complete. Its red-to-green, focused, race, full-suite, generated
code, security, build, image, GitOps, live-containment, PHI-leakage, rollback,
and automation-resume evidence is recorded above. Slice 1.2 is the next bounded
slice: add durable receipts, effective idempotency, trace/event/outbox writes,
restart proof, and concurrency semantics around the same evaluator.
