# fi-fhir-wasm: the browser kernel

`cmd/fi-fhir-wasm` compiles fi-fhir's own engine path to WebAssembly so a web
page can run it with nothing sent to a server. The path is the production
Source Profile compiler (`processor.CompileProfileRevision`), the production
HL7v2 parser, and the conditional FHIR R4 transaction Bundle that a `fhir`
destination receives (`internal/integration/fhirout`). The pure-Go core is
`internal/integration/kernel`. This command only registers the JavaScript
globals.

The module has no filesystem and no network access. Whatever a page pastes
into it stays in the page. The only data embedded in it is synthetic: six demo
messages and three example profiles, all from
`internal/integration/kernel/testdata/`.

It is the engine behind the public playground at
`flexinfer.ai/playground/fi-fhir` (flexinfer-site). The user-facing docs are in
`docs/user-guide/playground.md`.

## It is the IDE's engine

The IDE's Integration Session runner
(`internal/integration/session/runner.go`) and the kernel follow the same
steps: compile the profile, then `hl7v2.NewParser` with the profile's
timezone, then `ParseWithResult`, then normalize the diagnostics. When a parse
or a compile fails, both record the same `PARSE_FAILED` diagnostic.
`TestWASMKernelMatchesSessionRunner` runs the six IDE demo samples through
both paths, once with the adt-http golden profile and once without a profile.
It requires byte-identical canonical JSON for `events` and `diagnostics`. The
only fields masked are `id`, `timestamp` and `received_at`: both paths fill
them from `crypto/rand` and the clock, and the test first checks that they are
present and well formed. A negative control changes PID-5 and asserts that
both paths change, and change the same way.

Two more tests stop the embedded data from drifting.
`TestSamplesMatchIDEDemoSamples` requires the embedded samples to stay
byte-identical to `ui/src/lib/features/hl7/samples/demoSamples.ts`.
`TestBuiltInProfilesMatchGoldenJSON` requires the YAML golden profiles to stay
equal to `testdata/golden/integration/adt-http/*.json`.

The browser build leaves out one thing: the workflow planner.
`internal/integration/processor/workflow_plan.go` is built with `//go:build
!js`. In the js build, `workflow_plan_js.go` returns `ErrWorkflowUnavailable`.
The planner's dependency closure (cel-go, protobuf, lib/pq, prometheus, otel,
redis) would take the module from 8 MB to 72 MB.

## Build and smoke

```sh
make wasm          # dist/wasm/fi-fhir.wasm + dist/wasm/wasm_exec.js
make wasm-smoke    # wasm-deps-check + wasm-size-check + node scripts/wasm-smoke.mjs dist/wasm
node scripts/wasm-smoke.mjs <dir>   # the contract smoke alone (Node 20+, no dependencies)
```

`wasm_exec.js` is copied from the Go distribution: `$(go env GOROOT)/lib/wasm/`
since Go 1.24, `misc/wasm/` before that. Always serve the shim from the same
build as the module.

To load it in a page (flexinfer-site runs it in a Web Worker):

```js
importScripts('/wasm/wasm_exec.js');            // or a <script> tag
const go = new Go();
const { instance } = await WebAssembly.instantiateStreaming(fetch('/wasm/fi-fhir.wasm'), go.importObject);
go.run(instance);                               // never resolves: main blocks
while (globalThis.fiFhirReady !== true) await new Promise((r) => setTimeout(r, 10));
const result = JSON.parse(fiFhirPreview(JSON.stringify({ message, format: 'hl7v2' })));
```

The page's Content-Security-Policy needs `'wasm-unsafe-eval'`.

## Size budget

`test:wasm` (`ci/test-wasm.yml`) fails when `gzip -9 -c dist/wasm/fi-fhir.wasm
| wc -c` is over **2,621,440 bytes (2.5 MiB)**. It also fails when a
server-only package (cel-go, lib/pq, prometheus, otel, redis,
`internal/workflow`) gets back into the js dependency closure. The closure
includes `time/tzdata`, because the profile compiler loads IANA zones and wasm
has no system zoneinfo. The embedded zone data counts toward the budget.

| build | raw bytes | gzip -9 bytes |
|---|---|---|
| Go 1.26.6, `-trimpath -ldflags "-s -w"`, first build of this command | 8,258,603 | 2,242,136 |
| `wasm_exec.js` | 16,992 | - |

## Contract

Every global takes strings and returns a string (JSON). None of them throws.
Bad input comes back as `{"ok": false, "problems": [{code, path, message}]}`,
and so does a panic, which is recovered as `INTERNAL_ERROR`. Arrays are always
arrays, never `null`.

