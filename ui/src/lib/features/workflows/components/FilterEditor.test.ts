import { describe, it, expect, vi } from 'vitest';
import { render } from '@testing-library/svelte';

// Mock CodeMirror modules used by CodeEditor (imported by FilterEditor)
vi.mock('@codemirror/view', () => {
  function EditorViewCtor() {
    return {
      dispatch: vi.fn(),
      destroy: vi.fn(),
      state: { doc: { toString() { return ''; }, length: 0 } }
    };
  }
  EditorViewCtor.theme = vi.fn(() => []);
  EditorViewCtor.updateListener = { of: vi.fn(() => []) };
  return {
    EditorView: EditorViewCtor,
    keymap: { of: vi.fn(() => []) },
    placeholder: vi.fn(() => []),
    lineNumbers: vi.fn(() => [])
  };
});

vi.mock('@codemirror/state', () => {
  class Compartment {
    of(ext: unknown) { return ext; }
    reconfigure(ext: unknown) { return ext; }
  }
  return {
    EditorState: {
      create: vi.fn(() => ({ doc: { toString() { return ''; }, length: 0 } })),
      readOnly: { of: vi.fn(() => []) }
    },
    Compartment
  };
});

vi.mock('@codemirror/commands', () => ({
  defaultKeymap: [], history: vi.fn(() => []), historyKeymap: []
}));

vi.mock('@codemirror/language', () => ({
  bracketMatching: vi.fn(() => []),
  indentOnInput: vi.fn(() => []),
  HighlightStyle: { define: vi.fn(() => ({})) },
  syntaxHighlighting: vi.fn(() => []),
  StreamLanguage: { define: vi.fn(() => ({})) }
}));

vi.mock('@codemirror/lint', () => ({ linter: vi.fn(() => []) }));
vi.mock('@codemirror/autocomplete', () => ({ closeBrackets: vi.fn(() => []), closeBracketsKeymap: [] }));
vi.mock('@codemirror/lang-json', () => ({ json: vi.fn(() => []) }));
vi.mock('@codemirror/lang-yaml', () => ({ yaml: vi.fn(() => []) }));

vi.mock('$lib/ui/editor/lang-cel', () => ({ cel: vi.fn(() => []) }));
vi.mock('$lib/ui/editor/lang-hl7v2', () => ({ hl7v2: vi.fn(() => []) }));
vi.mock('$lib/ui/editor/cmTheme', () => ({
  createThemeExtension: vi.fn(() => ({
    extension: [], compartment: { of: vi.fn(() => []), reconfigure: vi.fn(() => []) },
    getThemeEffect: vi.fn(() => []), cleanup: vi.fn(), startObserving: vi.fn()
  })),
  getTheme: vi.fn(() => [])
}));
vi.mock('$lib/ui/editor/diagnostics', () => ({ toCM6Diagnostics: vi.fn((d: unknown[]) => d) }));

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
    const presetBar = container.querySelector('.preset-bar');
    expect(presetBar).toBeTruthy();
    // Preset bar contains one button per preset (no clear button when no types selected)
    const presetBtns = presetBar!.querySelectorAll('button');
    expect(presetBtns.length).toBe(EVENT_TYPE_PRESETS.length);
  });

  it('does not show CEL editor by default', () => {
    const { container } = render(FilterEditor, {
      props: { filter: defaultFilter }
    });
    const codeEditors = container.querySelectorAll('[data-testid="code-editor"]');
    expect(codeEditors.length).toBe(0);
  });

  it('shows CEL editor when condition is pre-populated', () => {
    const filter: FilterDraft = {
      eventTypes: [],
      sources: [],
      condition: 'event.isCritical == true'
    };
    const { container } = render(FilterEditor, {
      props: { filter }
    });
    const codeEditors = container.querySelectorAll('[data-testid="code-editor"]');
    expect(codeEditors.length).toBe(1);
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
