import { describe, it, expect } from 'vitest';
import { render } from '@testing-library/svelte';
import FilterEditor from './FilterEditor.svelte';
import { ALL_EVENT_TYPES, EVENT_TYPE_PRESETS } from '../workflowTypes';
import type { FilterDraft } from '../workflowTypes';

describe('FilterEditor', () => {
  const defaultFilter: FilterDraft = {
    eventTypes: [],
    sources: [],
    condition: ''
  };

  it('renders all event type checkboxes', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const checkboxes = container.querySelectorAll('input[type="checkbox"]');
    expect(checkboxes.length).toBe(ALL_EVENT_TYPES.length);
  });

  it('shows checked state for selected event types', () => {
    const filter: FilterDraft = {
      eventTypes: ['PATIENT_ADMIT'],
      sources: [],
      condition: ''
    };
    const { container } = render(FilterEditor, {
      props: { filter }
    });
    const checkboxes = container.querySelectorAll('input[type="checkbox"]');
    const checkedCount = Array.from(checkboxes).filter(
      (cb) => (cb as HTMLInputElement).checked
    ).length;
    expect(checkedCount).toBe(1);
  });

  it('renders preset buttons', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const presetBtns = container.querySelectorAll('.preset-btn:not(.clear)');
    expect(presetBtns.length).toBe(EVENT_TYPE_PRESETS.length);
  });

  it('does not show CEL input by default', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const monoInputs = container.querySelectorAll('input.mono');
    expect(monoInputs.length).toBe(0);
  });

  it('shows CEL input when condition is pre-populated', () => {
    const filter: FilterDraft = {
      eventTypes: [],
      sources: [],
      condition: 'event.isCritical == true'
    };
    const { container } = render(FilterEditor, {
      props: { filter }
    });
    const monoInputs = container.querySelectorAll('input.mono');
    expect(monoInputs.length).toBe(1);
  });

  it('renders category group labels', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const groupLabels = container.querySelectorAll('.group-label');
    expect(groupLabels.length).toBeGreaterThan(0);
  });

  it('renders sources input field', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const inputs = container.querySelectorAll('input[type="text"]');
    // Should have at least the sources input
    expect(inputs.length).toBeGreaterThanOrEqual(1);
  });
});
