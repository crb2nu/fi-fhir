### 2026-09-26 - R-E: HL7 intake newline normalization

- What changed:
  - **Send boundary.** `parseHL7Preview` (both the stateless
    `previewIntegrationMessage` path and the Integration Session sample) and
    `submitHL7Message` pass the editor text through the existing
    `normalizeHL7Newlines`, so the API always receives CR-terminated segments.
    The editor buffer is left as typed; "Normalize newlines" stays as an
    explicit action and no send path depends on it.
  - **Built-in sample.** `defaultSample` had three defects, not one: literal
    `\r` separators (backslash + `r`), an MSH-2 of `^~\\&` (two backslashes),
    and no EVN. It is now a CR-joined MSH, EVN, PID, PV1 in the executable
    A01 subset. `samples/demoSamples.ts` was already correct (template
    literals, one backslash, real LF — now normalized at send).
  - **Parser.** No change. `parseRaw` (`parser.go:280-283`) and
    `LiveParser.ParseStream` (`live_parse.go:36-38`) already normalize CRLF
    and LF to CR before splitting and skip empty segments. A new table test
    pins that contract.
- Why:
  - HL7 intake's Preview sent CodeMirror's LF-joined text unchanged, and the
    built-in sample never previewed (`.loom/36`, R-D finding).
  - The LF rejection comes from the preview kernel's **strict** gate, not the
    split: `processor/message_processor.go` parses with
    `StrictValidation: true`, and `validateStrictRawA01` returns
    `errStrictLineEndings` for any `\n` unless the source profile sets
    `line_ending_mode: tolerant` (then it warns `NON_STANDARD_LINE_ENDING`).
    `TestStrictValidationEnforcesClaimedLineEndingMode` pins that rejection,
    so loosening it is a contract change left to the coordinator.
- Evidence:
  - A throwaway processor test (not committed) ran six forms of the sample
    through the strict preview kernel with the executable profile: shipped
    form, CR + `^~\\&`, and CR without EVN all return
    `ErrInvalidSourceMessage`; CR + `^~\&` + EVN previews (1 event); the same
    message with LF or CRLF is rejected — the kernel needs the UI fix.
  - `ui/src/lib/features/hl7/hl7SendBoundary.test.ts` (fake `fetch` only; the
    real GraphQL and SSE clients build each request): 13 tests. Against the
    pre-fix sources 12 fail on the sent `input.data` or the sample's shape;
    the one that passes asserts CR text is sent unchanged (negative control).
  - `TestParseRawIsIdenticalAcrossLineEndings` fails when the two
    `ReplaceAll` lines are removed (negative control), passes as shipped.
  - `npx vitest run`: 792 passed / 3 skipped; `npm run check` 0 errors
    (9 pre-existing warnings in untouched files); `npm run lint` clean.
    `go test ./...` green (no golden or exact-equality output changed — the
    parser is untouched); `gofmt -l` and
    `golangci-lint run ./internal/parser/...` clean.
- What's next:
  - Coordinator decision: should the executable preview kernel accept LF and
    CRLF by default (warning instead of rejection), or keep strict CR unless
    a profile opts into `line_ending_mode: tolerant`? Non-UI clients that
    post LF text hit the same rejection.
  - Three private copies of the same newline normalizer remain
    (`domain/hl7v2.ts`, `domain/hl7Redact.ts`, `domain/hl7Access.ts`); they
    agree today.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` (Lane R-E brief via the
    coordinator)
  - [S2] `internal/parser/hl7v2/strict_validation.go` (`validateStrictRawA01`,
    `validateStrictA01Structure`, `validMSHDelimiterDeclaration`)
  - [S3] `internal/integration/processor/message_processor.go`
    (`StrictValidation: true`)
