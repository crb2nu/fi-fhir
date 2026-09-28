# End-to-end demo: a batch drop lands at a hospital

This guide walks the deployed engine (`fi-fhir.flexinfer.ai`) from an HL7v2
batch file dropped on SFTP or S3 to FHIR resources on a fake hospital's FHIR
server. The environment behind it is defined in
`platform/gitops/k3s/fi-fhir/demo/` (its README has the credentials and the
exact connection values); this page is the operator's script.

Everything is LAN-only. Nothing in it is reachable from the internet.

| Piece | Address |
|---|---|
| SFTP drop | `sftp-test.flexinfer.ai` (user `fifhir`, key or password), chroot with `/inbound` and `/archive` |
| S3 bucket | `https://s3-test.fi-fhir.flexinfer.ai`, bucket `fi-fhir-demo`, prefixes `incoming/` and `archive/`; console `https://minio-test.fi-fhir.flexinfer.ai` |
| Fake hospital | `https://hospital.fi-fhir.flexinfer.ai/` (HAPI FHIR R4 tester UI), base URL `https://hospital.fi-fhir.flexinfer.ai/fhir` |
| The engine | `https://fi-fhir.flexinfer.ai` (Mapping Studio), polling the SFTP source as definition `sftp-test-demo` |

## 1. A batch file

A batch file is one or more HL7v2 messages starting with `MSH`, separated by
CR, LF or CRLF. The v1 kernel admits ADT^A01-shaped messages (MSH, EVN, PID,
PV1, optional NTE); NK1 and IN1 segments are rejected, and an admit without a
visit number in PV1-19 cannot be projected to FHIR. Two synthetic admits:

```text
MSH|^~\&|DEMOADT|STELSEWHERE|FIFHIR|DEMO|20260928120000||ADT^A01^ADT_A01|DEMO-0001|P|2.5.1
EVN|A01|20260928120000
PID|1||MRN-100001^^^STELSEWHERE^MR||DOE^JANE^Q||19800101|F|||123 DEMO ST^^SPRINGFIELD^IL^62701||^PRN^PH^^^217^5550101
PV1|1|I|MED^101^A^STELSEWHERE||||1234^ATTEND^ALICE|||MED||||||||V-500001^^^STELSEWHERE^VN|||||||||||||||||||||||||20260928120000
MSH|^~\&|DEMOADT|STELSEWHERE|FIFHIR|DEMO|20260928121500||ADT^A01^ADT_A01|DEMO-0002|P|2.5.1
EVN|A01|20260928121500
PID|1||MRN-100002^^^STELSEWHERE^MR||ROE^JOHN^R||19751115|M|||456 DEMO AVE^^SPRINGFIELD^IL^62702||^PRN^PH^^^217^5550102
PV1|1|I|SURG^202^B^STELSEWHERE||||5678^ATTEND^BOB|||SURG||||||||V-500002^^^STELSEWHERE^VN|||||||||||||||||||||||||20260928121500
```

Save it as `adt-batch.hl7`. Every identifier above is invented.

## 2. Drop it

SFTP, with the client key (the gitops README shows how to decrypt it):

```text
sftp -i fifhir_ed25519 fifhir@sftp-test.flexinfer.ai
sftp> put adt-batch.hl7 /inbound/adt-batch.hl7.part
sftp> rename /inbound/adt-batch.hl7.part /inbound/adt-batch.hl7
```

Upload under a temporary name and rename: that is the immutable-drop
contract the batch runner relies on
([Batch ingestion](../operations/BATCH-INGESTION.md)). The server pins its
host keys, so the engine's `known_hosts` never changes.

S3, with `mc`:

```text
mc alias set fifhir-demo https://s3-test.fi-fhir.flexinfer.ai fifhir-demo <secret>
mc cp adt-batch.hl7 fifhir-demo/fi-fhir-demo/incoming/
```

## 3. See it from the Studio

Open **Connections**. Author the source once, with exactly the values in the
gitops README, so the compiled revision's digest equals the document the
engine mounts:

1. **New source connection → `batch_sftp`**, id `sftp-test`. Settings: the
   host, port, user and directories; `known_hosts_binding`
   `sftp-test-known-hosts`, `private_key_binding` `sftp-test-client-key`; the
   `workload` block; the poll and size numbers. Secrets tab: bind the two
   names to `env FI_FHIR_CONNECTION_SECRET_SFTP_TEST_KNOWN_HOSTS` and
   `env FI_FHIR_CONNECTION_SECRET_SFTP_TEST_CLIENT_KEY` (the API carries
   both). Save with a reason, then **Compile** → `r1`.
2. The Status column now reads **Compiled r1 · Mounted here**: the digest the
   Studio computed is the digest in `/app/batch-sources/sftp-test-r1.json`.
3. On the HL7 intake page open **Samples → From connection… → sftp-test →
   Browse objects…**. `adt-batch.hl7` is listed; **Read messages** pulls the
   two admits, redacted, into the Integration Session. Build or check the
   profile against them. Nothing was ingested: a peek takes no lease and
   moves no file.

The same works for the S3 source (`s3-test`, bindings
`FI_FHIR_CONNECTION_SECRET_S3_TEST_*`).

## 4. Ingest and deliver

The runner ingests only when a lifecycle definition named `sftp-test-demo`
is **deployed** with the `sftp-test` r1 source ref, a profile, a workflow and
the hospital as a destination ([Lifecycle](../operations/INTEGRATION-DEPLOYMENT-LIFECYCLE.md)).
Then every poll (15 s) lists `/inbound`, admits each message durably,
archives the file to `/archive` and deletes the input, and the delivery
worker projects each admit into a US Core Patient + Encounter and POSTs one
conditional transaction Bundle to the hospital
([Destination identity](../operations/DESTINATION-IDENTITY.md), "The FHIR
transport").

**Where this stands today.** The definition does not exist yet, and the
Studio cannot create it: publication targets a definition revision that is
already `validated`, there is no definition editor
(`.loom/38-connections-execution-specs.md`, follow-up 5), and nothing outside
tests creates a draft. Until one of the two closes that gap the API polls
and refuses closed, which is the designed behaviour:

- **A seed command** (`fi-fhir lifecycle seed`, proposed): read the mounted
  source revision, the profile and workflow refs from the static registry,
  and a destination revision; build the definition with
  `NewIntegrationDefinitionRevision`; run CreateDraft → ValidateConnection →
  Approve → Publish → Deploy against the engine's database. This is exactly
  the sequence `deployBatchRevision` runs in
  `internal/integration/batch/batch_integration_test.go`, so it is small,
  but it is an operator-facing bypass of the Studio's reviewed publication
  and needs a decision.
- **The definition editor** in the Studio, the recorded next slice of the
  connection catalog.

Once the definition exists, the last GitOps step is the destination registry
naming its revision plus the delivery worker settings (slice 2b in
`platform/gitops/.loom/30-implementation-plan-fi-fhir-demo-environment-2026-09-28.md`).

## 5. Show it at the hospital

`https://hospital.fi-fhir.flexinfer.ai/` opens HAPI's tester. Search
`Patient?identifier=MRN-100001` and `Encounter?identifier=V-500001`; both
arrive as conditional updates keyed by identifier, so re-delivering the same
event updates rather than duplicates. In the Studio, the message trace shows
a Delivery block under each attempt: transport `fhir`, the resource types,
the bundle entry count and the endpoint.

## Resetting between runs

Delete the hospital's PVC (`st-elsewhere-fhir-data`) to wipe it; clear
`/archive` on the SFTP server or `archive/` in the bucket to remove history.
The engine keeps its receipts and ledgers; that is the point.
