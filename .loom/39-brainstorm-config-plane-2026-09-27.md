# Brainstorm: a database-backed configuration option alongside GitOps

**Date**: 2026-09-27
**Triggered by**: Cody, after the Connections program (`.loom/38`) put a durable
catalog of connection drafts and compiled revisions behind the IDE while
leaving activation to GitOps: "design a db based config option vs only
gitops — research how this is best handled and brainstorm the best approach
for our stack, relevant for flexinfer and loom-core too."
**Constraints noted**: PHI and 45 CFR 164.312 audit/integrity apply to
fi-fhir; secret values must never live in a database or a UI-to-Git commit;
multi-replica; PostgreSQL is available; Flux CD is the GitOps engine for both
clusters; the LAN push to gitlab.flexinfer.ai is unreliable (hairpin, see
memory); `.loom/38` Decision 1 explicitly scoped hot-loading out of the
Connections program, so this document is the place to decide whether and how
it comes back in.
**Inputs**: a read-only survey of the three repos (fi-fhir main +
`feat/connections-0-catalog`, flexinfer `origin/master`, loom-core main,
platform/gitops) and an external precedent study (~80 primary and secondary
sources). Both are condensed in the appendices; the appendices are the
evidence, the phases are the reasoning.

---

## The question, sharpened

"DB config vs GitOps" is not one decision. Every mature product surveyed
answers a narrower question, **who owns this object right now?**, in one of
three ways:

1. **A global mode switch** that locks the imperative write API (Kong DB-less
   returns 405 on writes; Kuma on Kubernetes has a read-only API; Strimzi
   `use-connector-resources` reverts REST changes; Sourcegraph
   `SITE_CONFIG_FILE`).
2. **A per-object provenance flag**: provisioned objects are read-only in the
   UI, UI-created ones are editable (Grafana `allowUiUpdates: false`,
   InterSystems IRIS system overrides, Backstage location-owned entities).
3. **Routing UI writes back into the declarative store** as a commit or MR
   (Grafana Git Sync, Backstage scaffolder, Kratix, Flux image automation,
   Argo CD Image Updater git write-back).

Every product that lets both paths write the same object without an
ownership rule documents a "your edit will be silently overwritten" caveat.
Our three repos already contain all three answers in embryo (Appendix A):
fi-fhir refuses live writes to Git-owned objects by policy; Mills opens an MR
against platform/gitops instead of writing the live ConfigMap; flexinfer's
MCP tools patch Flux-managed CRs with no ownership marker, which is the
documented silent-overwrite case.

So the framings below are about **which ownership model**, **which tier of
configuration**, and **which reload class**, per repo.

## Phase 1 — Framings

### F1 — GitOps-only, better tooled (the lazy default)

