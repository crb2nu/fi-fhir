/**
 * Shared helpers for the browser smoke gate. See playwright.config.ts for the
 * four stacks and e2e/run.sh for how they are started.
 */
import {
  expect,
  type APIRequestContext,
  type Locator,
  type Page,
  type Request,
  type Response,
  type TestInfo
} from '@playwright/test';

/** The documented operator bundle, in the order the stack grants it. */
export const OPERATOR_BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

/**
 * The copy register's banned phrases (`.loom/37` "Copy register", and
 * `ui/docs/DESIGN.md` "Copy register"): the onboarding and marketing voice the
 * design uplift removed. The `visual` project fails when any of them appears,
 * case-sensitively as written, in `document.body.innerText` of a captured
 * state. Extend this list — here, and only here — when a later lane retires
 * another phrase.
 */
export const FORBIDDEN_COPY: readonly string[] = [
  'Mission control',
  'Build the interface',
  'Recommended move',
  'Demo data',
  'Continue to',
  'NEXT UP'
];

/** `GET /api/auth/status` for an authenticated caller (Lane R-A's contract). */
export interface AuthStatus {
  authenticated: boolean;
  authVia: string;
  principal: string;
  roles: string[];
  capabilities: {
    operatorRead: boolean;
    operatorDelivery: boolean;
    operatorDeployment: boolean;
    clinicalRead: boolean;
    integrationSessions: boolean;
    streaming: boolean;
    subscriptions: string[];
    llm: { configured: boolean };
    /** The connection catalog (.loom/38 C-0). */
    connectionsRead?: boolean;
    connectionsWrite?: boolean;
    controlPlane?: boolean;
    connectionCatalog?: boolean;
  };
  missingRoles: Record<string, string[]>;
}

/**
 * Runs one GraphQL operation through nginx — the path the browser takes, as
 * the trusted network (no credential) — and returns its `data`, failing on
 * any GraphQL error. For reading back what the UI wrote.
 */
export async function graphqlData<T>(
  request: APIRequestContext,
  query: string,
  variables: Record<string, unknown> = {}
): Promise<T> {
  const response = await request.post('/graphql', {
    headers: { 'content-type': 'application/json', accept: 'application/json' },
    data: { query, variables }
  });
  expect(response.status(), 'POST /graphql through nginx').toBe(200);
  const body = (await response.json()) as { data?: T; errors?: unknown[] };
  expect(body.errors ?? [], 'GraphQL answered without errors').toEqual([]);
  return body.data as T;
}

/**
 * Reads the status through nginx — the path the browser takes — and attaches
 * the body to the report as evidence.
 */
export async function fetchAuthStatus(request: APIRequestContext, testInfo: TestInfo): Promise<AuthStatus> {
  const response = await request.get('/api/auth/status', { headers: { accept: 'application/json' } });
  expect(response.status(), 'GET /api/auth/status through nginx').toBe(200);
  const body = (await response.json()) as AuthStatus;
  await testInfo.attach('auth-status.json', {
    body: JSON.stringify(body, null, 2),
    contentType: 'application/json'
  });
  return body;
}

export function isGraphQLRequest(request: Request): boolean {
  return request.method() === 'POST' && new URL(request.url()).pathname === '/graphql';
}

/** A GraphQL subscription over SSE (`subscriptions.ts` sends Accept: text/event-stream). */
export function isStreamRequest(request: Request): boolean {
  return isGraphQLRequest(request) && (request.headers()['accept'] ?? '').includes('text/event-stream');
}

/** True when the request's GraphQL document selects the root field `field`. */
export function selects(request: Request, field: string): boolean {
  let body: unknown;
  try {
    body = request.postDataJSON();
  } catch {
    return false;
  }
  const query = (body as { query?: unknown } | null)?.query;
  return typeof query === 'string' && new RegExp(`\\b${field}\\s*[({]`).test(query);
}

