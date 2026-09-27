### 2026-09-27: Connections: durable catalog, GitOps activation, tap-after-admission capture

- Decision:
  - **A durable catalog of drafts and immutable revisions, not hot-loading**
    (`.loom/38` Decision 1). `serve` keeps mounting content-addressed documents
    at startup and the lifecycle catalog keeps governing what runs; the
    connection catalog makes those documents authorable, compilable and
    exportable, and labels each connection against the digest the answering
    replica mounted (`engineRuntime`). Nothing in the catalog is live-reloaded.
    `.loom/39` reopens this deliberately; until its kill-test passes it stands.
  - **Compile with the existing constructors** (Decision 2):
    `mllp.NewSourceRevision`, `batch.NewSourceRevision`,
    `destination.NewRevision`, and a new `NewHTTPSourceRevision` (digest domain
    `fi-fhir/http-source/v1`) so an HTTP source has bytes behind its digest. A
    compiled revision is byte-for-byte what an operator would write by hand.
  - **Secrets stay references end to end** (Decision 3). The catalog stores
    binding names and `{provider, key, version?}` references; no GraphQL field
    can carry a value.
  - **No new role** (Decision 4). Reads ride `integration.operator`, writes
    add `integration.deployment.operator`, intake rides `integration.operator`;
    the production bundle already holds all of them. `controlPlane` and
    `connectionCatalog` are deployment capabilities so the IDE says "not
    configured" instead of "forbidden".
  - **The integration definition editor is a follow-up** (Decision 5), recorded
    in ROADMAP "Then".
  - **Capture is a tap after admission, redact-only, with superset redaction**
    (Decision 6); every peek and capture is a reason-required audited row.
  - **Evidence over pixel tests** (Decision 7): captures are review artifacts;
    the gate asserts honest `data-testid`s and the copy register.
  - Lane decisions a future reader needs (C-0 D1–D12, C-2 1–10):
    - **Seventh ledger** `integration_connection_schema_migrations`, advisory
      lock key `5064657639792058909`, `SchemaVersion` 1 at C-0 and 2 after
      C-2's `0002_connection_capture_intake.sql`.
    - **Revisions store `revision_text` and `revision_json`**, `CHECK`ed equal:
      the text is returned verbatim as `revisionJson` because JSONB reorders
      keys and the digest is over exact bytes.
    - **Drafts are permissive; the write gate refuses only secret material**
      (and, after review, unknown keys and malformed or dangling binding
      references). Incomplete specs save; their problems appear at validate and
      compile. Refusals keep one message, "connection spec carries secret
      material", with `extensions.problems` naming code and path.
    - **Compile is idempotent**: an unchanged draft version returns its
      existing revision and writes nothing; compile never raises the draft
      version; racing compiles claim one revision, and a compile racing an
      archive waits (`FOR SHARE`) and reports archived.
    - **Tenant scoping**: the service is bound to the deployment tenant and
      reads identity only from the request context; another tenant's rows are
      not found. `ConnectionReference.digest` is the digest of *this*
      connection's revision the definition names.
    - **`controlPlane` and `connectionCatalog` are set from what `runServe`
      wired** (the durable-database block), not from one env flag; intake is on
      iff the session workspace and the catalog are.
    - **Slot protocol**: a frame of an armed source locks the capture row with
      `SELECT … FOR UPDATE SKIP LOCKED`, writes the sample, and advances the row
      by an expected-version update in the same transaction, completing it on
      the last slot. A locked row is skipped, not waited on, so admission never
      queues behind the tap; a failed write fails the row and counts nothing.
      (A claim-first draft was replaced because a failed write would have left
      `captured` overstated.) One armed stream capture per source is a unique
      partial index, not a check-then-insert.
    - **Deterministic slot sample id** `sample_capture_<capture id>_<slot>`.
      A variant with a `UnixNano` suffix was tried on 2026-09-27 and rejected:
      with it `make connection-capture` failed two proofs (a rewritten slot
      became a second sample, so a session held more samples than `captured`),
      and the suspected fixture collision did not exist (capture ids are UUIDs;
      `AddSample` is already idempotent for a caller-named id).
    - **Superset redaction with whole-date masking**: captured and peeked
      samples go through `session.RedactCapturedHL7v2` (113 fields across PID,
      NK1, IN1, IN2, GT1, MRG, PV1); dates are masked whole, year included,
      because the pasted-sample redactor already masks PID-7 whole and the
      superset must hold.
    - **Peek secret allow-list**: a peek resolves bindings only as `env`
      `FI_FHIR_CONNECTION_SECRET_*` or `file` under `connections/` in
      `FI_FHIR_DELIVERY_IDENTITY_SECRET_DIR`; everything else is
      `SECRET_UNRESOLVABLE` before anything is contacted. No new env var.
    - **Tap timing**: synchronous after admission, under its own 2 s context
      detached from request cancellation, `recover()`-guarded; only an armed
      source's ACK can be up to 2 s slower.
    - **`SessionSample.redactedPayload`** (additive) returns captured text only
      to `integration.operator`; `rawPayload` stays retain-policy only.
    - **Found, not fixed**: the v1 kernel admits only MSH/EVN/PID/PV1 (+NTE), so
      a stream capture of a feed carrying NK1/IN1 captures nothing (ROADMAP
      Now). Peek is unaffected.
- Rationale:
  - Runtime activation was already GitOps on every roadmap slice, and a
    hot-loaded listener or credential is a data-plane change with its own
    safety story. The honest gap was authoring and labelling: no document had
    a durable home, and nothing said which one a replica ran.
  - Constructors as the compiler make "compiled here" and "mounted there" the
    same digest by construction, and the lifecycle catalog's `ValidateAgainst`
    checks keep working unchanged.
  - A live feed carries PHI an engineer's synthetic sample does not, so capture
    stores a copy masked more aggressively than pasted samples and never
    touches the ACK, the receipt, or the per-frame database path.
  - The peek resolves bindings any `integration.deployment.operator` can write
    and hands them to an endpoint the same draft names; without an allow-list a
    draft could exfiltrate any process variable or destination credential.
- Alternatives considered:
  - **Hot-load connections from the catalog** — out of scope (Decision 1);
    `.loom/39` weighs it properly, with a kill-test first.
  - **Re-derive revision bytes from JSONB** — rejected: key order changes the
    digest.
  - **Strict drafts that refuse any invalid spec** — rejected: an operator
    could not save a half-written connection; problems at validate/compile
    carry the same information.
  - **A separate authoring role** — deferred; one line per field in
    `rootFieldRoles` when wanted.
  - **Replay a received message into a session** — impossible by
    construction: production raw bytes are ephemeral, so a message can reach
    the IDE only at admission (the tap) or from an unconsumed batch object.
- Consequences:
  - Referenced and Deployed read from the lifecycle catalog, which nothing in
    production writes yet; they stay absent until the definition editor ships.
  - The Engine tab can show only keys `serve --help` documents (ROADMAP Now).
  - `docs/user-guide/connections.md` and `docs/operations/CONNECTION-CATALOG.md`
    are the reader-facing record.
- Sources:
  - [S1] `.loom/38-connections-execution-specs.md`, Decisions 1–7
  - [S2] MR !241 (C-0) description, deviations D1–D12 and review rounds R1–R4
  - [S3] MR !242 (C-2) description, decisions 1–10 and the slot-id adjudication
  - [S4] `.loom/39-brainstorm-config-plane-2026-09-27.md`