Keep Git as the sole source of runtime truth. Invest in the seams: the
catalog exports exact revision bytes (C-1 already does "Download
`<id>-rN.json`"), a "copy as manifest" affordance renders the ConfigMap or
SOPS stanza, Reloader or hash-suffixed ConfigMaps roll pods, and the Engine
tab keeps telling users which env key to change. Nothing new runs from the
database.

- **Bet**: operators accept minutes of latency and a Git review for every
  change, and the IDE's job is to author artifacts, not to activate them.
- **Risk**: the UI stays a design surface. Users route around it (the
  Airbyte "UI-only drift" story in reverse), and the lifecycle catalog stays
  empty in production because nothing writes it.

### F2 — Catalog-authoritative, Git as journal

The PostgreSQL catalogs (lifecycle + connection) become the runtime source of
truth for integration configuration. `serve` loads the active revision set
from the DB at startup and on generation change; Flux owns only the
structural tier (Deployment, secrets, network, DB DSN). A leader job exports
each activation as a signed OCI artifact (`flux push artifact` + cosign) or to
a bot-only Git branch, the way NiFi Registry's `GitFlowPersistenceProvider`
treats Git as a write-only journal. Flux never applies the export.

- **Bet**: auditors accept a DB as the state store (OpenGitOps' glossary
  allows "any other system" with immutability and history), and four-eyes
  approval in-app replaces the MR gate.
- **Risk**: DR needs an importer; the static registry and the mounted source
  documents have to be migrated in, or fi-fhir runs two truths for the same
  definitions during the transition (it already does, see Appendix A §A.4).

### F3 — Provisioned baseline + catalog overlay (per-object ownership)

Two tiers with a hard boundary. Flux delivers the structural tier and a
**baseline** of connection and definition revisions as today's
content-addressed JSON. At startup the engine imports every mounted document
into the catalog as `owner = gitops` revisions: visible, read-only in the UI,
with "clone to draft" (what the Grafana Operator docs recommend). UI-authored
objects are `owner = catalog` and go draft → compile → approve → activate
behind a generation-numbered pointer. The effective set is an ordered merge,
baseline first then catalog (flagd's ordered-merge semantics), and a
collision on the same key is **refused at publish**, never shadowed. This is
InterSystems IRIS's production-definition vs System Default Settings split,
with IRIS's "system overrides" as the lock.

- **Bet**: the boundary can be enforced by schema (a catalog row can never
  hold secret material, and can only reference a secret name on a
  Git-declared allow-list), so boundary creep is a compile error, not a
  policy.
- **Risk**: two provenance paths to explain and test; the merge rules must
  be exact or the UI's "Mounted here" label lies.

### F4 — UI proposes, Git disposes (the MR bridge)

Drafts live in the DB. "Publish" renders the compiled revision (secret
references only, never a sample message) and opens an MR against
platform/gitops through a narrowly scoped bot token. Flux deploys on merge;
the UI shows `pending-merge` then `applied @digest` from observed status.
Mills already does exactly this for its policy ConfigMap
(`handlers_policy.go:109-117`: "Flux owns that file").

- **Bet**: Git review is a compliance requirement worth minutes of latency,
  and a push credential in-cluster can be scoped to one path and a
  protected-branch MR flow.
- **Risk**: incident-time edits ("the partner rotated their CA, fix it now")
  wait on a merge; concurrent UI edits conflict in Git; our LAN push to
  GitLab hairpins (413/502/EOF), so the bridge's reliability is the cluster's
  Git-write reliability.

### F5 — Mode switch per deployment (Strimzi / Kong)

One deployment-level flag, `config.mode = gitops | catalog`. In `gitops` mode
the catalog write API is read-only (Kong's 405) and the UI offers "export as
MR"; in `catalog` mode Flux-mounted documents are imported once, then
ignored. Switching requires `generation == observedGeneration` and an
explicit adopt step, never delete-on-enable (the Strimzi footgun: enabling CR
mode before the CRs exist deletes every connector).

- **Bet**: regulated tenants want an all-or-nothing guarantee they can point
  an auditor at, and mixed ownership is a complexity nobody asked for.
- **Risk**: no mixed ownership means a tenant who wants GitOps for listeners
  but UI for destinations gets neither cleanly.

### F6 — CRDs as the catalog (etcd as the DB, operator as compiler)

Configuration objects are custom resources. The API and UI create CRs in a
path Flux does not manage; Flux-managed CRs carry an ownership label the API
refuses to mutate, or `kustomize.toolkit.fluxcd.io/ssa: IfNotPresent` for
seed-only objects. Status carries `observedGeneration` and the resolved
artifact digest, Crossplane-style. This is what flexinfer already is, minus
the ownership rule.

- **Bet**: Kubernetes RBAC and the API-server audit log are a sufficient
  compliance story, and the objects are small and PHI-free.
- **Risk**: etcd is not a document store (1 MiB ConfigMap cap, cluster-admin
  visibility, no approval workflow); sample messages and anything
  PHI-adjacent cannot go there. For flexinfer the risk is the one it has
  today: two writers on the same CR with no field manager.

### F7 — The reconciler is the product (intent/observed split first)

Whatever the store, the missing piece in all three repos is **drift
visibility**: no replica reports which digest it actually mounted, and no
surface shows desired vs observed. Build that first: every replica writes
`observed_revision_digest` per adapter with a heartbeat (per replica, never
leader-only, because leader-only hides divergence), and the UI shows
desired / observed / drift. Git and DB both become *intent sources* rendered
into one effective set; authoring can come later.

- **Bet**: the immediate value is knowing what is running, and a config
  plane without observed status is where "deployed" lies (a DB-deployed
  revision whose source digest mismatches the mounted file is refused by
  `ValidateAgainst` today, so it fails closed but the UI would still say
  deployed).
- **Risk**: it ships nothing a user can edit, so it reads as plumbing.

### F8 — Operational verbs are DB, configuration is Git

Strimzi's split: even in declarative mode, operational verbs (restart,
pause, status) stay on the imperative API. fi-fhir already lives here:
deploy / pause / resume / retire take effect on the next MLLP frame or batch
poll from the lifecycle DB; destination credentials are re-read per dispatch;
the MLLP rate quota is a DB lease; flexinfer's activation is an annotation
adopted into status. Formalize the line: the DB owns *state* (enable,
deploy, pause, activation, quotas, retention policy), Git owns *shape*
(listeners, TLS, identities, specs).

- **Bet**: most "I need to change it now" moments are operational, not
  structural.
- **Risk**: the brief's connections are configuration, not state. F8 alone
  does not deliver "define a destination connection in the UI and use it".

## Phase 2 — Cross-Pollinations & Tensions

### Combinations

- **F3 + F7 + F8 — the overlay with eyes and verbs.** F3's ownership model
  needs F7's observed status to be honest (a catalog activation is only
  "live" when every replica reports its digest), and F8's verb/shape split
  gives F3 its tier boundary for free: operational verbs already run from the
  DB and stay there; the catalog tier adds *hot-reloadable configuration*
  (destinations, routing, enable/disable); listeners, TLS and identities
  stay in the Git tier. Neither piece delivers this alone: F3 without F7 is a
  UI that can lie, F7 without F3 edits nothing, F8 without F3 never touches
  a connection spec.
- **F3 + F4 — promotion, not publication.** The MR bridge is weak as *the*
  activation path (latency, credential, hairpin) but strong as the
  **promotion** path from the catalog tier to the baseline tier: "this
  destination has run from the catalog for a month; promote it to GitOps"
  renders the exact bytes into an MR. The catalog stays fast; Git stays the
  long-term record; the bot token is used rarely and never on an incident
  path.
- **F6 + F3 for flexinfer.** flexinfer's store is already etcd; what it lacks
  is F3's ownership rule. An ownership label plus a field manager on MCP
  writes, `IfNotPresent` on Git-seeded objects that the API may later own,
  and refusing MCP mutation of a Git-declared spec field gives flexinfer
  "provisioned vs API-created" without a new database.

### Tensions

- **F2 vs F1 — "who is truth" is the wrong axis.** Both answer globally.
  The precedents that aged best (IRIS, Grafana, Strimzi) answer *per object*
  and *per tier*. The real decision is where the tier boundary sits and how
  it is enforced, which is F3's question.
- **F5 vs F3 — guarantee vs flexibility.** A global mode is what a regulated
  tenant can audit in one sentence; per-object ownership is what a mixed
  deployment needs. They are compatible only if F3's `gitops` tier can be
  made the *whole* set by policy (a deployment flag that forbids
  `owner = catalog` activations), which turns F5 into a degenerate case of
  F3 rather than a competitor.
- **Hot-reload class vs restart class (cuts across all framings).** The
  industry restart-required class is listener bind address and port, TLS
  server material, DB pool credentials, process-wide limits. The
  hot-reloadable class is routing, mapping, enable/disable, outbound
  endpoint parameters, rate limits. `.loom/38` Decision 1 was right to keep
  listeners out; the tension dissolves if the schema marks each spec field's
  class so the UI can say "live" or "applies on next restart" and activation
  of a restart-class change is a rollout, not a hot swap.

## Phase 3 — Convergence

### Recommended: F3 + F7 + F8 for fi-fhir, with F4 as the promotion path

**"Provisioned baseline + catalog overlay, observed per replica, verbs
always DB."** It wins because it is the only option that (a) does not
regress the GitOps guarantee anybody relies on today, (b) gives the IDE real
authoring within days of C-1 landing (the catalog, revisions, compile,
capabilities and roles exist as of MR !241), (c) is honest by construction
(no state without an observed digest), and (d) has the clearest precedent
articulation (IRIS's two tiers, Grafana's provisioned flag, flagd's ordered
merge, Strimzi's verb split). It also resolves fi-fhir's sharpest existing
hazard, two truths for integration definitions (static registry for HTTP and
preview, lifecycle DB for MLLP and batch, no production writer for either),
by making the lifecycle catalog the runtime truth for every adapter with the
static registry imported as the `gitops` baseline.

Concretely, in order (each is a `.loom/` execution spec when chosen):

1. **Observed status** (F7). `integration_runtime_observations` keyed by
   replica: adapter, mounted digest, generation, heartbeat. `engineRuntime`
   and the Connections Usage tab gain "observed on N/N replicas". No
   authoring change. This is also the kill-test's instrument.
2. **Baseline import** (F3, `gitops` tier). At startup `serve` imports the
   static registry, the MLLP and batch source documents and the destination
   registry into the catalogs as `owner = gitops` revisions with the
   deployment's Git revision (from a `FI_FHIR_GITOPS_REVISION` env Flux
   substitutes) as `created_reason`. The UI shows them read-only with
   "clone to draft". Nothing runs differently yet.
3. **Catalog activation for the hot-reload class** (F3 + F8, `catalog`
   tier). A single `integration_active_set` pointer table (generation, set
   digest, approver, reason; expected-version CAS), per-replica polling of
   `max(generation)` every few seconds with `LISTEN/NOTIFY` as a latency hint
   only, and adapters that can swap: destinations in the delivery registry,
   routing and enable/disable, per-integration deployment policy. Four-eyes
   approval (Unleash change-request idiom) when
   `FI_FHIR_CATALOG_APPROVALS_REQUIRED=2`. Secret references resolve only
   against a Git-declared allow-list; the observed row records the resolved
   Secret resourceVersion, never the value.
4. **Restart class stays Git** with a bridge (F4 as promotion). A catalog
   MLLP source can be compiled, exported, and "promoted" into an MR; the UI
   marks listener fields "applies via GitOps".
5. **Tenant guarantee** (F5 as a degenerate case).
   `FI_FHIR_CATALOG_ACTIVATION_ENABLED=false` makes the catalog tier
   read-only and the write API return the inventory-safe "forbidden", so a
   Git-only tenant has one flag to point an auditor at.

**For flexinfer**: F6 + F3's ownership rule. Add a field manager and an
ownership label to every MCP write (`models.go`, `scale.go`, `lora.go`,
`catalogs.go`), refuse MCP mutation of a spec field Git declares, seed
`ModelCatalog` from Git with `ssa: IfNotPresent` since it is the one kind
that exists only imperatively today, and surface drift (Flux inventory vs
live) in FlexDeck. No new database.

**For loom-core**: stay F1 (Git is truth, rendered configs are the compiled
cache, `loom sync mirror --check` is the drift gate) and formalize the two
overlays that already exist: `catalog-state.yaml` as the per-user
`owner = local` tier, and Mills' MR bridge as the F4 promotion path.
Prerequisite: collapse the four registry copies (canonical, gitops mirror,
gateway ConfigMap, mobile-hud ConfigMap) to one rendered artifact, and fix
the watcher's basename match so a ConfigMap `..data` swap reloads loomd. A
DB overlay on top of four unreconciled copies would be a fifth truth.

### Runner-up: F4 as the primary activation path

If compliance review concludes that every configuration change, including
destinations, must pass a Git review before it is live, the MR bridge
becomes the activation path and the catalog tier is authoring-only (F1 with
a good bridge). What tips it: an auditor who will not accept a PostgreSQL
state store under 164.312 even with append-only revisions and four-eyes
approval; or fixing the LAN push hairpin and provisioning a scoped bot
identity, which removes the bridge's two operational weaknesses. What tips
it back: the first incident where a partner endpoint change waits twenty
minutes on a merge.

### Open question

**Does the catalog tier ever activate a listener, or only outbound and
routing configuration?** `.loom/38` Decision 1 said listeners and
credentials stay a GitOps action. The recommendation keeps that (step 4),
but if Cody wants "define an MLLP source in the UI and have it listen" the
restart class must gain a supervised rollout path (activation triggers a
Deployment restart through the operator or a Flux `Kustomization`
reconcile), which is a different safety story and roughly doubles step 3.

## Riskiest assumption + kill-test

**Load-bearing assumption**: fi-fhir's `serve` can atomically swap a
destination in the delivery registry from a catalog revision at runtime,
with the same `destination.NewRevision` bytes and `ValidateAgainst`
fail-closed identity checks the file path enforces today, and every replica
observes the new digest within 5 s, without a process restart. The current
code resolves the destination registry once at startup
(`cmd/fi-fhir/destination_identity_runtime.go`) and the delivery worker
comments that the registry is "one server-owned file read at boot"
(`delivery_runtime.go:60-69`); credentials already re-read per dispatch, but
the registry object itself has never been swapped live.

**Kill test** (≤30 min, throwaway stack): run two `serve` replicas against
one Postgres on the 7900xtx context with the control plane, the delivery
worker (`FI_FHIR_QUEUE_DRIVER=kafka` against the compose Kafka) and the
`adt-http` registry; add a spike that (1) reads a destination revision from
`integration_connection_revisions` by digest and installs it into the live
registry behind an `atomic.Pointer`, (2) bumps a `generation` row, and (3)
polls it every 2 s. Compile an `https` destination in the catalog to a
local httptest sink, bump the generation, submit one message, and assert:
the sink receives the bundle from **both** replicas' workers without restart;
`fi_fhir_schema_ledger_version`-style observed rows show the new digest on
both replicas within 5 s; the negative control, a revision whose digest is
not the one the integration definition's destination ref names, is refused
by `ValidateAgainst` and the observed digest does not change. Pair with the
disconfirming search "Go atomic swap of a struct holding an http.Client and
TLS config while requests are in flight" for the connection-draining
gotchas.

**Failure mode if wrong**: the catalog tier collapses to "authoring only" and
activation becomes a rollout (restart-class for everything), which is the
runner-up, not a failure. The cost of not testing is designing step 3 around
a hot swap the runtime cannot do safely.

**Status**: not run

> The downstream slice plan is BLOCKED until this kill-test passes.

## Handoff

- If chosen → next step is: `plan-loom-core` for a `.loom/40-config-plane-execution-specs.md` with lanes P-0 (observed status), P-1 (baseline import), P-2 (kill-test spike, then catalog activation), P-3 (promotion MR bridge), plus a flexinfer ownership-rule lane and a loom-core registry-consolidation lane in their own repos.
- Linked spec/plan doc: not yet; `.loom/38-connections-execution-specs.md` is the predecessor (Decision 1 and Decision 5 are what this document reopens).

---

## Appendix A — What the three repos do today (survey, 2026-09-27)

Citations are `path:line` against fi-fhir main (worktree of `8b7647c57`) unless
tagged: `@conn` = `feat/connections-0-catalog` (merged as `eef1907ed`),
`FI@om` = flexinfer `origin/master` (local `master` is 138 commits behind),
`LC` = loom-core main, `GO` = platform/gitops, `KIT` = libs/fi-mcp-kit.

### A.1 fi-fhir: composition at startup

- `runServe` (`cmd/fi-fhir/main.go:4325`) → `loadIntegrationRuntimeFromEnv`
  (`cmd/fi-fhir/preview_runtime.go:95-368`), once. Always required:
  `FI_FHIR_DEPLOYMENT_TENANT_ID`, `FI_FHIR_GRAPHQL_ALLOWED_ORIGINS`,
  `FI_FHIR_INTEGRATION_REGISTRY_PATH` (`:99-134`).
- Each adapter is switched on by an env var being set: HTTP ingress by
  `FI_FHIR_HTTP_INGRESS_AUTH_MODE`, MLLP by `FI_FHIR_MLLP_SOURCE_CONFIG_PATH`,
  batch by `FI_FHIR_BATCH_SOURCE_CONFIG_PATH`, delivery by
  `FI_FHIR_DELIVERY_WORKER_ENABLED`, sessions and the operator plane by
  booleans (`:151-174`).
- **Two resolution paths for definitions.** HTTP ingress and preview resolve
  from the static registry (`:225-243`, `:668-692`); MLLP and batch resolve
  from the lifecycle catalog (`:246-299`; `cmd/fi-fhir/batch_runtime.go:65-97`).
  Profile and workflow bytes always come from the static registry (`:139`).
- Delivery requires `FI_FHIR_QUEUE_DRIVER=kafka` even for HTTPS-only
  destinations because the registry is "one server-owned file read at boot"
  (`cmd/fi-fhir/delivery_runtime.go:60-69`).
- Destination identity resolves every secret binding once at startup and
  fails closed (`cmd/fi-fhir/destination_identity_runtime.go:136-154`); the
  env/file resolver lives only in `cmd/` (`:156-244`).
- Anti-downgrade switches are deployment-owned by design:
  `FI_FHIR_MLLP_REQUIRE_CLIENT_IDENTITY` (`preview_runtime.go:462-473`),
  `FI_FHIR_BATCH_REQUIRE_WORKLOAD_IDENTITY` (`batch_runtime.go:105-120`).
- Live GitOps drift found: `GO/k3s/fi-fhir/fi-fhir-api.yaml:140` sets
  `FI_FHIR_FHIR_SERVER_URL`, which nothing reads (code reads
  `FI_FHIR_FHIR_BASE_URL`). Production enables none of MLLP, batch, delivery
  or the operator plane (`fi-fhir-api.yaml:73-143`).

### A.2 fi-fhir: content-addressed documents

| Document | Digest domain | Constructor | Reaches `serve` via |
|---|---|---|---|
| MLLP source revision | `fi-fhir/mllp-source/v1` (`internal/integration/mllp/source.go:32`) | `NewSourceRevision` `:124`, `ValidateAgainst` `:249` | file, `FI_FHIR_MLLP_SOURCE_CONFIG_PATH` |
| Batch source revision | `fi-fhir/batch-source/v1` (`batch/source.go:29`) | `:106`, `ValidateAgainst` `:166` | file |
| Destination revision | `fi-fhir/destination-revision/v1` (`destination/revision.go:48`) | `NewRevision` `:158`, `ValidateAgainst` `:282` | inside the destination registry file |
| Destination registry | schema `fi-fhir/destination-registry/v1` (`destination/registry.go:30`), not digested | `LoadRegistry` `:71` | file, `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` |
| Integration definition revision | `sha256:` over canonical JSON (`pkg/integration/revision.go:545-550`) | `:202` | static registry and `integration_definition_revisions.revision_json` |
| Static registry | `registry/static.go:46-57`, strict, size-bounded | `DecodeStaticRegistry` `:111` | immutable hash-suffixed ConfigMap (`GO/k3s/fi-fhir/kustomization.yaml:22-29`) |
| HTTP source (@conn) | `fi-fhir/http-source/v1` (`connection/http_source.go:43`) | `:130` | not consumed by the runtime |
| Retention policy | per-version digest (`retention/store.go:162-212`) | `DecodePolicyDocument` | file → DB upsert at startup (`retention_runtime.go:33-37, 86`) |

### A.3 fi-fhir: lifecycle and connection catalogs

- Lifecycle (`internal/integration/lifecycle/migrations/0001_deployment_lifecycle.sql`):
  four append-only tables, one expected-version snapshot table with states
  draft → validated → approved → published → deployed → paused → retired
  (`:43-62`), one active deployment per definition (`:64-66`), immutability
  triggers (`:87-105`), advisory-locked ledger (`postgres.go:20-116`).
- **Writers**: `CreateDraft` and `ValidateConnection` only from tests; every
  production catalog has a nil validator (`main.go:4615`,
  `preview_runtime.go:247`, `batch_runtime.go:65`). Operator plane calls
  Deploy/Pause/Resume/Retire (`operator/service.go:403-420`); session
  publication calls Approve/Publish/Deploy (`session/publication.go:265-305`);
  health reporter writes `ReportHealth` (`serve_observability.go:437`).
- **Readers**: MLLP `ResolveRunnable` per message (`mllp/service.go:186`),
  batch per poll (`batch/service.go:149`), admission takes `FOR SHARE`
  (`lifecycle/admission.go:16-61`).
- `docs/operations/INTEGRATION-DEPLOYMENT-LIFECYCLE.md:8-11` still says
  controls are "not yet exposed"; the operator plane made that stale (C-4
  fixes the line).
- Connection catalog (@conn): drafts (expected-version, identity frozen, no
  delete), revisions (append-only, `revision_text` exact bytes + JSONB
  checked equal, `UNIQUE(tenant_id, digest)`), captures; ledger key
  `5064657639792058909`; `types.go:7-11` "the catalog never hot-loads
  anything"; `compile.go:67-110` calls the real constructors;
  `runtime.go:9-16, 48-68` is the secret-free allowlisted description;
  references from lifecycle `ListDigestReferences` (`service.go:17-24`).

### A.4 fi-fhir: restart vs live today

Restart required: every env var; the static registry (immutable ConfigMap →
new ReplicaSet); MLLP/batch source documents; the destination registry; the
retention policy document; the legacy workflow file; TLS and session keys;
the Kafka CA. Only SIGINT/SIGTERM are handled (`main.go:4078`, `:5145`); no
SIGHUP, no fsnotify.

Already live without restart: lifecycle state (deploy/pause/resume/retire on
the next frame or poll); destination credentials and CA bundles re-read per
dispatch (`DESTINATION-IDENTITY.md:184-195, 534-539`); the MLLP rate quota
DB lease (`preview_runtime.go:268-287`); health; the 2 s armed-capture poll
planned by C-2.

**Sharpest hazard**: two sources of truth for integration definitions. A
DB-deployed revision whose source digest differs from the mounted file is
refused by `ValidateAgainst` (fails closed, not honoured), so a UI could show
"deployed" for something no replica can run.

fi-fhir primitives to reuse: lifecycle append-only facts + expected-version
snapshot + release digest + one-active index; connection drafts / revisions /
captures / "mounted here"; per-package ledgers with `SchemaVersion` and the
`migrationcompat` proof; the retention file→versioned-DB-record upsert; the
`engineRuntime` allowlist; the capabilities contract
(`internal/api/graphql/capabilities.go:9-179`); `SecretReference` bindings
resolved only in `cmd/`; operator roles.

### A.5 flexinfer

- Everything is a CRD; there is no config database. Kinds in
  `FI/api/v1alpha2/`: `Model`, `ModelCatalog`, `LoRAAdapter`, `GPUProfile`,
  `ModelExperiment`, `ModelBackfill`, `GamingSession`, and others.
- Flux Kustomization `flexinfer-models` (`GO/k3s/flux/apps/flexinfer-models.yaml:1-18`):
  5 m interval, `prune: true`, path `./deploy`. In Git: 77 `Model`, 23
  `ModelCache`, 2 `LoRAAdapter`, 3 `GPUProfile`, 1 `GPUGroup`, **0
  `ModelCatalog`**. `GamingSession` is explicitly not GitOps-authored
  (`deploy/kustomization.yaml:10-13`).
- MCP write tools (loom-core `cmd/mcp-flexinfer`): create/update/scale/
  delete `Model` (`models.go:131-293`, `scale.go:14-45`), activate via the
  annotation `flexinfer.ai/activate-requested-at` (`scale.go:48-107`),
  LoRA create/delete (`lora.go:93, 142`), `ModelCatalog` create
  (`catalogs.go:83-130`). All are JSON merge patches with **no field
  manager** (`main.go:598-618`) and no ownership marker; the only guardrails
  are per-call approval (absent from `always_allow`,
  `LC/mcp/context/registry.yaml:1509-1532`) and written policy
  (`FI@om:docs/dev/daily-driver-promotion-checklist.md:69-71`). The
  `gitops_flux` guardrail blocks exec-type `kubectl` only
  (`registry.yaml:2178-2213`), so MCP tools bypass it.
- The one write path designed to coexist with GitOps is activation:
  adopted only if strictly newer than `status.activationRequestedAt`
  (`FI@om:controllers/model_activation.go:66-91`, pinned by
  `model_activation_test.go:187`).
- Live check 2026-09-27 (read-only): 27 `Model`s, all in Git; 2
  `LoRAAdapter`s, both in Git; 0 `ModelCatalog`s. No drift today, but a
  Git-declared spec field patched by MCP is reverted within 5 m, an omitted
  field persists silently, and MCP-created objects carry no Flux inventory
  entry.
- Secondary finding: `FI@om:deploy/system/values-k3s.yaml:236` commits a
  Postgres DSN with an inline password.

Must stay GitOps: images and digests, `GPUProfile` images and the gaming
`approvedImages` gate (`values-k3s.yaml:622-630`), resources, node selectors,
Helm values, registry `secretRef`s, RBAC. Plausible UI edits: serverless
floor and activation, create/retire a Model from a catalog entry, LoRA
attach/detach, `ModelCatalog` registries, LiteLLM aliases.

### A.6 loom-core

- `mcp/context/registry.yaml` (2484 lines): header with substitution syntax
  (`${secret:KEY}` resolved env → keychain → 1Password → file, `:33-38`),
  `servers[]` (57), `sandbox_policy` (`:2145-2166`), `platform_permissions`
  with the `gitops_flux` guardrail (`:2170+`).
- `loom sync` (`cmd/loom/cmd_sync.go:6-40` → `pkg/sync/ops_sync.go:17, 296`
  → `ops_regen.go:17-60`): reads the registry, generates into a temp dir,
  copies to the profile dir or home; `--dry-run` reports drift.
  `loom sync mirror --check` compares canonical vs `platform/gitops/mcp/context`
  against `origin/main` blobs (`cmd_sync_mirror.go:13-70`; 39 diff lines at
  survey time). `LC/AGENTS.md:126` names gitops as the registry location
  while `mirror.go:4-5` names loom-core canonical.
- loomd hot-reloads via fsnotify on the registry's parent directory
  (`internal/daemon/daemon_lifecycle.go:223-242`) and SIGHUP
  (`daemon_reload.go:42-61`), publishing an epoch
  (`daemon_dispatch_ops.go:95-175`). **Watcher gap**: events matched by
  basename (`pkg/sync/watcher.go:196-203`), so a ConfigMap `..data` symlink
  swap is missed; Mills documents the same bug
  (`GO/k3s/mills/deployment.yaml:28-34`).
- A local overlay already exists: `~/.config/loom/catalog-state.yaml`
  (`pkg/registry/catalog_state.go:20-27`; `loom catalog enable|disable`).
- Mills is the one existing file-vs-DB split: policy ConfigMap loaded with
  fsnotify into a checksum snapshot (`pkg/mills/policy/manager.go:60-110`);
  squads reflected file → DB with a content SHA and last-good fallback
  (`pkg/mills/squads/loader.go`); `policy_proposals` lifecycle pending →
  applied_human | applied_auto | rejected | reverted with `revert_deadline`
  (`store/migrations/002_v2.sql:104-119`); the kill switch edits the policy
  ConfigMap by **opening an MR against platform/gitops**, never the live
  object (`cmd/loom-mills-operator/handlers_policy.go:109-117`).
- **Four copies of the registry run**: canonical, the gitops mirror, the
  gateway ConfigMap `loom-gateway-registry` (separate hub schema,
  `k8s/base/servers/gateway/registry-configmap.yaml:1-25`, no hash) and
  `mobile-hud-registry`. Only the first pair has a drift gate; the gateway
  reads `--registry` once at startup (`KIT/cmd/fi-mcp-gateway/main.go:21`).

### A.7 Cross-repo observations

Each repo already has something between Git and live state: fi-fhir's
retention file → versioned DB record and DB-driven lifecycle state;
flexinfer's annotation → status adoption; loom-core's catalog-state overlay
and Mills' UI → MR bridge. Two of three explicitly refuse live writes to
Git-owned objects (fi-fhir `.loom/38`; Mills). flexinfer's MCP tools are the
exception. None has drift detection between a DB/etcd writer and Git except
loom-core's Git-to-Git mirror gate.

## Appendix B — External precedents (condensed; primary docs unless marked [S])

| Product | Ownership answer | What to copy | Source |
|---|---|---|---|
| Mirth Connect / NextGen Connect | DB is truth; Git via REST export (mirthSync) [S] | Secret *references* in exports; the single Configuration Map "replaces everything" is the anti-pattern | github.com/nextgenhealthcare/connect/discussions/4689; github.com/SagaHealthcareIT/mirthsync |
| InterSystems IRIS / Ensemble | Two tiers: production definition (source control, "same in all environments") vs System Default Settings (per environment: "file paths, port numbers"); system overrides lock a setting | The tier boundary wording; rollback package before deploy; compile error rolls back the whole deployment; `Credentials` IDs; SQL projection returns `'xxx'` for passwords | docs.intersystems.com … `KEY=ECONFIG_other_default_settings`, `KEY=EGMG_deploy` |
| Apache NiFi + Registry | UI authors; Git is a write-only journal ("NOT supported to modify stored files outside of NiFi Registry") | Versioned process group with local-changes flag and "Change version"; closest match to drafts → revisions → deployed pointer | nifi.apache.org/docs/nifi-registry-docs/html/administration-guide.html |
| Kafka Connect + Strimzi | `strimzi.io/use-connector-resources` makes CRs truth; REST changes "are reverted"; enabling before CRs exist deletes all connectors; REST stays fine for restart/status | Verb/config split; mode switch must adopt, never delete | strimzi.io/docs/operators/latest/full/deploying; github.com/orgs/strimzi/discussions/7042 [S] |
| Camel K | CRDs, operator compiles | CRD-as-catalog idiom | camel.apache.org/camel-k |
| Airbyte | UI-first, Terraform provider over public API [S] | Drift between UI and IaC is the known cost | github.com/airbytehq/terraform-provider-airbyte |
| Grafana | `allowUiUpdates: false` → "Cannot save provisioned dashboard"; `true` → file overwrites DB on change; Operator overwrites on resync (duplicate-then-edit); Git Sync (12+) is bidirectional with UI → PR | Per-object provisioned flag; "clone to draft"; UI → PR as an option | grafana.com/docs/grafana/latest/administration/provisioning/; …/as-code/observability-as-code/git-sync/ |
| Argo CD | `selfHeal` reverts live edits; `ignoreDifferences.managedFieldsManagers`; Image Updater `argocd` write-back is "pseudo-persistent" | SSA field-manager ownership; git write-back for durable edits | argo-cd.readthedocs.io (auto_sync, diffing); argocd-image-updater.readthedocs.io |
| Keycloak | `--import-realm` skips existing realms (seed-only, drifts forever) | The weakest reconciliation mode; avoid | keycloak.org/server/importExport |
| Backstage | Catalog is a read model; deleted provider-owned entities "reappear"; scaffolder turns a form into an MR | UI intent → MR bridge | backstage.io/docs/features/software-catalog/life-of-an-entity; …/software-templates/builtin-actions |
| Kong / Kuma | DB-less: write endpoints return 405; Kuma k8s mode API is read-only | **Refuse at write time, don't revert later** | developer.konghq.com/gateway/db-less-mode/; kuma.io/docs |
| Sourcegraph | `SITE_CONFIG_FILE`; `ALLOW_EDITS` edits "overwritten … on restart" | Silent-overwrite caveat as documented | sourcegraph.com/docs/admin/config/advanced_config_file |
| Temporal dynamic config | File of hot keys, polled | Hot-reload class made explicit | docs.temporal.io/references/dynamic-configuration |
| Unleash | Change requests: draft → review → approved → applied, four-eyes, event log with `preData`/`data` | Compliance-grade approval and audit idiom | docs.getunleash.io/reference/change-requests, /events |
| OpenFeature flagd | Multiple sync sources merged in declared order, last wins | Ordered merge for baseline-under-overlay | flagd.dev/concepts/syncs/ |
| Crossplane | `forProvider` / `atProvider` / `managementPolicies` / observe-only | Intent/observed split | docs.crossplane.io/latest/managed-resources/managed-resources/ |
| Flux | Drift corrected every interval; escape hatches `ssa: Merge|IfNotPresent|Ignore`, `reconcile: disabled`, `prune: disabled`; `OCIRepository` pinned by digest with cosign/Notation verify; `flux push artifact` | Per-object exemptions; signed OCI export of an active set | fluxcd.io/flux/components/kustomize/kustomizations/; …/source/ocirepositories/ |
| OpenGitOps | "any other system that meets these criteria may be used" as state store | A DB of immutable content-addressed revisions qualifies | opengitops.dev; github.com/open-gitops/documents GLOSSARY.md |
| Kubernetes | ConfigMap 1 MiB cap; subPath mounts never update; env needs restart; SSA shared ownership | Limits of etcd as a catalog; fsnotify must watch the directory | kubernetes.io/docs/concepts/configuration/configmap/; …/server-side-apply/ |
| PostgreSQL | NOTIFY delivered on commit to *listening* sessions only; 8000-byte payload; queue can fill | NOTIFY is a latency hint over generation polling, never truth | postgresql.org/docs/current/sql-notify.html |
| client-go leader election | "does not guarantee that only one client is acting as a leader" | Leadership for background work only; state change = conditional DB write | pkg.go.dev/k8s.io/client-go/tools/leaderelection |
| HIPAA 45 CFR 164.312 | (b) audit controls, (c)(1) integrity | Append-only rows + DB-level immutability satisfy both | law.cornell.edu/cfr/text/45/164.312 |

Secret-reference pitfalls to design against: rotation breaks reproducibility
of a digest (record the resolved Secret `resourceVersion`, never the value,
in observed status); a by-name reference lets a UI user exfiltrate any Secret
unless references are allow-listed in Git and the engine's Role uses
`resourceNames`; validate resolution at approval time, not first use; never
echo values in previews, exports or audit diffs.

Anti-patterns for this stack, from the precedents: two writers for one object
with no ownership rule; a UI that writes Kubernetes objects Flux also applies;
secret values in DB rows or UI-to-Git commits; NOTIFY-only propagation;
leader-only apply without fencing; fsnotify on a single file or subPath
mount; Reloader's env-var strategy under GitOps; Keycloak-style
import-if-absent; a declarative mode that deletes on enable; committing
UI-authored sample messages (PHI) to Git.

Weakly sourced: Rhapsody (gated docs), NiFi 3.0 dropping Registry (secondary
only), Mirth Channel History tiering (search-surfaced), Consul-Terraform-Sync
internals, Temporal's exact poll interval.
