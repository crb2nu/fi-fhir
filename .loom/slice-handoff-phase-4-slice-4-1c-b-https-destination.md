# Slice Handoff — Phase 4 Slice 4.1c-b: First Durable HTTPS Destination Consumer

**Branch**: `feat/phase4-slice-4-1c-b-https-destination` (Lane S4-A of
`.loom/32-sprint4-execution-specs.md`)
**Merge order**: C → **A** → B → E.

## What shipped

The durable delivery worker now contacts `https`-transport destinations itself,
over real TLS, exactly once per claimed attempt, under the identity 4.1c-a's
`integration.deliver` decision authorized. `kafka`-transport destinations are
unchanged.

The mechanism is **transport substitution at the `Publisher` seam** inside
`Dispatcher.RunOnce`, recorded as a dated decision in `.loom/40-decisions.md`
(2026-08-09) together with its rejected alternatives.

| Piece | Where |
|---|---|
| `DestinationTransport` + `TransportFailure` seam | `internal/integration/delivery/transport.go` |
| Routing between transport and broker | `internal/integration/delivery/dispatcher.go` (`deliverToDestination`) |
| The HTTPS transport and its trust closure | `internal/integration/destination/transport.go` |
| Delivery provenance | `internal/integration/destination/migrations/0002_https_delivery_provenance.sql`, `postgres.go` (`RecordDelivery`) |
| Dispatch-time credential lifetime | `cmd/fi-fhir/destination_identity_runtime.go`, `delivery_runtime.go` |
| Operator contract | `docs/operations/DESTINATION-IDENTITY.md`, `.env.example` |

## The gate that fixed the slice's shape

`TestDeliveryTransport_HTTPSDestinationPublishesToBrokerToday` was written first
and **passed on unmodified `main`** (MR !153). A destination declaring
`transport: https`, with a live TLS endpoint at its URL and a resolvable
credential binding, was fully resolved, digest-verified, authorized, and
provenance-recorded with its address — and then published to Kafka anyway, with
the endpoint recording zero accepted TCP connections.

That killed the lane's riskiest assumption ("4.1c-a resolves the destination
revision on the dispatch path, so 4.1c-b is wiring a transport onto an existing
resolution") and converted the slice into **build the routing seam, the
dispatch-time credential lifetime, and the trust-closed client**. Without it the
lane would have widened `DestinationDecider` into a transport interface,
reintroduced the import cycle the S3-B handoff records solving, and discovered at
the end that a credential has to be resolved somewhere no design accounted for.

## Contracts a later slice must not break

1. **`delivery` and `destination` do not import each other.** `delivery` declares
   primitives-only interfaces; `destination` satisfies them structurally. There
   are now two such seams (`DestinationDecider`, `DestinationTransport`) plus two
   error contracts (`Refusal`, `TransportFailure`). Adding a third follows the
   same rule.
2. **`internal/integration/delivery/store.go` is 4.2a's file.** This slice
   inherits the entire durable state machine without touching it. If a future
   slice believes it needs `store.go`, that is a re-scope.
3. **Nothing observed on the destination side is a trust input.** No redirect is
   followed, no response header is read, the body is drained to a bound and
   discarded unparsed, `InsecureSkipVerify` appears nowhere, and no proxy is
   honored. The only property of a response that leaves the client is its status
   class.
4. **`http_status_class` is server-owned; `destination_endpoint_advisory` and
   `served_certificate_subject_advisory` are not.** Destination-derived facts
   carry an `_advisory` suffix and a `COMMENT ON COLUMN` saying they are never a
   trust input. New CHECKs land `NOT VALID`.
5. **The advisory columns of the two destination ledgers are the only place a
   destination address lives.** The five durable classes of correction 9 carry
   none, and the kill-test scans all five.
6. **No second timeout knob.** The HTTPS call reuses `PublishTimeout`, which
   `Config.validate` already requires to be shorter than the lease. That is the
   invariant that stops a slow destination outliving its lease and being
   delivered twice after reclaim.
7. **The credential is never cached.** File and env references cannot be
   version-pinned, so a cache would silently pin a rotated-out credential.
8. **`TestDeliveryDispatch_ContactsNoDestination` is narrowed, not inverted.**
   Same name, same place in `make delivery-identity`, same arity-2 guard on
   `test:delivery-identity`. It proves a `kafka`-class destination contacts
   nothing, and that stays true.

## Proofs and how to run them

```bash
export POSTGRES_TEST_URL=... KAFKA_TEST_BROKERS=...
make destination-transport    # kill-test + day-1 gate + the negative control
make delivery-identity        # 4.1c-a's two proofs, unchanged
make delivery-reliability     # Slice 2.3's proof, unchanged
```

CI: the new blocking job `test:destination-transport`, with its own
`-list | rg -x | awk 'END { if (NR != 2) exit 1 }'` existence guard.
`test:delivery-identity`'s guard is untouched at arity 2.

The kill-test runs its **negative control in the same invocation**: the whole
scenario repeats against a router that reports it owns no destination, and four
named assertions must fail there while the kafka-class assertion still passes. A
control that passes turns the job red, because it would mean the router is not on
the dispatch path.

Local gotcha: a Kafka broker reused across runs makes
`TestDeliveryReliability_PostgresKafkaFailureReplay` fail, because it reads the
first three records off the shared topic and inherits keys from an earlier run.
CI gives every job a fresh Kafka service container. Recreate the local broker
between full sweeps.

## Next actions

1. **A FHIR-class destination — the hard blocker for Slice 5.1.**
   `docs/planning/FHIR-CONFORMANCE-MATRIX.md` (Lane S4-D, MR !155) records the
   finding that no FHIR resource is produced anywhere on the durable path:
   `pkg/fhir` is reachable only from the legacy workflow engine and one CLI
   subcommand. 4.1c-b ships a **generic** `https` transport — it POSTs the
   `integration.delivery.v1` command, not a FHIR resource. 5.1 needs a
   destination *class* that declares a FHIR interaction and a producing path
   into it. Sequence that before pinning an IG or integrating a validator.
2. **mTLS to destinations.** `CABundleBinding` governs trust of the *server*
   only. A per-destination client certificate is a new binding kind, a new
   `tls.Config.Certificates` path, and a decision about where the private key
   lives — deliberately out of this slice.
3. **A multi-tenant destination registry.** `registryDocument` carries exactly
   one `tenant_id` and one `integration_revision`, and one
   `FI_FHIR_DELIVERY_IDENTITY_REGISTRY_PATH` is read at boot
   (correction 7 of `.loom/32`). Everything above inherits that limit.
4. **A broker-free delivery worker.** The named follow-up from the Kafka
   dependency decision. It needs the registry to be reloadable first, or the
   startup-vs-dispatch trade the decision rejects comes straight back.
5. **Startup reporting.** `serve` prints the delivery identity mode but says
   nothing about how many destinations are `https`-class. That line lives in
   `cmd/fi-fhir/main.go`'s component table, which is S3-A's shape and was out of
   this lane's file ownership.
