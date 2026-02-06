import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import TransformEditor from './TransformEditor.svelte';
import { TRANSFORM_FIELDS, TRANSFORM_TYPES } from '../workflowTypes';
import type { TransformDraft } from '../workflowTypes';

describe('TransformEditor', () => {
  const defaultTransform: TransformDraft = {
    _key: 'test-key',
    type: 'set_field',
    config: {}
  };

  it('renders a type selector with all transform types', () => {
    const { container } = render(TransformEditor, {
      props: { transform: defaultTransform }
    });
    const options = container.querySelectorAll('select option');
    expect(options.length).toBe(TRANSFORM_TYPES.length);
  });

  it('shows config fields for set_field type', () => {
    const { container } = render(TransformEditor, {
      props: { transform: defaultTransform }
    });
    const fields = TRANSFORM_FIELDS['set_field']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('shows different fields for map_terminology type', () => {
    const transform: TransformDraft = { _key: 'test', type: 'map_terminology', config: {} };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const fields = TRANSFORM_FIELDS['map_terminology']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('shows different fields for redact type', () => {
    const transform: TransformDraft = { _key: 'test', type: 'redact', config: {} };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const fields = TRANSFORM_FIELDS['redact']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('shows different fields for explain_warnings type', () => {
    const transform: TransformDraft = { _key: 'test', type: 'explain_warnings', config: {} };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const fields = TRANSFORM_FIELDS['explain_warnings']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('displays existing config values', () => {
    const transform: TransformDraft = {
      _key: 'test',
      type: 'set_field',
      config: { expression: 'event.status = "done"' }
    };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const input = container.querySelector('.config-fields input') as HTMLInputElement;
    expect(input.value).toBe('event.status = "done"');
  });

  it('renders field labels with required markers', () => {
    const transform: TransformDraft = { _key: 'test', type: 'map_terminology', config: {} };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const required = container.querySelectorAll('.required');
    expect(required.length).toBeGreaterThan(0);
  });

  it('has correct selected value in type dropdown', () => {
    const transform: TransformDraft = { _key: 'test', type: 'redact', config: {} };
    const { container } = render(TransformEditor, {
      props: { transform }
    });
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select.value).toBe('redact');
  });

  it('renders all transform types in the selector', () => {
    const { container } = render(TransformEditor, {
      props: { transform: defaultTransform }
    });
    const options = Array.from(container.querySelectorAll('select option')).map(
      (o) => (o as HTMLOptionElement).value
    );
    for (const type of TRANSFORM_TYPES) {
      expect(options).toContain(type);
    }
  });
});
