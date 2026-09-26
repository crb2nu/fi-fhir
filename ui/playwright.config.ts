/**
 * Browser smoke gate (IDE repair Lane R-D) — Playwright configuration.
 *
 * Three projects, one per API + nginx stack that `e2e/run.sh` starts. Each
 * stack differs from the first in exactly one variable, so each project
 * proves one thing the IDE must get right:
 *
 *   operator-bundle        the repaired deployment: full operator bundle, streaming on
 *   missing-operator-role  negative control: the bundle minus integration.operator
 *   sessions-off           negative control: FI_FHIR_INTEGRATION_SESSION_ENABLED unset
 *
 * A fourth project, `visual` (`.loom/37` U-4), reuses the operator-bundle
 * stack: it writes a 1440×900 PNG of every route to e2e-results/visual/ for
 * review and asserts the copy register. It is listed last so that, with one
 * worker, it runs after the functional projects.
 *
 * Specs live in e2e/, outside vitest's `src/**` include and outside the
 * SvelteKit tsconfig; e2e/tsconfig.json type-checks them. Run through
 * `make ui-e2e` (or ui/e2e/run.sh with the stacks' prerequisites) — the
 * projects need the stacks, so a bare `npx playwright test` fails fast.
 */
import { defineConfig, devices } from '@playwright/test';

const resultsDir = process.env.E2E_RESULTS_DIR ?? 'e2e-results';

export default defineConfig({
  testDir: './e2e',
  outputDir: `${resultsDir}/artifacts`,
  forbidOnly: true,
  // A gate that retries is a gate that hides the flake it should report.
  retries: 0,
  // Every project has its own stack, but the job's pod has two CPUs shared by
  // three APIs, nginx, and Chromium; one worker keeps timings honest.
  workers: 1,
  timeout: 60_000,
  expect: { timeout: 10_000 },
  reporter: [
    ['list'],
    ['json', { outputFile: `${resultsDir}/report.json` }],
    ['junit', { outputFile: `${resultsDir}/junit.xml` }],
    ['html', { outputFolder: `${resultsDir}/html`, open: 'never' }]
  ],
  use: {
    ...devices['Desktop Chrome'],
    screenshot: 'only-on-failure',
    trace: 'retain-on-failure',
    video: 'off'
  },
  projects: [
    {
      name: 'operator-bundle',
      testMatch: 'operator-bundle.spec.ts',
      use: { baseURL: process.env.E2E_BUNDLE_URL ?? 'http://127.0.0.1:3000' }
    },
    {
      name: 'missing-operator-role',
      testMatch: 'missing-operator-role.spec.ts',
      use: { baseURL: process.env.E2E_NO_OPERATOR_URL ?? 'http://127.0.0.1:3001' }
    },
    {
      name: 'sessions-off',
      testMatch: 'sessions-off.spec.ts',
      use: { baseURL: process.env.E2E_SESSIONS_OFF_URL ?? 'http://127.0.0.1:3002' }
    },
    {
      name: 'visual',
      testMatch: 'visual.spec.ts',
      use: {
        baseURL: process.env.E2E_BUNDLE_URL ?? 'http://127.0.0.1:3000',
        viewport: { width: 1440, height: 900 },
        colorScheme: 'dark'
      }
    }
  ]
});
