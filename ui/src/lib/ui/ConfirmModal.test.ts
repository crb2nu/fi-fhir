/**
 * Tests for the ConfirmModal component.
 */
import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent, waitFor } from '@testing-library/svelte';
import ConfirmModal from './ConfirmModal.svelte';

describe('ConfirmModal', () => {
  describe('visibility', () => {
    it('should not render when open is false', () => {
      render(ConfirmModal, { props: { open: false } });

      expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
    });

    it('should render when open is true', () => {
      render(ConfirmModal, { props: { open: true } });

      expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
  });

  describe('content', () => {
    it('should display default title and message', () => {
      render(ConfirmModal, { props: { open: true } });

      // Title is in h3 with id="modal-title"
      const title = screen.getByRole('heading', { name: 'Confirm' });
      expect(title).toBeInTheDocument();
      expect(screen.getByText('Are you sure?')).toBeInTheDocument();
    });

    it('should display custom title and message', () => {
      render(ConfirmModal, {
        props: {
          open: true,
          title: 'Delete Item',
          message: 'This action cannot be undone.'
        }
      });

      expect(screen.getByText('Delete Item')).toBeInTheDocument();
      expect(screen.getByText('This action cannot be undone.')).toBeInTheDocument();
    });

    it('should display custom button text', () => {
      render(ConfirmModal, {
        props: {
          open: true,
          confirmText: 'Yes, delete',
          cancelText: 'No, keep it'
        }
      });

      expect(screen.getByRole('button', { name: 'Yes, delete' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'No, keep it' })).toBeInTheDocument();
    });

    it('should display default button text', () => {
      render(ConfirmModal, { props: { open: true } });

      expect(screen.getByRole('button', { name: 'Confirm' })).toBeInTheDocument();
      expect(screen.getByRole('button', { name: 'Cancel' })).toBeInTheDocument();
    });
  });

  describe('accessibility', () => {
    it('should have dialog role with aria-modal', () => {
      render(ConfirmModal, { props: { open: true } });

      const dialog = screen.getByRole('dialog');
      expect(dialog).toHaveAttribute('aria-modal', 'true');
    });

    it('is labelled by its title and described by its message', () => {
      render(ConfirmModal, { props: { open: true, title: 'Delete Item', message: 'This action cannot be undone.' } });

      const dialog = screen.getByRole('dialog', { name: 'Delete Item' });
      const described = document.getElementById(dialog.getAttribute('aria-describedby') ?? '');
      expect(described).toHaveTextContent('This action cannot be undone.');
    });

    it('has a close button labelled for assistive tech', () => {
      render(ConfirmModal, { props: { open: true } });

      expect(screen.getByRole('button', { name: 'Close dialog' })).toBeInTheDocument();
    });

    it('cancels on Escape and dispatches cancel', async () => {
      const onCancel = vi.fn();
      render(ConfirmModal, { props: { open: true }, events: { cancel: onCancel } });

      await fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });

      expect(onCancel).toHaveBeenCalledTimes(1);
      await waitFor(() => expect(screen.queryByRole('dialog')).not.toBeInTheDocument());
    });

    it('ignores Escape while loading', async () => {
      render(ConfirmModal, { props: { open: true, loading: true } });

      await fireEvent.keyDown(screen.getByRole('dialog'), { key: 'Escape' });

      expect(screen.getByRole('dialog')).toBeInTheDocument();
    });
  });

  describe('interactions', () => {
    it('should close when cancel button is clicked', async () => {
      render(ConfirmModal, { props: { open: true } });

      const cancelButton = screen.getByRole('button', { name: 'Cancel' });
      await fireEvent.click(cancelButton);

      // The modal sets open = false internally
      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      });
    });

    it('should close when confirm button is clicked', async () => {
      render(ConfirmModal, { props: { open: true } });

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      await fireEvent.click(confirmButton);

      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      });
    });

    it('should close when the backdrop is clicked', async () => {
      render(ConfirmModal, { props: { open: true } });

      const backdrop = screen.getByRole('dialog').parentElement!;
      await fireEvent.click(backdrop);

      await waitFor(() => {
        expect(screen.queryByRole('dialog')).not.toBeInTheDocument();
      });
    });
  });

  describe('variants', () => {
    it('should render with primary variant by default', () => {
      render(ConfirmModal, { props: { open: true } });

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      expect(confirmButton).toHaveAttribute('data-variant', 'primary');
    });

    it('should render with danger variant when specified', () => {
      render(ConfirmModal, { props: { open: true, variant: 'danger' } });

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      expect(confirmButton).toHaveAttribute('data-variant', 'danger');
    });
  });
});
