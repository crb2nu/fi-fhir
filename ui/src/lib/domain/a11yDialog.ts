type Focusable = HTMLElement & { disabled?: boolean };

function isHidden(el: HTMLElement): boolean {
  // Fast path: skip elements not in layout tree.
  if (el.offsetParent === null && getComputedStyle(el).position !== 'fixed') return true;
  const style = getComputedStyle(el);
  return style.visibility === 'hidden' || style.display === 'none';
}

function isFocusable(el: Focusable): boolean {
  if (el.getAttribute('aria-hidden') === 'true') return false;
  if (isHidden(el)) return false;
  if ((el as HTMLInputElement).type === 'hidden') return false;
  if ('disabled' in el && Boolean(el.disabled)) return false;
  // We want tabbable elements (not just programmatically focusable).
  // This prevents modal backdrops with tabindex="-1" from entering the trap cycle.
  if (el.tabIndex < 0) return false;
  return true;
}

export function getFocusableElements(root: HTMLElement): HTMLElement[] {
  // Minimal focusable selector set; we can expand if needed.
  const candidates = root.querySelectorAll<Focusable>(
    [
      'a[href]',
      'button',
      'input',
      'select',
      'textarea',
      '[tabindex]:not([tabindex="-1"])'
    ].join(',')
  );
  return Array.from(candidates).filter((el) => isFocusable(el));
}

export type DialogFocusController = {
  focusInitial: () => void;
  onKeydown: (e: KeyboardEvent) => void;
  restoreFocus: () => void;
};

export function createDialogFocusController(
  dialog: HTMLElement,
  opts?: { initialFocus?: HTMLElement | null }
): DialogFocusController {
  let previouslyFocused: Element | null = null;

  function focusInitial(): void {
    previouslyFocused = document.activeElement;
    const target = opts?.initialFocus ?? null;
    if (target && dialog.contains(target)) {
      target.focus();
      return;
    }
    const focusables = getFocusableElements(dialog);
    (focusables[0] ?? dialog).focus();
  }

  function restoreFocus(): void {
    const prev = previouslyFocused as HTMLElement | null;
    previouslyFocused = null;
    if (!prev) return;
    // If element disappeared, fail silently.
    try {
      prev.focus();
    } catch {
      // ignore
    }
  }

  function onKeydown(e: KeyboardEvent): void {
    if (e.key !== 'Tab') return;
    const focusables = getFocusableElements(dialog);
    if (focusables.length === 0) {
      e.preventDefault();
      dialog.focus();
      return;
    }

    const active = document.activeElement as HTMLElement | null;
    const currentIndex = active ? focusables.indexOf(active) : -1;
    const goingBack = e.shiftKey;

    if (!goingBack) {
      if (currentIndex === -1 || currentIndex === focusables.length - 1) {
        e.preventDefault();
        focusables[0]?.focus();
      }
      return;
    }

    if (currentIndex <= 0) {
      e.preventDefault();
      focusables[focusables.length - 1]?.focus();
    }
  }

  return { focusInitial, onKeydown, restoreFocus };
}
