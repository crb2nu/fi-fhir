/**
 * Tests for IDE keyboard shortcuts.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { initKeyboardShortcuts } from './keyboardShortcuts';

describe('keyboardShortcuts', () => {
  let callbacks: {
    toggleSidebar: ReturnType<typeof vi.fn>;
    toggleBottomPanel: ReturnType<typeof vi.fn>;
    closeTab: ReturnType<typeof vi.fn>;
    splitEditor: ReturnType<typeof vi.fn>;
  };

  let cleanup: () => void;

  beforeEach(() => {
    callbacks = {
      toggleSidebar: vi.fn(),
      toggleBottomPanel: vi.fn(),
      closeTab: vi.fn(),
      splitEditor: vi.fn(),
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
      expect(callbacks.toggleSidebar).toHaveBeenCalledTimes(1);
    });

    it('should call toggleSidebar on Ctrl+B', () => {
      fireKey('b', { metaKey: false, ctrlKey: true });
      expect(callbacks.toggleSidebar).toHaveBeenCalledTimes(1);
    });

    it('should not call toggleSidebar without modifier', () => {
      fireKey('b', { metaKey: false, ctrlKey: false });
      expect(callbacks.toggleSidebar).not.toHaveBeenCalled();
    });
  });

  describe('Cmd+J toggles bottom panel', () => {
    it('should call toggleBottomPanel on Cmd+J', () => {
      fireKey('j', { metaKey: true });
      expect(callbacks.toggleBottomPanel).toHaveBeenCalledTimes(1);
    });

    it('should call toggleBottomPanel on Ctrl+J', () => {
      fireKey('j', { metaKey: false, ctrlKey: true });
      expect(callbacks.toggleBottomPanel).toHaveBeenCalledTimes(1);
    });
  });

  describe('Cmd+W closes tab', () => {
    it('should call closeTab on Cmd+W', () => {
      fireKey('w', { metaKey: true });
      expect(callbacks.closeTab).toHaveBeenCalledTimes(1);
    });
  });

  describe('Cmd+\\ splits editor', () => {
    it('should call splitEditor on Cmd+\\', () => {
      fireKey('\\', { metaKey: true });
      expect(callbacks.splitEditor).toHaveBeenCalledTimes(1);
    });
  });

  describe('cleanup', () => {
    it('should remove event listeners on cleanup', () => {
      cleanup();
      fireKey('b', { metaKey: true });
      expect(callbacks.toggleSidebar).not.toHaveBeenCalled();
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
      expect(callbacks.toggleSidebar).not.toHaveBeenCalled();
    });
  });
});
