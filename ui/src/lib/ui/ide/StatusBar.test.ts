/**
 * Tests for the StatusBar component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
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

    it('should display Disconnected when disconnected', () => {
      render(StatusBar, { props: { connectionState: 'disconnected', activeProfile: '', parserStatus: '' } });

      expect(screen.getByText('Disconnected')).toBeInTheDocument();
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
});
