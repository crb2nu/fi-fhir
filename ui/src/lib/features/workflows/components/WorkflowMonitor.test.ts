import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { get } from 'svelte/store';
import { traceSource, traceSpans, endSession } from '$lib/features/debug/debugStore';
import { ideState } from '$lib/ui/ide/ideStore';
import WorkflowMonitor from './WorkflowMonitor.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

const mocks = vi.hoisted(() => ({
  subscribe: vi.fn(),
  fetchWorkflowDefinitions: vi.fn(),
  fetchWorkflowRuns: vi.fn(),
  fetchWorkflowApprovalRequests: vi.fn(),
  fetchWorkflowRun: vi.fn(),
  fetchWorkflowRunTrace: vi.fn()
}));

vi.mock('$lib/features/debug/debugApi', async (importOriginal) => ({
  ...(await importOriginal<typeof import('$lib/features/debug/debugApi')>()),
  fetchWorkflowRunTrace: (...args: unknown[]) => mocks.fetchWorkflowRunTrace(...args)
}));

vi.mock('$lib/graphql/subscriptions', () => ({
  subscribe: (...args: unknown[]) => mocks.subscribe(...args)
}));

vi.mock('../workflowApi', () => ({
  fetchWorkflowDefinitions: (...args: unknown[]) => mocks.fetchWorkflowDefinitions(...args),
  fetchWorkflowRuns: (...args: unknown[]) => mocks.fetchWorkflowRuns(...args),
  fetchWorkflowApprovalRequests: (...args: unknown[]) =>
    mocks.fetchWorkflowApprovalRequests(...args),
  fetchWorkflowRun: (...args: unknown[]) => mocks.fetchWorkflowRun(...args),
  approveWorkflowVersion: vi.fn(),
  rejectWorkflowVersion: vi.fn()
}));

describe('WorkflowMonitor live stream honesty', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mocks.fetchWorkflowDefinitions.mockResolvedValue({ workflowDefinitions: [] });
    mocks.fetchWorkflowRuns.mockResolvedValue({ workflowRuns: [] });
    mocks.fetchWorkflowApprovalRequests.mockResolvedValue({ workflowApprovalRequests: [] });
  });

  afterEach(() => {
    resetAccessCapabilities();
    resetObservedStreams();
  });

  it('replaces the connect bar with the honest state and never subscribes', async () => {
    setAccessStatus({
      authenticated: true,
      authVia: 'network',
      capabilities: {
        operatorRead: false,
        streaming: true,
        subscriptions: ['integrationSessionEvents', 'sessionRunEvents']
      }
    });
    render(WorkflowMonitor, { props: { initialWorkflowName: 'adt-routing' } });

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-stream', 'workflowEvents');
    expect(state).toHaveTextContent('Live streaming for workflow events is not available');
    expect(screen.queryByRole('button', { name: 'Connect' })).not.toBeInTheDocument();
    expect(mocks.subscribe).not.toHaveBeenCalled();
    // The query-backed sections keep working.
    await waitFor(() => expect(mocks.fetchWorkflowRuns).toHaveBeenCalled());
    expect(screen.getByText('Run Diagnostics')).toBeInTheDocument();
  });

  it('keeps the Connect control while capabilities are unknown', () => {
    render(WorkflowMonitor);
    expect(screen.getByRole('button', { name: 'Connect' })).toBeInTheDocument();
    expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
  });

  it('opens a run’s recorded trace in the bottom Trace panel', async () => {
    const run = {
      id: 'run-7',
      workflowName: 'adt-routing',
      environment: 'staging',
      versionId: 'v1',
      eventId: 'evt-1',
      routesMatched: 1,
      actionsExecuted: 1,
      errors: [],
      durationMs: 3,
      startedAt: '2026-09-29T10:00:00Z',
      status: 'success'
    };
    const span = {
      id: 's1',
      name: 'workflow.process',
      parentId: null,
      startTime: '2026-09-29T10:00:00Z',
      endTime: '2026-09-29T10:00:00.003Z',
      status: 'ok' as const,
      attributes: {},
      events: []
    };
    mocks.fetchWorkflowRuns.mockResolvedValue({ workflowRuns: [run] });
    mocks.fetchWorkflowRun.mockResolvedValue({ workflowRun: run });
    mocks.fetchWorkflowRunTrace.mockResolvedValue([span]);
    try {
      render(WorkflowMonitor);
      await fireEvent.click(await screen.findByText('adt-routing'));
      await fireEvent.click(await screen.findByTestId('workflow-run-open-trace'));

      await waitFor(() => expect(get(traceSource)).toEqual({ kind: 'workflow-run', runId: 'run-7', state: 'loaded' }));
      expect(mocks.fetchWorkflowRunTrace).toHaveBeenCalledWith('run-7');
      expect(get(traceSpans)).toEqual([span]);
      expect(get(ideState).activePanelTab).toBe('trace');
      expect(get(ideState).bottomPanelOpen).toBe(true);
    } finally {
      endSession();
    }
  });
});

