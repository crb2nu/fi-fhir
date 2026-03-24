/**
 * Tests for the CodeEditor component.
 *
 * CodeMirror requires DOM APIs that jsdom does not fully implement,
 * so we mock EditorView and related modules to test component behavior.
 */
import { describe, it, expect, vi, beforeEach } from 'vitest';
import { render } from '@testing-library/svelte';

// Track EditorView constructor calls and instances via module-level closures
// inside the mock factory (which is hoisted).

vi.mock('@codemirror/view', () => {
  const instances: Array<{ dispatch: ReturnType<typeof vi.fn>; destroy: ReturnType<typeof vi.fn> }> = [];

  const EditorViewCtor = vi.fn(function () {
    const inst = {
      dispatch: vi.fn(),
      destroy: vi.fn(),
      state: {
        doc: {
          toString() { return ''; },
          length: 0
        }
      }
    };
    instances.push(inst);
    return inst;
  });
  (EditorViewCtor as unknown as Record<string, unknown>).theme = vi.fn(() => []);
  (EditorViewCtor as unknown as Record<string, unknown>).updateListener = { of: vi.fn(() => []) };
  (EditorViewCtor as unknown as Record<string, unknown>).__instances = instances;

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
      create: vi.fn(() => ({
        doc: { toString() { return ''; }, length: 0 }
      })),
      readOnly: { of: vi.fn(() => []) }
    },
    Compartment
  };
});

vi.mock('@codemirror/commands', () => ({
  defaultKeymap: [],
  history: vi.fn(() => []),
  historyKeymap: []
}));

vi.mock('@codemirror/language', () => ({
  bracketMatching: vi.fn(() => []),
  indentOnInput: vi.fn(() => []),
  HighlightStyle: { define: vi.fn(() => ({})) },
  syntaxHighlighting: vi.fn(() => []),
  StreamLanguage: { define: vi.fn(() => ({})) }
}));

vi.mock('@codemirror/lint', () => ({
  linter: vi.fn(() => [])
}));

vi.mock('@codemirror/autocomplete', () => ({
  closeBrackets: vi.fn(() => []),
  closeBracketsKeymap: []
}));

vi.mock('@codemirror/lang-json', () => ({
  json: vi.fn(() => [])
}));

vi.mock('@codemirror/lang-yaml', () => ({
  yaml: vi.fn(() => [])
}));

vi.mock('./lang-cel', () => ({
  cel: vi.fn(() => [])
}));

vi.mock('./lang-hl7v2', () => ({
  hl7v2: vi.fn(() => [])
}));

vi.mock('./cmTheme', () => ({
  createThemeExtension: vi.fn(() => ({
    extension: [],
    compartment: { of: vi.fn(() => []), reconfigure: vi.fn(() => []) },
    getThemeEffect: vi.fn(() => []),
    cleanup: vi.fn(),
    startObserving: vi.fn()
  })),
  getTheme: vi.fn(() => [])
}));

vi.mock('./diagnostics', () => ({
  toCM6Diagnostics: vi.fn((d: unknown[]) => d)
}));

// Import after all mocks
import CodeEditor from './CodeEditor.svelte';
import { EditorView } from '@codemirror/view';
import { EditorState } from '@codemirror/state';
import { linter } from '@codemirror/lint';

type MockInstances = Array<{ dispatch: ReturnType<typeof vi.fn>; destroy: ReturnType<typeof vi.fn> }>;

function getInstances(): MockInstances {
  return (EditorView as unknown as { __instances: MockInstances }).__instances;
}

describe('CodeEditor', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    // Clear tracked instances
    getInstances().length = 0;
  });

  it('should render without error', () => {
    const { container } = render(CodeEditor);
    const el = container.querySelector('[data-testid="code-editor"]');
    expect(el).toBeInTheDocument();
  });

  it('should render the container div with data-testid', () => {
    const { container } = render(CodeEditor);
    const el = container.querySelector('[data-testid="code-editor"]');
    expect(el).toBeTruthy();
    expect(el?.tagName).toBe('DIV');
  });

  it('should create an EditorView on mount', () => {
    render(CodeEditor, { props: { value: 'hello', language: 'text' } });
    expect(EditorView).toHaveBeenCalledTimes(1);
    expect(getInstances()).toHaveLength(1);
  });

  it('should apply height style', () => {
    const { container } = render(CodeEditor, { props: { height: '200px' } });
    const el = container.querySelector('[data-testid="code-editor"]') as HTMLElement;
    expect(el.style.height).toBe('200px');
  });

  it('should apply default height of 100%', () => {
    const { container } = render(CodeEditor);
    const el = container.querySelector('[data-testid="code-editor"]') as HTMLElement;
    expect(el.style.height).toBe('100%');
  });

  it('should call EditorState.create with the initial value', () => {
    render(CodeEditor, { props: { value: 'test content', language: 'json' } });
    expect(EditorState.create).toHaveBeenCalled();
  });

  it('should render with readOnly prop', () => {
    render(CodeEditor, { props: { readOnly: true, value: 'read only' } });
    expect(EditorView).toHaveBeenCalledTimes(1);
  });

  it('should render with diagnostics prop', () => {
    const diagnostics = [
      { from: 0, to: 5, severity: 'error' as const, message: 'test error' }
    ];
    render(CodeEditor, { props: { diagnostics, value: 'hello' } });
    expect(linter).toHaveBeenCalled();
  });

  it('should render with lineNumbers disabled', () => {
    render(CodeEditor, { props: { lineNumbers: false, value: '' } });
    expect(EditorView).toHaveBeenCalledTimes(1);
  });

  it('should destroy EditorView on component destroy', () => {
    const { unmount } = render(CodeEditor, { props: { value: 'test' } });
    expect(getInstances()).toHaveLength(1);
    const instance = getInstances()[0]!;
    unmount();
    expect(instance.destroy).toHaveBeenCalled();
  });
});
