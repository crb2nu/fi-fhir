/**
 * Tests for the DebugPanel component.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';
import DebugPanel from './DebugPanel.svelte';
import { endSession } from './debugStore';

describe('DebugPanel', () => {
  beforeEach(() => {
    endSession();
  });

  describe('rendering', () => {
    it('should render step controls toolbar', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const toolbar = container.querySelector('[role="toolbar"]');
      expect(toolbar).not.toBeNull();
    });

    it('should render breakpoint list section', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const bpTitle = container.querySelector('.bp-title');
      expect(bpTitle).not.toBeNull();
      expect(bpTitle!.textContent).toBe('Breakpoints');
    });

    it('should render variable inspector section', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const sectionTitle = container.querySelector('.section-title');
      expect(sectionTitle).not.toBeNull();
      expect(sectionTitle!.textContent).toBe('Variables');
    });

    it('should render step history section', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const historyTitle = container.querySelector('.history-title');
      expect(historyTitle).not.toBeNull();
      expect(historyTitle!.textContent).toBe('Step History');
    });
  });

  describe('mock data loading', () => {
    it('should load mock data on mount when no session active', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      // Mock data includes 3 breakpoints, so we should see breakpoint items
      const bpItems = container.querySelectorAll('.bp-item');
      expect(bpItems.length).toBeGreaterThan(0);
    });

    it('should render variable inspector with current step variables', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      // Mock session has steps with variables, so var-entry elements should exist
      const varEntries = container.querySelectorAll('.var-entry');
      expect(varEntries.length).toBeGreaterThan(0);
    });

    it('should display step badge with current step info', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const stepBadge = container.querySelector('.step-badge');
      expect(stepBadge).not.toBeNull();
      // Mock data last step is step 3: webhook
      expect(stepBadge!.textContent).toContain('webhook');
    });

    it('should show step count in history section', () => {
      const { container } = render(DebugPanel, { props: { useMockData: true } });

      const historyCount = container.querySelector('.history-count');
      expect(historyCount).not.toBeNull();
      expect(historyCount!.textContent).toBe('3');
    });
  });
});
