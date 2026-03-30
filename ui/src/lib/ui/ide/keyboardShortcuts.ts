/**
 * Keyboard shortcut handler for IDE shell.
 * Returns a cleanup function to remove event listeners.
 */

export interface ShortcutCallbacks {
  toggleSidebar: () => void;
  toggleBottomPanel: () => void;
  closeTab: () => void;
  splitEditor: () => void;
  openDebugPanel?: () => void;
}

function isEditableTarget(target: EventTarget | null): boolean {
  const el = target as HTMLElement | null;
  if (!el) return false;
  const tag = el.tagName;
  return (
    tag === 'INPUT' ||
    tag === 'TEXTAREA' ||
    tag === 'SELECT' ||
    el.isContentEditable
  );
}

export function initKeyboardShortcuts(callbacks: ShortcutCallbacks): () => void {
  function onKeydown(e: KeyboardEvent): void {
    if (e.defaultPrevented) return;

    const mod = e.metaKey || e.ctrlKey;
    if (!mod) return;

    // Allow editable targets for most shortcuts except Cmd+B and Cmd+J
    // which are global layout shortcuts
    const key = e.key.toLowerCase();

    if (key === 'b') {
      e.preventDefault();
      callbacks.toggleSidebar();
      return;
    }

    if (key === 'j') {
      e.preventDefault();
      callbacks.toggleBottomPanel();
      return;
    }

    if (key === 'd' && e.shiftKey && callbacks.openDebugPanel) {
      e.preventDefault();
      callbacks.openDebugPanel();
      return;
    }

    // These only fire outside editable contexts
    if (isEditableTarget(e.target)) return;

    if (key === 'w') {
      e.preventDefault();
      callbacks.closeTab();
      return;
    }

    if (key === '\\') {
      e.preventDefault();
      callbacks.splitEditor();
    }
  }

  window.addEventListener('keydown', onKeydown);
  return () => window.removeEventListener('keydown', onKeydown);
}
