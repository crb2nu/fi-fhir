### 2026-09-26 - IDE repair program close-out

- What changed:
  - The IDE repair program (`.loom/36-ide-repair-execution-specs.md`, MRs !223
    and !224) is delivered: R-A the auth capabilities contract (!225), R-B
    honest surfaces (!226), R-C streaming on the repo side (!228), R-D the
    Playwright browser smoke gate (!227), and the two platform/gitops changes
    the coordinator owned — the operator bundle for every IDE identity
    (MR 805) and the Integration Session workspace on in production (MR 811).
  - This entry's MR flips `ROADMAP.md` to record the program as delivered,
    writes the single `CHANGELOG.md` block, records the decision "Streaming
    flips before the UI image; the stream allowlist stays narrow", and closes
    Lane R-D's banked gap: `ci/test-ui-e2e.yml` runs when its own definition
    changes (the file is added to the runtime-verification anchor so
    `test:binary`, which it needs, runs with it).
- Why:
  - Cody's brief: the IDE was "busted", backend connections failed, and the
    trusted network did not carry operator permissions. Every symptom was
    measured against the live deployment before a lane was launched; none was
    in the trusted-network path itself. The operator plane was refused for
    every identity because the deployment granted the transport grant and not
    the service roles; streaming was off; the Copilot waited for a platform
    endpoint that was never set; the Problems badge counted an empty draft.
  - Lanes were barred from `ROADMAP.md`, `CHANGELOG.md`, `.loom/30`, the
    pointer pages and `platform/gitops`; the coordinator writes each once.
- Evidence:
  - Merges on `main`: c5038eee6 (!223 + !224), 887454859 (!225, pipeline
    29141), 164934af3 (!226, pipeline 29150 after one `build:docker-ui`
    BuildKit retry), 39e1fd73c (!228, pipeline 29183), 3c0a19171 (!227,
    pipeline 29180; `test:ui-e2e` job 312356 passed first time, 7/7 in 249 s).
  - platform/gitops: 972ed5ab4 (MR 805, carrying MR 806's restore-drill
    extension), 4ccc2214a (MR 811). Live after 811: pod
    `fi-fhir-api-c97bfcf46-brhs2` logged `integration session workspace
    configured` (postgres) and `integration session durable stream fanout
    enabled`; from the LAN `POST /graphql` with `Accept: text/event-stream`
    for `integrationSessionEvents` answered `200 text/event-stream`, and
    `eventStream` answered one `next` event `{"errors":[{"message":"GraphQL
    operation forbidden","extensions":{"code":"FORBIDDEN"}}]}` — the
    sanitized text R-C's classifier now keys on.
  - Lane R-C's first run stopped on its stop rule with the finding that
    changed the program (the UI flag had no fallback; the allowlist admits two
    roots); the corrections are in `.loom/36` and R-B absorbed them.
  - Lane R-D found that HL7 intake sends LF-separated editor text the preview
    kernel's strict validation rejects, and that the default sample's
    separators are the literal characters `\r`. Lane R-E (MR !229) normalizes
    at every send boundary, repairs the sample (three defects, not one), and
    pins the parser's already-tolerant split; the strict gate is unchanged
    because loosening it would invert an exact test that pins the claimed
    line-ending mode.
- What's next:
  - Re-probe `/api/auth/status` when a session mutation is refused with
    "legacy integration execution is unavailable" and after a bearer is
    entered, so a tab does not keep stale capabilities.
  - Expose operator-plane availability (`FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED`)
    as a capability, so the IDE can distinguish "not configured" from
    "forbidden".
- Sources:
  - [S1] `.loom/36-ide-repair-execution-specs.md`
  - [S2] `.loom/worklog/2026-09-25-ide-repair-r-a-the-auth-capabilities.md`,
    `.loom/worklog/2026-09-25-ide-repair-r-b-honest-surfaces.md`,
    `.loom/worklog/2026-09-25-r-c-streaming-on-repo-side.md`,
    `.loom/worklog/2026-09-26-r-d-browser-smoke-gate.md`
  - [S3] `.loom/decisions/2026-09-25-grant-the-operator-bundle-rather-than-alias.md`,
    `.loom/decisions/2026-09-25-the-copilot-runs-on-the-backend-llm.md`,
    `.loom/decisions/2026-09-26-streaming-flips-before-the-ui-image-the.md`
  - [S4] GitLab MRs !223–!228 and platform/gitops MRs 805, 806, 811; the
    pipeline and job ids above
