/**
 * Tests for IDE keyboard shortcuts.
 */
import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { initKeyboardShortcuts } from './keyboardShortcuts';
import type { ShortcutCallbacks } from './keyboardShortcuts';

describe('keyboardShortcuts', () => {
  let callbacks: ShortcutCallbacks;
  let callCounts: Record<'toggleSidebar' | 'toggleBottomPanel' | 'closeTab' | 'splitEditor', number>;

  let cleanup: () => void;

  beforeEach(() => {
    callCounts = {
      toggleSidebar: 0,
      toggleBottomPanel: 0,
      closeTab: 0,
      splitEditor: 0,
    };
    callbacks = {
      toggleSidebar: () => {
        callCounts.toggleSidebar += 1;
      },
      toggleBottomPanel: () => {
        callCounts.toggleBottomPanel += 1;
      },
      closeTab: () => {
        callCounts.closeTab += 1;
      },
      splitEditor: () => {
        callCounts.splitEditor += 1;
      },
    };
    cleanup = initKeyboardShortcuts(callbacks);
  });

  afterEach(() => {
    cleanup();
  });

  function fireKey(key: string, opts: Partial<KeyboardEvent> = {}): KeyboardEvent {
    const event = new KeyboardEvent('keydown', {
      key,
      metaKey: true,
      bubbles: true,
      cancelable: true,
      ...opts,
    });
    window.dispatchEvent(event);
    return event;
  }

  describe('Cmd+B toggles sidebar', () => {
    it('should call toggleSidebar on Cmd+B', () => {
      fireKey('b', { metaKey: true });
      expect(callCounts.toggleSidebar).toBe(1);
    });

    it('should call toggleSidebar on Ctrl+B', () => {
      fireKey('b', { metaKey: false, ctrlKey: true });
      expect(callCounts.toggleSidebar).toBe(1);
    });

    it('should not call toggleSidebar without modifier', () => {
      fireKey('b', { metaKey: false, ctrlKey: false });
      expect(callCounts.toggleSidebar).toBe(0);
    });
  });

  describe('Cmd+J toggles bottom panel', () => {
    it('should call toggleBottomPanel on Cmd+J', () => {
      fireKey('j', { metaKey: true });
      expect(callCounts.toggleBottomPanel).toBe(1);
    });

    it('should call toggleBottomPanel on Ctrl+J', () => {
      fireKey('j', { metaKey: false, ctrlKey: true });
      expect(callCounts.toggleBottomPanel).toBe(1);
    });
  });

  describe('Cmd+W closes tab', () => {
    it('should call closeTab on Cmd+W', () => {
      fireKey('w', { metaKey: true });
      expect(callCounts.closeTab).toBe(1);
    });
  });

  describe('Cmd+\\ splits editor', () => {
    it('should call splitEditor on Cmd+\\', () => {
      fireKey('\\', { metaKey: true });
      expect(callCounts.splitEditor).toBe(1);
    });
  });

  describe('cleanup', () => {
    it('should remove event listeners on cleanup', () => {
      cleanup();
      fireKey('b', { metaKey: true });
      expect(callCounts.toggleSidebar).toBe(0);
    });
  });

  describe('defaultPrevented', () => {
    it('should not fire when event is already prevented', () => {
      const event = new KeyboardEvent('keydown', {
        key: 'b',
        metaKey: true,
        bubbles: true,
        cancelable: true,
      });
      // Simulate default prevention
      Object.defineProperty(event, 'defaultPrevented', { value: true });
      window.dispatchEvent(event);
      expect(callCounts.toggleSidebar).toBe(0);
    });
  });
});
