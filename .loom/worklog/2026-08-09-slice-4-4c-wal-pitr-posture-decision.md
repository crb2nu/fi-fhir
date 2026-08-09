### 2026-08-09 - Slice 4.4c WAL PITR posture decision

Lane S5-B's required day-1 decision, landed as a docs-only commit before the
lane touches `deploy/` — which is the order `.loom/33` specifies, and the order
that keeps the decision from being reverse-engineered from whatever got built.

- What changed:
  - One dated entry appended to `.loom/40-decisions.md`: "WAL/PITR posture —
    this repository certifies RTO and hands RPO to the operator, with a
    method". Options A, B, and C are all presented; A is adopted, B is filed
    with its cost written down, C is presented and rejected.

- Why:
  `docs/operations/PRODUCTION-HARDENING.md` already states, in shipped
  documentation, that the documented `pg_dump` method cannot meet the product
  spec's RPO of 5 minutes and that nothing in this repository configures WAL
  archiving. So the lane's question was never *whether* the gap exists. It was
  what this repository ships in response, and that is a decision, not a
  discovery.

  Option A splits budget 5 along the line where the evidence actually falls.
  The RTO half is provable here — time the documented procedure end to end,
  archive the number — and slice 4.4c will prove it. The RPO half depends on an
  archiving method this repository does not own: the only PostgreSQL in
  `deploy/` is a single-replica `Deployment` on a ReadWriteOnce PVC, which is a
  development convenience, and a production deployment uses a managed service
  or an operator that owns WAL archiving through its own interface. A reference
  `archive_command` written against the dev manifest would be a configuration
  almost nobody runs, carrying the authority of a product guarantee.

  Option B was rejected on scope and on accountability, not on difficulty. It
  makes this repository the owner of an operational posture it does not
  otherwise own, and it competes for the same lane budget as budget 4
  (destination recovery under an injected fault) and budget 7 (upgrade/rollback
  provability), both of which are provable in CI today. Its full cost is
  written into the decision so it can be scheduled rather than rediscovered.

  Option C — relaxing budget 5 — is the one worth rejecting explicitly. The
  5-minute target is right for a PHI integration engine; the documented method
  is what is wrong, and amending a product-spec target to match a weak method
  is the wrong repair.

- Evidence:
  Nothing is measured by this commit. It records a position and the follow-up
  work it defers, and it names what slice 4.4c will measure: the RTO of
  `scripts/pgdump-roundtrip.sh` end to end — dump, restore, first successful
  delivery `Claim` — archived alongside the row counts it was measured against,
  because a fixture-scale number is not a capacity claim.

- What's next:
  - `PRODUCTION-HARDENING.md:990-1042` and `SUPPORTED-1.0.md:86` updated to
    match the decision rather than restate the gap, in the slice 4.4c
    implementation MR. Lane S5-C also edits `PRODUCTION-HARDENING.md` (the
    unemitted log schema at `:583-598`); different sections, and S5-C merges
    first.
  - One consequence recorded here so it is not lost: under this decision slice
    4.4c ships no new durable schema, so the two pre-existing rollback-unsafe
    columns the 2026-08-09 rollback decision filed to 4.4c —
    `integration_delivery_attempts.scheduled_at` and
    `integration_delivery_outbox.updated_at`, both from processor ledger 2 —
    are re-filed to Lane S5-F, which owns processor `0006` this sprint. They
    stay in `knownRollbackUnsafeColumns` with their dated reason until then.

- Sources:
  - [S1] `.loom/33-sprint5-execution-specs.md` — Lane S5-B, "The WAL/PITR
    posture decision (required deliverable)"
  - [S2] `docs/operations/PRODUCTION-HARDENING.md:990-1042`
  - [S3] `.loom/40-decisions.md`, 2026-08-09 "A logical `pg_dump` cannot meet
    the 5-minute RPO, and a newer client makes it unrestorable"
  - [S4] `deploy/kubernetes/base/postgres.yaml`
