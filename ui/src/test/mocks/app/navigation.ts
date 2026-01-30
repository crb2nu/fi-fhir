/**
 * Mock for $app/navigation
 */
/* eslint-disable @typescript-eslint/no-unused-vars */
import { writable } from 'svelte/store';

export const navigating = writable(null);
export const updated = { check: async () => false, subscribe: writable(false).subscribe };

export async function goto(_url: string, _opts?: { replaceState?: boolean; noScroll?: boolean }) {
  // No-op in tests
}

export async function invalidate(_url?: string) {
  // No-op in tests
}

export async function invalidateAll() {
  // No-op in tests
}

export function beforeNavigate(_callback: (navigation: unknown) => void) {
  // No-op in tests
}

export function afterNavigate(_callback: (navigation: unknown) => void) {
  // No-op in tests
}
