# Browser Playground

The playground at **<https://flexinfer.ai/playground/fi-fhir>** runs fi-fhir's
own engine in your browser tab. It is not a mock-up and does not call a
server. The Source Profile compiler, the HL7v2 parser and the FHIR projection
that `fi-fhir serve` uses are compiled from Go to WebAssembly (the
**browser kernel**, `cmd/fi-fhir-wasm`), and the page runs that module in a Web
Worker.

- **Nothing leaves the page.** The module has no network and no filesystem
  access. What you paste is parsed in the tab and discarded when you close it.
- **Synthetic data only.** The six samples and three example profiles built
  into the module are synthetic test fixtures. The page is a public web page,
  so paste only synthetic or de-identified messages, never real patient data.
- **One download.** The module is about 8 MB, or 2.2 MB compressed, and the
  browser caches it after the first visit.

For a walk through the three tools, see the
[Playground Tutorial](playground-tutorial.md). This page covers what the
playground runs, what it returns, and where it stops.

## What runs in the browser and what needs the IDE

| | Browser playground | Mapping Studio IDE with `fi-fhir serve` |
|---|---|---|
| Parse HL7v2 into segments, one semantic event, and diagnostics | yes, the same code path | yes |
| Compile and validate a Source Profile | yes, the executable v1 subset | yes |
| Conditional FHIR R4 transaction Bundle for the event | yes, shown: built the way a `fhir` destination receives it | built and delivered by the engine; HL7 intake does not display it |
| CSV, EDI X12, CDA/CCDA, FHIR input | no, HL7v2 only | yes (CLI and engine) |
| Workflows: routing, CEL filters, actions, simulation | no, the workflow planner is not linked | yes |
| Integration Sessions, saved drafts, revisions, publishing | no | yes, with the durable database |
| Connections, sample intake from a source, engine properties | no | yes, see [Connections](connections.md) |
| Terminology, event history, operator console, Copilot | no | yes, with the matching roles |

The playground answers one question: what does fi-fhir make of this message
with this profile? Everything that stores, routes or delivers needs a running
engine. The [Mapping Studio guide](ide.md) describes the IDE, and
[Getting Started](getting-started.md) covers the CLI.

## It is the IDE's engine

The kernel follows the same steps as the IDE's Integration Session preview:
compile the profile, parse with the profile's timezone, then normalize the
diagnostics. A Go test (`TestWASMKernelMatchesSessionRunner`) runs the IDE's
six demo samples through both paths, with and without the golden `adt-http`
profile. It requires the events and diagnostics to be byte-identical, apart
from the event id and the two timestamps that both paths generate. Two more
tests keep the embedded samples identical to the IDE's demo samples and the
example profiles identical to the golden profiles. So a sample gives the same
result in the playground as in HL7 intake.

The one thing left out is the workflow planner, whose dependencies (CEL,
PostgreSQL, Prometheus, OpenTelemetry, Redis) would make the module about
nine times larger. In the browser build it reports `ErrWorkflowUnavailable`.

## Built-in samples and profiles

| Sample | Event | Bundle |
|---|---|---|
| ADT A01 - ICU Admission | `patient_admit` | Patient and Encounter |
| ORU R01 - Lab Results (CBC) | `lab_result` | DiagnosticReport and six Observations |
| SIU S12 - Appointment Scheduled | `appointment_scheduled` | none: no FHIR projection for appointments |
| ADT A03 - Discharge (with warnings) | `patient_discharge` | Patient and Encounter |
| ORM O01 - Lab Order | none: `PARSE_FAILED`, unsupported message type (the IDE reports the same) | none |
| MDM T02 - Document with Content | `document_original` | none: no FHIR projection for documents |

