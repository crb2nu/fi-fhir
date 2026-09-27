import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { writable } from 'svelte/store';
import { GraphQLResponseError } from '$lib/graphql/client';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { EMPTY_INTAKE, type IntakeController, type IntakeControllerState } from './intakeController';
import { adapter, authStatus, batchConnection, capture, connection, runtime } from './intakeFixtures';

// The source reads (C-1's module) are mocked; the controller is a fake.
const connectionsApi = vi.hoisted(() => ({
  fetchEngineRuntime: vi.fn(),
  fetchConnections: vi.fn()
}));
vi.mock('$lib/features/connections/connectionsApi', () => connectionsApi);

const { default: ConnectionIntakeDialog } = await import('./ConnectionIntakeDialog.svelte');

const env = import.meta.env as Record<string, string | undefined>;
let previousFlag: string | undefined;

function fakeController(initial: Partial<IntakeControllerState> = {}) {
  const state = writable<IntakeControllerState>({ ...EMPTY_INTAKE, ...initial });
  const controller = {
    state: { subscribe: state.subscribe },
    refresh: vi.fn(async () => {}),
    startCapture: vi.fn(),
    cancelCapture: vi.fn(),
    peek: vi.fn(),
    dispose: vi.fn()
  };
  return { controller: controller as unknown as IntakeController & typeof controller, state };
}

function open(controller: IntakeController, extra: Record<string, unknown> = {}) {
  const onclose = vi.fn();
  render(ConnectionIntakeDialog, { props: { open: true, controller, onclose, ...extra } });
  return { onclose, dialog: () => screen.getByTestId('connection-intake-dialog') };
}

beforeEach(() => {
  previousFlag = env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED;
  env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED = 'true';
  setAccessStatus(authStatus());
  connectionsApi.fetchEngineRuntime.mockReset().mockResolvedValue(runtime());
  connectionsApi.fetchConnections.mockReset().mockResolvedValue([]);
});

afterEach(() => {
  cleanup();
  resetAccessCapabilities();
  env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED = previousFlag;
});

