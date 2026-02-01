/**
 * Tests for the EmptyState component.
 */
import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import EmptyState from './EmptyState.svelte';

describe('EmptyState', () => {
  describe('rendering', () => {
    it('should render title', () => {
      render(EmptyState, { props: { title: 'No results found' } });

      expect(screen.getByRole('heading', { name: 'No results found' })).toBeInTheDocument();
    });

    it('should render description when provided', () => {
      render(EmptyState, {
        props: {
          title: 'No results',
          description: 'Try adjusting your search criteria'
        }
      });

      expect(screen.getByText('Try adjusting your search criteria')).toBeInTheDocument();
    });

    it('should not render description when not provided', () => {
      const { container } = render(EmptyState, { props: { title: 'No results' } });

      expect(container.querySelector('.description')).not.toBeInTheDocument();
    });
  });

  describe('icons', () => {
    const icons = ['search', 'file', 'folder', 'data', 'upload', 'error', 'inbox'] as const;

    icons.forEach((icon) => {
      it(`should render ${icon} icon`, () => {
        const { container } = render(EmptyState, { props: { title: 'Test', icon } });

        const iconContainer = container.querySelector('.icon-container');
        expect(iconContainer).toBeInTheDocument();
        expect(iconContainer?.querySelector('svg')).toBeInTheDocument();
      });
    });

    it('should render inbox icon by default', () => {
      const { container } = render(EmptyState, { props: { title: 'Test' } });

      // Inbox icon has specific path
      const svg = container.querySelector('.icon-container svg');
      expect(svg).toBeInTheDocument();
    });
  });

  describe('compact mode', () => {
    it('should not be compact by default', () => {
      const { container } = render(EmptyState, { props: { title: 'Test' } });

      expect(container.querySelector('.empty-state')).not.toHaveClass('compact');
    });

    it('should apply compact class when compact is true', () => {
      const { container } = render(EmptyState, { props: { title: 'Test', compact: true } });

      expect(container.querySelector('.empty-state')).toHaveClass('compact');
    });
  });

  describe('structure', () => {
    it('should have proper heading level', () => {
      render(EmptyState, { props: { title: 'Empty State Title' } });

      const heading = screen.getByRole('heading', { level: 3 });
      expect(heading).toHaveTextContent('Empty State Title');
    });

    it('should render all components in correct order', () => {
      const { container } = render(EmptyState, {
        props: {
          title: 'No items',
          description: 'No items to display'
        }
      });

      const emptyState = container.querySelector('.empty-state');
      const children = Array.from(emptyState?.children || []);

      expect(children[0]).toHaveClass('icon-container');
      expect(children[1]).toHaveClass('title');
      expect(children[2]).toHaveClass('description');
    });
  });

  describe('centering and layout', () => {
    it('should have centered text', () => {
      const { container } = render(EmptyState, { props: { title: 'Test' } });

      expect(container.querySelector('.empty-state')).toBeInTheDocument();
    });
  });
});
