/**
 * Tests for the toast notification store.
 */
import { describe, it, expect, beforeEach, vi, afterEach } from 'vitest';
import { get } from 'svelte/store';
import { toasts, toastList } from './toastStore';

describe('toastStore', () => {
  beforeEach(() => {
    // Clear all toasts before each test
    toasts.dismissAll();
    // Use fake timers for auto-dismiss testing
    vi.useFakeTimers();
  });

  afterEach(() => {
    vi.useRealTimers();
  });

  describe('add', () => {
    it('should add a toast with default values', () => {
      const id = toasts.add({ message: 'Test message' });

      const list = get(toastList);
      expect(list).toHaveLength(1);
      expect(list[0]!.id).toBe(id);
      expect(list[0]!.message).toBe('Test message');
      expect(list[0]!.variant).toBe('info');
      expect(list[0]!.dismissible).toBe(true);
    });

    it('should add a toast with custom variant', () => {
      toasts.add({ message: 'Error!', variant: 'error' });

      const list = get(toastList);
      expect(list[0]!.variant).toBe('error');
    });

    it('should auto-dismiss after duration', () => {
      toasts.add({ message: 'Will disappear', duration: 1000 });

      expect(get(toastList)).toHaveLength(1);

      vi.advanceTimersByTime(1000);

      expect(get(toastList)).toHaveLength(0);
    });

    it('should not auto-dismiss when duration is 0', () => {
      toasts.add({ message: 'Persistent', duration: 0 });

      expect(get(toastList)).toHaveLength(1);

      vi.advanceTimersByTime(60000); // 1 minute

      expect(get(toastList)).toHaveLength(1);
    });
  });

  describe('convenience methods', () => {
    it('success() should create success toast', () => {
      toasts.success('Operation completed');

      const list = get(toastList);
      expect(list[0]!.variant).toBe('success');
      expect(list[0]!.message).toBe('Operation completed');
    });

    it('error() should create error toast with longer duration', () => {
      toasts.error('Something went wrong');

      const list = get(toastList);
      expect(list[0]!.variant).toBe('error');
      expect(list[0]!.duration).toBe(8000); // Errors stay longer
    });

    it('warning() should create warning toast', () => {
      toasts.warning('Be careful');

      const list = get(toastList);
      expect(list[0]!.variant).toBe('warning');
    });

    it('info() should create info toast', () => {
      toasts.info('FYI');

      const list = get(toastList);
      expect(list[0]!.variant).toBe('info');
    });
  });

  describe('dismiss', () => {
    it('should remove a specific toast by ID', () => {
      const id1 = toasts.add({ message: 'First' });
      const id2 = toasts.add({ message: 'Second' });

      expect(get(toastList)).toHaveLength(2);

      toasts.dismiss(id1);

      const list = get(toastList);
      expect(list).toHaveLength(1);
      expect(list[0]!.id).toBe(id2);
    });

    it('should handle dismissing non-existent ID gracefully', () => {
      toasts.add({ message: 'Test' });

      // Should not throw
      toasts.dismiss('non-existent-id');

      expect(get(toastList)).toHaveLength(1);
    });
  });

  describe('dismissAll', () => {
    it('should remove all toasts', () => {
      toasts.add({ message: 'First' });
      toasts.add({ message: 'Second' });
      toasts.add({ message: 'Third' });

      expect(get(toastList)).toHaveLength(3);

      toasts.dismissAll();

      expect(get(toastList)).toHaveLength(0);
    });
  });

  describe('toastList derived store', () => {
    it('should return just the array of toasts', () => {
      toasts.add({ message: 'A' });
      toasts.add({ message: 'B' });

      const list = get(toastList);
      expect(Array.isArray(list)).toBe(true);
      expect(list).toHaveLength(2);
    });
  });

  describe('unique IDs', () => {
    it('should generate unique IDs for each toast', () => {
      const ids = new Set<string>();

      for (let i = 0; i < 100; i++) {
        ids.add(toasts.add({ message: `Toast ${i}` }));
      }

      expect(ids.size).toBe(100);
    });
  });

  describe('createdAt', () => {
    it('should set createdAt timestamp', () => {
      const before = Date.now();
      toasts.add({ message: 'Test' });
      const after = Date.now();

      const list = get(toastList);
      expect(list[0]!.createdAt).toBeGreaterThanOrEqual(before);
      expect(list[0]!.createdAt).toBeLessThanOrEqual(after);
    });
  });
});
