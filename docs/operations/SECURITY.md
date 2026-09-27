# Security findings register

Production security findings against `fi-fhir serve`, what each one allowed,
and how it was closed. General deployment guidance lives in
[Production Hardening](PRODUCTION-HARDENING.md); this page is the record of
specific findings and the deployment-owned settings that close them.

| ID | Finding | Found | Closed | Reachable by |
|---|---|---|---|---|
| [SEC-2026-09-27-1](#sec-2026-09-27-1-the-workflow-debugger-executed-actions) | The workflow debugger executed actions from caller-supplied YAML, and `exec`'s allowlist came from the same YAML | 2026-09-27 | 2026-09-27 | `graphql:operator` |
| [SEC-2026-09-27-2](#sec-2026-09-27-2-createfhirsubscription-connected-to-any-url) | `createFhirSubscription` connected to any URL; its client map was unbounded | 2026-09-27 | 2026-09-27 | `graphql:operator` |

Both were found by the public-demo deployment survey (`.loom/40` § "Two
production findings") and gate any hosted public deployment. Neither was
reachable by the `integration:preview` role: the transport gate admits only
`health` and `previewIntegrationMessage` for it. `graphql:operator` is held by
the LAN trusted-network CIDR, the Cloudflare Access identities mapped to it,
and the static bearer, so exposure was low but real. The cluster-side
complement is a NetworkPolicy for the `fi-fhir` namespace in `platform/gitops`
(`k3s/fi-fhir/network-policy.yaml`), which limits what the API pod can reach
even if an allowlist below is set too wide.

## SEC-2026-09-27-1: the workflow debugger executed actions

**What it was.** `startDebugSession`
(`internal/api/graphql/resolvers/schema.resolvers.go:1766-1787` before the fix)
parsed caller-supplied workflow YAML, built a production engine with
`workflow.NewEngine`, and ran it (`internal/workflow/debug.go:152` →
`Engine.ProcessWithContext`). `NewEngine` registers `log`, `webhook`, `fhir`,
`email`, `exec`, `file`, `database`, `queue`, `event_store` and `athena`
(`internal/workflow/engine.go:124-133`), so a debug session sent real webhooks,
wrote files, published to queues, and so on. The `exec` action's allowlist was
the action's own `allowlist` config key
(`internal/workflow/actions.go:383-409`) — part of the same YAML that names the
command — so a caller could run any binary in the image. Debug sessions were
held in an unbounded map and never expired.

**How it is closed.**

- The debugger builds its engine with `workflow.NewDebugEngine`. Every action
  type not named in `FI_FHIR_WORKFLOW_DEBUG_ACTIONS` is replaced by a recording
  no-op stub. The default is empty, so **every action is stubbed**. `exec` can
  never be enabled there: serve refuses to start if the list names it, or names
  an action type that does not exist.
- Each action step in the debug trace carries `action.stubbed` and
  `action.inputs` — the action's config with templates resolved against the
  event, credential-named keys (`password`, `token`, `secret`, `dsn`, …) and URL
  passwords redacted, bounded to 64 keys of 2 KiB each — so the debugger still
  shows what the action would have done.
- `exec` on every engine `workflow.NewEngine` builds (`serve --workflow`,
  published workflow versions, the workflow CLI commands) requires the command
  in the deployment-owned
  `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST` **and** in the action's own `allowlist`. The
  YAML list can only narrow the deployment's. Empty (the default) refuses every
  command. Serve refuses to start on a relative or unclean path.
- Debug sessions are bounded per process: at most
  `FI_FHIR_WORKFLOW_DEBUG_MAX_SESSIONS` (default 8; `0` disables the debugger),
  each living `FI_FHIR_WORKFLOW_DEBUG_SESSION_TTL` (default `15m`) from
  creation. An expired session is stopped and forgotten on the next debugger
  call. At capacity, `startDebugSession` returns "workflow debugger is at
  capacity; end a debug session or retry later", which names no other session
  or principal. Metrics: `fi_fhir_workflow_debug_sessions` (gauge) and
  `fi_fhir_workflow_debug_sessions_total{outcome}` (`accepted`, `rejected`,
  `processed` = ended, `dropped` = expired).

**Proof.** `internal/workflow/debug_actions_test.go`
(`TestDebugSessionRecordsStubbedActions`: a webhook to a live test server and
an `exec` whose command is in both allowlists run to completion with zero
requests and no process), `internal/workflow/exec_policy_test.go`
(`TestEngineExecAllowlistIsDeploymentOwned`, with a positive control),
`internal/api/graphql/resolvers/debug_policy_test.go` (capacity, TTL, metrics,
default stubbing), `cmd/fi-fhir/workflow_hardening_runtime_test.go` (startup
validation).

## SEC-2026-09-27-2: `createFhirSubscription` connected to any URL

**What it was.** `createFhirSubscription`
(`internal/api/graphql/resolvers/schema.resolvers.go:850-870` before the fix)
called `getOrCreateSubscriptionClient(input.Server)`
(`internal/api/graphql/resolvers/resolver.go:528-554`), which built a client
for whatever URL the caller supplied and POSTed a Subscription resource to it —
any scheme, any host, following redirects. Each distinct URL added a client to
a map that never shrank. The four subscription mutations are
`graphql:operator` only (`internal/api/graphql/operation_authorization_roles.go:256`).

**How it is closed.**

- A destination must use `https` (plain `http` only to a loopback host, which
  exists for tests), carry no user info, and have a host matching
  `FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS`: comma-separated exact host names or
  IP literals, or `*.suffix` for any name strictly below a multi-label suffix.
  A wildcard never matches an IP literal or `localhost`. Empty (the default)
  refuses every destination.
- Every refusal returns the same message, "FHIR subscription destination not
  allowed", and makes no request. It does not echo the URL or reveal the list.
- The policy is re-applied to every redirect hop, so an allowed server cannot
  bounce the request elsewhere; at most five hops.
- Clients live in an LRU bounded by `FI_FHIR_FHIR_SUBSCRIPTION_MAX_CLIENTS`
  (default 32). Refused destinations never occupy a slot.
- Resolving the host and refusing private address ranges is deliberately not
  done here; the namespace NetworkPolicy covers that at the network layer.

**Proof.** `internal/api/graphql/resolvers/subscription_policy_test.go`
(allowlist parsing and matching tables, the refusal making zero requests, an
allowed loopback destination, a redirect off the allowlist refused before the
target is contacted, LRU eviction).

## Deployment settings

All six keys are optional, reported by the Engine tab (`engineRuntime`) and
documented in `fi-fhir serve --help`. A malformed value fails startup.

| Key | Default | Meaning |
|---|---|---|
| `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST` | empty: `exec` refuses everything | Absolute executables `exec` may run; workflow YAML can only narrow it |
| `FI_FHIR_WORKFLOW_DEBUG_ACTIONS` | empty: every action stubbed | Action types the debugger executes for real; never `exec` |
| `FI_FHIR_WORKFLOW_DEBUG_MAX_SESSIONS` | `8` | Debug sessions per process; `0` disables the debugger |
| `FI_FHIR_WORKFLOW_DEBUG_SESSION_TTL` | `15m` | Debug session lifetime from creation |
| `FI_FHIR_FHIR_SUBSCRIPTION_ALLOWED_HOSTS` | empty: every destination refused | Exact hosts or `*.suffix` subscription management may contact |
| `FI_FHIR_FHIR_SUBSCRIPTION_MAX_CLIENTS` | `32` | Cached subscription clients |

## Known residual risks

- `requestsecurity/trusted_network.go` accepts `0.0.0.0/0` as a trusted CIDR
  without an explicit opt-in (tracked by the public-demo program, lane D-3).
- There is no GraphQL rate limit.
- A command allowed by `FI_FHIR_WORKFLOW_EXEC_ALLOWLIST` inherits the full
  process environment, including any credential the pod holds. Keep the list
  to purpose-built wrappers.
