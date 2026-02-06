import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import ActionEditor from './ActionEditor.svelte';
import { ACTION_FIELDS, ACTION_TYPES } from '../workflowTypes';
import type { ActionDraft } from '../workflowTypes';

describe('ActionEditor', () => {
  const defaultAction: ActionDraft = {
    _key: 'test-key',
    type: 'log',
    config: {}
  };

  it('renders a type selector with all action types', () => {
    const { container } = render(ActionEditor, {
      props: { action: defaultAction }
    });
    const options = container.querySelectorAll('select option');
    expect(options.length).toBe(ACTION_TYPES.length);
  });

  it('shows config fields for the selected action type', () => {
    const { container } = render(ActionEditor, {
      props: { action: defaultAction }
    });
    const fields = ACTION_FIELDS['log']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('shows different fields for webhook type', () => {
    const action: ActionDraft = { _key: 'test', type: 'webhook', config: {} };
    const { container } = render(ActionEditor, {
      props: { action }
    });
    const fields = ACTION_FIELDS['webhook']!;
    const inputs = container.querySelectorAll('.config-fields input');
    expect(inputs.length).toBe(fields.length);
  });

  it('displays existing config values', () => {
    const action: ActionDraft = {
      _key: 'test',
      type: 'webhook',
      config: { url: 'https://example.com' }
    };
    const { container } = render(ActionEditor, {
      props: { action }
    });
    const urlInput = container.querySelector(
      '.config-fields input'
    ) as HTMLInputElement;
    expect(urlInput.value).toBe('https://example.com');
  });

  it('renders field labels with required markers', () => {
    const action: ActionDraft = { _key: 'test', type: 'webhook', config: {} };
    const { container } = render(ActionEditor, {
      props: { action }
    });
    const required = container.querySelectorAll('.required');
    // webhook has at least one required field (url)
    expect(required.length).toBeGreaterThan(0);
  });

  it('has correct selected value in type dropdown', () => {
    const action: ActionDraft = { _key: 'test', type: 'fhir', config: {} };
    const { container } = render(ActionEditor, {
      props: { action }
    });
    const select = container.querySelector('select') as HTMLSelectElement;
    expect(select.value).toBe('fhir');
  });

  it('renders all action types in the selector', () => {
    const { container } = render(ActionEditor, {
      props: { action: defaultAction }
    });
    const options = Array.from(container.querySelectorAll('select option')).map(
      (o) => (o as HTMLOptionElement).value
    );
    for (const type of ACTION_TYPES) {
      expect(options).toContain(type);
    }
  });
});
