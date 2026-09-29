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
visit number in PV1-19 cannot be projected to FHIR. The `adt-east` profile
the demo definition uses knows one assigning authority, `HOSP`, so patient
and visit identifiers carry it. Two synthetic admits in the shape the batch
proof uses (verified on 2026-09-29: both delivered to the hospital):

```text
MSH|^~\&|STELSEWHERE-ADT|FAC|APP|FAC|20260929120000-0400||ADT^A01^ADT_A01|DEMO-2001|P|2.5.1
EVN|A01|20260929120000||||20260929120000-0400
PID|1||MRN-100001^^^HOSP^MR||Doe^Jane||19800101|F
PV1|1|I|UNIT^101^A^FAC||||||||||||||||V-500001|||||||||||||||||||||||||20260929120000
MSH|^~\&|STELSEWHERE-ADT|FAC|APP|FAC|20260929121500-0400||ADT^A01^ADT_A01|DEMO-2002|P|2.5.1
EVN|A01|20260929121500||||20260929121500-0400
PID|1||MRN-100002^^^HOSP^MR||Roe^John||19751115|M
PV1|1|I|UNIT^202^B^FAC||||||||||||||||V-500002|||||||||||||||||||||||||20260929121500
```

Save it as `adt-batch.hl7` with CR segment terminators. Every identifier above
is invented. Messages the kernel refuses (for example a PID-3 with an
assigning authority the profile does not know) quarantine the whole file
with `INVALID_MESSAGE`: it stays in `/inbound`, nothing is admitted, and the
runner will not retry it. Fix the content and drop it under a new name.

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

Until that definition exists the API polls and refuses closed, which is the
designed behaviour. The Studio cannot create it (publication targets a
definition revision that is already `validated`, and there is no definition
editor yet), so `fi-fhir lifecycle seed` does
([CLI reference](cli-reference.md#lifecycle-seed)).

**Where the profile and workflow come from.** The runner loads them from the
static registry (`FI_FHIR_INTEGRATION_REGISTRY_PATH`), not from the Studio's
stores, so the seed takes them from a registry entry. The deployed registry's
entry `adt-east` carries the ADT profile and a workflow whose `fhir` action
delivers to the destination named `fhir-primary`; the hospital's destination
revision therefore carries that id
(`platform/gitops/k3s/fi-fhir/config/destinations/fhir-primary-r1.json`,
transport `fhir`, base URL `https://hospital.fi-fhir.flexinfer.ai/fhir`).
Every non-log action in the workflow must name a destination passed to the
seed, or it refuses.

**Seed from GitOps.** `platform/gitops/k3s/fi-fhir/demo/seed/` renders a
one-shot Job that runs the command inside the cluster with the same source
revision, registry and SFTP credentials the API mounts, validates the source
for real (connects with the pinned host key and lists `/inbound`, moving
nothing), approves and publishes:

```text
kubectl kustomize --load-restrictor LoadRestrictionsNone k3s/fi-fhir/demo/seed > /tmp/seed.yaml
kubectl apply -f /tmp/seed.yaml
kubectl -n fi-fhir logs -f job/fi-fhir-lifecycle-seed-sftp-test-demo
```

The Job pins `--principal gitops-seed`, its `--reason`, and
`--created-at 2026-09-29T00:00:00Z`, so its definition digest is stable and
a re-run resumes the same revision instead of refusing a different one. The
delivery worker's registry for that revision is already committed
(`platform/gitops/k3s/fi-fhir/config/destination-registry-v1.json`,
`integration_revision` `sftp-test-demo`/`v1`); if the Job ever prints a
different digest, update that file.

The same thing by hand, from the gitops checkout, inside the API pod (which
already has the tenant, registry path, database settings, SFTP key and
`known_hosts`; the destination revision goes in on standard input):

```text
kubectl -n fi-fhir exec -i deploy/fi-fhir-api -- /fi-fhir lifecycle seed \
  --source /app/batch-sources/sftp-test-r1.json \
  --definition-id sftp-test-demo --revision-id v1 \
  --integration adt-east \
  --destination - \
  --principal gitops-seed \
  --reason "demo: seed sftp-test-demo from platform/gitops k3s/fi-fhir/demo/seed" \
  --created-at 2026-09-29T00:00:00Z \
  --validation-max-age 900 \
  --destination-registry-out - \
  < k3s/fi-fhir/config/destinations/fhir-primary-r1.json
```

Both stop at `published` and print the definition ref, the validation codes
and their expiry, and `FI_FHIR_BATCH_DEFINITION_ID=sftp-test-demo`. Deploy the
release from the Studio's **Operator** page within the validation window
(fifteen minutes with `--validation-max-age 900`), or run the command again
with `--through deployed`. The next poll ingests.

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
