/**
 * Tests for the Tabs component.
 */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Tabs, { type TabItem } from './Tabs.svelte';

describe('Tabs', () => {
  const defaultTabs: TabItem[] = [
    { key: 'tab1', label: 'First Tab' },
    { key: 'tab2', label: 'Second Tab' },
    { key: 'tab3', label: 'Third Tab' }
  ];

  describe('rendering', () => {
    it('should render all tabs', () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      expect(screen.getByRole('tab', { name: 'First Tab' })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Second Tab' })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Third Tab' })).toBeInTheDocument();
    });

    it('should render tablist role on container', () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      expect(screen.getByRole('tablist')).toBeInTheDocument();
    });

    it('should mark active tab with aria-selected', () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab2', onChange } });

      const secondTab = screen.getByRole('tab', { name: 'Second Tab' });
      expect(secondTab).toHaveAttribute('aria-selected', 'true');

      const firstTab = screen.getByRole('tab', { name: 'First Tab' });
      expect(firstTab).toHaveAttribute('aria-selected', 'false');
    });

    it('should apply active class to active tab', () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      const firstTab = screen.getByRole('tab', { name: 'First Tab' });
      expect(firstTab).toHaveClass('active');

      const secondTab = screen.getByRole('tab', { name: 'Second Tab' });
      expect(secondTab).not.toHaveClass('active');
    });
  });

  describe('disabled tabs', () => {
    it('should disable tab when disabled is true', () => {
      const tabsWithDisabled: TabItem[] = [
        { key: 'tab1', label: 'Enabled' },
        { key: 'tab2', label: 'Disabled', disabled: true }
      ];
      const onChange = vi.fn();

      render(Tabs, { props: { tabs: tabsWithDisabled, active: 'tab1', onChange } });

      const disabledTab = screen.getByRole('tab', { name: 'Disabled' });
      expect(disabledTab).toBeDisabled();

      const enabledTab = screen.getByRole('tab', { name: 'Enabled' });
      expect(enabledTab).not.toBeDisabled();
    });

    it('should have disabled attribute that prevents browser click', () => {
      // Note: fireEvent.click() in jsdom bypasses the disabled check,
      // but the disabled attribute is correctly set, which prevents clicks
      // in real browsers. We test that the attribute is present.
      const tabsWithDisabled: TabItem[] = [
        { key: 'tab1', label: 'Enabled' },
        { key: 'tab2', label: 'Disabled', disabled: true }
      ];
      const onChange = vi.fn();

      render(Tabs, { props: { tabs: tabsWithDisabled, active: 'tab1', onChange } });

      const disabledTab = screen.getByRole('tab', { name: 'Disabled' });
      expect(disabledTab).toBeDisabled();
      expect(disabledTab).toHaveAttribute('disabled');
    });
  });

  describe('onChange callback', () => {
    it('should call onChange with tab key when tab is clicked', async () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      const secondTab = screen.getByRole('tab', { name: 'Second Tab' });
      await fireEvent.click(secondTab);

      expect(onChange).toHaveBeenCalledWith('tab2');
    });

    it('should call onChange when clicking already active tab', async () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      const firstTab = screen.getByRole('tab', { name: 'First Tab' });
      await fireEvent.click(firstTab);

      expect(onChange).toHaveBeenCalledWith('tab1');
    });

    it('should call onChange with correct key for each tab', async () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: defaultTabs, active: 'tab1', onChange } });

      await fireEvent.click(screen.getByRole('tab', { name: 'Third Tab' }));
      expect(onChange).toHaveBeenLastCalledWith('tab3');

      await fireEvent.click(screen.getByRole('tab', { name: 'First Tab' }));
      expect(onChange).toHaveBeenLastCalledWith('tab1');

      await fireEvent.click(screen.getByRole('tab', { name: 'Second Tab' }));
      expect(onChange).toHaveBeenLastCalledWith('tab2');
    });
  });

  describe('empty tabs', () => {
    it('should render empty tablist when no tabs provided', () => {
      const onChange = vi.fn();
      render(Tabs, { props: { tabs: [], active: '', onChange } });

      const tablist = screen.getByRole('tablist');
      expect(tablist).toBeInTheDocument();
      expect(screen.queryAllByRole('tab')).toHaveLength(0);
    });
  });

  describe('single tab', () => {
    it('should handle single tab', () => {
      const singleTab: TabItem[] = [{ key: 'only', label: 'Only Tab' }];
      const onChange = vi.fn();

      render(Tabs, { props: { tabs: singleTab, active: 'only', onChange } });

      expect(screen.getByRole('tab', { name: 'Only Tab' })).toBeInTheDocument();
      expect(screen.getByRole('tab', { name: 'Only Tab' })).toHaveClass('active');
    });
  });
});
