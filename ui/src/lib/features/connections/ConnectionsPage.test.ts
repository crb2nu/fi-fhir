import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { GraphQLResponseError } from '$lib/graphql/client';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { requestConnectionsView } from './connectionsIntent';
import type { ConnectionRevisionRow, ConnectionRow, EngineRuntimeView } from './connectionsApi';

// The catalog and runtime boundary is mocked so the page renders deterministically.
const api = vi.hoisted(() => ({
  fetchConnections: vi.fn(),
  fetchConnection: vi.fn(),
  fetchConnectionRevisions: vi.fn(),
  fetchConnectionRevision: vi.fn(),
  fetchEngineRuntime: vi.fn(),
  createConnection: vi.fn(),
  updateConnection: vi.fn(),
  archiveConnection: vi.fn(),
  compileConnection: vi.fn(),
  validateConnectionSpec: vi.fn()
}));
vi.mock('./connectionsApi', () => api);

const { default: ConnectionsPage } = await import('./ConnectionsPage.svelte');

// jsdom has no layout; the popover and table rows scroll into view.
Element.prototype.scrollIntoView ??= function scrollIntoView() {};

const BUNDLE = [
  'integration:preview',
  'graphql:operator',
  'clinical:read',
  'integration.operator',
  'integration.delivery.operator',
  'integration.deployment.operator'
];

function status(capabilities: Record<string, unknown> = {}, missingRoles: Record<string, string[]> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'e2e-ide-operator',
    roles: BUNDLE,
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment: true,
      clinicalRead: true,
      integrationSessions: true,
      streaming: true,
      subscriptions: [],
      llm: { configured: false },
      connectionsRead: true,
      connectionsWrite: true,
      controlPlane: true,
      connectionCatalog: true,
      ...capabilities
    },
    missingRoles
  };
}

const DIGEST = 'sha256:' + 'ab'.repeat(32);

function mllpRow(overrides: Partial<ConnectionRow> = {}): ConnectionRow {
  return {
    id: 'adt-east-mllp',
    direction: 'SOURCE',
    kind: 'MLLP',
    name: 'ADT east',
    description: '',
    spec: {
      source_id: 'adt-east',
      listen_address: '0.0.0.0:22575',
      encoding: 'utf-8',
      framing: { start_byte: 11, end_byte: 28, trailer_byte: 13 },
      timeouts: { read_seconds: 5, write_seconds: 5, idle_seconds: 60, process_seconds: 30 },
      tls: { mode: 'disabled' },
      clients: { allowed_cidrs: ['10.20.0.0/16'] },
      acknowledgements: { mode: 'application', include_error_segment: false },
      max_message_bytes: 1048576,
      max_connections: 16
    },
    secretBindings: [],
    version: 2,
    archived: false,
    latestRevision: null,
    references: [],
    runtime: { mounted: false, role: null, detail: null },
    createdBy: { id: 'e2e-ide-operator', kind: 'service' },
    createdAt: '2026-09-26T10:00:00Z',
    updatedBy: { id: 'e2e-ide-operator', kind: 'service' },
    updatedReason: 'synthetic source for the page test',
    updatedAt: '2026-09-26T10:05:00Z',
    ...overrides
  };
}

function revision(overrides: Partial<ConnectionRevisionRow> = {}): ConnectionRevisionRow {
  return {
    artifactId: 'adt-east-mllp',
    revisionId: '1',
    digest: DIGEST,
    direction: 'SOURCE',
    kind: 'MLLP',
    revisionJson: '{"schema_version":"1","artifact_id":"adt-east-mllp","revision_id":"1","digest":"' + DIGEST + '"}',
    compiledFromVersion: 2,
    createdBy: { id: 'e2e-ide-operator', kind: 'service' },
    createdReason: 'first compile',
    createdAt: '2026-09-26T10:06:00Z',
    ...overrides
  };
}

function adapter(kind: string, enabled: boolean, extra: Partial<EngineRuntimeView['adapters'][number]> = {}) {
  return {
    kind,
    enabled,
    definitionId: null,
    integrationId: null,
    sourceId: null,
    sourceRevisionId: null,
    sourceDigest: null,
    listenAddress: null,
    path: null,
    authMode: null,
    tlsMode: null,
    provider: null,
    pollSeconds: null,
    maxConnections: null,
    maxMessageBytes: null,
    maxBodyBytes: null,
    requireClientIdentity: null,
    requireWorkloadIdentity: null,
    queueDriver: null,
    maxAttempts: null,
    workerId: null,
    ...extra
  };
}

