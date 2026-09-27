/**
 * Test helper: a deep `$state` proxy of `value`, the way the page holds its
 * edit buffers, so a component rendered alone binds to reactive state.
 */
export function reactive<T extends object>(value: T): T {
  const state = $state(value);
  return state;
}
