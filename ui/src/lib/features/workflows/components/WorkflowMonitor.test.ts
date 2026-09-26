import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen, waitFor } from '@testing-library/svelte';
import WorkflowMonitor from './WorkflowMonitor.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

const mocks = vi.hoisted(() => ({
  subscribe: vi.fn(),
  fetchWorkflowDefinitions: vi.fn(),
  fetchWorkflowRuns: vi.fn(),
  fetchWorkflowApprovalRequests: vi.fn()
}));

vi.mock('$lib/graphql/subscriptions', () => ({
  subscribe: (...args: unknown[]) => mocks.subscribe(...args)
}));

vi.mock('../workflowApi', () => ({
  fetchWorkflowDefinitions: (...args: unknown[]) => mocks.fetchWorkflowDefinitions(...args),
  fetchWorkflowRuns: (...args: unknown[]) => mocks.fetchWorkflowRuns(...args),
  fetchWorkflowApprovalRequests: (...args: unknown[]) =>
    mocks.fetchWorkflowApprovalRequests(...args),
  fetchWorkflowRun: vi.fn(),
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
});
