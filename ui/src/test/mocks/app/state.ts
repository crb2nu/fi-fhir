/**
 * Mock for $app/state
 *
 * `page.url` is backed by a store through `fromStore`, so assigning it in a
 * test re-runs the effects (and `toStore` bridges) that read it.
 */
import { fromStore, writable } from 'svelte/store';

const url = writable(new URL('http://localhost'));
const currentUrl = fromStore(url);

export const page = {
  get url(): URL {
    return currentUrl.current;
  },
  set url(value: URL) {
    url.set(value);
  },
  params: {},
  route: { id: '/' },
  status: 200,
  error: null,
  data: {},
  form: null,
  state: {}
};

export const navigating = {
  from: null,
  to: null,
  type: null,
  willUnload: null,
  delta: null,
  complete: null
};

export const updated = { current: false, check: async () => false };