| global | input | output |
|---|---|---|
| `fiFhirReady` | - | `true` once `main` has registered the functions |
| `fiFhirVersion()` | - | `{version, commit, builtAt, kernel: "slim"}` |
| `fiFhirProfiles()` | - | `[{id, name, description, yaml}]` |
| `fiFhirSamples()` | - | `[{id, name, source, format, text}]` |
| `fiFhirPreview(json)` | `{message, format: "hl7v2", profileYaml?, timezone?, source?}` | `{ok, segments, events, diagnostics, bundle?, bundleProblem?, problems}` |
| `fiFhirValidateProfile(yaml)` | profile YAML (or JSON) | `{ok, problems}` |

### Limits

- `message` and a profile: at most 1 MiB each
  (`processor.MaxPreviewSourceBytes`, the server-side preview limit). The JSON
  request envelope: at most 3 MiB.
- `diagnostics`: at most 200. Past that, one `info` diagnostic with code
  `DIAGNOSTICS_TRUNCATED` gives the number dropped.
- `segments`: at most 1000. Past that, one `info` diagnostic with code
  `SEGMENTS_TRUNCATED`.
- `events`: zero or one. A message is one event.

### `fiFhirVersion()`

```json
{"version": "0.9.0", "commit": "1f6ac119821a4214f9ab0dcd0a69438805a5b3ec", "builtAt": "2026-09-27T19:46:03Z", "kernel": "slim"}
```

`version` is the tag without its `v` (a release), `git describe` (a local
build), or `dev`. `builtAt` is the commit time, which keeps builds
reproducible. `kernel: "slim"` means the workflow planner is not linked.

### `fiFhirProfiles()`

```json
[{"id": "adt-http-strict", "name": "ADT A01 over HTTP (strict)",
  "description": "The golden adt-http profile: HL7 2.5.1, UTC, ...",
  "yaml": "hl7v2:\n  default_version: \"2.5.1\"\n  timezone: UTC\n  ..."}]
```

There are three: `adt-http-strict`, `adt-http-tolerant` and
`adt-patient-class`. The executable profile subset is the one the processor
compiles: HL7v2 `ADT^A01` classification rules, PV1 tolerance, and
assigning-authority mapping. Anything else is `PROFILE_UNSUPPORTED`.

### `fiFhirSamples()`

```json
[{"id": "siu-s12-appointment-scheduled", "name": "SIU S12 - Appointment Scheduled",
  "source": "demo_nextgen", "format": "hl7v2", "text": "MSH|^~\\&|NEXTGEN|CLINIC|..."}]
```

These are the IDE's six demo samples, in the IDE's order. `source` is the
event source the IDE uses for the sample. Pass it to `fiFhirPreview` to get
the IDE's exact event.

### `fiFhirPreview(json)`

Request:

```json
{"message": "MSH|^~\\&|EPIC|HOSPITAL|...", "format": "hl7v2", "source": "demo_epic_adt"}
```

- `format` must be `"hl7v2"`. Unknown members are refused (`INPUT_INVALID`).
- `profileYaml` is optional. It takes a Source Profile as YAML or JSON. It
  compiles under the artifact id `playground-profile`, which becomes the
  event's `source_profile_id`. Without it, the parser uses its defaults, the
  same as a session run with no profile draft.
- `timezone` is optional. It is an IANA zone for unzoned HL7 timestamps and
  applies only when there is no profile. If you pass a profile and a different
  `timezone`, the result is `TIMEZONE_CONFLICT`. `"Local"` is refused.
- `source` is optional and defaults to `"playground"`.

Response (ADT A01 ICU admission, shortened):

```json
{
  "ok": true,
  "segments": [
    {"index": 0, "id": "MSH", "fields": ["MSH", "|", "^~\\&", "EPIC", "HOSPITAL", "...", "ADT^A01^ADT_A01", "MSG00001", "P", "2.5.1"]},
    {"index": 2, "id": "PID", "fields": ["PID", "1", "", "MRN123456^^^HOSPITAL^MR~999-88-7777^^^SSA^SS", "", "DOE^JANE^MARIE^^MS"]}
  ],
  "events": [
    {"type": "patient_admit",
     "payload": {"id": "…uuid…", "type": "patient_admit", "timestamp": "…", "received_at": "…",
                 "source": "demo_epic_adt", "source_format": "hl7v2", "source_message_id": "MSG00001",
                 "parse_warnings": [ … ], "patient": {"mrn": "MRN123456", "family_name": "DOE", … }, "encounter": { … }}}
  ],
  "diagnostics": [
    {"severity": "warning", "code": "INVALID_SSN_AREA", "path": "PID.3[1]", "message": "SSN area number cannot start with 9"}
  ],
  "bundle": {
    "resourceType": "Bundle", "type": "transaction",
    "entry": [
      {"fullUrl": "urn:uuid:c4732856-702a-579f-8033-0b997ffd2883",
       "resource": {"resourceType": "Patient", "identifier": [ … ], "name": [{"family": "DOE", "given": ["JANE", "MARIE"]}], … },
       "request": {"method": "PUT", "url": "Patient?identifier=urn%3Afi-fhir%3Asource%3Ademo_epic_adt%7CMRN123456"}}
    ]
  },
  "problems": []
}
```

