# Mapping Studio (IDE)

The Mapping Studio is fi-fhir's web IDE (`ui/`, SvelteKit). You start from
sample messages, see what the parser makes of them, shape a Source Profile
until the feed parses cleanly, and then route, deliver and verify, all against
a running `fi-fhir serve`. It never simulates data. When the deployment or your
identity cannot provide something, the page says what is missing and does not
send the query.

To try the engine without a deployment, use the
[Browser Playground](playground.md). It runs the same parser and profile
compiler in your tab, with no backend. To click through the IDE itself, open
the [hosted demo](https://fi-fhir-demo.flexinfer.ai). It has a preview-only
identity and no database: HL7 Preview works, and every other page shows the
honest state described below.

## Signing in

On load the IDE asks the API who you are (`/api/auth/status`):

- **Trusted network** or **Cloudflare Access**: you are in. The status bar's
  access chip reads "Trusted network" or "Cloudflare Access · *your email*".
- **Otherwise** a dialog asks for the deployment bearer token ("Enter access
  token"). The token is held in the tab's memory only and never stored.
  Reloading the page, or **Clear access** on the access chip, drops it.

The same answer carries your roles and the deployment's **capabilities**
(operator plane, Integration Sessions, streaming, connection catalog, and so
on). Pages read them before they query. If the API is older and reports no
capabilities, pages try the request and report any failure inline.

## The shell

Editor tabs remember the last session or record opened by a deep link. Switch
back through a tab, the activity bar, or a **Go to** command to resume there.
An explicit link to the base page clears that selection. This remembers the
record location, not an unsaved form or filter state.

⌘/Ctrl+K opens Commands even while typing in a field or the HL7 editor.
Escape returns focus to where you were working. Other open dialogs keep their
own keyboard focus.

- **Header**: the fi-fhir mark (Home), the five **stages**, a breadcrumb,
  **Commands** (⌘K / Ctrl+K) and the theme toggle (system, light, dark).
- **Stages**: 1 Source Intake (`/hl7`) → 2 Normalization (`/profiles`) →
  3 Translation (`/terminology`) → 4 Delivery (`/workflows`) →
  5 Verification (`/events`). Each stage is a link. The current one is
  highlighted and earlier ones carry a check. With the stages focused, the
  arrow keys, Home and End move between them. Home (`/`), Connections and
  Operator are not stages.
- **Activity bar**: Home, HL7 / Intake, Profiles, Terminology, Workflows,
  Verification, Connections, Operator.
- **Status bar**: the API connection ("Connected", "Connecting", "Offline",
  from `/health` every 30 s), the access chip, **Next:** *the following
  stage*, and the build tag.
- **Bottom panel** (⌘/Ctrl+J): Output, Problems (with a count when a draft
  has problems), Debug, Trace and Copilot.
- **Command palette** (⌘K / Ctrl+K): "Go to" any route, new source or
  destination connection, engine properties, and the panel toggles. On
  `/hl7` the same shortcut opens the HL7 commands.

## Pages

### Home (`/`)

Recent work (open documents, and your Integration Sessions when the session
workspace is on; selecting a session reopens it in HL7 intake, see
[Sessions](#sessions)), deployed Integrations, Health, and Alerts. Integrations
needs `integration.operator`. Without it the panel says so and queries
nothing. Alerts reads "No alert source configured." unless one is.

### HL7 intake (`/hl7`), stage 1

An editor for one HL7v2 message, and tabs for what the engine made of it:
**Samples, Warnings, Events, Extraction, Inspector, Profile draft, Process,
Live events**.

- **Preview** (⌘/Ctrl+Enter) parses the editor text and fills Warnings and
  Events. When the Integration Session engine is available, Preview runs in a
  session, which keeps the samples, profile draft and diagnostics together.
  That needs the UI build flag `VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED`, the
  API's `integrationSessions` capability, and its session stream. Otherwise
  Preview uses the stateless preview mutation, and a note says so.
- **Process** submits the message to the backend pipeline and shows the event
  id, correlation id, the workflows it matched, and a link to the live events.
- **Samples** holds messages for the feed you are working on. Import files,
  save the current editor text, **Load examples** (six synthetic demo
  messages, the same ones the playground embeds), or **From connection…**,
  which pulls real messages from a source connection into the session. See
  [Connections: sampling from a connection](connections.md#sampling-from-a-connection).
- **Redaction**: the toolbar chooses a mode for the editor, Preview and
  Process ("Mask basic (PID/NK1/PV1)", "Sanitize segments (PID/NK1/IN*)",
  "Pattern replacement (SSN/phone/email)"). It is best-effort, and free-text
  fields can still carry PHI. Pasted samples stay in the tab's memory and
  clear on reload. Paste PHI only on an approved machine and profile.
- **Line endings**: HL7 needs CR segment terminators. Both paths convert
  pasted LF or CRLF to CR before sending, and leave the editor text as typed.
  **Normalize newlines** rewrites the editor text itself.

The IDE shows events and diagnostics but not the FHIR Bundle. To see the
Bundle a message produces, use the [Browser Playground](playground.md) or a
`fhir` destination.

#### Sessions

With the session engine on, the page's Integration Session is a piece of work
you can come back to. The first Preview (or a capture) creates it, and the
address becomes `/hl7?session=<id>`: reload the page, bookmark it, or pick
the session in Home › Recent, and the page reopens it. The **Session** toolbar
button shows or hides the session sidebar, which reads everything back from
the API:

- **Runs** (`sessionRuns`), newest first. Selecting one lists its diagnostics
  (`sessionDiagnostics`); **Show in results** loads that run's warnings and
  events into the tabs. On a deep link the newest run is shown. While a run is
  pending or running, the sidebar follows it on `sessionRunEvents` and stops
  when it finishes; where that stream is not allowed it says so instead.
- **Accept fix** records, with your identity, that you accept a diagnostic's
  fix suggestion (`acceptDiagnosticFix`). A warning in the Warnings tab that
  came from a session diagnostic has the same **Accept fix**, and shows
  "fix accepted" afterwards.
- **Publications** and **Simulations** made from this session, its samples
  and its saved profile and workflow drafts.
- **Export…** downloads the session as `fi-fhir-session-<id>-<UTC time>.json`:
  runs (status, stages, diagnostics, lineage, and each event's id, type, time
  and correlation), drafts, simulations and publications. It carries no raw
  sample text and no parsed patient fields; diagnostic messages are included
  as the parser wrote them. An export is a PHI disclosure: it needs a reason,
  and the API records the reason and your identity on an append-only export
  record. Raw sample payloads are offered only when `/api/auth/status`
  reports `capabilities.phiExport: true` (your identity holds
  `integration.phi.export`); otherwise the dialog names the missing role, or
  says the deployment did not report the grant.
- **Archive…** takes the session off Home › Recent. Its runs, publications
  and export records stay, and its link still opens it, marked Archived.
  Previews on that page still record runs in it; open `/hl7` without a session
  link to start a new one. The API records no reason for an archive, so the
  dialog asks for none.

Sample text is not read back: a pasted sample's text stays on the server.
Captured and peeked samples reload into Samples, because intake reads them
back (with `integration.operator`). Operations detail is in
[Integration Sessions](../operations/INTEGRATION-SESSIONS.md).

### Profiles (`/profiles`), stage 2

Source Profiles as a table with a details pane. **Builder** has Tolerance,
Events, Identifiers and Terminology sections. There is also a **YAML** view
and **Revisions**, and the actions New, Duplicate, Delete and **Review &
publish**. What a profile can express is in [Source Profiles](source-profiles.md).

### Terminology (`/terminology`), stage 3

Browse, Upload, Review (pending mappings), Resolver and Workflows. See
[Terminology Management](terminology.md).

### Workflows (`/workflows`), stage 4

Inventory, Design and Verification. A workflow draft can be simulated, then
published, approved and deployed through the lifecycle. Design validates the
draft beside each field and disables Save and Publish with the reason; keys the
builder cannot edit are listed as YAML-only fields and saved as written.
Inventory renames, re-describes, archives and restores definitions, with an
Active/Archived filter. Verification opens a recorded run's trace in the Trace
panel. The live monitor needs the workflow event stream. On deployments that
stream Integration Sessions only, it says so and points to the recorded runs.
Details are in [Managing Workflows in the IDE](workflows.md#managing-workflows-in-the-ide);
the DSL is in [Workflow Configuration](workflows.md).

### Verification (`/events`), stage 5

**Admissions**, **Statistics** and **Retention**, read from the durable
admission records through the operator control plane (`integration.operator`).
The page does not stream and has no patient timeline, and it says why. See
[Verification](verification.md).

### Connections (`/connections`)

**Sources**, **Destinations** and **Engine**. You define, validate and compile
connections, see which ones this replica runs, and read every engine property
of the replica that answered. It needs the connection catalog (the durable
database) and `integration.operator`, and editing needs
`integration.deployment.operator`. The page names whichever is missing. See
[Connections](connections.md) and
[Connection catalog operations](../operations/CONNECTION-CATALOG.md).

### Operator (`/operator`)

**Messages**, **Delivery** and **Deployments** for the running engine. Every
mutating action asks for a reason. Without `integration.operator` the page
runs no queries and names the role and where it is granted
(`FI_FHIR_GRAPHQL_ROLES`, `FI_FHIR_GRAPHQL_ACCESS_PRINCIPALS`). Without
`integration.delivery.operator` or `integration.deployment.operator`, the
matching controls are disabled with the reason. See the
[Runbook](../operations/RUNBOOK.md).

## Copilot

The Copilot tab in the bottom panel (or **Open Copilot** in the palette) has
Explain, Suggest, Generate and Review, running on the API's own LLM. Its
status line says whether an LLM is configured and answering ("Backend LLM
ready · *model*", "No LLM is configured for this deployment"). The input is
locked when there is none. On `/hl7` the Warnings tab can also explain
warnings. See [LLM Features](llm-features.md).

## Honest states at a glance

| You see | Why | Fix |
|---|---|---|
| "…does not hold `integration.operator`…" | your identity lacks the operator role, so nothing was queried | grant the role (`FI_FHIR_GRAPHQL_ROLES` or the Access principal map) |
| "The connection catalog is not configured on this deployment." | `serve` opened no durable PostgreSQL database | configure `FI_FHIR_DATABASE_*` and a durable feature |
| "Live streaming for … is not available on this deployment" | that subscription root is not allowed, or sessions are off | expected in production for the legacy streams |
| "Preview runs on the stateless path instead." | the session engine is unavailable | enable Integration Sessions on the API |
| "Session … is not in this deployment's session store." | a `/hl7?session=` link to an id this deployment and tenant do not hold | open the session on the deployment that created it |
| "Session … cannot be opened here." | a `/hl7?session=` link on a deployment without the session engine | enable Integration Sessions on the API |
| "Raw sample payloads are not offered: exporting them needs integration.phi.export…" | the identity lacks the PHI export grant | grant `integration.phi.export` only to identities approved for raw disclosure |
| "No LLM is configured for this deployment" | no LLM endpoint on the API | configure the API's LLM settings |

Running the IDE locally, its build and its tests are in
[`ui/README.md`](https://github.com/crb2nu/fi-fhir/blob/main/ui/README.md)
and `ui/docs/DEVELOPER-GUIDE.md`. The design rules are in `ui/docs/DESIGN.md`.
