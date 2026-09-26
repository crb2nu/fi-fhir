/**
 * Visual evidence (`.loom/37` U-4), against stack `operator-bundle`: a
 * 1440×900 PNG of every route, of the bottom panel open on Problems and of the
 * command palette, written to `$E2E_RESULTS_DIR/visual/<route>-<state>.png`,
 * and the copy register asserted on every captured state.
 *
 * The PNGs are review artifacts, never golden files: nothing here diffs
 * pixels, and nothing under `visual/` is committed (`.loom/37` decision 4).
 * `ui/docs/DESIGN.md` "How to review a UI MR" says how to read them.
 *
 * A capture is taken only after the page's data has settled: the state's own
 * anchor — an element that exists only once its data arrived — is visible,
 * nothing is busy (`aria-busy="true"`, a visible "Loading…"), and every data
 * request has been answered. Never a fixed delay.
 */
import path from 'node:path';
import { expect, test, type Locator, type Page, type Request } from '@playwright/test';
import { FORBIDDEN_COPY, hl7PreviewButton, openIDE } from './support';

const VISUAL_DIR = path.resolve(process.env.E2E_RESULTS_DIR ?? 'e2e-results', 'visual');

interface Capture {
  /** Leading check id; `check-report.mjs` requires each one to run and pass. */
  id: string;
  /** `<route>-<state>`: the PNG is `visual/<name>.png`. */
  name: string;
  /** Brings a fresh page to the state; resolves once the state's anchor is visible. */
  reach: (page: Page) => Promise<void>;
}

/** Waits for one of several mutually exclusive terminal states of a region. */
async function expectOneOf(...candidates: [Locator, ...Locator[]]): Promise<void> {
  const [first, ...rest] = candidates;
  await expect(rest.reduce((any, candidate) => any.or(candidate), first)).toBeVisible();
}

async function home(page: Page): Promise<void> {
  await openIDE(page, '/');
  await expect(page.getByTestId('health-summary')).toHaveText(/Healthy|Degraded/);
  const integrations = page.getByTestId('integrations-panel');
  await expectOneOf(
    integrations.getByText('No integration is deployed.'),
    integrations.getByRole('table', { name: 'Integration deployments', exact: true }),
    integrations.getByRole('button', { name: 'Retry' })
  );
  await expect(page.getByTestId('alerts-panel')).toBeVisible();
}

async function hl7Idle(page: Page): Promise<void> {
  await openIDE(page, '/hl7');
  // The built-in sample is in the editor and nothing has run yet.
  await expect(page.getByTestId('code-editor').locator('.cm-content')).toContainText('MSH|');
  await expect(page.getByText('Preview the message to list parse warnings by phase.')).toBeVisible();
}

