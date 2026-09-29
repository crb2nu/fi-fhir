/**
 * Tests for the TraceTimeline component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import TraceTimeline from './TraceTimeline.svelte';
import { mockTraceSpans } from './__fixtures__/debugFixtures';
import { endSession, traceSource } from './debugStore';

describe('TraceTimeline', () => {
  describe('rendering', () => {
    it('should render span bars for each span', () => {
      const { container } = render(TraceTimeline, {
        props: { spans: mockTraceSpans }
      });

      const rows = container.querySelectorAll('.span-row');
      expect(rows).toHaveLength(4);
    });

    it('should render span names as labels', () => {
      const { container } = render(TraceTimeline, {
        props: { spans: mockTraceSpans }
      });

      const labels = container.querySelectorAll('.span-label');
      expect(labels).toHaveLength(4);
      const [first, second, third, fourth] = Array.from(labels);
      expect(first?.textContent).toBe('workflow.process');
      expect(second?.textContent).toBe('workflow.route');
      expect(third?.textContent).toBe('workflow.transform');
      expect(fourth?.textContent).toBe('workflow.action');
    });

    it('should render span bars with status classes', () => {
      const { container } = render(TraceTimeline, {
        props: { spans: mockTraceSpans }
      });

      const bars = container.querySelectorAll('.span-bar');
      expect(bars).toHaveLength(4);
      bars.forEach((bar) => {
        expect(bar).toHaveClass('status-ok');
      });
    });

    it('should render duration labels', () => {
      const { container } = render(TraceTimeline, {
        props: { spans: mockTraceSpans }
      });

      const durations = container.querySelectorAll('.span-duration');
      expect(durations).toHaveLength(4);
    });
  });

  describe('empty state', () => {
    it('should show empty message when no spans', () => {
      const { container } = render(TraceTimeline, {
        props: { spans: [] }
      });

      const empty = container.querySelector('.timeline-empty');
      expect(empty).not.toBeNull();
      expect(empty!.textContent).toBe('No trace spans');
    });
  });

  describe('correct number of elements', () => {
    it('should render correct number of span elements for single span', () => {
      const firstSpan = mockTraceSpans[0];
      expect(firstSpan).toBeDefined();
      if (!firstSpan) {
        throw new Error('expected first mock trace span');
      }
      const singleSpan = [firstSpan];
      const { container } = render(TraceTimeline, {
        props: { spans: singleSpan }
      });

      const rows = container.querySelectorAll('.span-row');
      expect(rows).toHaveLength(1);
    });

    it('should render figure role', () => {
      render(TraceTimeline, { props: { spans: mockTraceSpans } });
      expect(screen.getByRole('figure')).toBeInTheDocument();
    });
  });

  describe('workflow run traces', () => {
    it('says which run it shows and how many spans it has', () => {
      traceSource.set({ kind: 'workflow-run', runId: 'run-1', state: 'loaded' });
      try {
        render(TraceTimeline, { props: { spans: mockTraceSpans } });
        expect(screen.getByTestId('trace-source')).toHaveTextContent('Workflow run run-1 · 4 spans');
      } finally {
        endSession();
      }
    });

    it('explains an empty run trace instead of a bare "No trace spans"', () => {
      traceSource.set({ kind: 'workflow-run', runId: 'run-2', state: 'loaded' });
      try {
        render(TraceTimeline, { props: { spans: [] } });
        expect(screen.getByTestId('trace-run-empty')).toHaveTextContent(
          'Run traces are kept in memory by the process that executed the run'
        );
        expect(screen.queryByText('No trace spans')).not.toBeInTheDocument();
      } finally {
        endSession();
      }
    });

    it('shows a load failure', () => {
      traceSource.set({ kind: 'workflow-run', runId: 'run-3', state: 'error', message: 'GraphQL operation forbidden' });
      try {
        render(TraceTimeline, { props: { spans: [] } });
        expect(screen.getByTestId('trace-error')).toHaveTextContent('GraphQL operation forbidden');
      } finally {
        endSession();
      }
    });
  });
});

