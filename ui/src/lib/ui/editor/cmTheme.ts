/**
 * CodeMirror 6 theme that maps to CSS design tokens.
 *
 * Uses MutationObserver on the document root to detect data-theme changes
 * and provides reactive theme extensions for light/dark modes.
 */
import { EditorView } from '@codemirror/view';
import { Extension, Compartment } from '@codemirror/state';
import { HighlightStyle, syntaxHighlighting } from '@codemirror/language';
import { tags } from '@lezer/highlight';

const lightTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'var(--editor-bg, var(--color-bg-input))',
      color: 'var(--color-text-primary)',
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--text-sm)',
      borderRadius: 'var(--radius-lg)',
      border: '1px solid var(--color-border-default)'
    },
    '&.cm-focused': {
      outline: 'none',
      borderColor: 'var(--color-border-focus)',
      boxShadow: 'var(--shadow-focus)'
    },
    '.cm-content': {
      caretColor: 'var(--editor-cursor, var(--color-text-primary))',
      padding: '4px 0'
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--editor-cursor, var(--color-text-primary))'
    },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--editor-selection) !important'
    },
    '&.cm-focused .cm-selectionBackground': {
      backgroundColor: 'var(--editor-selection) !important'
    },
    '.cm-activeLine': {
      backgroundColor: 'var(--editor-line-highlight)'
    },
    '.cm-gutters': {
      backgroundColor: 'var(--editor-gutter-bg, var(--color-bg-elevated))',
      color: 'var(--editor-gutter-text, var(--color-text-muted))',
      border: 'none',
      borderRight: '1px solid var(--color-border-subtle)'
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'var(--editor-line-highlight)'
    },
    '&.cm-focused .cm-matchingBracket': {
      backgroundColor: 'var(--editor-matching-bracket)',
      outline: 'none'
    },
    '.cm-foldPlaceholder': {
      backgroundColor: 'transparent',
      border: 'none',
      color: 'var(--editor-fold-placeholder, var(--color-text-tertiary))'
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--color-bg-elevated)',
      border: '1px solid var(--color-border-default)',
      borderRadius: 'var(--radius-md)'
    },
    '.cm-tooltip-autocomplete': {
      '& > ul > li[aria-selected]': {
        backgroundColor: 'var(--color-primary-muted)'
      }
    },
    '.cm-panels': {
      backgroundColor: 'var(--color-bg-elevated)',
      color: 'var(--color-text-primary)'
    },
    '.cm-placeholder': {
      color: 'var(--color-text-muted)',
      fontStyle: 'italic'
    },
    '.cm-scroller': {
      overflow: 'auto'
    }
  },
  { dark: false }
);

const darkTheme = EditorView.theme(
  {
    '&': {
      backgroundColor: 'var(--editor-bg, var(--color-bg-input))',
      color: 'var(--color-text-primary)',
      fontFamily: 'var(--font-mono)',
      fontSize: 'var(--text-sm)',
      borderRadius: 'var(--radius-lg)',
      border: '1px solid var(--color-border-default)'
    },
    '&.cm-focused': {
      outline: 'none',
      borderColor: 'var(--color-border-focus)',
      boxShadow: 'var(--shadow-focus)'
    },
    '.cm-content': {
      caretColor: 'var(--editor-cursor, var(--color-text-primary))',
      padding: '4px 0'
    },
    '.cm-cursor, .cm-dropCursor': {
      borderLeftColor: 'var(--editor-cursor, var(--color-text-primary))'
    },
    '.cm-selectionBackground': {
      backgroundColor: 'var(--editor-selection) !important'
    },
    '&.cm-focused .cm-selectionBackground': {
      backgroundColor: 'var(--editor-selection) !important'
    },
    '.cm-activeLine': {
      backgroundColor: 'var(--editor-line-highlight)'
    },
    '.cm-gutters': {
      backgroundColor: 'var(--editor-gutter-bg, var(--color-bg-elevated))',
      color: 'var(--editor-gutter-text, var(--color-text-muted))',
      border: 'none',
      borderRight: '1px solid var(--color-border-subtle)'
    },
    '.cm-activeLineGutter': {
      backgroundColor: 'var(--editor-line-highlight)'
    },
    '&.cm-focused .cm-matchingBracket': {
      backgroundColor: 'var(--editor-matching-bracket)',
      outline: 'none'
    },
    '.cm-foldPlaceholder': {
      backgroundColor: 'transparent',
      border: 'none',
      color: 'var(--editor-fold-placeholder, var(--color-text-tertiary))'
    },
    '.cm-tooltip': {
      backgroundColor: 'var(--color-bg-elevated)',
      border: '1px solid var(--color-border-default)',
      borderRadius: 'var(--radius-md)'
    },
    '.cm-tooltip-autocomplete': {
      '& > ul > li[aria-selected]': {
        backgroundColor: 'var(--color-primary-muted)'
      }
    },
    '.cm-panels': {
      backgroundColor: 'var(--color-bg-elevated)',
      color: 'var(--color-text-primary)'
    },
    '.cm-placeholder': {
      color: 'var(--color-text-muted)',
      fontStyle: 'italic'
    },
    '.cm-scroller': {
      overflow: 'auto'
    }
  },
  { dark: true }
);

const lightHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: '#7c3aed' },
  { tag: tags.string, color: '#059669' },
  { tag: tags.number, color: '#d97706' },
  { tag: tags.bool, color: '#7c3aed' },
  { tag: tags.null, color: '#7c3aed' },
  { tag: tags.comment, color: '#6b7280', fontStyle: 'italic' },
  { tag: tags.operator, color: '#dc2626' },
  { tag: tags.punctuation, color: '#4b5563' },
  { tag: tags.meta, color: '#2563eb' },
  { tag: tags.variableName, color: '#0f172a' },
  { tag: tags.propertyName, color: '#0891b2' },
  { tag: tags.bracket, color: '#6b7280' },
  { tag: tags.escape, color: '#d97706' }
]);

const darkHighlightStyle = HighlightStyle.define([
  { tag: tags.keyword, color: '#a78bfa' },
  { tag: tags.string, color: '#34d399' },
  { tag: tags.number, color: '#fbbf24' },
  { tag: tags.bool, color: '#a78bfa' },
  { tag: tags.null, color: '#a78bfa' },
  { tag: tags.comment, color: '#9ca3af', fontStyle: 'italic' },
  { tag: tags.operator, color: '#f87171' },
  { tag: tags.punctuation, color: '#d1d5db' },
  { tag: tags.meta, color: '#60a5fa' },
  { tag: tags.variableName, color: '#f9fafb' },
  { tag: tags.propertyName, color: '#22d3ee' },
  { tag: tags.bracket, color: '#9ca3af' },
  { tag: tags.escape, color: '#fbbf24' }
]);

function isDarkTheme(): boolean {
  if (typeof document === 'undefined') return false;
  const root = document.documentElement;
  const explicit = root.getAttribute('data-theme');
  if (explicit === 'dark') return true;
  if (explicit === 'light') return false;
  return window.matchMedia('(prefers-color-scheme: dark)').matches;
}

/**
 * Create a theme compartment that watches for data-theme changes.
 * Returns the compartment, its initial extension, and a cleanup function.
 */
export function createThemeExtension(): {
  compartment: Compartment;
  extension: Extension;
  getThemeEffect: () => Extension;
  cleanup: () => void;
} {
  const compartment = new Compartment();
  const dark = isDarkTheme();
  const initial = getThemeForMode(dark);

  function getThemeForMode(isDark: boolean): Extension {
    return isDark
      ? [darkTheme, syntaxHighlighting(darkHighlightStyle)]
      : [lightTheme, syntaxHighlighting(lightHighlightStyle)];
  }

  let observer: MutationObserver | null = null;
  let viewRef: EditorView | null = null;

  function startObserving(view: EditorView) {
    viewRef = view;
    if (typeof document === 'undefined' || typeof MutationObserver === 'undefined') return;

    observer = new MutationObserver(() => {
      if (!viewRef) return;
      const newDark = isDarkTheme();
      viewRef.dispatch({
        effects: compartment.reconfigure(getThemeForMode(newDark))
      });
    });

    observer.observe(document.documentElement, {
      attributes: true,
      attributeFilter: ['data-theme']
    });
  }

  function cleanup() {
    observer?.disconnect();
    observer = null;
    viewRef = null;
  }

  return {
    compartment,
    extension: compartment.of(initial),
    getThemeEffect: () => getThemeForMode(isDarkTheme()),
    cleanup,
    /** @internal - call after view is created */
    ...({ startObserving } as { startObserving: (view: EditorView) => void })
  };
}

/** Get a static theme extension for the current mode. */
export function getTheme(): Extension[] {
  const dark = isDarkTheme();
  return dark
    ? [darkTheme, syntaxHighlighting(darkHighlightStyle)]
    : [lightTheme, syntaxHighlighting(lightHighlightStyle)];
}