export interface PageWatch {
  /** Every GraphQL POST the page issued, streams included, across navigations. */
  graphql: Request[];
  /** Text of every error toast that was ever rendered, even briefly. */
  errorToasts: string[];
}

/**
 * Records GraphQL traffic and error toasts for the page's whole life. Toasts
 * dismiss themselves, so a DOM assertion at the end of a test could miss one;
 * a MutationObserver installed before the app boots cannot.
 */
export async function watchPage(page: Page): Promise<PageWatch> {
  const watch: PageWatch = { graphql: [], errorToasts: [] };
  page.on('request', (request) => {
    if (isGraphQLRequest(request)) watch.graphql.push(request);
  });
  await page.exposeFunction('__e2eErrorToast', (text: string) => {
    watch.errorToasts.push(text);
  });
  await page.addInitScript(() => {
    const report = (window as unknown as { __e2eErrorToast: (text: string) => void }).__e2eErrorToast;
    new MutationObserver((records) => {
      for (const record of records) {
        for (const node of Array.from(record.addedNodes)) {
          if (!(node instanceof Element)) continue;
          const toasts = node.matches('.toast.error')
            ? [node]
            : Array.from(node.querySelectorAll('.toast.error'));
          for (const toast of toasts) report(toast.textContent?.trim() ?? '');
        }
      }
    }).observe(document, { childList: true, subtree: true });
  });
  return watch;
}

/**
 * A synthetic ADT^A01 the test:ui preview registry's `adt-east` integration
 * admits (the same shape cmd/fi-fhir's operator runtime test previews). No
 * patient data: every identifier is a placeholder.
 */
export const SYNTHETIC_ADT_A01 = [
  'MSH|^~\\&|FI_FHIR_E2E|TEST_FACILITY|FI_FHIR|TEST_FACILITY|20260101090000||ADT^A01|E2E-SMOKE-001|T|2.5.1',
  'EVN|A01|20260101090000',
  'PID|1||SYNTHETIC-0001^^^E2E^MR||SYNTHETIC^PATIENT||20000101|U',
  'PV1|1|I|TEST_WARD^TEST_ROOM^TEST_BED'
].join('\r');

/**
 * Replaces HL7 intake's editor content with `message` the way a user pastes
 * one, then presses "Normalize newlines". Since Lane R-E every send path
 * normalizes line endings itself and the built-in sample is executable, so
 * the click is no longer required; it stays so the gate also exercises the
 * explicit action and does not depend on the send-boundary fix.
 */
export async function enterHL7Message(page: Page, message: string): Promise<void> {
  await page.getByTestId('code-editor').locator('.cm-content').click();
  await page.keyboard.press('ControlOrMeta+A');
  await page.keyboard.insertText(message);
  await page.getByRole('button', { name: 'Normalize newlines' }).click();
}

/**
 * HL7 intake's Preview action: the toolbar button beside "Process", not the
 * workflow stepper's "Preview" chip (the stepper has its own "Load file" too,
 * so "Process" is the unique anchor).
 */
export function hl7PreviewButton(page: Page) {
  return page
    .getByRole('button', { name: 'Process', exact: true })
    .locator('xpath=..')
    .getByRole('button', { name: 'Preview', exact: true });
}

/**
 * A session Preview, from the click, in two budgets that follow the run's own
 * lifecycle rather than one fixed wait (`previewOnSessionStream`, then
 * `expectPreviewComplete`).
 *
 * What one 10 s `toHaveClass(/state-complete/)` was really covering, per the
 * traces of the two jobs that failed under runner saturation on 2026-09-27
 * (pipelines 29733 and 29713): createIntegrationSession and addSessionSample
 * at ~3 s each, the stream opening, and then runSessionPreview — some 25
 * sequential store round trips, each committed — still unanswered when the
 * budget ran out. The run is slow under load, not broken; the retries passed.
 *
 * PREVIEW_STREAM_OPEN_TIMEOUT: from the click to the SSE response — the
 * session and its sample created, the stream answered 200 text/event-stream.
 * `runStreamingSessionPreview` itself abandons a stream that has not opened
 * in 5 s, so this already holds the client's own limit.
 */
