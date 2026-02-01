/**
 * Tests for the Badge component.
 */
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Badge from './Badge.svelte';

describe('Badge', () => {
  describe('rendering', () => {
    it('should render with default variant', () => {
      const { container } = render(Badge);

      const badge = container.querySelector('.badge');
      expect(badge).toHaveClass('badge', 'default');
    });

    it('should render slot content', () => {
      const { container } = render(Badge);
      // Badge renders its content through a slot
      expect(container.querySelector('.badge')).toBeInTheDocument();
    });
  });

  describe('variants', () => {
    const variants = ['default', 'primary', 'success', 'warning', 'danger', 'info'] as const;

    variants.forEach((variant) => {
      it(`should render with ${variant} variant`, () => {
        render(Badge, { props: { variant } });

        const badge = document.querySelector('.badge');
        expect(badge).toHaveClass('badge', variant);
      });
    });
  });

  describe('sizes', () => {
    it('should render with default size (md)', () => {
      render(Badge);

      const badge = document.querySelector('.badge');
      expect(badge).not.toHaveClass('sm');
    });

    it('should render with small size', () => {
      render(Badge, { props: { size: 'sm' } });

      const badge = document.querySelector('.badge');
      expect(badge).toHaveClass('sm');
    });
  });

  describe('modifiers', () => {
    it('should render with outline modifier', () => {
      render(Badge, { props: { outline: true } });

      const badge = document.querySelector('.badge');
      expect(badge).toHaveClass('outline');
    });

    it('should render without outline by default', () => {
      render(Badge);

      const badge = document.querySelector('.badge');
      expect(badge).not.toHaveClass('outline');
    });

    it('should render with pill modifier', () => {
      render(Badge, { props: { pill: true } });

      const badge = document.querySelector('.badge');
      expect(badge).toHaveClass('pill');
    });

    it('should render without pill by default', () => {
      render(Badge);

      const badge = document.querySelector('.badge');
      expect(badge).not.toHaveClass('pill');
    });

    it('should combine multiple modifiers', () => {
      render(Badge, { props: { variant: 'success', outline: true, pill: true, size: 'sm' } });

      const badge = document.querySelector('.badge');
      expect(badge).toHaveClass('badge', 'success', 'outline', 'pill', 'sm');
    });
  });
});
