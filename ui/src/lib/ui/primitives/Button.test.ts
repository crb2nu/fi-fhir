import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import Play from '@lucide/svelte/icons/play';
import Button from './Button.svelte';
import { text } from './test-utils';

describe('Button', () => {
  it('renders a secondary sm button of type="button" by default', () => {
    render(Button, { props: { children: text('Save') } });
    const button = screen.getByRole('button', { name: 'Save' });
    expect(button).toHaveAttribute('type', 'button');
    expect(button).toHaveClass('ui-button', 'ui-button--secondary', 'ui-button--sm');
    expect(button).toHaveAttribute('data-variant', 'secondary');
  });

  it.each(['primary', 'secondary', 'ghost', 'danger'] as const)('applies the %s variant', (variant) => {
    render(Button, { props: { variant, children: text(variant) } });
    expect(screen.getByRole('button')).toHaveClass(`ui-button--${variant}`);
  });

  it('applies the md size and icon-only shape', () => {
    render(Button, { props: { size: 'md', iconOnly: true, 'aria-label': 'Refresh', icon: Play } });
    const button = screen.getByRole('button', { name: 'Refresh' });
    expect(button).toHaveClass('ui-button--md', 'ui-button--icon-only');
    expect(button.querySelector('svg')).toHaveAttribute('aria-hidden', 'true');
  });

  it('passes class, data-testid and other attributes through', () => {
    render(Button, {
      props: { class: 'extra', 'data-testid': 'run-button', title: 'Run now', children: text('Run') }
    });
    const button = screen.getByTestId('run-button');
    expect(button).toHaveClass('ui-button', 'extra');
    expect(button).toHaveAttribute('title', 'Run now');
  });

  it('calls onclick and disables natively', async () => {
    const onclick = vi.fn();
    const { rerender } = render(Button, { props: { onclick, children: text('Go') } });
    await fireEvent.click(screen.getByRole('button'));
    expect(onclick).toHaveBeenCalledTimes(1);

    // Native `disabled`: browsers never dispatch user clicks to it.
    await rerender({ onclick, disabled: true, children: text('Go') });
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('shows a spinner, sets aria-busy and disables itself while loading', () => {
    render(Button, { props: { loading: true, icon: Play, children: text('Run') } });
    const button = screen.getByRole('button', { name: 'Run' });
    expect(button).toBeDisabled();
    expect(button).toHaveAttribute('aria-busy', 'true');
    expect(button.querySelector('.ui-button-spinner')).not.toBeNull();
    expect(button.querySelector('svg')).toBeNull(); // spinner replaces the icon
  });
});
