import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import DryRunPanel from './DryRunPanel.svelte';
import { workflowDraft } from '../workflowStore';
import {
  dryRunWorkflow,
  fetchWorkflowSimulationSessions,
  saveSessionWorkflowDraft,
  simulateSessionWorkflow
} from '../workflowApi';

vi.mock('../workflowApi', () => ({
  dryRunWorkflow: vi.fn(),
  fetchWorkflowSimulationSessions: vi.fn(),
  saveSessionWorkflowDraft: vi.fn(),
  simulateSessionWorkflow: vi.fn()
}));

vi.mock('$lib/features/integration-session', () => ({
  isIntegrationSessionEngineEnabled: () => true
}));

vi.mock('$lib/graphql/client', () => ({
  isErrorToasted: vi.fn(() => false)
}));

describe('DryRunPanel session simulation', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    workflowDraft.loadDraft({
      name: 'session-routing',
      version: '1.0',
      routes: [
        {
          _key: 'route-1',
          name: 'admit_route',
          filter: { eventTypes: ['patient_admit'], sources: [], condition: '' },
          transforms: [{ _key: 'transform-1', type: 'set_field', config: { path: 'x' } }],
          actions: [{ _key: 'action-1', type: 'webhook', config: { url: 'https://trap.invalid' } }],
          expanded: false
        }
      ]
    });

    vi.mocked(fetchWorkflowSimulationSessions).mockResolvedValue({
      integrationSessions: [
        {
          id: 'session-1',
          name: 'ADT investigation',
          archived: false,
          runs: [
            {
              id: 'run-1',
              status: 'completed',
              events: [{ __typename: 'PatientAdmitEvent', id: 'event-1', type: 'PATIENT_ADMIT' }]
            }
          ],
          workflowSimulations: [
            {
              id: 'simulation-baseline',
              workflowRevisionId: 'revision-old',
              workflowRevisionDigest: 'digest-old',
              sourceRunIds: ['run-1'],
              createdAt: '2026-07-18T12:00:00Z'
            }
          ]
        }
      ]
    } as Awaited<ReturnType<typeof fetchWorkflowSimulationSessions>>);

    vi.mocked(saveSessionWorkflowDraft).mockResolvedValue({
      updateSessionWorkflowDraft: {
        id: 'workflow-artifact',
        revisionId: 'revision-new',
        digest: 'digest-new',
        version: 2
      }
    });

    vi.mocked(simulateSessionWorkflow).mockResolvedValue({
      simulateSessionWorkflow: {
        id: 'simulation-new',
        sessionId: 'session-1',
        workflowArtifactId: 'workflow-artifact',
        workflowRevisionId: 'revision-new',
        workflowRevisionDigest: 'digest-new',
        sourceRunIds: ['run-1'],
        createdAt: '2026-07-18T12:01:00Z',
        events: [
          {
            runId: 'run-1',
            eventId: 'event-1',
            eventType: 'patient_admit',
            routes: [
              {
                name: 'admit_route',
                matched: true,
                skipReason: null,
                diagnosticCodes: [],
                transforms: [{ index: 0, type: 'set_field', status: 'planned' }],
                actions: [{ id: 'action-1', type: 'webhook', destinationArtifactId: null }]
              }
            ]
          }
        ],
        delta: {
          baselineSimulationId: 'simulation-baseline',
          candidateSimulationId: 'simulation-new',
          addedEvents: [],
          removedEvents: [],
          addedMatchedRoutes: ['run-1:event-1:admit_route'],
          removedMatchedRoutes: [],
          addedTransforms: ['run-1:event-1:admit_route:0:set_field'],
          removedTransforms: [],
          addedActions: ['run-1:event-1:admit_route:action-1:webhook'],
          removedActions: []
        }
      }
    });
  });

  afterEach(cleanup);

  it('saves an exact draft revision and simulates only server-owned run IDs', async () => {
    render(DryRunPanel);

    await fireEvent.click(screen.getByRole('tab', { name: 'Session' }));
    expect(await screen.findByRole('option', { name: 'ADT investigation' })).toBeInTheDocument();
    expect(screen.getByText('1 event')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Run Simulation' }));

    await waitFor(() => expect(simulateSessionWorkflow).toHaveBeenCalledTimes(1));
    expect(saveSessionWorkflowDraft).toHaveBeenCalledWith(
      'session-1',
      expect.stringContaining('name: session-routing')
    );
    expect(simulateSessionWorkflow).toHaveBeenCalledWith({
      sessionId: 'session-1',
      workflowRevisionId: 'revision-new',
      sourceRunIds: ['run-1'],
      baselineSimulationId: 'simulation-baseline'
    });
    expect(dryRunWorkflow).not.toHaveBeenCalled();

    expect(await screen.findByText('digest-new')).toBeInTheDocument();
    expect(screen.getByText('Server-owned event traces')).toBeInTheDocument();
    expect(screen.getByText('Transform 1: set_field')).toBeInTheDocument();
    expect(screen.getByText('Action: webhook')).toBeInTheDocument();
    expect(screen.getByLabelText('Simulation delta')).toHaveTextContent('+1 routes');
  });
});
