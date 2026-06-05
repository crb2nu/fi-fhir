/**
 * Tests for the Button component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Button from './Button.svelte';

describe('Button', () => {
  describe('rendering', () => {
    it('should render with default variant (primary)', () => {
      render(Button);

      const button = screen.getByRole('button');
      expect(button).toHaveClass('btn', 'primary');
    });

    it('should render with secondary variant', () => {
      render(Button, { props: { variant: 'secondary' } });

      const button = screen.getByRole('button');
      expect(button).toHaveClass('btn', 'secondary');
    });

    it('should render with danger variant', () => {
      render(Button, { props: { variant: 'danger' } });

      const button = screen.getByRole('button');
      expect(button).toHaveClass('btn', 'danger');
    });
  });

  describe('disabled state', () => {
    it('should be enabled by default', () => {
      render(Button);

      const button = screen.getByRole('button');
      expect(button).not.toBeDisabled();
    });

    it('should be disabled when disabled prop is true', () => {
      render(Button, { props: { disabled: true } });

      const button = screen.getByRole('button');
      expect(button).toBeDisabled();
    });
  });

  describe('click handling', () => {
    it('should be clickable when enabled', async () => {
      render(Button);

      const button = screen.getByRole('button');
      // Just verify the button exists and is clickable (no error thrown)
      await fireEvent.click(button);

      expect(button).not.toBeDisabled();
    });

    it('should not be clickable when disabled', () => {
      render(Button, { props: { disabled: true } });

      const button = screen.getByRole('button');
      expect(button).toBeDisabled();
    });
  });

  describe('attribute forwarding', () => {
    it('forwards a title attribute to the native button (used for disabled-precondition tooltips)', () => {
      render(Button, { props: { disabled: true, title: 'Create or open a definition first' } });

      const button = screen.getByRole('button');
      expect(button).toHaveAttribute('title', 'Create or open a definition first');
      expect(button).toBeDisabled();
    });
  });
});
