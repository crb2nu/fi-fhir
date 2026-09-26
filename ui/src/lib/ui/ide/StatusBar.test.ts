/**
 * Tests for the StatusBar component.
 */
import { describe, it, expect } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import StatusBar from './StatusBar.svelte';

describe('StatusBar', () => {
  describe('connection state', () => {
    it('should display Connected when connected', () => {
      render(StatusBar, { props: { connectionState: 'connected', activeProfile: '', parserStatus: '' } });

      expect(screen.getByText('Connected')).toBeInTheDocument();
    });

    it('should display Connecting when connecting', () => {
      render(StatusBar, { props: { connectionState: 'connecting', activeProfile: '', parserStatus: '' } });

      expect(screen.getByText('Connecting')).toBeInTheDocument();
    });

    it('should display Offline when the health check fails', () => {
      render(StatusBar, { props: { connectionState: 'disconnected', activeProfile: '', parserStatus: '' } });

      expect(screen.getByText('Offline')).toBeInTheDocument();
    });
  });

  describe('active profile', () => {
    it('should display active profile when provided', () => {
      render(StatusBar, { props: { connectionState: 'connected', activeProfile: 'EPIC-PROD', parserStatus: '' } });

      expect(screen.getByText('EPIC-PROD')).toBeInTheDocument();
    });

    it('should not show profile section when empty', () => {
      render(StatusBar, { props: { connectionState: 'connected', activeProfile: '', parserStatus: '' } });

      expect(screen.queryByTitle('Active profile')).not.toBeInTheDocument();
    });
  });

  describe('parser status', () => {
    it('should display parser status when provided', () => {
      render(StatusBar, { props: { connectionState: 'connected', activeProfile: '', parserStatus: 'HL7v2.5.1' } });

      expect(screen.getByText('HL7v2.5.1')).toBeInTheDocument();
    });

    it('should not show parser section when empty', () => {
      render(StatusBar, { props: { connectionState: 'connected', activeProfile: '', parserStatus: '' } });

      expect(screen.queryByTitle('Parser status')).not.toBeInTheDocument();
    });
  });

  describe('branding', () => {
    it('should display fi-fhir branding', () => {
      render(StatusBar, { props: { connectionState: 'disconnected', activeProfile: '', parserStatus: '' } });

      expect(screen.getByText('fi-fhir')).toBeInTheDocument();
    });
  });

  describe('status role', () => {
    it('should have role=status on the footer', () => {
      render(StatusBar, { props: { connectionState: 'disconnected', activeProfile: '', parserStatus: '' } });

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

  describe('all fields populated', () => {
    it('should render all fields together', () => {
      render(StatusBar, {
        props: {
          connectionState: 'connected',
          activeProfile: 'CERNER-TEST',
          parserStatus: 'CDA R2',
        },
      });

      expect(screen.getByText('Connected')).toBeInTheDocument();
      expect(screen.getByText('CERNER-TEST')).toBeInTheDocument();
      expect(screen.getByText('CDA R2')).toBeInTheDocument();
      expect(screen.getByText('fi-fhir')).toBeInTheDocument();
    });
  });

  describe('next stage', () => {
    it('links to the following stage from a stage route', () => {
      render(StatusBar, { props: { connectionState: 'connected', pathname: '/profiles' } });

      const next = screen.getByTestId('status-next');
      expect(next).toHaveTextContent('Next: Translation');
      expect(next).toHaveAttribute('href', '/terminology');
    });

    it('offers Source Intake from the dashboard', () => {
      render(StatusBar, { props: { connectionState: 'connected', pathname: '/' } });

      expect(screen.getByTestId('status-next')).toHaveAttribute('href', '/hl7');
    });

    it('offers nothing after the last stage or off the stage routes', () => {
      const { unmount } = render(StatusBar, { props: { connectionState: 'connected', pathname: '/events' } });
      expect(screen.queryByTestId('status-next')).not.toBeInTheDocument();
      unmount();

      render(StatusBar, { props: { connectionState: 'connected', pathname: '/operator' } });
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
