/**
 * Shared helpers for the browser smoke gate. See playwright.config.ts for the
 * three stacks and e2e/run.sh for how they are started.
 */
import { expect, type APIRequestContext, type Page, type Request, type TestInfo } from '@playwright/test';

/** The documented operator bundle, in the order the stack grants it. */
export const OPERATOR_BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
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
  };
  missingRoles: Record<string, string[]>;
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
 * one, then presses "Normalize newlines": the editor joins lines with LF,
 * Preview sends the editor text as-is, and the API parses CR-separated
 * segments only. (The page's built-in default sample does not preview either:
 * its separators are the literal characters `\r`.)
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
 * Loads a route and waits until the credential gate has stepped aside for the
 * trusted network — the IDE renders nothing before that.
 */
export async function openIDE(page: Page, path: string): Promise<void> {
  await page.goto(path);
  await expect(page.getByText('Trusted network access active')).toBeVisible();
}