function runtime(overrides: Partial<EngineRuntimeView> = {}): EngineRuntimeView {
  return {
    version: '0.0.0-test',
    tenantId: 'tenant-a',
    replicaId: 'e2e-host-4242',
    authMode: 'static',
    trustedNetwork: true,
    accessIdentity: false,
    controlPlane: true,
    integrationSessions: true,
    streaming: true,
    retentionPurge: false,
    llmConfigured: false,
    registry: {
      integrationCount: 1,
      integrations: [
        {
          integrationId: 'adt-east',
          definitionId: 'adt-to-fhir',
          revisionId: '1',
          digest: DIGEST,
          sourceId: 'adt-east',
          format: 'hl7v2'
        }
      ]
    },
    adapters: [
      adapter('http', false),
      adapter('mllp', true, {
        definitionId: 'adt-to-fhir',
        sourceId: 'adt-east',
        sourceRevisionId: '1',
        sourceDigest: DIGEST,
        listenAddress: '0.0.0.0:2575',
        tlsMode: 'disabled',
        maxConnections: 16,
        maxMessageBytes: 1048576,
        requireClientIdentity: false
      }),
      adapter('batch', false),
      adapter('delivery', false)
    ],
    destinationIdentity: null,
    ledgers: [
      { name: 'submission', version: 9 },
      { name: 'connection', version: 1 }
    ],
    properties: [
      { key: 'FI_FHIR_MLLP_DEFINITION_ID', value: 'adt-to-fhir', secret: false, source: 'env' },
      { key: 'FI_FHIR_HTTP_INGRESS_INTEGRATION_ID', value: '', secret: false, source: 'default' },
      { key: 'FI_FHIR_GRAPHQL_BEARER_TOKEN', value: 'set', secret: true, source: 'env' },
      // A contract break on purpose: the page must still show only "set".
      { key: 'FI_FHIR_HTTP_INGRESS_SECRET', value: 'synthetic-raw-secret-value', secret: true, source: 'env' },
      { key: 'FI_FHIR_DATABASE_PASSWORD', value: 'unset', secret: true, source: 'default' }
    ],
    ...overrides
  };
}

/** jsdom's Blob has no text()/arrayBuffer(); FileReader reads it. */
function blobBytes(blob: Blob): Promise<Uint8Array> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(new Uint8Array(reader.result as ArrayBuffer));
    reader.onerror = () => reject(reader.error);
    reader.readAsArrayBuffer(blob);
  });
}

async function selectRow(name: string): Promise<void> {
  const table = await screen.findByTestId('connections-table');
  await fireEvent.click(within(table).getByText(name));
}

async function confirmReason(confirm: string, reason = 'synthetic change for the page test'): Promise<HTMLElement> {
  const dialog = await screen.findByTestId('connection-reason-dialog');
  await fireEvent.input(within(dialog).getByRole('textbox', { name: /Reason/ }), { target: { value: reason } });
  await fireEvent.click(within(dialog).getByRole('button', { name: confirm }));
  return dialog;
}

