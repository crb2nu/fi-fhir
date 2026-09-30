### 2026-09-30: IDE unlock: definitions on connections, refs from the runtime resolver, verification over durable admissions

- Decision:
  - **Definitions live on `/connections`, deploy stays on `/operator`**
    (`.loom/42` Decision 1). A definition binds connections, so it is a
    fourth tab (Sources · Destinations · Definitions · Engine); deployment is
    an operator act and stays on Operator › Deployments. No new route, so the
    eight route registration points stay untouched; the two pages link each
    other by deep link (`/connections?definition=&revision=`,
    `/operator?definition=&revision=`).
  - **Refs come from what the runtime can resolve** (Decision 2). The editor
    offers profile and workflow refs only from the static-registry
    integrations, proven by the same `processor.RevisionResolver` `serve`
    uses (`integrationRegistryArtifacts`), and says so in a sentence, because
    that is what admission resolves today. A profile and a workflow must come
    from the same registry entry. Moving resolution onto the lifecycle
    catalog stays with the configuration-plane spec (`.loom/39`, reserved
    `.loom/41`).
  - **Validation is per source kind and always recorded** (Decision 3).
    `REAL` is the batch validator the seed CLI runs, now hosted in `serve`
    for the batch source the replica mounts only; `STATIC` (MLLP/HTTP, code
    `VALIDATION_STATIC`) checks that the source digest is mounted on this
    replica or has a fresh observations-ledger row and that the source's
    binding names are bound (values are not resolved: `BINDINGS_NOT_CHECKED`);
    `SKIP` needs a reason of at least 16 bytes (`VALIDATION_SKIPPED`). A
    catalog with no mode on the context fails closed
    (`CONNECTION_CHECK_ERROR`). The page never claims a check it did not run.
  - **Verification means durable admissions** (Decision 4). `/events` reads
    `integration_canonical_events` joined to receipts and lineage through
    `integration.operator` (`operatorCanonicalEvents`,
    `operatorAdmissionStatistics`), never payload values (field paths and
    kinds only). The legacy clinical views left the IDE; the legacy
    `events`/`patientTimeline`/`eventStatistics`/`eventStream` fields stay in
    the schema; there is no patient timeline and the page says why once.
  - **Sessions are resumable and exportable through the audited path only**
    (Decision 5). Export always carries a reason recorded on the append-only
    export row; raw sample payloads only with `integration.phi.export`,
    surfaced as `capabilities.phiExport`.
  - **Journey stages are evidence, not position** (Decision 6). A stage is
    complete when its evidence exists; "unknown" when its read is not
    configured or forbidden, and then no query is sent.
  - **Evidence over pixel tests** (Decision 7), unchanged from `.loom/37`:
    screenshots are review artifacts; the gate asserts `data-testid`s and copy.
  - **Kill-test (E-1) PASSED 2026-09-29.** (a) Byte parity:
    `TestDefinitionAuthoringParity_SeedAndEditorAuthorTheSameBytes` builds the
    seed fixture's definition through `buildSeedDefinition` and through the
    editor path (`authoring.SourceFromDocument`/`DestinationFromDocument` over
    compiled connection documents, `authoring.Registry.Artifact`, bindings
    listed out of order, `authoring.BuildDefinition`): equal `Digest`, equal
    canonical JSON; negative control (destination class `sandbox`) moves both
    digests to the same new digest; table-driven over SFTP and S3. (b)
    In-process validation:
    `TestDefinitionAuthoringPostgres_ServeCatalogHostsSkipAndStaticValidation`
    on PostgreSQL 16 builds the catalog exactly as `serve` does plus the
    validator: SKIP → `[VALIDATION_SKIPPED]` and `validated`; STATIC mounted →
    `[VALIDATION_STATIC, SOURCE_MOUNTED]`; STATIC unmounted → failed record,
    stays draft. Disconfirming search: `TestBatchValidator_DeadlineLeaksNoGoroutineOrProvider`
    (a probe that outlives its deadline exits and closes its provider; the
    SFTP dial is bounded at 10 s TCP + 10 s SSH); REAL opens no connection the
    pod does not already open, one probe per replica at a time, API-started
    REAL capped at 20 s. Full authoring shipped; the read-only fallback was
    not needed.
