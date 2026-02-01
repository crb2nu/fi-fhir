/**
 * Tests for the Skeleton component.
 */
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import Skeleton from './Skeleton.svelte';

describe('Skeleton', () => {
  describe('variants', () => {
    it('should render with text variant by default', () => {
      const { container } = render(Skeleton);

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveClass('text');
    });

    it('should render with circular variant', () => {
      const { container } = render(Skeleton, { props: { variant: 'circular' } });

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveClass('circular');
    });

    it('should render with rectangular variant', () => {
      const { container } = render(Skeleton, { props: { variant: 'rectangular' } });

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveClass('rectangular');
    });
  });

  describe('animation', () => {
    it('should have animation enabled by default', () => {
      const { container } = render(Skeleton);

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveClass('animate');
    });

    it('should not animate when animate is false', () => {
      const { container } = render(Skeleton, { props: { animate: false } });

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).not.toHaveClass('animate');
    });
  });

  describe('multiple lines', () => {
    it('should render single skeleton for lines=1', () => {
      const { container } = render(Skeleton, { props: { lines: 1 } });

      const skeletons = container.querySelectorAll('.skeleton');
      expect(skeletons).toHaveLength(1);
    });

    it('should render multiple skeletons for lines > 1', () => {
      const { container } = render(Skeleton, { props: { variant: 'text', lines: 3 } });

      const skeletons = container.querySelectorAll('.skeleton');
      expect(skeletons).toHaveLength(3);
    });

    it('should wrap multiple lines in skeleton-group', () => {
      const { container } = render(Skeleton, { props: { variant: 'text', lines: 3 } });

      expect(container.querySelector('.skeleton-group')).toBeInTheDocument();
    });

    it('should mark last line with last class', () => {
      const { container } = render(Skeleton, { props: { variant: 'text', lines: 3 } });

      const skeletons = container.querySelectorAll('.skeleton');
      expect(skeletons[2]).toHaveClass('last');
      expect(skeletons[0]).not.toHaveClass('last');
      expect(skeletons[1]).not.toHaveClass('last');
    });
  });

  describe('custom dimensions', () => {
    it('should apply custom width', () => {
      const { container } = render(Skeleton, { props: { width: '200px' } });

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveStyle({ width: '200px' });
    });

    it('should apply custom height', () => {
      const { container } = render(Skeleton, { props: { height: '50px' } });

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveStyle({ height: '50px' });
    });

    it('should apply width to skeleton-group for multi-line', () => {
      const { container } = render(Skeleton, { props: { variant: 'text', lines: 2, width: '300px' } });

      const group = container.querySelector('.skeleton-group');
      expect(group).toHaveStyle({ width: '300px' });
    });
  });

  describe('accessibility', () => {
    it('should have aria-hidden="true"', () => {
      const { container } = render(Skeleton);

      const skeleton = container.querySelector('.skeleton');
      expect(skeleton).toHaveAttribute('aria-hidden', 'true');
    });

    it('should have aria-hidden on all lines in multi-line mode', () => {
      const { container } = render(Skeleton, { props: { variant: 'text', lines: 3 } });

      const skeletons = container.querySelectorAll('.skeleton');
      skeletons.forEach((skeleton) => {
        expect(skeleton).toHaveAttribute('aria-hidden', 'true');
      });
    });
  });
});
