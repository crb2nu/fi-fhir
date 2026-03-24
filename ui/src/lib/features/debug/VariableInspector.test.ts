/**
 * Tests for the VariableInspector component.
 */
import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import VariableInspector from './VariableInspector.svelte';

describe('VariableInspector', () => {
  describe('rendering', () => {
    it('should render key-value pairs', () => {
      const { container } = render(VariableInspector, {
        props: {
          variables: {
            'event.type': 'LAB_RESULT',
            'event.source': 'epic',
            'event.isCritical': true
          }
        }
      });

      const keys = container.querySelectorAll('.var-key');
      expect(keys).toHaveLength(3);
      expect(keys[0].textContent).toContain('event.type');
      expect(keys[1].textContent).toContain('event.source');
      expect(keys[2].textContent).toContain('event.isCritical');
    });

    it('should render string values with quotes', () => {
      const { container } = render(VariableInspector, {
        props: { variables: { name: 'test' } }
      });

      const value = container.querySelector('.var-value.string');
      expect(value).not.toBeNull();
      expect(value!.textContent).toBe('"test"');
    });

    it('should render number values', () => {
      const { container } = render(VariableInspector, {
        props: { variables: { count: 42 } }
      });

      const value = container.querySelector('.var-value.number');
      expect(value).not.toBeNull();
      expect(value!.textContent).toBe('42');
    });

    it('should render boolean values', () => {
      const { container } = render(VariableInspector, {
        props: { variables: { active: true } }
      });

      const value = container.querySelector('.var-value.boolean');
      expect(value).not.toBeNull();
      expect(value!.textContent).toBe('true');
    });

    it('should render null values', () => {
      const { container } = render(VariableInspector, {
        props: { variables: { missing: null } }
      });

      const value = container.querySelector('.var-value.null');
      expect(value).not.toBeNull();
      expect(value!.textContent).toBe('null');
    });
  });

  describe('empty state', () => {
    it('should show empty message when no variables', () => {
      const { container } = render(VariableInspector, {
        props: { variables: {} }
      });

      const empty = container.querySelector('.empty');
      expect(empty).not.toBeNull();
      expect(empty!.textContent).toBe('No variables');
    });

    it('should show empty message with default props', () => {
      const { container } = render(VariableInspector);

      const empty = container.querySelector('.empty');
      expect(empty).not.toBeNull();
    });
  });

  describe('nested objects', () => {
    it('should render expandable section for objects', () => {
      const { container } = render(VariableInspector, {
        props: {
          variables: {
            nested: { a: 1, b: 2 }
          }
        }
      });

      const expandable = container.querySelector('.expandable');
      expect(expandable).not.toBeNull();

      const typeLabel = container.querySelector('.var-type');
      expect(typeLabel).not.toBeNull();
      expect(typeLabel!.textContent).toBe('Object');
    });

    it('should render expandable section for arrays', () => {
      const { container } = render(VariableInspector, {
        props: {
          variables: {
            items: [1, 2, 3]
          }
        }
      });

      const typeLabel = container.querySelector('.var-type');
      expect(typeLabel).not.toBeNull();
      expect(typeLabel!.textContent).toBe('Array(3)');
    });
  });
});