beforeEach(() => {
  vi.clearAllMocks();
  api.fetchConnections.mockResolvedValue([]);
  api.fetchEngineRuntime.mockResolvedValue(runtime());
  api.validateConnectionSpec.mockResolvedValue([]);
  api.fetchConnectionRevisions.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  resetAccessCapabilities();
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

describe('Connections — honest states', () => {
  it('is a toolbar titled Connections with Sources, Destinations and Engine tabs', async () => {
    setAccessStatus(status());
    render(ConnectionsPage);

    expect(screen.getByRole('heading', { level: 1, name: 'Connections' })).toBeInTheDocument();
    expect(screen.getAllByRole('tab').map((tab) => tab.textContent?.trim())).toEqual([
      'Sources',
      'Destinations',
      'Engine'
    ]);
    await waitFor(() => expect(api.fetchConnections).toHaveBeenCalledWith('SOURCE', false));
    expect(await screen.findByText('No source connections are defined.')).toBeInTheDocument();
  });

  it('without connectionsRead, names the missing role and issues no query', async () => {
    setAccessStatus(
      status(
        { connectionsRead: false, connectionsWrite: false },
        { connectionsRead: ['integration.operator'], connectionsWrite: ['integration.operator'] }
      )
    );
    render(ConnectionsPage);

    const preflight = await screen.findByTestId('connections-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'missing-role');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    const codes = Array.from(preflight.querySelectorAll('code')).map((node) => node.textContent);
    expect(codes).toContain('integration.operator');
    expect(codes).toContain('FI_FHIR_GRAPHQL_ROLES');
    expect(screen.queryByRole('tablist')).not.toBeInTheDocument();
    await Promise.resolve();
    expect(api.fetchConnections).not.toHaveBeenCalled();
    expect(api.fetchEngineRuntime).not.toHaveBeenCalled();
    expect(document.body.textContent ?? '').not.toMatch(/forbidden/i);
  });

  it('says the catalog is not configured (before any role) and queries no catalog', async () => {
    setAccessStatus(status({ controlPlane: false, connectionCatalog: false }));
    render(ConnectionsPage);

    const preflight = await screen.findByTestId('connections-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'not-configured');
    expect(preflight).toHaveTextContent('The connection catalog is not configured on this deployment.');
    const codes = Array.from(preflight.querySelectorAll('code')).map((node) => node.textContent);
    expect(codes).toEqual(['FI_FHIR_DATABASE_*', 'FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED=true']);
    await Promise.resolve();
    expect(api.fetchConnections).not.toHaveBeenCalled();

    // The engine runtime does not depend on the catalog: its tab still reads it.
    await fireEvent.click(screen.getByRole('tab', { name: 'Engine' }));
    expect(await screen.findByTestId('engine-runtime')).toBeInTheDocument();
    expect(api.fetchEngineRuntime).toHaveBeenCalledTimes(1);
  });

  it('keeps querying when capabilities are unknown, and shows a failed read inline with Retry', async () => {
    setAccessStatus({ authenticated: true, authVia: 'network' });
    api.fetchConnections.mockRejectedValueOnce(new Error('connection catalog unavailable'));
    render(ConnectionsPage);

    const alert = await screen.findByRole('alert');
    expect(alert).toHaveTextContent(/connection catalog is unavailable/);
    expect(screen.queryByTestId('connections-preflight')).not.toBeInTheDocument();
    await fireEvent.click(screen.getByRole('button', { name: 'Retry' }));
    await waitFor(() => expect(api.fetchConnections).toHaveBeenCalledTimes(2));
    expect(await screen.findByText('No source connections are defined.')).toBeInTheDocument();
  });

  it('without connectionsWrite, every form is read only with one status line', async () => {
    setAccessStatus(
      status({ connectionsWrite: false }, { connectionsWrite: ['integration.deployment.operator'] })
    );
    api.fetchConnections.mockResolvedValue([mllpRow()]);
    render(ConnectionsPage);

    const line = await screen.findByTestId('connections-read-only');
    expect(line).toHaveTextContent('integration.deployment.operator');
    expect(screen.getByTestId('connections-new')).toBeDisabled();

    await selectRow('ADT east');
    const form = await screen.findByTestId('connection-form');
    for (const control of form.querySelectorAll<HTMLInputElement>('input, textarea')) {
      expect(control.readOnly).toBe(true);
    }
    expect(screen.queryByRole('button', { name: 'Save' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Compile' })).not.toBeInTheDocument();
    await new Promise((resolve) => setTimeout(resolve, 450));
    expect(api.validateConnectionSpec).not.toHaveBeenCalled();
  });
});

describe('Connections — the catalog table and details', () => {
  it('renders each connection with its kind, endpoint, revision and honest status', async () => {
    setAccessStatus(status());
    api.fetchConnections.mockResolvedValue([
      mllpRow({
        latestRevision: { artifactId: 'adt-east-mllp', revisionId: '1', digest: DIGEST, compiledFromVersion: 2, createdAt: '2026-09-26T10:06:00Z' },
        references: [{ definitionId: 'adt-to-fhir', revisionId: '1', digest: DIGEST, state: 'draft', health: 'unknown' }],
        runtime: { mounted: true, role: 'mllp-listener', detail: 'revision 1: MLLP listener on 0.0.0.0:22575 for definition adt-to-fhir' }
      })
    ]);
    render(ConnectionsPage);

    const table = await screen.findByTestId('connections-table');
    const row = table.querySelector<HTMLElement>('[data-row="adt-east-mllp"]') as HTMLElement;
    expect(row).toHaveAttribute('data-status', 'Compiled r1 · Referenced · Mounted here');
    const cells = within(row).getAllByRole('cell').map((cell) => cell.textContent?.trim());
    expect(cells.slice(0, 5)).toEqual(['ADT east', 'MLLP', 'adt-east-mllp', '0.0.0.0:22575', 'r1 abababab…']);
    expect(within(row).getAllByRole('cell')[4]).toHaveAttribute('title', DIGEST);
    expect(cells[6]).toBe('2026-09-26 10:05');
  });

  it('checks a selected draft with validateConnectionSpec and lands its problems on their fields', async () => {
    setAccessStatus(status());
    const row = mllpRow();
    api.fetchConnections.mockResolvedValue([row]);
    api.validateConnectionSpec.mockResolvedValue([
      { code: 'OUT_OF_RANGE', path: 'timeouts.read_seconds', message: 'must be between 1 and 300' },
      { code: 'UNKNOWN_FIELD', path: 'timeouts.linger', message: 'is not a field of this connection kind' }
    ]);
    render(ConnectionsPage);
    await selectRow('ADT east');

    await waitFor(() => expect(api.validateConnectionSpec).toHaveBeenCalledTimes(1));
    expect(api.validateConnectionSpec).toHaveBeenCalledWith({ kind: 'MLLP', spec: row.spec, secretBindings: [] });
    const field = await screen.findByText('must be between 1 and 300');
    expect(field.closest('[data-path]')).toHaveAttribute('data-path', 'timeouts.read_seconds');
    expect(screen.getByTestId('connection-problems')).toHaveTextContent('timeouts.linger');
    expect(screen.getByRole('button', { name: 'Compile' })).toBeDisabled();
  });

  it('creates an MLLP source through the reason dialog, carrying only what the form holds', async () => {
    setAccessStatus(status());
    const created = mllpRow({ id: 'e2e-mllp-east', name: 'E2E MLLP east', version: 1, spec: {} });
    api.createConnection.mockResolvedValue(created);
    render(ConnectionsPage);

    await fireEvent.click(await screen.findByRole('button', { name: 'New source connection' }));
    const menu = await screen.findByRole('dialog', { name: 'New source connection' });
    expect(within(menu).getAllByRole('button').map((button) => button.getAttribute('data-kind'))).toEqual([
      'mllp',
      'http',
      'batch_s3',
      'batch_sftp'
    ]);
    await fireEvent.click(within(menu).getByText('MLLP'));

    const form = await screen.findByTestId('connection-form');
    const input = (path: string) => form.querySelector(`[data-path="${path}"] input, [data-path="${path}"] textarea`) as HTMLInputElement;
    await fireEvent.input(input('id'), { target: { value: 'e2e-mllp-east' } });
    await fireEvent.input(input('name'), { target: { value: 'E2E MLLP east' } });
    await fireEvent.input(input('source_id'), { target: { value: 'adt-east' } });
    await fireEvent.input(input('clients.allowed_cidrs'), { target: { value: '10.20.0.0/16\n' } });

    const details = screen.getByTestId('connection-details');
    await fireEvent.click(within(details).getByRole('button', { name: 'Create' }));
    await confirmReason('Create');

    await waitFor(() => expect(api.createConnection).toHaveBeenCalledTimes(1));
    expect(api.createConnection).toHaveBeenCalledWith({
      id: 'e2e-mllp-east',
      direction: 'SOURCE',
      kind: 'MLLP',
      name: 'E2E MLLP east',
      description: null,
      spec: {
        source_id: 'adt-east',
        encoding: 'utf-8',
        framing: { start_byte: 11, end_byte: 28, trailer_byte: 13 },
        clients: { allowed_cidrs: ['10.20.0.0/16'] },
        acknowledgements: { include_error_segment: false }
      },
      secretBindings: [],
      reason: 'synthetic change for the page test'
    });
    await waitFor(() => expect(screen.queryByTestId('connection-reason-dialog')).not.toBeInTheDocument());
    const table = screen.getByTestId('connections-table');
    expect(table.querySelector('[data-row="e2e-mllp-east"]')).not.toBeNull();
  });

  it('lands every refused path of a save on its field and keeps the dialog open', async () => {
    setAccessStatus(status());
    api.fetchConnections.mockResolvedValue([mllpRow()]);
    api.updateConnection.mockRejectedValue(
      new GraphQLResponseError('connection spec carries secret material', [
        {
          message: 'connection spec carries secret material',
          extensions: {
            code: 'SECRET_VALUE_FORBIDDEN',
            problems: [
              { code: 'SECRET_VALUE_FORBIDDEN', path: 'listen_address', message: 'a spec never carries a secret value' },
              { code: 'UNKNOWN_FIELD', path: 'tls.password', message: 'is not a field of this connection kind' }
            ]
          }
        }
      ])
    );
    render(ConnectionsPage);
    await selectRow('ADT east');

    const form = await screen.findByTestId('connection-form');
    const connections = form.querySelector('[data-path="max_connections"] input') as HTMLInputElement;
    await fireEvent.input(connections, { target: { value: '32' } });
    await fireEvent.click(within(screen.getByTestId('connection-details')).getByRole('button', { name: 'Save' }));
    const dialog = await confirmReason('Save');

    await waitFor(() => expect(api.updateConnection).toHaveBeenCalledTimes(1));
    expect(api.updateConnection.mock.calls[0]?.[0]).toMatchObject({ id: 'adt-east-mllp', expectedVersion: 2 });
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('Nothing was saved');
    expect(screen.getByTestId('connection-reason-dialog')).toBeInTheDocument();
    const field = form.querySelector<HTMLElement>('[data-path="listen_address"]') as HTMLElement;
    expect(within(field).getByText('a spec never carries a secret value')).toBeInTheDocument();
    expect(screen.getByTestId('connection-problems')).toHaveTextContent('tls.password');
  });

  it('tells the user to reload on a version conflict', async () => {
    setAccessStatus(status());
    api.fetchConnections.mockResolvedValue([mllpRow()]);
    api.updateConnection.mockRejectedValue(new Error('connection version conflict'));
    render(ConnectionsPage);
    await selectRow('ADT east');

    const form = await screen.findByTestId('connection-form');
    await fireEvent.input(form.querySelector('[data-path="name"] input') as HTMLInputElement, {
      target: { value: 'ADT east renamed' }
    });
    await fireEvent.click(within(screen.getByTestId('connection-details')).getByRole('button', { name: 'Save' }));
    const dialog = await confirmReason('Save');

    expect(await within(dialog).findByRole('alert')).toHaveTextContent(/saved first\. Reload it/);
    expect(within(dialog).getByRole('button', { name: 'Reload connection' })).toBeInTheDocument();
  });

  it('blocks Compile and offers Reload when the catalog moved past the version the form holds', async () => {
    setAccessStatus(status());
    const first = mllpRow();
    const moved = mllpRow({ version: 3, updatedReason: 'changed elsewhere' });
    api.fetchConnections.mockResolvedValueOnce([first]).mockResolvedValue([moved]);
    api.fetchConnection.mockResolvedValue(moved);
    render(ConnectionsPage);
    await selectRow('ADT east');

    // Edit, reload the list (the catalog is now at version 3), then undo the edit.
    const form = await screen.findByTestId('connection-form');
    const connections = form.querySelector('[data-path="max_connections"] input') as HTMLInputElement;
    await fireEvent.input(connections, { target: { value: '32' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Refresh connections' }));
    await waitFor(() => expect(api.fetchConnections).toHaveBeenCalledTimes(2));
    await fireEvent.input(connections, { target: { value: '16' } });

    const details = screen.getByTestId('connection-details');
    const compile = within(details).getByRole('button', { name: 'Compile' });
    await waitFor(() => expect(compile).toBeDisabled());
    expect(compile).toHaveAttribute('title', 'Reload first: this connection changed since you opened it.');
    const stale = within(details).getByTestId('connection-stale');
    expect(stale).toHaveTextContent('draft version 3');

    await fireEvent.click(within(stale).getByRole('button', { name: 'Reload' }));
    await waitFor(() => expect(api.fetchConnection).toHaveBeenCalledWith('adt-east-mllp'));
    await waitFor(() => expect(within(details).queryByTestId('connection-stale')).not.toBeInTheDocument());
    expect(api.compileConnection).not.toHaveBeenCalled();
  });

  it('compiles, shows the digest in Revisions, and downloads exactly revisionJson as <id>-r<N>.json', async () => {
    setAccessStatus(status());
    const row = mllpRow();
    const compiled = mllpRow({
      latestRevision: { artifactId: row.id, revisionId: '1', digest: DIGEST, compiledFromVersion: 2, createdAt: '2026-09-26T10:06:00Z' }
    });
    const first = revision();
    api.fetchConnections.mockResolvedValue([row]);
    api.compileConnection.mockResolvedValue({ connection: compiled, revision: first, problems: [] });
    api.fetchConnectionRevisions.mockResolvedValue([first]);
    const blobs: Blob[] = [];
    vi.stubGlobal('URL', Object.assign(URL, {
      createObjectURL: vi.fn((blob: Blob) => {
        blobs.push(blob);
        return 'blob:connection-revision';
      }),
      revokeObjectURL: vi.fn()
    }));
    const downloads: string[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(function (this: HTMLAnchorElement) {
      downloads.push(this.download);
    });
    render(ConnectionsPage);
    await selectRow('ADT east');

    const compile = within(screen.getByTestId('connection-details')).getByRole('button', { name: 'Compile' });
    await waitFor(() => expect(compile).toBeEnabled());
    await fireEvent.click(compile);
    await confirmReason('Compile');

    await waitFor(() => expect(api.compileConnection).toHaveBeenCalledWith({
      id: 'adt-east-mllp',
      expectedVersion: 2,
      reason: 'synthetic change for the page test'
    }));
    const revisions = await screen.findByTestId('connection-revisions');
    const detail = await within(revisions).findByTestId('revision-detail');
    expect(detail).toHaveAttribute('data-digest', DIGEST);
    expect(detail).toHaveTextContent(DIGEST);

    await fireEvent.click(within(revisions).getByRole('button', { name: 'Download' }));
    expect(downloads).toEqual(['adt-east-mllp-r1.json']);
    expect(blobs).toHaveLength(1);
    // Byte for byte: the UTF-8 encoding of revisionJson, nothing re-serialised or appended.
    expect(Array.from(await blobBytes(blobs[0]!))).toEqual(Array.from(new TextEncoder().encode(first.revisionJson)));
    expect(blobs[0]!.type).toBe('application/json');
  });
});

describe('Connections — Engine', () => {
  it('renders four adapter panels with their honest state and the env key beside a property', async () => {
    setAccessStatus(status());
    render(ConnectionsPage);
    await fireEvent.click(screen.getByRole('tab', { name: 'Engine' }));

    const engine = await screen.findByTestId('engine-runtime');
    const adapters = await within(engine).findAllByTestId('engine-adapter');
    expect(adapters.map((panel) => [panel.getAttribute('data-kind'), panel.getAttribute('data-enabled')])).toEqual([
      ['http', 'false'],
      ['mllp', 'true'],
      ['batch', 'false'],
      ['delivery', 'false']
    ]);
    const [http, mllp] = adapters as [HTMLElement, HTMLElement];
    expect(http).toHaveTextContent('Disabled');
    expect(http).toHaveTextContent('Not enabled on this replica.');
    // Only a variable the runtime lists is named.
    expect(http).toHaveTextContent('FI_FHIR_HTTP_INGRESS_INTEGRATION_ID');
    expect(mllp).toHaveTextContent('Enabled');
    expect(mllp).toHaveTextContent('0.0.0.0:2575');
    expect(within(mllp).getByText('FI_FHIR_MLLP_DEFINITION_ID').tagName).toBe('CODE');
    expect(mllp).not.toHaveTextContent('FI_FHIR_MLLP_SOURCE_CONFIG_PATH');

    expect(within(engine).getByTestId('engine-destination-identity')).toHaveTextContent(
      'No delivery identity registry is loaded on this replica.'
    );
    const properties = within(engine).getByTestId('engine-properties');
    const secrets = Array.from(properties.querySelectorAll<HTMLElement>('[data-secret="true"]'));
    expect(secrets.map((row) => row.querySelectorAll('td')[1]?.textContent?.trim())).toEqual(['set', 'set', 'unset']);
    expect(secrets[1]).toHaveTextContent('FI_FHIR_HTTP_INGRESS_SECRET');
    expect(document.body.textContent ?? '').not.toContain('synthetic-raw-secret-value');
    expect(within(engine).getByTestId('engine-ledgers')).toHaveTextContent('connection');
    expect(within(engine).getByTestId('engine-registry')).toHaveTextContent('adt-east');
  });

  it('opens on the view the command palette asked for', async () => {
    setAccessStatus(status());
    requestConnectionsView({ view: 'engine' });
    render(ConnectionsPage);

    expect(screen.getByRole('tab', { name: 'Engine' })).toHaveAttribute('aria-selected', 'true');
    expect(await screen.findByTestId('engine-runtime')).toBeInTheDocument();
  });
});
