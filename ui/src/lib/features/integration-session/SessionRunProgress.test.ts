import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import SessionRunProgress from './SessionRunProgress.svelte';
import type { IntegrationSessionPreviewMeta } from './types';

function meta(overrides: Partial<IntegrationSessionPreviewMeta> = {}): IntegrationSessionPreviewMeta {
  return {
    mode: 'session',
    id: 'session-1',
    sampleId: 'sample-1',
    runId: 'run-1',
    state: 'running',
    diagnostics: [],
    stages: [],
    lineage: [],
    streamState: 'connecting',
    error: null,
    ...overrides
  };
}

describe('SessionRunProgress', () => {
  it('announces the connection state before the first stage arrives', () => {
    render(SessionRunProgress, { session: meta() });

    expect(screen.getByLabelText('Server preview progression')).toBeInTheDocument();
    expect(screen.getByText('Connecting to server diagnostics')).toHaveAttribute('aria-live', 'polite');
    expect(screen.getByText('The stream is ready; waiting for the first server stage.')).toBeInTheDocument();
  });

  it('shows server stage status, duration, and terminal stream errors', () => {
    render(SessionRunProgress, {
      session: meta({
        streamState: 'error',
        error: 'The server stream ended before the run completed.',
        stages: [
          {
            id: 'semantic_extract',
            name: 'semantic_extract',
            status: 'failed',
            startedAt: '2026-07-16T20:00:00Z',
            completedAt: '2026-07-16T20:00:01Z',
            durationMs: 8
          }
        ]
      })
    });

    expect(screen.getByText('Stream needs attention')).toBeInTheDocument();
    expect(screen.getByText('semantic extract')).toBeInTheDocument();
    expect(screen.getByText('failed')).toHaveClass('sr-only');
    expect(screen.getByText('8 ms')).toBeInTheDocument();
    expect(screen.getByRole('status')).toHaveTextContent('ended before the run completed');
  });
});