describe('ConnectionIntakeDialog', () => {
  it('reaches the honest empty state when nothing is mounted and the catalog holds no source', async () => {
    const { controller } = fakeController();
    const { dialog } = open(controller);
    const empty = await within(dialog()).findByTestId('connection-intake-empty');
    expect(empty).toHaveTextContent('No source connection is mounted on this deployment.');
    expect(connectionsApi.fetchConnections).toHaveBeenCalledWith('SOURCE', false);
    expect(screen.queryByTestId('connection-intake-sources')).toBeNull();
  });

  it('names the missing role in mono and queries nothing', async () => {
    setAccessStatus(authStatus({ connectionsRead: false }, { connectionsRead: ['integration.operator'] }));
    const { controller } = fakeController();
    const { dialog } = open(controller);
    const preflight = within(dialog()).getByTestId('connection-intake-preflight');
    expect(preflight).toHaveAttribute('data-missing-roles', 'integration.operator');
    expect(within(preflight).getByText('integration.operator').tagName).toBe('CODE');
    expect(connectionsApi.fetchEngineRuntime).not.toHaveBeenCalled();
    expect(connectionsApi.fetchConnections).not.toHaveBeenCalled();
    expect(controller.refresh).not.toHaveBeenCalled();
  });

  it('renders a failed read inline with Retry, never as an empty claim', async () => {
    connectionsApi.fetchConnections.mockRejectedValue(new GraphQLResponseError('connection catalog unavailable', []));
    const { controller } = fakeController();
    const { dialog } = open(controller);
    const alert = await within(dialog()).findByRole('alert');
    expect(alert).toHaveTextContent('Connection catalog:');
    expect(screen.queryByTestId('connection-intake-empty')).toBeNull();
    connectionsApi.fetchConnections.mockResolvedValue([]);
    await fireEvent.click(within(alert).getByRole('button', { name: 'Retry' }));
    expect(await within(dialog()).findByTestId('connection-intake-empty')).toBeInTheDocument();
  });

  it('arms a capture on a stream source with N, TTL and a reason, then closes', async () => {
    connectionsApi.fetchEngineRuntime.mockResolvedValue(
      runtime([adapter('mllp', true, { sourceId: 'adt-east', sourceRevisionId: '1' })])
    );
    const { controller } = fakeController();
    controller.startCapture.mockResolvedValue(capture());
    const { dialog, onclose } = open(controller);
    const sources = await within(dialog()).findByTestId('connection-intake-sources');
    await fireEvent.click(within(sources).getByRole('button', { name: 'Capture…' }));

    expect(dialog()).toHaveAttribute('data-step', 'capture');
    expect(dialog()).toHaveTextContent('messages with segments beyond MSH/EVN/PID/PV1 are currently rejected');
    const arm = within(dialog()).getByRole('button', { name: 'Arm capture' });
    expect(arm).toBeDisabled();
    expect(arm).toHaveAttribute('title', expect.stringMatching(/reason is required/));

    await fireEvent.input(within(dialog()).getByLabelText(/^Messages/), { target: { value: '2' } });
    await fireEvent.input(within(dialog()).getByLabelText(/^Expires after/), { target: { value: '60' } });
    await fireEvent.input(within(dialog()).getByLabelText(/^Reason/), { target: { value: 'check the east feed' } });
    await fireEvent.click(arm);

    expect(controller.startCapture).toHaveBeenCalledWith({
      sourceId: 'adt-east',
      maxMessages: 2,
      ttlSeconds: 60,
      reason: 'check the east feed'
    });
    await waitFor(() => expect(onclose).toHaveBeenCalled());
  });

  it('shows a refused start inside the dialog, naming the fix for SOURCE_UNAVAILABLE', async () => {
    connectionsApi.fetchConnections.mockResolvedValue([connection()]);
    const { controller } = fakeController();
    controller.startCapture.mockRejectedValue(
      new GraphQLResponseError('capture source unavailable: no mounted or compiled MLLP or HTTP source has this id', [
        { message: 'capture source unavailable', extensions: { code: 'SOURCE_UNAVAILABLE' } }
      ])
    );
    const { dialog, onclose } = open(controller);
    const sources = await within(dialog()).findByTestId('connection-intake-sources');
    expect(sources).toHaveTextContent('Compiled r1 · not mounted here');
    await fireEvent.click(within(sources).getByRole('button', { name: 'Capture…' }));
    await fireEvent.input(within(dialog()).getByLabelText(/^Reason/), { target: { value: 'try it' } });
    await fireEvent.click(within(dialog()).getByRole('button', { name: 'Arm capture' }));

    const alert = await within(dialog()).findByRole('alert');
    expect(alert).toHaveTextContent('FI_FHIR_MLLP_SOURCE_CONFIG_PATH');
    expect(onclose).not.toHaveBeenCalled();
  });

  it('disables Capture on a source this session has armed, with the reason in its title', async () => {
    connectionsApi.fetchConnections.mockResolvedValue([connection()]);
    const { controller } = fakeController({ sessionId: 'session-1', captures: [capture()] });
    const { dialog } = open(controller);
    const sources = await within(dialog()).findByTestId('connection-intake-sources');
    const button = within(sources).getByRole('button', { name: 'Capture…' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('title', expect.stringMatching(/already armed/));
    expect(within(sources).getByText('Armed')).toBeInTheDocument();
  });

  it('browses a batch source: list (audited), choose an object, read N, problems by path', async () => {
    connectionsApi.fetchConnections.mockResolvedValue([batchConnection()]);
    const { controller } = fakeController();
    const peekRow = capture({ id: 'peek-1', mode: 'PEEK', status: 'COMPLETE' });
    controller.peek
      .mockResolvedValueOnce({
        objects: [
          { path: 'adt/one.hl7', size: 2048, version: 'v1', modifiedAt: '2026-09-27T11:00:00Z' },
          { path: 'adt/two.hl7', size: 90, version: 'v2', modifiedAt: null }
        ],
        samples: [],
        capture: peekRow,
        problems: []
      })
      .mockResolvedValueOnce({
        objects: [],
        samples: [],
        capture: { ...peekRow, status: 'FAILED' },
        problems: [{ code: 'MESSAGE_UNREADABLE', path: 'objectPath', message: 'no HL7v2 message in the object' }]
      });
    const { dialog, onclose } = open(controller);
    const sources = await within(dialog()).findByTestId('connection-intake-sources');
    await fireEvent.click(within(sources).getByRole('button', { name: 'Browse objects…' }));

    await fireEvent.input(within(dialog()).getByLabelText(/^Reason/), { target: { value: 'find a sample' } });
    await fireEvent.click(within(dialog()).getByRole('button', { name: 'List objects' }));
    expect(controller.peek).toHaveBeenLastCalledWith({ connectionId: 'adt-batch', maxObjects: 10, reason: 'find a sample' });

    const objects = await within(dialog()).findByTestId('connection-intake-objects');
    const read = within(dialog()).getByRole('button', { name: 'Read messages' });
    expect(read).toHaveAttribute('title', 'Choose an object from the list first.');
    await fireEvent.click(within(objects).getByText('adt/two.hl7'));
    await fireEvent.click(read);
    expect(controller.peek).toHaveBeenLastCalledWith({
      connectionId: 'adt-batch',
      objectPath: 'adt/two.hl7',
      maxMessages: 5,
      reason: 'find a sample'
    });

    const problems = await within(dialog()).findByTestId('connection-intake-problems');
    expect(within(problems).getByText('objectPath')).toBeInTheDocument();
    expect(problems).toHaveTextContent('MESSAGE_UNREADABLE');
    expect(dialog()).toHaveTextContent('0 samples added to Samples before the peek stopped.');
    expect(onclose).not.toHaveBeenCalled();
  });

  it('cancels a capture with a reason', async () => {
    const armed = capture({ captured: 1 });
    const { controller } = fakeController({ sessionId: 'session-1', captures: [armed] });
    controller.cancelCapture.mockResolvedValue({ ...armed, status: 'CANCELLED' });
    const { dialog, onclose } = open(controller, { cancelTarget: armed });
    expect(dialog()).toHaveAttribute('data-step', 'cancel');
    expect(dialog()).toHaveTextContent('1 / 5 captured');
    expect(connectionsApi.fetchConnections).not.toHaveBeenCalled();
    await fireEvent.input(within(dialog()).getByLabelText(/^Reason/), { target: { value: 'wrong feed' } });
    await fireEvent.click(within(dialog()).getByRole('button', { name: 'Cancel capture' }));
    expect(controller.cancelCapture).toHaveBeenCalledWith('cap-1', 'wrong feed');
    await waitFor(() => expect(onclose).toHaveBeenCalled());
  });
});
