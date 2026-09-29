# Browser gate (`test:ui-e2e`)

`run.sh` starts four `fi-fhir serve` stacks behind the production nginx
template and runs the Playwright projects in `ui/playwright.config.ts` against
them; `ci.sh` provisions the CI image and calls it; `docker.sh` (`make ui-e2e`)
runs `ci.sh` in the same image on a docker context. `check-report.mjs` fails
the job when a required check did not run and pass.

| project | UI | API | what differs |
|---|---|---|---|
| operator-bundle | :3000 | :18081 | the full operator bundle, sessions on, **plus the fixture below** |
| missing-operator-role | :3001 | :18082 | the bundle minus `integration.operator` |
| sessions-off | :3002 | :18083 | `FI_FHIR_INTEGRATION_SESSION_ENABLED` unset |
| preview-only | :3003 | :18084 | the hosted demo: `integration:preview`, no database, no control plane |
| visual | — | — | review PNGs of the operator-bundle (and preview-only) routes |

## The operator-bundle fixture (`fixture.sh`, lane E-0)

Before Playwright runs, `run.sh` calls `fixture.sh` against the
operator-bundle database (`fi_fhir_e2e_bundle`). Everything it writes goes
through the real code paths of the same binary:

1. **A deployed definition.** `fi-fhir lifecycle seed --validate skip
   --through deployed --validation-max-age 86400` puts `e2e-batch-adt/v1`
   through create_draft → validate_connection (`VALIDATION_SKIPPED`) → approve
   → publish → deploy. Inputs: `fixtures/batch-source-s3.json` (a copy of
   `testdata/golden/integration/adt-batch-s3/source-revision.json`),
   `fixtures/destination-fhir-primary.json` (a synthetic FHIR destination
   revision whose digest the seed verifies), and the preview registry's
   `adt-east` integration for the profile and workflow refs.
2. **One dead-lettered admission.** Side process A is a short-lived
   `fi-fhir serve` on :18095 with the durable HTTP ingress (`/v1/hl7v2`,
   bearer) and the delivery worker pointed at a Kafka broker that does not
   exist (`127.0.0.1:9`, `FI_FHIR_DELIVERY_MAX_ATTEMPTS=3`). `E2E-FIXTURE-001`
   is admitted; the worker claims it three times and dead-letters it with
   `KAFKA_PUBLISH_FAILED`: six audit rows (`claimed`, `retry_scheduled` ×2,
   `dlq_entered`) and one open dead letter. A stops once the dead letter
   exists.
3. **Two queued admissions.** Side process B on :18096 runs the ingress
   alone; `E2E-FIXTURE-002` and `-003` are admitted and their attempts stay
   `queued`.

So the stack starts with **3 accepted receipts, 2 queued attempts, 1 open dead
letter and 1 deployed definition**; `fixture.sh` asserts those counts with
`psql` and writes them to `e2e-results/fixture.json`. Deliveries are never
asserted: nothing in the stack reaches a destination.

Why side processes: the ingress and worker run beside the bundle API, never
on it, so the bundle replica still mounts no adapter (check 8) and HL7
intake's "From connection…" is still honestly empty (check 9). A and B write
their own runtime-observation heartbeats and then stop, so the fleet views
(Engine tab, Home › Health) show three replicas, two of them going stale.

Every message is synthetic (`SYNTHETIC^PATIENT`, placeholder identifiers,
MSH-10 `E2E-FIXTURE-00n`); the bearer values are test strings. The ingress
refuses any request carrying an `Origin` header, so `fixture.sh` posts with
curl.

Checks that rely on the fixture: operator-bundle 2 (Messages lists the
receipts) and `E0-1`…`E0-6`; `E0-4` resubmits the dead letter, so later checks
see one more queued attempt. Later lanes may rely on the counts above.

## Lane checks

Each lane prefixes its check ids with the lane (`E0-1.`) and adds one line to
`laneChecks` in `check-report.mjs`.
