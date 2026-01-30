/**
 * Tests for the WarningList component.
 *
 * Tests the warning display, filtering, grouping, and event dispatch functionality.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import { SvelteSet } from 'svelte/reactivity';
import WarningList from './WarningList.svelte';
import type { WarningGroup, WarningLike } from '$lib/domain/warnings';

// Helper to create test warnings
function createWarning(overrides: Partial<WarningLike> = {}): WarningLike {
  return {
    phase: 'syntactic',
    code: 'W001',
    message: 'Test warning message',
    path: 'MSH.1',
    ...overrides
  };
}

// Helper to create warning groups
function createGroups(warnings: WarningLike[]): WarningGroup[] {
  const map = new Map<string, WarningLike[]>();
  for (const w of warnings) {
    const existing = map.get(w.phase);
    if (existing) {
      existing.push(w);
    } else {
      map.set(w.phase, [w]);
    }
  }
  return Array.from(map.entries()).map(([phase, items]) => ({ phase, items }));
}

describe('WarningList', () => {
  beforeEach(() => {
    vi.resetAllMocks();
  });

  describe('empty state', () => {
    it('should show "No warnings" when groups is empty', () => {
      render(WarningList, { props: { groups: [] } });

      expect(screen.getByText('No warnings')).toBeInTheDocument();
    });
  });

  describe('warning display', () => {
    it('should display warning code and message', () => {
      const warning = createWarning({ code: 'SEG_MISSING', message: 'Required segment PID is missing' });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByText('SEG_MISSING')).toBeInTheDocument();
      expect(screen.getByText('Required segment PID is missing')).toBeInTheDocument();
    });

    it('should display warning path when present', () => {
      const warning = createWarning({ path: 'PID.3.1' });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByText('PID.3.1')).toBeInTheDocument();
    });

    it('should group warnings by phase', () => {
      // Create groups directly to ensure proper structure
      const groups: WarningGroup[] = [
        { phase: 'syntactic', items: [createWarning({ phase: 'syntactic', code: 'W001' })] },
        { phase: 'semantic', items: [createWarning({ phase: 'semantic', code: 'W002' })] }
      ];

      render(WarningList, { props: { groups } });

      // Phase chips should be visible
      expect(screen.getByRole('button', { name: 'syntactic' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'semantic' })).toBeInTheDocument();
    });

    it('should display group counts', () => {
      const warnings = [
        createWarning({ phase: 'syntactic', code: 'W001' }),
        createWarning({ phase: 'syntactic', code: 'W002' })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      // The count badge should show "2" for the syntactic group
      const counts = screen.getAllByText('2');
      expect(counts.length).toBeGreaterThan(0);
    });
  });

  describe('filtering', () => {
    it('should render search input for filtering', () => {
      const warnings = [
        createWarning({ code: 'SEG_MISSING', message: 'Segment missing' }),
        createWarning({ code: 'FIELD_EMPTY', message: 'Field is empty' })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      // Verify the filter input exists
      const input = screen.getByPlaceholderText(/filter warnings/i);
      expect(input).toBeInTheDocument();
    });

    it('should filter by phase using chip buttons', async () => {
      const groups: WarningGroup[] = [
        { phase: 'syntactic', items: [createWarning({ phase: 'syntactic', code: 'W001' })] },
        { phase: 'semantic', items: [createWarning({ phase: 'semantic', code: 'W002' })] }
      ];

      render(WarningList, { props: { groups } });

      // Click on syntactic chip
      const syntacticChip = screen.getByRole('button', { name: 'syntactic' });
      await fireEvent.click(syntacticChip);

      await waitFor(() => {
        expect(screen.getByText('W001')).toBeInTheDocument();
        expect(screen.queryByText('W002')).not.toBeInTheDocument();
      });
    });

    it('should show all warnings when "all" chip is clicked', async () => {
      const groups: WarningGroup[] = [
        { phase: 'syntactic', items: [createWarning({ phase: 'syntactic', code: 'W001' })] },
        { phase: 'semantic', items: [createWarning({ phase: 'semantic', code: 'W002' })] }
      ];

      render(WarningList, { props: { groups } });

      // First filter to syntactic
      const syntacticChip = screen.getByRole('button', { name: 'syntactic' });
      await fireEvent.click(syntacticChip);

      // Then click all
      const allChip = screen.getByRole('button', { name: 'all' });
      await fireEvent.click(allChip);

      await waitFor(() => {
        expect(screen.getByText('W001')).toBeInTheDocument();
        expect(screen.getByText('W002')).toBeInTheDocument();
      });
    });

    it('should render "Has path" checkbox', () => {
      const groups: WarningGroup[] = [
        {
          phase: 'syntactic',
          items: [
            createWarning({ code: 'W001', path: 'MSH.1' }),
            createWarning({ code: 'W002', path: null })
          ]
        }
      ];

      render(WarningList, { props: { groups } });

      const checkbox = screen.getByLabelText('Has path');
      expect(checkbox).toBeInTheDocument();
    });

    it('should show count of filtered vs total warnings', () => {
      const warnings = [
        createWarning({ code: 'W001' }),
        createWarning({ code: 'W002' }),
        createWarning({ code: 'W003' })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      expect(screen.getByText('3/3')).toBeInTheDocument();
    });

    it('should show Clear button when input has value', async () => {
      const warnings = [
        createWarning({ code: 'W001' }),
        createWarning({ code: 'W002' })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      const input = screen.getByPlaceholderText(/filter warnings/i) as HTMLInputElement;

      // Initially, Clear button should not be visible
      expect(screen.queryByRole('button', { name: 'Clear' })).not.toBeInTheDocument();

      // Type something
      await fireEvent.input(input, { target: { value: 'test' } });

      // After input, Clear button should appear
      await waitFor(() => {
        expect(screen.getByRole('button', { name: 'Clear' })).toBeInTheDocument();
      });
    });
  });

  describe('LLM explanations', () => {
    it('should show "Explain" button for warnings without explanation', () => {
      const warning = createWarning({ explanation: null });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByRole('button', { name: 'Explain' })).toBeInTheDocument();
    });

    it('should not show "Explain" button for warnings with explanation', () => {
      const warning = createWarning({ explanation: 'This is an explanation' });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.queryByRole('button', { name: 'Explain' })).not.toBeInTheDocument();
    });

    it('should show loading state for explain button', () => {
      const warning = createWarning({ code: 'W001', explanation: null });
      const groups = createGroups([warning]);
      const loadingCodes = new SvelteSet(['W001']);

      render(WarningList, { props: { groups, explainLoadingCodes: loadingCodes } });

      expect(screen.getByRole('button', { name: '...' })).toBeInTheDocument();
    });

    it('should show explanation toggle when explanation exists', () => {
      const warning = createWarning({
        explanation: 'This warning indicates a syntax error',
        fixSuggestion: 'Add the missing segment'
      });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByText(/View Explanation/)).toBeInTheDocument();
    });

    it('should render explanation toggle button', () => {
      const warning = createWarning({
        explanation: 'This warning indicates a syntax error',
        fixSuggestion: 'Add the missing segment'
      });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      const toggle = screen.getByText(/View Explanation/);
      expect(toggle).toBeInTheDocument();
      expect(toggle.tagName).toBe('BUTTON');
    });

    it('should show "Explain All" button when there are unexplained warnings', () => {
      const warnings = [
        createWarning({ code: 'W001', explanation: null }),
        createWarning({ code: 'W002', explanation: null })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      expect(screen.getByText(/Explain All \(2\)/)).toBeInTheDocument();
    });

    it('should not show "Explain All" when all warnings are explained', () => {
      const warnings = [
        createWarning({ code: 'W001', explanation: 'Explained' }),
        createWarning({ code: 'W002', explanation: 'Also explained' })
      ];
      const groups = createGroups(warnings);

      render(WarningList, { props: { groups } });

      expect(screen.queryByText(/Explain All/)).not.toBeInTheDocument();
    });

    it('should show cache badge for cached explanations', () => {
      const warning = createWarning({
        explanation: 'Cached explanation',
        fromCache: true
      });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByText('cached')).toBeInTheDocument();
    });

    it('should render explanation toggle for warnings with impact', () => {
      const warning = createWarning({
        explanation: 'Test explanation',
        impact: 'May cause data loss'
      });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      // The toggle exists, meaning the explanation container is rendered
      const toggle = screen.getByText(/View Explanation/);
      expect(toggle).toBeInTheDocument();
    });
  });

  describe('controls visibility', () => {
    it('should show controls by default', () => {
      const groups = createGroups([createWarning()]);

      render(WarningList, { props: { groups } });

      expect(screen.getByPlaceholderText(/filter warnings/i)).toBeInTheDocument();
    });

    it('should hide controls when enableControls is false', () => {
      const groups = createGroups([createWarning()]);

      render(WarningList, { props: { groups, enableControls: false } });

      expect(screen.queryByPlaceholderText(/filter warnings/i)).not.toBeInTheDocument();
    });
  });

  describe('selection', () => {
    it('should highlight selected warning by path', () => {
      const warnings = [
        createWarning({ code: 'W001', path: 'MSH.1' }),
        createWarning({ code: 'W002', path: 'PID.3' })
      ];
      const groups = createGroups(warnings);

      const { container } = render(WarningList, {
        props: { groups, selectedPath: 'MSH.1' }
      });

      const selectedItem = container.querySelector('[data-selected="true"]');
      expect(selectedItem).toBeInTheDocument();
    });
  });

  describe('action buttons', () => {
    it('should show Copy button for warnings with path', () => {
      const warning = createWarning({ path: 'MSH.1' });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByRole('button', { name: 'Copy' })).toBeInTheDocument();
    });

    it('should show Inspect button for warnings with path', () => {
      const warning = createWarning({ path: 'MSH.1' });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.getByRole('button', { name: 'Inspect' })).toBeInTheDocument();
    });

    it('should not show Copy/Inspect for warnings without path', () => {
      const warning = createWarning({ path: null });
      const groups = createGroups([warning]);

      render(WarningList, { props: { groups } });

      expect(screen.queryByRole('button', { name: 'Copy' })).not.toBeInTheDocument();
      expect(screen.queryByRole('button', { name: 'Inspect' })).not.toBeInTheDocument();
    });
  });
});
