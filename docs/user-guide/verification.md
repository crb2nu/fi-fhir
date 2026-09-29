# Verification

**Verification** (`/events`, the fifth stage of the Mapping Studio journey)
answers one question from the engine's own records: *what did the engine
actually admit, and what happened to it?* It reads the durable tables the
admission path commits (receipts, canonical events, lineage and delivery
attempts) through the operator control plane. It shows nothing it did not
read, and it never reads back a payload value.

Operators: every figure on this page comes from two GraphQL reads,
`operatorCanonicalEvents` and `operatorAdmissionStatistics`, described in the
schema (`internal/api/graphql/schema.graphql`, "Verification over durable
admissions").

## Before you start

The page checks two things before it sends any query, and names the one that
is missing:

| You see | Meaning | Fix |
|---|---|---|
| "Verification reads the durable admissions of the operator control plane, which is not configured on this deployment" | `capabilities.controlPlane` is false: `serve` has no PostgreSQL submission store, so there are no durable admissions to read. The hosted demo is like this. | Set `FI_FHIR_DATABASE_*`, or `FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true` with the database it needs. |
| "This identity (…) does not hold `integration.operator` …" | `capabilities.operatorRead` is false. No query was sent. | Grant `integration.operator` in `FI_FHIR_GRAPHQL_ROLES` or `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS`. |

The production operator bundle already carries the role. Verification needs no
role of its own; it reads what the Operator page reads.

## Admissions

One row per canonical event, newest first. Each row shows:

- **Recorded**: when the admission committed (UTC).
- **Event type**: the semantic event, for example `patient_admit`.
- **MSH-10**: the source message ID. For HL7v2 this is MSH-10.
- **Receipt**: links to that receipt's trace on **Operator**
  (`/operator?receipt=<id>`). A rejected receipt carries a red badge.
- **Definition**: the integration definition revision the receipt recorded.
  It links to **Connections › Definitions**.
- **Source**: the source connection revision from the event's lineage. It
  links to **Connections**. The cell shows "—" when the event has no lineage
  row.
- **Retention**: "purge after *date*" when retention has stamped a deadline,
  or "tombstoned" once the purge has replaced the payload.

Select a row to see the event's identifiers and its **payload structure**:
the field paths the document carried and each one's JSON kind (`string`,
`object`, `array` and so on). This is the same projection the Operator trace
uses. Stored values are never returned, and there is no switch that returns
them.

**Filters** are exact matches on columns: event type, definition, receipt,
MSH-10, correlation ID, and a recorded-at window (From/To, in your local time,
sent as UTC). Tombstoned events are hidden unless you check **Include
tombstoned**. **Previous**/**Next** page through the results 25 at a time with
the server's cursor.

`/events?receipt=<id>` opens Admissions filtered to that receipt. The Operator
trace links here from its receipt.

## Statistics

Counts for one window, all read from table columns in a single database
snapshot:

- **Accepted** and **rejected receipts**, and **receipts by definition**.
- **Canonical events**, and **events by type**.
- **Delivery attempts** by status (succeeded, failed, queued) and **by
  destination**. Each destination links to its connection.
- A **series** of plain bars, one per bucket: receipts (accepted over
  rejected) and delivery attempts (succeeded, failed, queued). Hover a bar
  for its exact counts.

Pick the window with **Last hour**, **Last 24 hours** (both counted by hour),
**Last 7 days** (by day) or **Custom**. A custom window of two days or less is
counted by hour, and a longer one by day. The server counts at most 744
buckets. It refuses a wider window rather than cutting it short, and the page
says so before it asks. The line beside the picker states the window exactly
as it was counted, in UTC.

**Delivered** is the attempts ledger's own word: the attempts created in the
window that have succeeded, out of all the attempts created in it. Queued
attempts count as not delivered yet, and the sentence says how many there
are. With no attempts in the window, the page says so and gives no
percentage.

Receipts are counted by when they were recorded. Attempts are counted by when
they were created, under their current status. A grouped list stops at 100
groups, and a line says when one was cut. The totals are never cut.

## Retention

- **Purge on / off**: whether the retention purge runs on the replica that
  answered (`engineRuntime.retentionPurge`). It runs only when
  `FI_FHIR_RETENTION_POLICY_PATH` names a policy document. Off means that
  replica tombstones nothing.
- For the chosen window: **Tombstoned** events (the purge replaced the payload),
  **Scheduled for purge** (a deadline is stamped and the payload is intact),
  and **No deadline**.

A purge keeps the row, its identifiers and its receipt, and replaces only the
payload. So every count here is a count of rows that still exist. See
[PHI retention](../operations/PHI-RETENTION.md) for the policy and the purge.

## What this page does not do

- **It does not stream.** Admissions are read from what the engine committed,
  and the page updates when you refresh or apply a filter. An Integration
  Session's run stream is on **HL7 intake** (`/hl7`).
- **It has no patient timeline.** Payload values are never read back, by
  design, so nothing on this page can be arranged by patient.
- **It does not read the legacy event store.** The older `events`,
  `patientTimeline`, `eventStatistics` and `eventStream` GraphQL fields still
  exist in the schema, but the IDE no longer calls them: on `serve` they have
  no writer and would always be empty.