export const PREVIEW_STREAM_OPEN_TIMEOUT = 10_000;

/**
 * PREVIEW_RUN_TIMEOUT: from an open stream to the run's terminal state. The
 * run's own budget, separate from the stream's, sized for the saturated
 * runner rather than the idle one.
 */
export const PREVIEW_RUN_TIMEOUT = 60_000;

/**
 * Clicks HL7 intake's Preview and proves the run went to the session engine
 * on an open stream: the `integrationSessionEvents` SSE POST answered 200
 * `text/event-stream` within PREVIEW_STREAM_OPEN_TIMEOUT of the click. The
 * wait is armed with the click, so the budget is the stream's alone.
 *
 * The response, not the progression region, is the "stream opened" signal:
 * `SessionRunProgress` stays `state-connecting` until the first event of the
 * run arrives, which is the run's business (job 322278 sat in `connecting`
 * for 10 s with the stream long open).
 */
export async function previewOnSessionStream(page: Page): Promise<Response> {
  const [response] = await Promise.all([
    page.waitForResponse(
      (response) =>
        isStreamRequest(response.request()) && selects(response.request(), 'integrationSessionEvents'),
      { timeout: PREVIEW_STREAM_OPEN_TIMEOUT }
    ),
    hl7PreviewButton(page).click()
  ]);
  expect(response.status(), 'SSE POST /graphql for integrationSessionEvents').toBe(200);
  expect(response.headers()['content-type'] ?? '').toMatch(/^text\/event-stream/);
  return response;
}

/**
 * Waits for a session Preview whose stream is open to complete, and returns
 * the progression region.
 *
 * `SessionRunProgress` puts `state-{streamState}` on the region: `connecting`
 * until the run's first event, `running` from then, and `complete` only when
 * the run mutation answered `completed` and the stream raised no error
 * (`error` otherwise). So:
 *
 *   1. the region reaches a terminal state within PREVIEW_RUN_TIMEOUT — the
 *      run finished, however slow the host;
 *   2. that state is `complete`, with the wording and no `.stream-error` —
 *      an `error` fails here at once instead of idling out the run budget.
 *
 * The run's budget is added to the test's, so the test timeout (60 s) never
 * pre-empts step 1's own, more specific, failure.
 */
export async function expectPreviewComplete(page: Page, testInfo: TestInfo): Promise<Locator> {
  testInfo.setTimeout(testInfo.timeout + PREVIEW_RUN_TIMEOUT);
  const progress = page.getByRole('region', { name: 'Server preview progression' });
  await expect(progress, 'the run reached a terminal state').toHaveClass(/state-(complete|error)/, {
    timeout: PREVIEW_RUN_TIMEOUT
  });
  await expect(progress, 'the run completed on a stream that raised no error').toHaveClass(/state-complete/);
  await expect(progress).toContainText('Preview complete');
  await expect(progress.locator('.stream-error')).toHaveCount(0);
  return progress;
}

/**
 * HL7 intake's Samples tab (the results pane's first tab), where "From
 * connection…" sits beside "Load examples" (.loom/38 C-3). Resolves once the
 * inbox has rendered.
 */
export async function openHL7Samples(page: Page): Promise<void> {
  await openIDE(page, '/hl7');
  await page.getByRole('region', { name: 'Results' }).getByRole('tab', { name: /^Samples/ }).click();
  await expect(page.getByRole('button', { name: 'Load examples' })).toBeVisible();
}

/**
 * Loads a route and waits until the credential gate has stepped aside for the
 * trusted network — the IDE renders nothing before that. The gate shows no
 * strip once signed in; the access state is the status bar's chip (its
 * popover carries "Trusted network access active").
 */
export async function openIDE(page: Page, path: string): Promise<void> {
  await page.goto(path);
  const accessChip = page.getByTestId('access-chip');
  await expect(accessChip).toBeVisible();
  await expect(accessChip).toContainText('Trusted network');
}
