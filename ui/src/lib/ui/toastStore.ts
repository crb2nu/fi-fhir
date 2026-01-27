/**
 * Toast notification store for displaying transient messages to users.
 *
 * Uses a Svelte writable store pattern for reactive updates across the app.
 * Toasts auto-dismiss after a configurable duration but can also be manually dismissed.
 */
import { writable, derived, type Readable } from 'svelte/store';

export type ToastVariant = 'success' | 'error' | 'warning' | 'info';

export interface Toast {
  id: string;
  message: string;
  variant: ToastVariant;
  duration: number;
  dismissible: boolean;
  createdAt: number;
}

export interface ToastInput {
  message: string;
  variant?: ToastVariant | undefined;
  /** Duration in milliseconds. 0 = no auto-dismiss. Default: 5000 */
  duration?: number | undefined;
  /** Whether the toast can be manually dismissed. Default: true */
  dismissible?: boolean | undefined;
}

interface ToastState {
  toasts: Toast[];
}

const DEFAULT_DURATION = 5000;
let nextId = 1;

function createToastStore() {
  const { subscribe, update } = writable<ToastState>({ toasts: [] });

  /**
   * Adds a new toast notification.
   * Returns the toast ID for manual dismissal if needed.
   */
  function add(input: ToastInput): string {
    const id = `toast-${nextId++}`;
    const toast: Toast = {
      id,
      message: input.message,
      variant: input.variant ?? 'info',
      duration: input.duration ?? DEFAULT_DURATION,
      dismissible: input.dismissible ?? true,
      createdAt: Date.now()
    };

    update((state) => ({
      toasts: [...state.toasts, toast]
    }));

    // Auto-dismiss after duration (if duration > 0)
    if (toast.duration > 0) {
      setTimeout(() => dismiss(id), toast.duration);
    }

    return id;
  }

  /**
   * Removes a toast by ID.
   */
  function dismiss(id: string): void {
    update((state) => ({
      toasts: state.toasts.filter((t) => t.id !== id)
    }));
  }

  /**
   * Removes all toasts.
   */
  function dismissAll(): void {
    update(() => ({ toasts: [] }));
  }

  // Convenience methods for common toast types
  const success = (message: string, duration?: number) =>
    add({ message, variant: 'success', duration });

  const error = (message: string, duration?: number) =>
    add({ message, variant: 'error', duration: duration ?? 8000 }); // Errors stay longer

  const warning = (message: string, duration?: number) =>
    add({ message, variant: 'warning', duration });

  const info = (message: string, duration?: number) =>
    add({ message, variant: 'info', duration });

  return {
    subscribe,
    add,
    dismiss,
    dismissAll,
    success,
    error,
    warning,
    info
  };
}

/** Singleton toast store instance */
export const toasts = createToastStore();

/** Derived store with just the toast array for simpler consumption */
export const toastList: Readable<Toast[]> = derived(toasts, ($state) => $state.toasts);
