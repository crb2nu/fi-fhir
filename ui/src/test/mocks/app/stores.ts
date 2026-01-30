/**
 * Mock for $app/stores
 */
import { readable, writable } from 'svelte/store';

export const page = readable({
  url: new URL('http://localhost'),
  params: {},
  route: { id: '/' },
  status: 200,
  error: null,
  data: {},
  form: null
});

export const navigating = writable(null);
export const updated = { check: async () => false, subscribe: writable(false).subscribe };
