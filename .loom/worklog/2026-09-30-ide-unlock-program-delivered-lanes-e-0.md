### 2026-09-30 - IDE unlock program delivered (lanes E-0..E-6)

- What changed (merge order on `main`, first-parent):
  - **E-4 shell honesty** — MR !265, merged 2026-09-29 20:20Z as `94c0e2f63`
    (head `1ef550c27`, MR pipeline 30217 green, no retries): journey from
    evidence (`journeyState.ts`), one command registry and palette on the new
    `Dialog` primitive, `TabItem.badge`/`dirty`, `markDirty`/`clearDirty`,
    split pane and Cmd+\ removed, legacy `$lib/ui/*` components and the
    orphaned collaboration/observability modules deleted; e2e E4-1..E4-3.
  - **E-5 workflow authoring** — MR !266, merged 21:00Z as `cb5a1e996` (head
    `0901e379f`, pipeline 30237 green): inline validation (builder toasts
    33 → 16), faithful YAML round trip with a YAML-only fields panel,
    rename/archive/restore of workflow definitions, run traces in the Trace
    panel, `WarningList` on primitives with `onAcceptFix`, `PID-3[0].1`;
    e2e E5-1..E5-3.
  - **E-0 operator depth** — MR !267, merged 2026-09-30 00:28Z as
    `fb4a68a0c` (head `7a256bfe5`, pipeline 30251 green): deployment history,
    `validationCurrent`, attempt search and inspector with paged audit, the
    full trace (resubmit chain, lease, ledger row, tombstones), received-at
    window, auto-refresh, control-plane pre-flight, fleet observations, deep
    links; the operator-bundle e2e fixture (`ui/e2e/fixture.sh`); e2e
    E0-1..E0-8.
  - **E-3 sessions** — MR !264, merged 00:29Z as `aeb009781` (head
    `19cde3e57`, pipeline 30252 green): `/hl7?session=` reopen, session rail
    (runs, diagnostics, per-run stream, publications), Accept fix from the
    rail and from Warnings rows, archive, PHI-minimal export with the
    `phiExport` capability; HL7 registers into E-4's registry and the
    CommandPalette shim is deleted; e2e E3-1..E3-4.
  - **E-2 verification** — MR !269, merged 02:28Z as `af212930a` (head
    `bcd955c9c`, pipeline 30319 green): `operatorCanonicalEvents`,
    `operatorAdmissionStatistics`, processor migration
    `0006_verification_reads.sql` (index-only, `SchemaVersion` 6),
    `test:verification-reads`; `/events` is Verification (Admissions,
    Statistics, Retention); legacy Events views deleted; e2e E2-1..E2-5.
  - **E-1 definition editor** — MR !268, merged 04:55Z as `d996cd610` (head
    `8249f62ea`, pipeline 30356 green): `internal/integration/lifecycle/authoring`
    shared by the seed CLI and the API, seven GraphQL operations, the
    mode-dispatching validator in `serve`,
    `FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE`, `capabilities.definitionAuthoring`,
    the Definitions tab on `/connections`, `test:definition-authoring`; e2e
    E1-1, E1-2.
  - **E-6 close-out** — this entry, the decision entry, ROADMAP (Now/Then
    rewritten, Delivered block), `.loom/42` status and fixture numbers,
    `.loom/38`'s stale "called only from tests" line, `docs/STATUS.md` rows
    for the authoring and operator packages, and the two ops docs that still
    named the removed Events › Live Stream.
- Why: `.loom/42` — Cody's brief to keep improving the UI/UX surfaces "to
  unlock the power of our backend": production had a real definition, receipts
  and deliveries (the demo hospital flow), and the IDE showed a fraction of it.
- Evidence:
  - Every lane's final MR pipeline green before merge (above). Retries and
    breaks along the way: main pipeline 30316 (E-0+E-3) retried
    `build:docker-ui` once (Alpine DNS); main 30342 (E-2 merge) failed only on
    `security:npm-audit-ui` advisory drift, fixed by E-1's lockfile-only
    commit `8249f62ea`; E-1's pipeline 30344 failed the same job for the same
    reason before that commit.
  - E-1 kill-test PASSED 2026-09-29 (parity + in-process validation on
    PostgreSQL 16 + goroutine/provider leak probe); see the decision entry.
  - Local browser gate before each lane's last push: E-0 59/59, E-2 62/62,
    E-1 42/42 (operator-bundle + preview-only), E-3 19/19 operator-bundle,
    E-5 49/49, E-4 green; each plus the existence guard.
  - `internal/workflow` `TestExecAction_RunsAllowedCommand` failed once under
    full-suite load in four lanes' local runs and passed on rerun (package
    untouched by the program).
  - Close-out: `bash scripts/validate-docs.sh`, `scripts/worklog.sh check`,
    `scripts/decisions.sh check`, `scripts/ci-job-inventory.sh --check`, and
    `scripts/docs-status.sh --check-drift` against the `coverage.out` artifact
    of E-1's `test:unit` job 333856 (all components within 5.0%).
  - Main pipelines 30372 (push) and 30373 (full, by API) on `d996cd610` were
    still running when this entry was written; production and demo
    verification after them is the coordinator's.
- What's next: ROADMAP Now and Then — the open handoffs from the lanes
  (connection rows' replica counts, `?connection=` deep links, connection
  edit-buffer dirty state, unresolvable registry entries, definition list
  paging, Inventory's Run event, E-4 accessibility nits, a mounted catalog
  revision for E1-1's STATIC check, the journey's unbounded session read,
  the legacy event fields) and the flexinfer-site docs sync for the changed
  user-guide pages.
- Findings:
  - The spec's fixture had two admissions; E-0's has four accepted receipts
    written by the real admission path (two dead-lettered by the real delivery
    worker against a broker at `127.0.0.1:9`, two queued) plus one deployed
    batch definition. E0-4 resubmits one dead letter, leaving three queued and
    one open dead letter.
  - E1-1's STATIC is recorded honestly as `SOURCE_NOT_MOUNTED`: the bundle
    replica mounts no compiled catalog revision (checks 8 and 9 pin "no
    adapter"), and the fixture's ingress side processes bind the registry's
    `adt-east` with a non-catalog digest.
  - `integrationSession(id)` now answers `null` for an id the tenant does not
    hold (was "GraphQL request failed"), so a deep link can say "not found".
  - `archiveIntegrationSession` records no reason; the dialog says so rather
    than collecting text the API drops.
- Sources:
  - [S1] `.loom/42-ide-unlock-execution-specs.md`
  - [S2] MRs !264–!269 and the lanes' status files (session 391c0868 scratchpad)
