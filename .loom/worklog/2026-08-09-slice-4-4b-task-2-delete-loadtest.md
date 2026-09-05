### 2026-08-09 - Slice 4.4b task 2 delete loadtest and repair the shipped ADT config

- What changed:
  - Deleted `fi-fhir workflow loadtest`: the subcommand, its dispatch, its usage
    text, `internal/workflow/loadtest.go` (719 lines), `loadtest_test.go`, and 33
    CLI tests across six files that only exercised it.
  - Repaired `configs/adt-workflow.yaml` to the engine's real schema and added
    `internal/workflow/shipped_config_test.go` to hold it there.
- Why:
  - Per the lane's decision entry (option A, delete not relabel). Nothing outside
    the subcommand's own dispatch and tests referenced any of `loadtest.go`'s 20
    exported symbols, so the delete is clean.
- Evidence — the shipped config was broken three ways, not one:
  - `.loom/33` correction 8 named one defect (the `{{ event.type }}` templating).
    Executing the file found two more, and the templating was the least serious.
  - **`condition:` was at the route's top level, but the engine reads it under
    `filter:`** (`internal/workflow/types.go:19-31`). An unrecognized key is
    silently dropped, leaving a zero-value `Filter`, and a zero-value `Filter`
    matches every event. Measured on 4 events against the old file: **20 route
    matches** — all five routes fired on all four events, including `lab_result`,
    which the file has no route for. After the repair: **3 route matches**,
    0 errors.
  - **`type: emit` is not a registered action type.** The engine registers `log`,
    `webhook`, `fhir`, `email`, `exec`, `file`, `database`, `queue`,
    `event_store`, `athena` (`engine.go:125-134`). Every emit failed with
    `unknown action type: emit`. This also corrects my own earlier reading, which
    attributed the loadtest's 100% error rate to a missing event store — the
    action never existed, so no event store would have helped.
  - The three defects partly masked each other, and the one signal that would
    have caught them — the route-match count — was asserted nowhere.
  - **This is not an unused sample.** `docker-compose.yaml:80` mounts it as
    `FI_FHIR_WORKFLOW_CONFIG_PATH` and `docs/user-guide/getting-started.md:253`
    passes it to `fi-fhir serve --workflow`, so the match-everything routing was
    live in the default development stack.
  - Chose `log` actions over `event_store`: `event_store` requires a `connection`
    DSN (`event_store.go:152-154`), so a config using it cannot run out of the
    box. The header documents the `event_store` upgrade with the real key names.
  - Negative control: `TestShippedADTConfigRoutesWhatItClaims` fails on the
    pre-repair file with
    `matched [patient-admit patient-discharge patient-transfer patient-update adt-other], want [patient-admit]`.
  - `go build ./...`, `go test ./internal/workflow/ ./cmd/fi-fhir/`,
    `golangci-lint run ./cmd/... ./internal/workflow/...` → 0 issues.
- What's next:
  - Correct the causal claim in `.loom/33` correction 8 and in the decision entry
    on MR !167 — both currently say the loadtest's error rate came from a missing
    event store. It came from a nonexistent action type.
  - Tasks 3-7.
- Sources:
  - [S1] `internal/workflow/types.go:19-31` — `Filter` owns `condition`
  - [S2] `internal/workflow/engine.go:125-134` — the registered action types
  - [S3] `docker-compose.yaml:80`; `docs/user-guide/getting-started.md:253`
  - [S4] Executed 2026-08-09: old config 4 events → 20 route matches; repaired
    config 4 events → 3 route matches, 0 errors
