/**
 * Tests for the StatusBar component.
 */
import { describe, it, expect } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import StatusBar from './StatusBar.svelte';
import { evidenceOf } from './__fixtures__/journeyEvidence';

describe('StatusBar', () => {
  describe('connection state', () => {
    it('should display Connected when connected', () => {
      render(StatusBar, { props: { connectionState: 'connected' } });

      expect(screen.getByText('Connected')).toBeInTheDocument();
    });

    it('should display Connecting when connecting', () => {
      render(StatusBar, { props: { connectionState: 'connecting' } });

      expect(screen.getByText('Connecting')).toBeInTheDocument();
    });

    it('should display Offline when the health check fails', () => {
      render(StatusBar, { props: { connectionState: 'disconnected' } });

      expect(screen.getByText('Offline')).toBeInTheDocument();
    });
  });

  describe('removed items', () => {
    it('shows no active-profile or parser item (nothing ever fed them)', () => {
      render(StatusBar, { props: { connectionState: 'connected' } });

      expect(screen.queryByTitle('Active profile')).not.toBeInTheDocument();
      expect(screen.queryByTitle('Parser status')).not.toBeInTheDocument();
    });
  });

  describe('branding', () => {
    it('should display fi-fhir branding', () => {
      render(StatusBar, { props: { connectionState: 'disconnected' } });

      expect(screen.getByText('fi-fhir')).toBeInTheDocument();
    });
  });

  describe('status role', () => {
    it('should have role=status on the footer', () => {
      render(StatusBar, { props: { connectionState: 'disconnected' } });

      expect(screen.getByRole('status')).toBeInTheDocument();
    });
  });

  describe('platform chrome', () => {
    it('renders no Platform indicator when no platform is configured', () => {
      render(StatusBar, { props: { connectionState: 'connected', platformEnabled: false } });

      expect(screen.queryByTestId('platform-indicator')).not.toBeInTheDocument();
      expect(screen.queryByText('Platform')).not.toBeInTheDocument();
    });

    it('hides the indicator by default (PLATFORM_CONFIG unset)', () => {
      render(StatusBar, { props: { connectionState: 'connected' } });

      expect(screen.queryByTestId('platform-indicator')).not.toBeInTheDocument();
    });

    it('renders the Platform indicator when a platform endpoint is configured', () => {
      render(StatusBar, {
        props: { connectionState: 'connected', platformEnabled: true, platformConnected: false }
      });

      const indicator = screen.getByTestId('platform-indicator');
      expect(indicator).toHaveTextContent('Platform');
      expect(indicator).toHaveAttribute('title', 'Platform disconnected');
    });
  });

  describe('next stage', () => {
    it('offers nothing until the evidence is known', () => {
      render(StatusBar, { props: { connectionState: 'connected', pathname: '/', evidence: null } });

      expect(screen.queryByTestId('status-next')).not.toBeInTheDocument();
    });

    it('links to the earliest stage whose evidence is missing, not the following one', () => {
      const evidence = evidenceOf({ 'source-intake': 'complete', normalization: 'complete', translation: 'unknown' });
      render(StatusBar, { props: { connectionState: 'connected', pathname: '/profiles', evidence } });

      const next = screen.getByTestId('status-next');
      expect(next).toHaveTextContent('Next: Delivery');
      expect(next).toHaveAttribute('href', '/workflows');
      expect(next).toHaveAttribute('data-stage', 'delivery');
      expect(next).toHaveAttribute('title', 'Stage 4 of 5: Delivery — not complete. delivery is incomplete.');
    });

    it('offers Source Intake from Home on a fresh deployment', () => {
      render(StatusBar, { props: { connectionState: 'connected', pathname: '/', evidence: evidenceOf() } });

      expect(screen.getByTestId('status-next')).toHaveAttribute('href', '/hl7');
    });

    it('offers nothing when no stage is incomplete, or off the stage routes', () => {
      const done = evidenceOf({
        'source-intake': 'complete',
        normalization: 'complete',
        translation: 'complete',
        delivery: 'unknown',
        verification: 'complete',
      });
      const { unmount } = render(StatusBar, { props: { connectionState: 'connected', pathname: '/events', evidence: done } });
      expect(screen.queryByTestId('status-next')).not.toBeInTheDocument();
      unmount();

      render(StatusBar, { props: { connectionState: 'connected', pathname: '/operator', evidence: evidenceOf() } });
      expect(screen.queryByTestId('status-next')).not.toBeInTheDocument();
    });
  });

  describe('access chip', () => {
    it('renders no chip before a session exists', () => {
      render(StatusBar, { props: { connectionState: 'connected' } });

      expect(screen.queryByTestId('access-chip')).not.toBeInTheDocument();
    });

    it('shows the session and explains it in a popover, with Clear access for bearer only', async () => {
      let cleared = 0;
      render(StatusBar, {
        props: {
          connectionState: 'connected',
          access: { via: 'bearer', principal: '' },
          onClearAccess: () => {
            cleared += 1;
          },
        },
      });

      const chip = screen.getByTestId('access-chip');
      expect(chip).toHaveTextContent('Bearer');
      // The chip sits inside the status bar strip.
      expect(within(screen.getByRole('status')).getByTestId('access-chip')).toBe(chip);

      await fireEvent.click(chip);
      const popover = screen.getByRole('dialog', { name: 'Access' });
      expect(popover).toHaveTextContent('Preview access active');
      await fireEvent.click(within(popover).getByRole('button', { name: 'Clear access' }));
      expect(cleared).toBe(1);
      expect(screen.queryByRole('dialog', { name: 'Access' })).not.toBeInTheDocument();
    });
  });
});
