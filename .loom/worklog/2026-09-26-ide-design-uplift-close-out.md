### 2026-09-26 - IDE design uplift close-out

- What changed:
  - The IDE design uplift (`.loom/37-ide-design-uplift-execution-specs.md`,
    MR !232) is delivered: U-0 the design system (!233), U-2 home, events and
    operator (!236), U-1 the shell (!235), U-3 intake, workflows, profiles and
    terminology (!234), and U-4 visual evidence in `test:ui-e2e` (!237).
  - This entry's MR records the program in `ROADMAP.md` (Current Status, the
    "Delivered — IDE design uplift" section, and the Now items it surfaced)
    and writes the single `CHANGELOG.md` block. The Copilot Now item now points
    at the scoped LiteLLM key plan (platform/gitops MR 815).
- Why:
  - Cody's brief after the repair program: the IDE "still looks rough / like a
    toy". Production screenshots at 1440×900 showed the same defects on every
    route: four navigation layers above the content, marketing copy, simulated
    "Demo data" alerts on Home, and decoration where records belonged.
  - Lanes were barred from `ROADMAP.md`, `CHANGELOG.md` and the pointer pages;
    the coordinator writes each once.
- Evidence:
  - Merges on `main`: 6702cbc3e (!233), f03bf334e (!236), fc0ee0dc2 (!235),
    4480af9ab (!234), 38d87a5aa (!237). Every lane MR carries before/after
    PNGs and a coordinator review note; review order was screenshots, then
    diff.
  - U-3's first review found a failed HL7 preview invisible with the session
    engine on (production's configuration) and a lost per-row sample edit;
    both were fixed and tested before merge.
  - Production: main pipeline 29328 deployed UI `v0.1.29328` after one retry
    of `build:docker-ui` (the DinD sidecar was not reachable). Verified from
    the LAN at 1440×900 on `/`, `/hl7` (Preview of the built-in sample on the
    session engine: complete, 1 event, 0 warnings), `/workflows`, `/profiles`,
    `/terminology`, `/events` and `/operator`.
  - U-4's first CI run (job 315082) captured 11 states in `ui/e2e-results/visual/`
    and passed the copy register; peak job memory 1.53 GiB against the 4 GiB
    limit.
  - CI noise during the program: `runner_system_failure` with
    `etcdserver: too many requests` on runner `ci-7900xtx`, an etcd member of
    the k3s cluster; single retries recovered every job.
- What's next:
  - The Now items this program added: operator-plane and terminology-store
    availability in the status contract, the Events browser's legacy store,
    and the IDE polish list.
  - The UI build tag uses `CI_PIPELINE_IID` while the image tag uses
    `CI_PIPELINE_ID`, so Home › Health names a build no image carries.
- Sources:
  - [S1] `.loom/37-ide-design-uplift-execution-specs.md`
  - [S2] `.loom/worklog/2026-09-26-u-0-design-system-and-primitives.md`,
    `.loom/worklog/2026-09-26-u-1-shell.md`,
    `.loom/worklog/2026-09-26-u-2-home-events-operator.md`,
    `.loom/worklog/2026-09-26-u-3-intake-workflows-profiles-terminology.md`,
    `.loom/worklog/2026-09-26-u-4-visual-evidence-in-the-gate.md`
  - [S3] GitLab MRs !232–!237 and main pipeline 29328
