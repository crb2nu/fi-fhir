### 2026-09-26 - R-D: browser smoke gate

- What changed:
  - **`test:ui-e2e`** (`ci/test-ui-e2e.yml`, blocking, one job): Playwright
    1.63.0 (Chromium) against the **built** UI (session engine flag on),
    served by `ui/nginx/default.conf.template` through Debian's nginx, in
    front of three real `fi-fhir serve` processes on the PostgreSQL 16
    service, reached from the trusted network. `needs: test:binary`; image
    `node:22-bookworm` through the Harbor cache; the browser is downloaded by
    `playwright install --with-deps chromium` and cached per version.
  - **Three stacks, one Playwright project each**, each differing from the
    first in one variable: `operator-bundle` (checks 1–5),
    `missing-operator-role` (negative control 6a), `sessions-off` (negative
    control 6b). Specs in `ui/e2e/`, outside vitest's include, type-checked by
    `ui/e2e/tsconfig.json`.
  - **`ui/e2e/check-report.mjs`** — existence guard: every check and both
    negative controls must have run and passed in their own project.
  - **`make ui-e2e`** runs `ui/e2e/ci.sh` (the job's whole script) in the
    job's image on docker context 7900xtx, PostgreSQL sharing its network
    namespace (the k8s-executor pod topology). Docs: `ui/docs/DEVELOPER-GUIDE.md`
    "Browser smoke gate".
- Why: `.loom/36` R-D. Every repair in R-A/R-B/R-C is a state the browser
  shows; nothing asserted it end to end, and the live deployment's operator
  page had been forbidden for three weeks behind a green pipeline.
- Evidence:
  - Local mirror (`make ui-e2e`): 7/7 passed, existence guard green.
  - Trusted CIDR: nginx access log shows `$remote_addr=127.0.0.1` and a
    forwarded `X-Forwarded-For` of `127.0.0.1` on every request; the API reads
    X-Real-IP (unset), then the first forwarded hop → `127.0.0.1/32`.
  - Red runs (scratch copies, repo untouched), each red for its reason:
    bundle stack with production's pre-R-0 roles → checks 1 and 2 fail (the
    pre-flight renders); bundle stack with sessions unset → checks 1 and 3
    fail (no stream within 10 s); 6a asserting the wrong missing role → red
    (`Received: "integration.operator"`); sessions-off spec renamed away →
    every remaining test green, guard red (`"6b. …" did not run and pass`).
- What's next: coordinator review; merges last, after R-C.
- Findings for other lanes (not fixed here, out of R-D's files):
  - HL7 intake Preview sends the editor text as-is; CodeMirror joins lines
    with LF, and `previewIntegrationMessage` against the `adt-east` preview
    registry fails LF-separated input ("integration preview failed"). The
    page's built-in default sample fails too: its separators are the literal
    characters `\r` (`hl7PreviewStore.ts` `defaultSample`). The gate presses
    "Normalize newlines" before Preview, as a user must today.
  - Over SSE, a non-allowlisted root answers 200 `text/event-stream` with
    `"GraphQL operation forbidden"`; R-B's `classifyStreamError` matches
    `/stream operation forbidden/i`, so a stale-capability client would not
    classify it as not-allowlisted. Unreachable while capabilities are known.
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md` R-D and Corrections (2026-09-26)
  - [S2] `internal/api/requestsecurity/trusted_network.go` `trustedClientAddress`
  - [S3] `ui/src/lib/features/integration-session/api.ts` `runStreamingSessionPreview`
