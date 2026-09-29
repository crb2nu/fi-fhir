/**
 * Tests for the DebugPanel component.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DebugPanel from './DebugPanel.svelte';
import { endSession, startSession } from './debugStore';
import { mockSession } from './__fixtures__/debugFixtures';
import { workflowDraft } from '$lib/features/workflows/workflowStore';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

describe('DebugPanel', () => {
  beforeEach(() => {
    endSession();
  });

  describe('rendering', () => {
    it('should render step controls toolbar', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const toolbar = container.querySelector('[role="toolbar"]');
      expect(toolbar).not.toBeNull();
    });

    it('should render breakpoint list section', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const bpTitle = container.querySelector('.bp-title');
      expect(bpTitle).not.toBeNull();
      expect(bpTitle!.textContent).toBe('Breakpoints');
    });

    it('should render variable inspector section', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const sectionTitle = container.querySelector('.section-title');
      expect(sectionTitle).not.toBeNull();
      expect(sectionTitle!.textContent).toBe('Variables');
    });

    it('should render step history section', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const historyTitle = container.querySelector('.history-title');
      expect(historyTitle).not.toBeNull();
      expect(historyTitle!.textContent).toBe('Step History');
    });
  });

  describe('streaming honesty', () => {
    afterEach(() => {
      resetAccessCapabilities();
      resetObservedStreams();
    });

    it('says live debug steps are unavailable when the deployment cannot stream them', () => {
      setAccessStatus({
        authenticated: true,
        authVia: 'network',
        capabilities: {
          operatorRead: true,
          streaming: true,
          subscriptions: ['integrationSessionEvents', 'sessionRunEvents']
        }
      });
      render(DebugPanel);

      const note = screen.getByTestId('streaming-unavailable');
      expect(note).toHaveAttribute('data-stream', 'debugStepEvent');
      expect(note).toHaveAttribute('data-reason', 'not-allowlisted');
      expect(note).toHaveTextContent('Live streaming for debug steps is not available');
      // The session itself stays usable.
      expect(screen.getByRole('toolbar')).toBeInTheDocument();
    });

    it('shows no streaming note while capabilities are unknown', () => {
      render(DebugPanel);
      expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
    });
  });

  describe('with a session', () => {
    it('lists the session’s breakpoints', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      // The fixture session has 3 breakpoints.
      const bpItems = container.querySelectorAll('.bp-item');
      expect(bpItems.length).toBeGreaterThan(0);
    });

    it('should render variable inspector with current step variables', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      // Mock session has steps with variables, so var-entry elements should exist
      const varEntries = container.querySelectorAll('.var-entry');
      expect(varEntries.length).toBeGreaterThan(0);
    });

    it('should display step badge with current step info', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const stepBadge = container.querySelector('.step-badge');
      expect(stepBadge).not.toBeNull();
      // Mock data last step is step 3: webhook
      expect(stepBadge!.textContent).toContain('webhook');
    });

    it('should show step count in history section', () => {
      const { container } = (startSession(mockSession), render(DebugPanel));

      const historyCount = container.querySelector('.history-count');
      expect(historyCount).not.toBeNull();
      expect(historyCount!.textContent).toBe('3');
    });
  });

  describe('play preconditions (.loom/22 B1/B2)', () => {
    afterEach(() => {
      workflowDraft.reset();
    });

    it('disables Play and says why while the draft is invalid, instead of toasting', () => {
      workflowDraft.loadDraft({ name: '', version: '1.0', routes: [] });
      render(DebugPanel);

      expect(screen.getByRole('button', { name: 'Play' })).toBeDisabled();
      expect(screen.getByTestId('debug-play-blocked')).toHaveTextContent(
        'Play is unavailable: The workflow draft is not ready to debug: Workflow name is required (and 1 more in Problems).'
      );
    });

    it('enables Play for a valid draft and a JSON event', () => {
      workflowDraft.loadDraft({
        name: 'adt-routing',
        version: '1.0',
        routes: [
          {
            _key: 'r1',
            name: 'admits',
            filter: { eventTypes: ['PATIENT_ADMIT'], sources: [], condition: '' },
            transforms: [],
            actions: [{ _key: 'a1', type: 'log', config: {} }],
            expanded: true
          }
        ]
      });
      render(DebugPanel);

      expect(screen.getByRole('button', { name: 'Play' })).toBeEnabled();
      expect(screen.queryByTestId('debug-play-blocked')).not.toBeInTheDocument();
    });

    it('disables the breakpoint controls without a session', () => {
      render(DebugPanel);
      expect(screen.getByTitle(/Start a debug session/)).toBeDisabled();
    });
  });
});

