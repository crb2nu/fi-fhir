import { createRawSnippet, type Snippet } from 'svelte';

/** A static snippet for tests: `text('Run')` renders <span>Run</span>. */
export function text(value: string, tag = 'span'): Snippet {
  return createRawSnippet(() => ({ render: () => `<${tag}>${value}</${tag}>` }));
}
