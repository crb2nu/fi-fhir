/**
 * Tests for the TraceTimeline component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import TraceTimeline from './TraceTimeline.svelte';
import { mockTraceSpans } from './debugMocks';

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
      expect(labels[0].textContent).toBe('workflow.process');
      expect(labels[1].textContent).toBe('workflow.route');
      expect(labels[2].textContent).toBe('workflow.transform');
      expect(labels[3].textContent).toBe('workflow.action');
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
      const singleSpan = [mockTraceSpans[0]];
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
});
