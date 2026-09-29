import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import type { ConnectionChoice, DefinitionDetail, RegistryArtifact } from './definitionsApi';

// Every GraphQL boundary the Connections page touches is mocked.
const connections = vi.hoisted(() => ({
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
vi.mock('./connectionsApi', () => connections);
const definitions = vi.hoisted(() => ({
  MAX_AGE_PROPERTY: 'FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE',
  fetchDefinitions: vi.fn(),
  fetchDefinition: vi.fn(),
  fetchRegistryArtifacts: vi.fn(),
  fetchConnectionChoices: vi.fn(),
  fetchDefaultMaxAge: vi.fn(),
  checkDraft: vi.fn(),
  createDraft: vi.fn(),
  validateDefinition: vi.fn(),
  approveDefinition: vi.fn(),
  publishDefinition: vi.fn()
}));
vi.mock('./definitionsApi', () => definitions);
const operator = vi.hoisted(() => ({ fetchDeploymentEvents: vi.fn() }));
vi.mock('$lib/features/operator/operatorApi', () => operator);

const { default: ConnectionsPage } = await import('./ConnectionsPage.svelte');

Element.prototype.scrollIntoView ??= function scrollIntoView() {};

const DIGEST = 'sha256:' + 'cd'.repeat(32);

function status(capabilities: Record<string, unknown> = {}, missingRoles: Record<string, string[]> = {}) {
  return {
    authenticated: true,
    authVia: 'network',
    principal: 'e2e-ide-operator',
    roles: ['integration.operator', 'integration.deployment.operator'],
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
      definitionAuthoring: true,
      ...capabilities
    },
    missingRoles
  };
}

const source: ConnectionChoice = {
  id: 'adt-mllp',
  kind: 'MLLP',
  name: 'ADT east',
  secretBindings: [],
  latestRevision: {
    artifactId: 'adt-mllp',
    revisionId: '1',
    digest: DIGEST,
    sourceId: 'adt-east',
    destinationClass: null,
    secretBindingNames: []
  }
};
const destination: ConnectionChoice = {
  id: 'fhir-primary',
  kind: 'FHIR',
  name: 'Hospital FHIR',
  secretBindings: [{ name: 'fhir-token', provider: 'env', key: 'FI_FHIR_CONNECTION_SECRET_TOKEN', version: null }],
  latestRevision: {
    artifactId: 'fhir-primary',
    revisionId: '2',
    digest: DIGEST,
    sourceId: null,
    destinationClass: 'sandbox',
    secretBindingNames: ['fhir-token']
  }
};
const artifact: RegistryArtifact = {
  integrationId: 'adt-east',
  profile: { artifactId: 'profile-adt', revisionId: '1', digest: DIGEST },
  workflow: { artifactId: 'workflow-adt', revisionId: 'workflow-version-1', digest: DIGEST },
  sourceId: 'adt-east',
  format: 'hl7v2'
};

function detail(state: string, version: number, overrides: Partial<DefinitionDetail> = {}): DefinitionDetail {
  return {
    definition: {
      definitionId: 'adt-east-mllp',
      revisionId: 'v1',
      digest: DIGEST,
      parentRevisionId: null,
      state,
      version,
      health: 'unknown',
      releaseId: null,
      validationPassed: state !== 'draft',
      validationCheckedAt: state === 'draft' ? null : new Date().toISOString(),
      validationExpiresAt: state === 'draft' ? null : new Date(Date.now() + 300_000).toISOString(),
      source: { artifactId: 'adt-mllp', revisionId: '1', digest: DIGEST, sourceId: 'adt-east' },
      profile: artifact.profile,
      workflow: artifact.workflow,
      destinations: [{ artifactId: 'fhir-primary', revisionId: '2', digest: DIGEST, class: 'sandbox' }],
      secretBindings: [{ name: 'fhir-token', provider: 'env', key: 'FI_FHIR_CONNECTION_SECRET_TOKEN', version: null }],
      policy: {
        classification: 'phi',
        rawRetention: {
          mode: 'ephemeral',
          ttlSeconds: null,
          purpose: null,
          storageRevision: null,
          encryptionKey: null,
          accessAuditRequired: false
        }
      },
      deployment: {
        validationTimeoutSeconds: 5,
        validationMaxAgeSeconds: 300,
        scheduleMode: 'continuous',
        cronExpression: null,
        timezone: null,
        healthStartupGraceSeconds: 5,
        healthCheckIntervalSeconds: 30,
        healthTimeoutSeconds: 5,
        healthFailureThreshold: 3,
        maxInFlight: 2,
        maxQueued: 10,
        maxMessagesPerSecond: 100
      },
      createdBy: { id: 'e2e-ide-operator', kind: 'human' },
      createdReason: 'author the east listener',
      createdAt: '2026-09-29T12:00:00Z',
      updatedBy: { id: 'e2e-ide-operator', kind: 'human' },
      updatedReason: 'author the east listener',
      updatedAt: '2026-09-29T12:00:00Z'
    },
    validation: null,
    approval: null,
    release: null,
    realValidationAvailable: false,
    ...overrides
  };
}

beforeEach(() => {
  resetAccessCapabilities();
  connections.fetchConnections.mockResolvedValue([]);
  definitions.fetchDefinitions.mockResolvedValue([]);
  definitions.fetchConnectionChoices.mockImplementation(async (direction: string) =>
    direction === 'SOURCE' ? [source] : [destination]
  );
  definitions.fetchRegistryArtifacts.mockResolvedValue([artifact]);
  definitions.fetchDefaultMaxAge.mockResolvedValue(300);
  operator.fetchDeploymentEvents.mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  vi.clearAllMocks();
  window.history.replaceState(null, '', '/');
});

async function openDefinitions(): Promise<void> {
  await fireEvent.click(screen.getByTestId('connections-tab-definitions'));
}

describe('Connections › Definitions', () => {
  it('says not configured before any query on a deployment without the control plane', async () => {
    setAccessStatus(status({ controlPlane: false, connectionCatalog: false, definitionAuthoring: false }));
    render(ConnectionsPage);
    await openDefinitions();
    const preflight = screen.getByTestId('definitions-preflight');
    expect(preflight).toHaveAttribute('data-reason', 'not-configured');
    expect(definitions.fetchDefinitions).not.toHaveBeenCalled();
  });

  it('is read only without the deployment grant: the line names the role and New is disabled', async () => {
    setAccessStatus(
      status({ connectionsWrite: false, definitionAuthoring: false }, { definitionAuthoring: ['integration.deployment.operator'] })
    );
    render(ConnectionsPage);
    await openDefinitions();
    expect(screen.getByTestId('definitions-preflight')).toHaveAttribute('data-reason', 'read-only');
    expect(screen.getByTestId('definitions-new')).toBeDisabled();
    await waitFor(() => expect(definitions.fetchDefinitions).toHaveBeenCalledWith(false));
  });

  it('opens the definition a deep link names', async () => {
    window.history.replaceState(null, '', '/connections?definition=adt-east-mllp&revision=v1');
    setAccessStatus(status());
    definitions.fetchDefinition.mockResolvedValue(detail('published', 5, { realValidationAvailable: false }));
    render(ConnectionsPage);
    await waitFor(() => expect(definitions.fetchDefinition).toHaveBeenCalledWith('adt-east-mllp', 'v1'));
    const link = await screen.findByTestId('definition-deploy-link');
    expect(link).toHaveAttribute('href', '/operator?definition=adt-east-mllp&revision=v1');
    expect(screen.getByTestId('definition-mode-real')).toBeDisabled();
  });

  it('checks, creates a draft, and validates it STATIC at its version', async () => {
    setAccessStatus(status());
    definitions.checkDraft.mockResolvedValue([]);
    definitions.createDraft.mockResolvedValue({ definition: detail('draft', 1), problems: [] });
    definitions.fetchDefinition.mockResolvedValue(detail('draft', 1));
    definitions.validateDefinition.mockResolvedValue(
      detail('validated', 2, {
        validation: {
          validationId: 'validation-1',
          passed: true,
          codes: ['VALIDATION_STATIC', 'SOURCE_MOUNTED'],
          checkedAt: new Date().toISOString(),
          expiresAt: new Date(Date.now() + 300_000).toISOString(),
          sourceRevision: { artifactId: 'adt-mllp', revisionId: '1', digest: DIGEST },
          actor: { id: 'e2e-ide-operator', kind: 'human' },
          reason: 'static check of the east listener'
        }
      })
    );
    render(ConnectionsPage);
    await openDefinitions();
    await fireEvent.click(await screen.findByTestId('definitions-new'));
    const form = await screen.findByTestId('definition-new');
    await waitFor(() => expect(within(form).getByTestId('definition-new-source')).toBeInTheDocument());
    await fireEvent.input(within(form).getByTestId('definition-new-id'), { target: { value: 'adt-east-mllp' } });
    await fireEvent.change(within(form).getByTestId('definition-new-source'), { target: { value: 'adt-mllp' } });
    await fireEvent.change(within(form).getByTestId('definition-new-artifacts'), { target: { value: 'adt-east' } });
    await fireEvent.click(within(form).getByRole('checkbox', { name: /fhir-primary/ }));
    // The destination's binding is derived from its own reference.
    expect(await within(form).findByLabelText('fhir-token key')).toHaveValue('FI_FHIR_CONNECTION_SECRET_TOKEN');
    expect(within(form).getByTestId('definition-create')).toBeDisabled();

    await fireEvent.click(within(form).getByTestId('definition-check'));
    await waitFor(() => expect(definitions.checkDraft).toHaveBeenCalledTimes(1));
    const input = definitions.checkDraft.mock.calls[0]?.[0];
    expect(input).toMatchObject({
      definitionId: 'adt-east-mllp',
      source: { artifactId: 'adt-mllp', revisionId: '1' },
      destinations: [{ artifactId: 'fhir-primary', revisionId: '2' }],
      secretBindings: [{ name: 'fhir-token', provider: 'env', key: 'FI_FHIR_CONNECTION_SECRET_TOKEN', version: null }]
    });
    expect(await within(form).findByTestId('definition-problems')).toHaveAttribute('data-blocking', 'false');

    await fireEvent.click(within(form).getByTestId('definition-create'));
    const dialog = await screen.findByTestId('connection-reason-dialog');
    await fireEvent.input(within(dialog).getByRole('textbox'), { target: { value: 'author the east listener' } });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Create draft' }));
    await waitFor(() => expect(definitions.createDraft).toHaveBeenCalledWith(input, 'author the east listener'));

    const details = await screen.findByTestId('definition-details');
    await waitFor(() => expect(details).toHaveAttribute('data-state', 'draft'));
    await fireEvent.click(within(details).getByTestId('definition-validate'));
    const validateDialog = await screen.findByTestId('connection-reason-dialog');
    await fireEvent.input(within(validateDialog).getByRole('textbox'), {
      target: { value: 'static check of the east listener' }
    });
    await fireEvent.click(within(validateDialog).getByRole('button', { name: 'Record validation' }));
    await waitFor(() =>
      expect(definitions.validateDefinition).toHaveBeenCalledWith({
        definitionId: 'adt-east-mllp',
        revisionId: 'v1',
        expectedVersion: 1,
        reason: 'static check of the east listener',
        mode: 'STATIC'
      })
    );
    await waitFor(() => expect(screen.getByTestId('definition-details')).toHaveAttribute('data-state', 'validated'));
    expect(screen.getByTestId('definition-approve')).toBeEnabled();
  });

  it('refuses SKIP with a short reason inside the dialog, without a request', async () => {
    window.history.replaceState(null, '', '/connections?definition=adt-east-mllp&revision=v1');
    setAccessStatus(status());
    definitions.fetchDefinition.mockResolvedValue(detail('draft', 1));
    render(ConnectionsPage);
    const details = await screen.findByTestId('definition-details');
    await waitFor(() => expect(details).toHaveAttribute('data-state', 'draft'));
    await fireEvent.click(within(details).getByTestId('definition-mode-skip'));
    await fireEvent.click(within(details).getByTestId('definition-validate'));
    const dialog = await screen.findByTestId('connection-reason-dialog');
    await fireEvent.input(within(dialog).getByRole('textbox'), { target: { value: 'too short' } });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Record validation' }));
    expect(await within(dialog).findByRole('alert')).toHaveTextContent('at least 16 characters');
    expect(definitions.validateDefinition).not.toHaveBeenCalled();
  });
});
