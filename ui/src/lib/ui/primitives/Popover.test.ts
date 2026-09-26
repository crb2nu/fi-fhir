import { describe, expect, it } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import PopoverFixture from './__fixtures__/PopoverFixture.svelte';

function trigger(): HTMLElement {
  return screen.getByRole('button', { name: 'About this view' });
}

describe('Popover', () => {
  it('wires the trigger with aria-expanded/aria-controls/aria-haspopup', () => {
    render(PopoverFixture);
    expect(trigger()).toHaveAttribute('aria-expanded', 'false');
    expect(trigger()).toHaveAttribute('aria-haspopup', 'dialog');
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('opens on click as a labelled dialog that takes focus', async () => {
    render(PopoverFixture);
    await fireEvent.click(trigger());
    const dialog = screen.getByRole('dialog', { name: 'About this view' });
    expect(dialog).toHaveAttribute('data-testid', 'help-popover');
    expect(trigger()).toHaveAttribute('aria-expanded', 'true');
    expect(trigger()).toHaveAttribute('aria-controls', dialog.id);
    await waitFor(() => expect(dialog).toHaveFocus());
  });

  it('closes on Escape and returns focus to the trigger', async () => {
    render(PopoverFixture, { props: { open: true } });
    expect(screen.getByRole('dialog')).toBeInTheDocument();
    await fireEvent.keyDown(document, { key: 'Escape' });
    expect(screen.queryByRole('dialog')).toBeNull();
    await waitFor(() => expect(trigger()).toHaveFocus());
  });

  it('closes on an outside pointer-down but not on one inside', async () => {
    render(PopoverFixture, { props: { open: true } });
    await fireEvent.pointerDown(screen.getByRole('button', { name: 'Inside action' }));
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    await fireEvent.pointerDown(screen.getByTestId('outside'));
    expect(screen.queryByRole('dialog')).toBeNull();
  });

  it('toggles closed from the trigger', async () => {
    render(PopoverFixture);
    await fireEvent.click(trigger());
    await fireEvent.click(trigger());
    expect(screen.queryByRole('dialog')).toBeNull();
  });
});