const CAPTURES: Capture[] = [
  { id: 'V1', name: 'home-default', reach: home },
  { id: 'V2', name: 'hl7-idle', reach: hl7Idle },
  {
    id: 'V3',
    name: 'hl7-preview',
    reach: async (page) => {
      await hl7Idle(page);
      // With sessions on, Preview of the built-in sample runs on the session
      // engine; `complete` is the stream having stayed open for the whole run.
      await hl7PreviewButton(page).click();
      const progress = page.getByRole('region', { name: 'Server preview progression' });
      await expect(progress).toHaveClass(/state-complete/);
      await expect(page.getByText('Preview the message to list parse warnings by phase.')).toHaveCount(0);
    }
  },
  {
    id: 'V4',
    name: 'workflows-inventory',
    reach: async (page) => {
      await openIDE(page, '/workflows');
      await page.getByRole('tab', { name: 'Inventory', exact: true }).click();
      const view = page.getByRole('tabpanel', { name: 'Inventory' });
      await expect(view.getByRole('button', { name: 'Refresh', exact: true })).toBeEnabled();
      await expectOneOf(
        view.getByText('No managed workflows. Create a definition in Design.'),
        view.getByRole('table', { name: 'Managed workflows', exact: true }),
        view.getByText(/^Failed to load workflows/)
      );
    }
  },
  {
    id: 'V5',
    name: 'workflows-design',
    reach: async (page) => {
      // Design is the route's default tab; the click keeps the capture
      // independent of that default.
      await openIDE(page, '/workflows');
      await page.getByRole('tab', { name: 'Design', exact: true }).click();
      await expect(page.getByRole('tabpanel', { name: 'Design' })).toBeVisible();
      await expect(page.getByRole('complementary', { name: 'Managed version' })).toBeVisible();
    }
  },
  {
    id: 'V6',
    name: 'profiles-default',
    reach: async (page) => {
      await openIDE(page, '/profiles');
      const list = page.getByRole('region', { name: 'Source profiles' });
      await expect(list.getByText(/^\d+ profiles?$/)).toBeVisible();
    }
  },
  {
    id: 'V7',
    name: 'terminology-browse',
    reach: async (page) => {
      await openIDE(page, '/terminology');
      const view = page.getByRole('tabpanel', { name: 'Browse' });
      await expectOneOf(
        view.getByText(/^No mappings (yet|match)/),
        view.getByRole('table', { name: 'Mappings', exact: true }),
        view.getByText(/^Mappings could not be loaded/)
      );
    }
  },
  {
    id: 'V8',
    name: 'events-browse',
    reach: async (page) => {
      await openIDE(page, '/events');
      const view = page.getByRole('tabpanel', { name: 'Browse' });
      await expectOneOf(
        view.getByText('No events match these filters.'),
        view.getByRole('table', { name: 'Events', exact: true }),
        view.getByText(/^Events could not be loaded/)
      );
    }
  },
  {
    id: 'V9',
    name: 'operator-messages',
    reach: async (page) => {
      await openIDE(page, '/operator');
      await expectOneOf(
        page.getByText('No messages match these filters'),
        page.getByRole('table', { name: 'Durable admission receipts', exact: true }),
        page.getByTestId('operator-preflight')
      );
    }
  },
  {
    id: 'V10',
    name: 'home-panel-problems',
    reach: async (page) => {
      await home(page);
      const tab = page.getByRole('tab', { name: /^Problems/ });
      await tab.click();
      await expect(tab).toHaveAttribute('aria-selected', 'true');
      await expect(page.locator('.problems-panel')).toBeVisible();
    }
  },
  {
    id: 'V11',
    name: 'home-command-palette',
    reach: async (page) => {
      await home(page);
      await page.getByRole('button', { name: 'Open commands' }).click();
      const palette = page.getByRole('dialog', { name: 'Commands', exact: true });
      await expect(palette).toBeVisible();
      await expect(palette.getByRole('textbox', { name: 'Search commands' })).toBeFocused();
    }
  }
];

/**
 * Lists the page's data requests not yet answered (method, URL, GraphQL
 * operation). A request is answered when its response arrives or it fails —
 * not on `requestfinished`, which the shell's `/health` poll (its body is never
 * read) did not reach in Chromium. SSE streams stay open by design and are
 * excluded.
 */
function inflightDataRequests(page: Page): () => string[] {
  const inflight = new Map<Request, string>();
  const isData = (request: Request) =>
    (request.resourceType() === 'fetch' || request.resourceType() === 'xhr') &&
    !(request.headers()['accept'] ?? '').includes('text/event-stream');
  page.on('request', (request) => {
    if (!isData(request)) return;
    const operation = /\b(query|mutation)\s+(\w+)/.exec(request.postData() ?? '')?.[2];
    inflight.set(request, `${request.method()} ${request.url()}${operation ? ` ${operation}` : ''}`);
  });
  page.on('response', (response) => inflight.delete(response.request()));
  page.on('requestfailed', (request) => inflight.delete(request));
  return () => [...inflight.values()];
}

async function expectSettled(page: Page, inflight: () => string[]): Promise<void> {
  await expect(page.locator('[aria-busy="true"]'), 'nothing is busy').toHaveCount(0);
  await expect(page.getByText(/^Loading\b/).filter({ visible: true }), 'nothing is loading').toHaveCount(0);
  await expect.poll(inflight, { message: 'every data request was answered' }).toEqual([]);
}

for (const capture of CAPTURES) {
  test(`${capture.id}. ${capture.name}.png: captured once settled, copy register clean`, async ({ page }) => {
    const inflight = inflightDataRequests(page);
    await capture.reach(page);
    await expectSettled(page, inflight);

    // Evidence first, so a copy-register failure still leaves the PNG to look at.
    await page.screenshot({
      path: path.join(VISUAL_DIR, `${capture.name}.png`),
      animations: 'disabled',
      caret: 'hide'
    });

    const text = await page.evaluate(() => document.body.innerText);
    const found = FORBIDDEN_COPY.filter((phrase) => text.includes(phrase));
    expect(found, `copy register (.loom/37): banned phrases rendered in ${capture.name}`).toEqual([]);
  });
}
