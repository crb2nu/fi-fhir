/**
 * Tests for the EditorTabs component.
 */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import EditorTabs from './EditorTabs.svelte';
import type { EditorTab } from './types';

describe('EditorTabs', () => {
  const defaultTabs: EditorTab[] = [
    { id: 'tab-1', title: 'HL7 Message', dirty: false, view: 'hl7' },
    { id: 'tab-2', title: 'Workflow Config', dirty: true, view: 'workflows' },
    { id: 'tab-3', title: 'Event Log', dirty: false, view: 'events' },
  ];

  describe('rendering', () => {
    it('should render all tabs', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-1' } });

      expect(screen.getByRole('tab', { name: 'HL7 Message' })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Workflow Config' })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Event Log' })).toBeInTheDocument();
    });

    it('should render tablist', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-1' } });

      expect(screen.getByRole('tablist', { name: 'Open editors' })).toBeInTheDocument();
    });

    it('should render empty when no tabs', () => {
      render(EditorTabs, { props: { tabs: [], activeTabId: null } });

      expect(screen.queryAllByRole('tab')).toHaveLength(0);
    });
  });

  describe('active tab', () => {
    it('should mark active tab with aria-selected', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-2' } });

      const activeTab = screen.getByRole('tab', { name: 'Workflow Config' });
      expect(activeTab).toHaveAttribute('aria-selected', 'true');

      const inactiveTab = screen.getByRole('tab', { name: 'HL7 Message' });
      expect(inactiveTab).toHaveAttribute('aria-selected', 'false');
    });

    it('should apply active class', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-1' } });

      const activeTab = screen.getByRole('tab', { name: 'HL7 Message' });
      expect(activeTab).toHaveClass('active');
    });
  });

  describe('dirty indicator', () => {
    it('should show dirty indicator for unsaved tabs', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-1' } });

      const dirtyIndicators = screen.getAllByLabelText('Unsaved changes');
      expect(dirtyIndicators).toHaveLength(1);
    });

    it('should not show dirty indicator for clean tabs', () => {
      const cleanTabs: EditorTab[] = [
        { id: 'tab-1', title: 'Clean Tab', dirty: false, view: 'hl7' },
      ];
      render(EditorTabs, { props: { tabs: cleanTabs, activeTabId: 'tab-1' } });

      expect(screen.queryByLabelText('Unsaved changes')).not.toBeInTheDocument();
    });
  });

  describe('close button', () => {
    it('should render close buttons for each tab', () => {
      render(EditorTabs, { props: { tabs: defaultTabs, activeTabId: 'tab-1' } });

      const closeButtons = screen.getAllByLabelText(/^Close /);
      expect(closeButtons).toHaveLength(3);
    });

    it('should dispatch close event when close button is clicked', async () => {
      const closeFn = vi.fn();
      render(EditorTabs, {
        props: { tabs: defaultTabs, activeTabId: 'tab-1' },
        events: { close: closeFn },
      });

      const closeBtn = screen.getByLabelText('Close HL7 Message');
      await fireEvent.click(closeBtn);

      expect(closeFn).toHaveBeenCalledTimes(1);
      expect(closeFn.mock.calls[0]![0]!.detail).toBe('tab-1');
    });
  });

  describe('select event', () => {
    it('should dispatch select event when tab is clicked', async () => {
      const selectFn = vi.fn();
      render(EditorTabs, {
        props: { tabs: defaultTabs, activeTabId: 'tab-1' },
        events: { select: selectFn },
      });

      const tab = screen.getByRole('tab', { name: 'Workflow Config' });
      await fireEvent.click(tab);

      expect(selectFn).toHaveBeenCalledTimes(1);
      expect(selectFn.mock.calls[0]![0]!.detail).toBe('tab-2');
    });
  });
});
