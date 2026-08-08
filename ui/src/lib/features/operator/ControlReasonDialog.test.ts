import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import ControlReasonDialog from './ControlReasonDialog.svelte';

function renderDialog(
  props: Record<string, unknown> = {},
  events: Record<string, (event: CustomEvent) => void> = {}
) {
  return render(ControlReasonDialog, {
    props: {
      open: true,
      title: 'Replay delivery attempt',
      confirmText: 'Replay',
      action: 'replay',
      targetId: 'attempt-a',
      ...props
    },
    events
  });
}

describe('ControlReasonDialog', () => {
  it('disables confirm until a reason is entered and says why', async () => {
    renderDialog();

    const confirm = screen.getByRole('button', { name: 'Replay' });
    expect(confirm).toBeDisabled();
    // B2: a control the operator cannot use explains itself instead of
    // allowing a dead click that a toast then rejects.
    expect(confirm).toHaveAttribute('title', expect.stringMatching(/reason is required/i));

    await fireEvent.input(screen.getByLabelText(/reason/i), {
      target: { value: 'Destination outage repaired' }
    });

    await waitFor(() => expect(confirm).toBeEnabled());
    expect(confirm).not.toHaveAttribute('title');
  });

  it('surfaces the missing reason inline when confirm is forced', async () => {
    const onConfirm = vi.fn();
    renderDialog({}, { confirm: onConfirm });

    // Whitespace passes the browser's own required check but not ours.
    await fireEvent.input(screen.getByLabelText(/reason/i), { target: { value: '   ' } });
    await fireEvent.click(screen.getByRole('button', { name: 'Replay' }));

    expect(onConfirm).not.toHaveBeenCalled();
  });

  it('emits the trimmed reason and a derived idempotency key', async () => {
    const onConfirm = vi.fn();
    renderDialog({}, { confirm: onConfirm });

    await fireEvent.input(screen.getByLabelText(/reason/i), {
      target: { value: '  Destination outage repaired  ' }
    });
    await fireEvent.click(screen.getByRole('button', { name: 'Replay' }));

    await waitFor(() => expect(onConfirm).toHaveBeenCalledTimes(1));
    const detail = onConfirm.mock.calls[0]?.[0]?.detail;
    expect(detail.reason).toBe('Destination outage repaired');
    expect(detail.idempotencyKey).toMatch(/^op-replay-attempt-a-/);
  });

  it('omits the idempotency key field for lifecycle commands', () => {
    renderDialog({ requiresIdempotencyKey: false, confirmText: 'Pause' });
    expect(screen.queryByLabelText(/idempotency key/i)).not.toBeInTheDocument();
  });

  it('renders a submission failure inline inside the dialog', () => {
    renderDialog({ submitError: 'Another operator changed this deployment first.' });
    const alert = screen.getByRole('alert');
    expect(alert).toHaveTextContent(/another operator changed this deployment first/i);
  });

  it('renders nothing when closed', () => {
    render(ControlReasonDialog, { props: { open: false } });
    expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
  });
});