- `segments[i].fields` is indexed by HL7 field number. `fields[0]` is the
  segment ID. For MSH, `fields[1]` is the field separator and `fields[2]` is
  the encoding characters, so `fields[9]` is always MSH-9. This is the
  parser's own tokenization (`hl7v2.SplitMessage`). A failed parse still
  returns segments.
- `events[0].payload` is the canonical event JSON the IDE shows.
- `bundle` is present when the event type has a FHIR projection (admit,
  transfer, update, discharge, lab result, and the clinical families). It is
  built the way the durable engine builds it: `integration.NewProcessedEvent`
  produces the raw-free payload that gets stored, then `fhirout.Project` and
  `CreateConditionalTransactionBundle` run on it. Every entry is a conditional
  `PUT` with a deterministic `urn:uuid:` fullUrl, so the same message always
  gives the same Bundle.
- When there is an event but no Bundle, `bundleProblem` says why:

  ```json
  {"code": "FHIR_PROJECTION_UNSUPPORTED", "path": "events[0].type", "message": "\"appointment_scheduled\" events have no FHIR projection"}
  ```

  `FHIR_PROJECTION_FAILED` means a projectable event could not become a
  Bundle, for example because a resource has no usable identifier.

A failed parse (the IDE reports the same thing for this sample):

```json
{"ok": false, "segments": [ … 8 segments … ], "events": [],
 "diagnostics": [{"severity": "error", "code": "PARSE_FAILED", "path": "", "message": "unsupported message type: ORM^O01^ORM_O01"}],
 "problems": [{"code": "PARSE_FAILED", "path": "message", "message": "unsupported message type: ORM^O01^ORM_O01"}]}
```

### `fiFhirValidateProfile(yaml)`

```json
{"ok": true, "problems": []}
{"ok": false, "problems": [{"code": "PROFILE_INVALID", "path": "hl7v2.timezone", "message": "invalid source profile: hl7v2.timezone"}]}
```

Paths here are relative to the profile. Inside `fiFhirPreview` the same
problem is reported at `profileYaml.hl7v2.timezone`.

### Problem codes

| code | path | meaning |
|---|---|---|
| `INPUT_TOO_LARGE` | `$` | request envelope over 3 MiB |
| `INPUT_INVALID` | `$` | not one JSON object, an unknown member, or no string argument |
| `MESSAGE_EMPTY` / `MESSAGE_TOO_LARGE` | `message` | empty, or over 1 MiB |
| `FORMAT_UNSUPPORTED` | `format` | anything but `hl7v2` |
| `PROFILE_EMPTY` / `PROFILE_TOO_LARGE` | `profileYaml` / `$` | blank, or over 1 MiB |
| `PROFILE_SYNTAX` | `profileYaml` / `$` | not one YAML document of JSON values |
| `PROFILE_INVALID` | the offending member | the compiler rejected it (`processor.ErrInvalidSourceProfile`) |
| `PROFILE_UNSUPPORTED` | the offending member | valid, but outside the executable v1 subset |
| `TIMEZONE_INVALID` / `TIMEZONE_CONFLICT` | `timezone` | not an IANA zone, or differs from the profile's |
| `PARSE_FAILED` | `message` | the parser refused the message |
| `INTERNAL_ERROR` | `$` | a recovered panic (a bug, so please report it) |

## Release flow

1. Merge to `main` on GitLab (canonical), then cut a `vX.Y.Z` tag there.
2. The next `main` pipeline's `mirror:github` job runs `git push --tags` to
   `github.com/crb2nu/fi-fhir`.
3. On the mirror, `.github/workflows/wasm-release.yml` runs on the `v*` tag. It
   runs `make wasm-smoke` (closure, size budget, contract), packages
   `fi_fhir_<version>_wasm.tar.gz` (`fi-fhir.wasm`, `wasm_exec.js`,
   `wasm-smoke.mjs`, `LICENSE`, `samples/*.hl7`, `profiles/*.yaml`, with
   reproducible tar and gzip), and writes `checksums.txt` (`sha256sum`
   format). It then extracts the archive, runs the archive's own
   `wasm-smoke.mjs` on the extracted files, and publishes a GitHub Release
   with both assets.
4. flexinfer-site's `scripts/fetch-fi-fhir-release.mjs` resolves the newest
   release, verifies the SHA-256 against `checksums.txt`, runs
   `node wasm-smoke.mjs <extracted dir>` from the archive, and installs the
   module into `public/wasm/`.

Test the workflow locally with `actionlint`. GitHub Actions cannot be run from
GitLab.
