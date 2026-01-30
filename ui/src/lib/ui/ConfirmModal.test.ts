/**
 * Tests for the ConfirmModal component.
 */
import { describe, it, expect } from 'vitest';
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

    it('should have aria-labelledby for title', () => {
      render(ConfirmModal, { props: { open: true } });

      const dialog = screen.getByRole('dialog');
      expect(dialog).toHaveAttribute('aria-labelledby', 'modal-title');
    });

    it('should have aria-describedby for message', () => {
      render(ConfirmModal, { props: { open: true } });

      const dialog = screen.getByRole('dialog');
      expect(dialog).toHaveAttribute('aria-describedby', 'modal-message');
    });

    it('should have backdrop with close aria-label', () => {
      render(ConfirmModal, { props: { open: true } });

      const backdrop = screen.getByRole('button', { name: 'Close dialog' });
      expect(backdrop).toBeInTheDocument();
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

    it('should close when backdrop is clicked', async () => {
      render(ConfirmModal, { props: { open: true } });

      const backdrop = screen.getByRole('button', { name: 'Close dialog' });
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
      expect(confirmButton).toHaveClass('primary');
    });

    it('should render with danger variant when specified', () => {
      render(ConfirmModal, { props: { open: true, variant: 'danger' } });

      const confirmButton = screen.getByRole('button', { name: 'Confirm' });
      expect(confirmButton).toHaveClass('danger');
    });
  });
});
