/**
 * Tests for the Input component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Input from './Input.svelte';

describe('Input', () => {
  describe('rendering', () => {
    it('should render an input element', () => {
      render(Input);

      const input = screen.getByRole('textbox');
      expect(input).toBeInTheDocument();
    });

    it('should render with text type by default', () => {
      render(Input);

      const input = screen.getByRole('textbox');
      expect(input).toHaveAttribute('type', 'text');
    });

    it('should render with provided placeholder', () => {
      render(Input, { props: { placeholder: 'Enter value...' } });

      const input = screen.getByPlaceholderText('Enter value...');
      expect(input).toBeInTheDocument();
    });
  });

  describe('label', () => {
    it('should render label when provided', () => {
      render(Input, { props: { label: 'Username' } });

      expect(screen.getByText('Username')).toBeInTheDocument();
    });

    it('should not render label when not provided', () => {
      const { container } = render(Input);

      expect(container.querySelector('.label')).not.toBeInTheDocument();
    });

    it('should show required indicator when required', () => {
      render(Input, { props: { label: 'Email', required: true } });

      expect(screen.getByText('*')).toBeInTheDocument();
    });

    it('should associate label with input via htmlFor', () => {
      render(Input, { props: { label: 'Username', id: 'test-input' } });

      const label = screen.getByText('Username');
      expect(label).toHaveAttribute('for', 'test-input');
    });
  });

  describe('input types', () => {
    const types = ['email', 'password', 'number', 'search', 'url', 'tel'] as const;

    types.forEach((type) => {
      it(`should render with ${type} type`, () => {
        const { container } = render(Input, { props: { type } });

        const input = container.querySelector('input');
        expect(input).toHaveAttribute('type', type);
      });
    });
  });

  describe('states', () => {
    it('should be enabled by default', () => {
      render(Input);

      const input = screen.getByRole('textbox');
      expect(input).not.toBeDisabled();
    });

    it('should be disabled when disabled prop is true', () => {
      render(Input, { props: { disabled: true } });

      const input = screen.getByRole('textbox');
      expect(input).toBeDisabled();
    });

    it('should be readonly when readonly prop is true', () => {
      render(Input, { props: { readonly: true } });

      const input = screen.getByRole('textbox');
      expect(input).toHaveAttribute('readonly');
    });

    it('should be required when required prop is true', () => {
      render(Input, { props: { required: true } });

      const input = screen.getByRole('textbox');
      expect(input).toHaveAttribute('required');
    });
  });

  describe('error state', () => {
    it('should show error message when error is provided', () => {
      render(Input, { props: { error: 'This field is required' } });

      expect(screen.getByText('This field is required')).toBeInTheDocument();
    });

    it('should have aria-invalid when error is present', () => {
      render(Input, { props: { error: 'Invalid' } });

      const input = screen.getByRole('textbox');
      expect(input).toHaveAttribute('aria-invalid', 'true');
    });

    it('should have role="alert" on error message', () => {
      render(Input, { props: { error: 'Error message' } });

      const errorMsg = screen.getByRole('alert');
      expect(errorMsg).toHaveTextContent('Error message');
    });

    it('should not show error when not provided', () => {
      const { container } = render(Input);

      expect(container.querySelector('.error-message')).not.toBeInTheDocument();
    });
  });

  describe('hint', () => {
    it('should show hint when provided', () => {
      render(Input, { props: { hint: 'Enter your email address' } });

      expect(screen.getByText('Enter your email address')).toBeInTheDocument();
    });

    it('should hide hint when error is present', () => {
      render(Input, { props: { hint: 'Hint text', error: 'Error text' } });

      expect(screen.queryByText('Hint text')).not.toBeInTheDocument();
      expect(screen.getByText('Error text')).toBeInTheDocument();
    });
  });

  describe('sizes', () => {
    it('should render with default size (md)', () => {
      const { container } = render(Input);

      const input = container.querySelector('.input');
      expect(input).toHaveClass('md');
    });

    it('should render with small size', () => {
      const { container } = render(Input, { props: { size: 'sm' } });

      const input = container.querySelector('.input');
      expect(input).toHaveClass('sm');
    });

    it('should render with large size', () => {
      const { container } = render(Input, { props: { size: 'lg' } });

      const input = container.querySelector('.input');
      expect(input).toHaveClass('lg');
    });
  });

  describe('value binding', () => {
    it('should display initial value', () => {
      render(Input, { props: { value: 'initial value' } });

      const input = screen.getByRole('textbox');
      expect(input).toHaveValue('initial value');
    });

    it('should update value on input', async () => {
      render(Input, { props: { value: '' } });

      const input = screen.getByRole('textbox');
      await fireEvent.input(input, { target: { value: 'new value' } });

      expect(input).toHaveValue('new value');
    });
  });

  describe('fullWidth', () => {
    it('should have full-width class by default', () => {
      const { container } = render(Input);

      expect(container.querySelector('.input-wrapper')).toHaveClass('full-width');
    });

    it('should not have full-width class when fullWidth is false', () => {
      const { container } = render(Input, { props: { fullWidth: false } });

      expect(container.querySelector('.input-wrapper')).not.toHaveClass('full-width');
    });
  });
});