These are the results with no profile. The three example profiles are
**ADT A01 over HTTP (strict)** (the golden `adt-http` profile; a message
without PV1 fails), **ADT A01 over HTTP (tolerant)** (the same profile with a
`MISSING_PV1` warning instead), and **ADT A01 by patient class** (classifies
admits by PV1-2 and reads unzoned timestamps as `America/New_York`). The
executable profile subset is the one the engine compiles: `ADT^A01`
classification rules, PV1 tolerance and assigning-authority mapping. Any other
profile section is reported as `PROFILE_UNSUPPORTED` rather than ignored.

## The kernel's contract

The module registers six JavaScript globals. Each takes and returns strings
(JSON) and never throws: bad input, and even a recovered panic, comes back as
`{"ok": false, "problems": [...]}`.

| Global | Input | Returns |
|---|---|---|
| `fiFhirReady` | - | `true` once the functions are registered |
| `fiFhirVersion()` | - | `{version, commit, builtAt, kernel: "slim"}` |
| `fiFhirProfiles()` | - | the example profiles: `[{id, name, description, yaml}]` |
| `fiFhirSamples()` | - | the demo samples: `[{id, name, source, format, text}]` |
| `fiFhirPreview(json)` | `{message, format: "hl7v2", profileYaml?, timezone?, source?}` | `{ok, segments, events, diagnostics, bundle?, bundleProblem?, problems}` |
| `fiFhirValidateProfile(yaml)` | a Source Profile as YAML or JSON | `{ok, problems}` |

What `fiFhirPreview` returns:

- `segments`: the parser's own tokenization, `{index, id, fields}` per
  segment, with `fields` indexed by HL7 field number (so `fields[9]` of MSH is
  always MSH-9). A failed parse still returns its segments.
- `events`: zero or one `{type, payload}`. `payload` is the canonical event
  JSON the IDE shows.
- `diagnostics`: `{severity, code, path, message}`, the parser's warnings and
  errors (for example `INVALID_SSN_AREA` at `PID.3[1]`).
- `bundle`: a FHIR R4 `transaction` Bundle whose entries are conditional `PUT`
  requests with deterministic `urn:uuid:` full URLs, so the same message
  always yields the same Bundle. When the event has no FHIR projection,
  `bundleProblem` says so (`FHIR_PROJECTION_UNSUPPORTED`) or explains why the
  projection failed (`FHIR_PROJECTION_FAILED`).
- `problems`: why the request could not run, for example `MESSAGE_TOO_LARGE`,
  `PROFILE_INVALID` with the offending path, `TIMEZONE_CONFLICT`, or
  `PARSE_FAILED`. `ok` is false exactly when `problems` is non-empty.

Limits: a message and a profile are at most 1 MiB each (the server's preview
limit), the request at most 3 MiB, at most 200 diagnostics and 1000 segments
(each with an `info` diagnostic saying how many were dropped), and one event
per message.

The full contract, every problem code, example payloads, the size budget that
CI enforces (2.5 MiB compressed) and the release flow are in the kernel's
[README](https://github.com/crb2nu/fi-fhir/blob/main/cmd/fi-fhir-wasm/README.md).

## Which version is running

`fiFhirVersion()` names the fi-fhir release (or commit) the module was built
from. The site takes the module from fi-fhir's GitHub releases
(`fi_fhir_<version>_wasm.tar.gz`, checked against `checksums.txt` and
smoke-tested before it is installed). Until the first tagged release, the site
ships a module built from a fi-fhir commit and records that commit.

## Hosted demo

A hosted demo of the Mapping Studio itself will be at
**<https://fi-fhir-demo.flexinfer.ai>** once it is live. It runs the real IDE
and API with a preview-only identity and no database: HL7 Preview works, and
every surface that needs an operator role or durable storage shows its "not
available on this deployment" state instead of failing. It is a public
deployment, so the same rule applies: synthetic messages only.

## See Also

- [Playground Tutorial](playground-tutorial.md): the three tools, step by step
- [Mapping Studio (IDE)](ide.md): the full IDE
- [Source Profiles](source-profiles.md): what a profile configures
- [FHIR Output](fhir-output.md): how events become FHIR resources
