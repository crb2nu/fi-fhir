### 2026-09-27 - Connections program delivered (C-0..C-4)

- What changed:
  - **C-0** (MR !241, merge `eef1907ed`): `internal/integration/connection`
    — drafts, append-only revisions compiled by the existing constructors plus
    a new HTTP source document, the capture audit table, the seventh ledger
    (`integration_connection_schema_migrations`, key `5064657639792058909`);
    GraphQL catalog fields and `engineRuntime`; four new auth capabilities;
    `test:connection-catalog` (nine proofs); migrationcompat at seven ledgers.
  - **C-1** (MR !243, merge `907281c70`): `/connections` with Sources,
    Destinations and Engine tabs, generated forms, honest status, bindings
    without values, exact-bytes Download and Copy JSON; visual captures V12–V14.
  - **C-2** (MR !242, merge `78f1e385f`): batch peek and the admission-time
    capture tap, `0002_connection_capture_intake.sql` (`SchemaVersion` 2), the
    capture redactor and its 113-field table in `PHI-RETENTION.md`, the peek
    secret allow-list, two Prometheus counters, `test:connection-capture`
    (seven proofs).
  - **C-3** (MR !245): **From connection…** in HL7 intake.
  - **C-4** (MR !C4_IID): `docs/user-guide/connections.md`,
    `docs/operations/CONNECTION-CATALOG.md`, the lifecycle doc's exposure line,
    "seven ledgers" in `AGENTS.md`, `testing.md`, `PRODUCTION-HARDENING.md`
    and `SUPPORTED-1.0.md`, ROADMAP Delivered/Now/Then, the decision entry,
    `ui/docs/DESIGN.md` capture table, docs indexes.
- Why: `.loom/38` — Cody's brief to view and configure engine properties,
  define source and destination connections, and sample messages from a source
  to build profiles against.
- Evidence:
  - Every lane's MR pipeline green before merge (C-1 29626; C-2 29622 after one
    BuildKit and one e2e timing retry; C-0's final head verified by the
    coordinator).
  - `make connection-catalog` 9/9, `make connection-capture` 7/7, `make
    migration-compatibility` 4/4 at seven ledgers, each lane's negative control
    failing as designed (C-2: "a capture armed for adt-west captured 2 samples
    of adt-east frames" with `armedFor` ignoring the source id).
  - C-4: `scripts/validate-docs.sh`, `scripts/docs-status.sh --check-drift`,
    `scripts/worklog.sh check`, `scripts/decisions.sh check`.
- What's next:
  - ROADMAP Then: the integration definition editor (Decision 5) and the
    configuration-plane execution spec, blocked on `.loom/39`'s kill-test.
  - ROADMAP Now: stream capture of real ADT feeds (kernel admits only
    MSH/EVN/PID/PV1), Engine properties missing from `serve --help`, the
    operator-bundle e2e timing budget.
- Findings:
  - The spec said `controlPlane`/`connectionCatalog` are true iff
    `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`; the code sets them whenever
    `serve` opened the durable database (any of HTTP ingress, MLLP, batch,
    delivery, sessions, or the control plane). The docs follow the code.
  - The spec's peek provenance `peek:<connectionId>@<digest>:<objectPath>#<n>`
    became `peek:<capture id>`: the object path lives on the audit row, never
    in the sample.
  - At runtime the MLLP listener and batch runner read fixed env keys for their
    credentials, not the connection's bindings; only the peek resolves a
    connection's bindings, through the allow-list.
- Sources:
  - [S1] `.loom/38-connections-execution-specs.md`
  - [S2] MRs !240–!244 and the lanes' status files
