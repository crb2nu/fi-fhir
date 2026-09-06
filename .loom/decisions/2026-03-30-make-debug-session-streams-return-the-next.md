### 2026-03-30: Make Debug Session Streams Return the Next Pause, Not the Current One

- Decision:
  - Drain stale paused-step notifications before servicing `debugStep`/`debugContinue`, and short-circuit all future debug spans once a session is marked stopped.
- Rationale:
  - Starting sessions in default stepping mode leaves the current paused step buffered; without draining it, the control mutation can return the already-visible pause while subscriptions emit the newly reached pause, splitting the API contract.
  - Stopped sessions may still enter later spans while the workflow unwinds, so the tracer must refuse to pause again or close can hang indefinitely.
- Alternatives considered:
  - Adjust only the subscription test expectations (rejected; would preserve inconsistent runtime behavior).
  - Remove buffered step delivery entirely (rejected; existing direct debug-session tests and synchronous stepping behavior still rely on it).
- Consequences:
  - `debugStep`, `debugContinue`, and `debugStepEvent` now agree on the same "advance to next pause" semantics.
  - Session shutdown is robust even if the engine continues traversing spans after a stop command.
- Sources:
  - [S1] `internal/workflow/debug.go`
  - [S2] `internal/api/graphql/resolvers/debug_subscription_test.go`
  - [S3] `internal/workflow/debug_test.go`
