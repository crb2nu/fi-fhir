/**
 * Tests for BreakpointList — focused on session-gated preconditions (UX policy B2/D2).
 *
 * Adding/toggling a breakpoint requires an active debug session. Rather than
 * letting a dead click fire a "Start a debug session…" toast, the controls are
 * disabled with an explanatory tooltip when no session is active.
 */
import { describe, it, expect, afterEach, vi } from 'vitest';
import { render, cleanup, fireEvent } from '@testing-library/svelte';
import BreakpointList from './BreakpointList.svelte';
import type { Breakpoint } from './types';

const NO_SESSION_HINT = 'Start a debug session to manage breakpoints';

const breakpoints: Breakpoint[] = [
  { id: 'bp1', type: 'route', name: 'orders', enabled: true }
];

afterEach(() => cleanup());

describe('BreakpointList session gating', () => {
  it('disables the add control with an explanatory tooltip when no session', () => {
    const { getByLabelText } = render(BreakpointList, { props: { breakpoints, hasSession: false } });

    const addBtn = getByLabelText('Add breakpoint');
    expect(addBtn).toBeDisabled();
    expect(addBtn).toHaveAttribute('title', NO_SESSION_HINT);
  });

  it('disables the toggle checkbox when no session', () => {
    const { getByLabelText } = render(BreakpointList, { props: { breakpoints, hasSession: false } });

    expect(getByLabelText('Toggle orders')).toBeDisabled();
  });

  it('keeps the remove control enabled when no session (remove works without one)', () => {
    const { getByLabelText } = render(BreakpointList, { props: { breakpoints, hasSession: false } });

    expect(getByLabelText('Remove orders')).not.toBeDisabled();
  });

  it('does not invoke onAdd via the disabled control when no session', async () => {
    const onAdd = vi.fn();
    const { getByLabelText, queryByRole } = render(BreakpointList, {
      props: { breakpoints, hasSession: false, onAdd }
    });

    await fireEvent.click(getByLabelText('Add breakpoint'));
    // Disabled add button cannot open the form, so no add can be dispatched.
    expect(queryByRole('form', { name: 'Add breakpoint form' })).toBeNull();
    expect(onAdd).not.toHaveBeenCalled();
  });

  it('enables add and toggle when a session is active', () => {
    const { getByLabelText } = render(BreakpointList, { props: { breakpoints, hasSession: true } });

    const addBtn = getByLabelText('Add breakpoint');
    expect(addBtn).not.toBeDisabled();
    expect(addBtn).toHaveAttribute('title', 'Add breakpoint');
    expect(getByLabelText('Toggle orders')).not.toBeDisabled();
  });

  it('defaults hasSession to true (backward compatible)', () => {
    const { getByLabelText } = render(BreakpointList, { props: { breakpoints } });

    expect(getByLabelText('Add breakpoint')).not.toBeDisabled();
  });
});
