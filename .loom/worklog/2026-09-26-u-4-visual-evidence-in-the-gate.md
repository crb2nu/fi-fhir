### 2026-09-26 - U-4 visual evidence in the gate

- What changed:
  - **`visual` Playwright project** (`ui/e2e/visual.spec.ts`), fourth project
    of `test:ui-e2e`, on the operator-bundle stack, listed last so it runs
    after the functional projects (one worker). Eleven captures at 1440×900,
    dark: `/` (default, bottom panel on Problems, command palette), `/hl7`
    (idle, and after Preview of the built-in sample on the session engine),
    `/workflows` (Inventory, Design), `/profiles`, `/terminology` (Browse),
    `/events` (Browse), `/operator` (Messages), written to
    `ui/e2e-results/visual/<route>-<state>.png`. Each waits on the state's
    own anchor, then on no `aria-busy="true"`, no visible "Loading…" and every
    data request answered — no fixed sleep. No pixel diffing, no golden PNGs.
  - **Copy register asserted** on `document.body.innerText` of every capture:
    `FORBIDDEN_COPY` in `ui/e2e/support.ts` (the six `.loom/37` phrases), the
    one place to extend it.
  - `check-report.mjs` requires V1..V11 to run and pass; the job's artifacts
    name `ui/e2e-results/visual/` explicitly (2 weeks, `when: always`);
    `ui/e2e/docker.sh` takes `UI_E2E_NAME` so two worktrees can run
    `make ui-e2e` on one docker host.
  - Docs: `ui/docs/DESIGN.md` "How to review a UI MR" (open the job's
    `visual/` artifact, compare with main's, file table);
    `DEVELOPER-GUIDE.md` project table; Makefile usage line.
- Why: `.loom/37` lane U-4 (decision 4: evidence over pixel tests).
- Evidence:
  - Local `make ui-e2e` (7900xtx): 18 passed across 4 projects, existence
    guard green, visual project ~4 s.
  - Memory, job container cgroup (1 s sampler + `memory.peak`): whole-job
    peak 2.73 GiB, set by the Vite build; the Playwright phase with all four
    projects peaked at 1.37 GiB (anon 0.42 GiB). The visual project does not
    move the job's peak; the 4 GiB limit stands.
- What's next: coordinator review of the MR; later lanes add routes to
  `CAPTURES` and phrases to `FORBIDDEN_COPY`.
- Findings:
  - The shell's `/health` poll (`connectionStore`, body never read) never
    reported `requestfinished` in Chromium, so "settled" counts a request
    answered at its response, not at `requestfinished`.
  - `/workflows` opens on Design, not Inventory.
  - On the e2e stack `/terminology` Browse shows "Mappings could not be
    loaded: GraphQL request failed" plus an error toast: the API has no
    mapping store (`terminology mapping store not configured`) and the UI
    shows only the generic message.
- Sources:
  - [S1] `.loom/37-ide-design-uplift-execution-specs.md` › U-4