- Rationale:
  - The backend already kept the data (145 GraphQL root fields, 44 never
    reached by the IDE; a full lifecycle state machine the IDE could drive
    only from `published`); the gap was surfaces, not storage. Every new
    surface reads durable data through existing roles and every new write
    rides `integration.deployment.operator`, so no role or grant changed.
  - The seed CLI and the API now call one package
    (`internal/integration/lifecycle/authoring`), so they cannot drift; the
    kill-test pins that before any UI shipped on it.
  - `/events` could never fill on `serve`: its only writer is gated off and
    its projections are fed by nothing. Reading the durable tables is the
    only honest verification view.
- Alternatives considered:
  - **A new `/definitions` route** — rejected: eight registration points for
    a page whose objects are bindings of connections.
  - **Refs from the lifecycle catalog** — deferred: admission does not
    resolve them from there yet; offering them would author definitions the
    runtime cannot ingest.
  - **Read-only Definitions with the CLI command printed** — the kill-test's
    failure branch; not needed.
  - **Feed the legacy event projections from admission** — rejected: it
    would read payload values back into the IDE, which the operator plane
    never does by design.
- Consequences (review lessons the coordinator applied across lanes):
  - **Paged lists with auto-refresh snapshot their filters.** E-0's first
    push and E-2's Admissions both re-read half-typed filter inputs on Next,
    Refresh, auto-refresh and post-action reloads. The rule: only Apply,
    Clear and a deep link write the applied filter set; every other read
    uses the snapshot (vitest "pages and refreshes with the applied
    filters").
  - **Export fragments are PHI-minimal by selection, not by redaction.**
    E-3's export selects `SessionExportRunFields` (status, stages,
    diagnostics, lineage, event envelopes) with no parsed patient, encounter,
    test or appointment fields, and the e2e check reads the downloaded JSON
    for forbidden keys.
  - **A capability is a deployment fact; roles go in `missingRoles`.** E-1's
    first `definitionAuthoring` mixed "the service is composed" with "the
    caller may write"; it is now the deployment fact alone, with the write
    roles in `missingRoles.definitionAuthoring`, and the UI precedence is
    not configured → missing role → read only (same as `controlPlane` and
    `phiExport`).
  - **Stacked lanes fold, never rebase, and fold order is merge order.** E-2
    stacked on E-0 and folded `origin/main` with `git merge --no-ff` after
    each merge; E-1 folded four times (E-4, E-5, E-0+E-3, E-2), each time
    re-running gqlgen/codegen, the role-map count test (final 155 total / 50
    fine-grained) and `ci/job-inventory.txt`. Shared one-line-per-lane files
    (`check-report.mjs`, `playwright.config.ts` `testMatch`,
    `capabilities.go`) resolved by keeping every lane's line.
  - **The `security:npm-audit-ui` drift trap.** Any MR touching
    `.gitlab-ci.yml` runs the UI audit against a daily-moving advisory DB;
    main pipeline 30342 (E-2 merge) and E-1's pipeline 30344 failed on nine
    new advisories with no dependency change. The fix is a lockfile-only
    `npm audit fix --package-lock-only` riding in the next MR (E-1's
    `8249f62ea`), and a push cancels an armed merge-when-pipeline-succeeds,
    so re-arm after it.
- Sources:
  - [S1] `.loom/42-ide-unlock-execution-specs.md`, Decisions 1–7 and the kill-test
  - [S2] MR !268 (E-1) description: kill-test, review fixes B1 and S1–S9
  - [S3] MRs !265, !266, !267, !264, !269 descriptions and review rounds
  - [S4] Coordinator ledger, session 391c0868 (2026-09-29/30)
