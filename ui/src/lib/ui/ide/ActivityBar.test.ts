/**
 * Tests for the ActivityBar component.
 */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import ActivityBar from './ActivityBar.svelte';

describe('ActivityBar', () => {
  describe('rendering', () => {
    it('should render 6 view icons', () => {
      render(ActivityBar, { props: { activeView: 'hl7' } });

      const buttons = screen.getAllByRole('button');
      expect(buttons).toHaveLength(6);
    });

    it('should render all expected aria labels', () => {
      render(ActivityBar, { props: { activeView: 'hl7' } });

      expect(screen.getByRole('button', { name: 'HL7 Messages' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Workflows' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Events' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Profiles' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Terminology' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'System' })).toBeInTheDocument();
    });

    it('should render activity bar navigation', () => {
      render(ActivityBar, { props: { activeView: 'hl7' } });

      expect(screen.getByRole('navigation', { name: 'Activity bar' })).toBeInTheDocument();
    });
  });

  describe('active highlight', () => {
    it('should mark active view with aria-current', () => {
      render(ActivityBar, { props: { activeView: 'workflows' } });

      const workflowBtn = screen.getByRole('button', { name: 'Workflows' });
      expect(workflowBtn).toHaveAttribute('aria-current', 'true');

      const hl7Btn = screen.getByRole('button', { name: 'HL7 Messages' });
      expect(hl7Btn).not.toHaveAttribute('aria-current');
    });

    it('should apply active class to current view', () => {
      render(ActivityBar, { props: { activeView: 'events' } });

      const eventsBtn = screen.getByRole('button', { name: 'Events' });
      expect(eventsBtn).toHaveClass('active');

      const hl7Btn = screen.getByRole('button', { name: 'HL7 Messages' });
      expect(hl7Btn).not.toHaveClass('active');
    });
  });

  describe('change event', () => {
    it('should dispatch change event when icon is clicked', async () => {
      const changeFn = vi.fn();
      render(ActivityBar, {
        props: { activeView: 'hl7' },
        events: { change: changeFn },
      });

      const workflowBtn = screen.getByRole('button', { name: 'Workflows' });
      await fireEvent.click(workflowBtn);

      expect(changeFn).toHaveBeenCalledTimes(1);
    });

    it('should dispatch correct view for each icon', async () => {
      const changeFn = vi.fn();
      render(ActivityBar, {
        props: { activeView: 'hl7' },
        events: { change: changeFn },
      });

      const views = ['HL7 Messages', 'Workflows', 'Events', 'Profiles', 'Terminology', 'System'];
      const expected = ['hl7', 'workflows', 'events', 'profiles', 'terminology', 'system'];

      for (let i = 0; i < views.length; i++) {
        await fireEvent.click(screen.getByRole('button', { name: views[i] }));
      }

      expect(changeFn).toHaveBeenCalledTimes(6);
      for (let i = 0; i < expected.length; i++) {
        expect(changeFn.mock.calls[i]![0]!.detail).toBe(expected[i]);
      }
    });
  });
});
