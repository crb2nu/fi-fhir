### 2026-09-24: The operator trace reads the delivery ledger through one bounded per-attempt read

- Decision: the operator control plane reads `integration_destination_deliveries`
  only through `destination.PostgresProvenance.ListDeliveriesForAttempt(ctx,
  tenant, attempt, limit)`, behind an operator-owned `DestinationDeliveryReader`
  interface, one indexed lookup per attempt. Bounds: 25 rows on a single
  attempt read (and a control action's result), 5 per attempt on the attempt
  list and the message trace. `serve` migrates the destination ledger wherever
  it wires the operator control plane. A ledger read error fails the attempt
  read; it never degrades to an empty list.
- Rationale: the destination package stays the one owner of the ledger's SQL
  (operator -> destination is acyclic, so no adapter in `cmd/` is needed).
  Five is the default delivery retry budget, so a never-replayed attempt shows
  every exchange in the trace; 25 is five such runs. The attempt set is already
  bounded by `MaxPageSize` (100), so the worst case is 100 indexed point reads.
  Before this slice the ledger was migrated only when
  `FI_FHIR_DELIVERY_IDENTITY_MODE` was set, so a `serve` without it would have
  queried a missing table; migrating it beside the lifecycle catalog follows the
  existing precedent in the same wiring block, applies only the existing
  0001–0003 set under its advisory lock, and adds no migration. An empty
  `deliveries` list is a statement — "this process contacted no destination" —
  so it must not be produced by a read failure.
- Alternatives considered: (a) join the ledger in the operator's own SQL —
  rejected, it spreads the ledger's schema into a second package; (b) a batched
  `attempt_id = ANY(...)` read with a per-attempt window — deferred, the spec
  fixed the single-attempt signature and 100 indexed lookups is not yet a
  measured cost; (c) tolerate a missing table with `to_regclass` — rejected, it
  would also have to tolerate a v2 ledger without the fhir columns, and
  migrating is simpler and truthful; (d) a nil reader meaning "no deliveries" —
  rejected, it would make an unwired deployment claim no destination was ever
  contacted.
- Consequences: every `serve` with a submission database now carries the
  (empty) destination ledger tables. `NewService` requires the reader. The UI
  selects six of the ledger's columns and renders only those.
- Sources:
  - [S1] `.loom/35-sprint7-execution-specs.md` (Lane S7-C)
  - [S2] `internal/integration/operator/service.go` (`attachDeliveries`)
  - [S3] `cmd/fi-fhir/main.go` (operator control-plane wiring)
