import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import Settings from '@lucide/svelte/icons/settings';
import IconButton from './IconButton.svelte';

describe('IconButton', () => {
  it('uses label as the accessible name and the tooltip', () => {
    render(IconButton, { props: { icon: Settings, label: 'Settings' } });
    const button = screen.getByRole('button', { name: 'Settings' });
    expect(button).toHaveAttribute('title', 'Settings');
    expect(button).toHaveClass('ui-button--ghost', 'ui-button--icon-only');
    expect(button.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
  });

  it('renders aria-pressed only for toggles', async () => {
    const { rerender } = render(IconButton, { props: { icon: Settings, label: 'Trace' } });
    expect(screen.getByRole('button')).not.toHaveAttribute('aria-pressed');

    await rerender({ icon: Settings, label: 'Trace', pressed: true });
    expect(screen.getByRole('button', { name: 'Trace', pressed: true })).toBeInTheDocument();

    await rerender({ icon: Settings, label: 'Trace', pressed: false });
    expect(screen.getByRole('button')).toHaveAttribute('aria-pressed', 'false');
  });

  it('passes onclick, variant and data-testid through', async () => {
    const onclick = vi.fn();
    render(IconButton, {
      props: { icon: Settings, label: 'Open', variant: 'secondary', onclick, 'data-testid': 'open' }
    });
    const button = screen.getByTestId('open');
    expect(button).toHaveClass('ui-button--secondary');
    await fireEvent.click(button);
    expect(onclick).toHaveBeenCalledOnce();
  });
});
