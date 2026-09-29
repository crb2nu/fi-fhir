import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { tick } from 'svelte';
import DialogFixture from './__fixtures__/DialogFixture.svelte';

async function openFromTrigger(): Promise<HTMLElement> {
  const opener = screen.getByTestId('opener');
  opener.focus();
  await fireEvent.click(opener);
  await tick();
  await tick();
  return opener;
}

describe('Dialog', () => {
  it('renders nothing while closed', () => {
    render(DialogFixture);
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('is a modal dialog labelled by its title and described by its description', async () => {
    render(DialogFixture);
    await openFromTrigger();

    const dialog = screen.getByRole('dialog', { name: 'Rename definition' });
    expect(dialog).toHaveAttribute('aria-modal', 'true');
    expect(dialog).toHaveAttribute('data-testid', 'fixture-dialog');
    const described = document.getElementById(dialog.getAttribute('aria-describedby') ?? '');
    expect(described).toHaveTextContent('The new name is recorded in the audit trail.');
    expect(screen.getByRole('heading', { name: 'Rename definition' })).toBeInTheDocument();
  });

  it('focuses the initialFocus element on open', async () => {
    render(DialogFixture);
    await openFromTrigger();
    await waitFor(() => expect(screen.getByTestId('second')).toHaveFocus());
  });

  it('keeps Tab and Shift+Tab inside the dialog', async () => {
    render(DialogFixture);
    await openFromTrigger();
    const dialog = screen.getByRole('dialog');
    const done = screen.getByRole('button', { name: 'Done' });

    // jsdom has no layout, so the trap sees no tabbable element and holds
    // focus on the dialog itself; in a browser it cycles first <-> last.
    done.focus();
    await fireEvent.keyDown(dialog, { key: 'Tab' });
    expect(dialog.contains(document.activeElement)).toBe(true);

    await fireEvent.keyDown(dialog, { key: 'Tab', shiftKey: true });
    expect(dialog.contains(document.activeElement)).toBe(true);
    expect(screen.getByTestId('opener')).not.toHaveFocus();
  });

  it('closes on Escape, consumes the key, and returns focus to the opener', async () => {
    render(DialogFixture);
    const opener = await openFromTrigger();
    const pageHandler = vi.fn();
    window.addEventListener('keydown', pageHandler);
    try {
      await fireEvent.keyDown(screen.getByTestId('second'), { key: 'Escape' });
      await tick();
      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      expect(opener).toHaveFocus();
      expect(pageHandler).not.toHaveBeenCalled();
    } finally {
      window.removeEventListener('keydown', pageHandler);
    }
  });

  it('closes on a backdrop click but not on a click inside', async () => {
    render(DialogFixture);
    await openFromTrigger();
    const dialog = screen.getByRole('dialog');

    await fireEvent.click(dialog);
    expect(screen.getByRole('dialog')).toBeInTheDocument();

    await fireEvent.click(dialog.parentElement!);
    await tick();
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });

  it('stays open on Escape, backdrop and close when not dismissible', async () => {
    render(DialogFixture, { props: { dismissible: false } });
    await openFromTrigger();
    const dialog = screen.getByRole('dialog');

    await fireEvent.keyDown(dialog, { key: 'Escape' });
    await fireEvent.click(dialog.parentElement!);
    expect(screen.getByRole('button', { name: 'Close dialog' })).toBeDisabled();
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('leaves closing to the owner when onclose is given', async () => {
    const onclose = vi.fn();
    render(DialogFixture, { props: { onclose } });
    await openFromTrigger();

    await fireEvent.click(screen.getByRole('button', { name: 'Close dialog' }));
    expect(onclose).toHaveBeenCalledTimes(1);
    expect(screen.getByRole('dialog')).toBeInTheDocument();
  });

  it('bare layout keeps the accessible name without a visible header or footer', async () => {
    render(DialogFixture, { props: { layout: 'bare' } });
    await openFromTrigger();

    expect(screen.getByRole('dialog', { name: 'Rename definition' })).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Close dialog' })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: 'Done' })).not.toBeInTheDocument();
  });
});
