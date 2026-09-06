### 2026-08-09: Keep the Kafka Requirement for HTTPS-Only Delivery Deployments, and Document It

- Decision:
  - `buildDeliveryDispatcher` continues to require `FI_FHIR_QUEUE_DRIVER=kafka`
    and a non-empty `FI_FHIR_QUEUE_BROKERS` for the durable delivery worker,
    **even when every destination in the loaded registry declares
    `transport: https`** [S1]. The dependency is documented in `.env.example`
    and in `docs/operations/DESTINATION-IDENTITY.md` rather than relaxed.
  - Relaxing it is filed as a named follow-up ("broker-free delivery worker"),
    not left implicit.
- Rationale:
  - "Every destination is `https`" is a property of one file at one startup, not
    of the deployment. The registry is a single server-owned file read at boot
    [S2]; an operator adding one `kafka`-class destination to it would turn a
    startup configuration error into a runtime dead letter. For a fail-closed
    system that trade goes the wrong way.
  - A mixed registry is the expected steady state during adoption. The Kafka
    topic remains the transport for every destination that has not moved, so the
    broker is not dead weight in any deployment that is actually migrating.
  - Relaxing it means `Dispatcher` must accept a nil `Publisher`, weakening a
    constructor invariant that currently holds for every deployment, in order to
    save a broker for the one deployment class that has already finished
    migrating. The invariant is worth more than the saving today.
  - It costs no new environment variable, so `make check-runtime-config` and
    `scripts/check-runtime-config.sh` are unaffected.
- Alternatives considered:
  - **Relax the requirement when the registry is all-`https`** (rejected for the
    reasons above; revisit once the registry is multi-tenant and reloadable,
    which correction 7 of `.loom/32` records it is not.)
  - **Leave it undecided** (rejected: `.loom/32` Lane S4-A task 8 explicitly
    forbids it.)
- Consequences:
  - An HTTPS-only deployment still stands up a broker it never produces to. This
    is now a documented, deliberate cost with a named follow-up instead of an
    undocumented surprise.
- Sources:
  - [S1] `cmd/fi-fhir/delivery_runtime.go:60-66`
  - [S2] `internal/integration/destination/registry.go:47-53`; `cmd/fi-fhir/destination_identity_runtime.go:42,98-114`
  - [S3] `.loom/32-sprint4-execution-specs.md` correction 8, Lane S4-A task 8
