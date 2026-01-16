<script lang="ts">
  import { onMount } from 'svelte';

  export let data: unknown;
  export let maxHeight = '520px';

  let copied = false;
  let copyTimeout: ReturnType<typeof setTimeout>;

  $: jsonString = JSON.stringify(data, null, 2);
  $: highlighted = highlightJson(jsonString);

  function highlightJson(json: string): string {
    if (!json) return '';

    // Escape HTML first
    const escaped = json
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;');

    // Apply syntax highlighting
    return escaped
      // Strings (must come before other patterns)
      .replace(/"([^"\\]*(\\.[^"\\]*)*)"/g, (match, content) => {
        // Check if it's a key (followed by :) or a value
        return `<span class="json-string">"${content}"</span>`;
      })
      // Numbers
      .replace(/\b(-?\d+\.?\d*(?:[eE][+-]?\d+)?)\b/g, '<span class="json-number">$1</span>')
      // Booleans
      .replace(/\b(true|false)\b/g, '<span class="json-boolean">$1</span>')
      // Null
      .replace(/\bnull\b/g, '<span class="json-null">null</span>')
      // Keys (strings followed by :)
      .replace(/<span class="json-string">"([^"]+)"<\/span>:/g, '<span class="json-key">"$1"</span>:');
  }

  async function copyToClipboard() {
    try {
      await navigator.clipboard.writeText(jsonString);
      copied = true;
      clearTimeout(copyTimeout);
      copyTimeout = setTimeout(() => {
        copied = false;
      }, 2000);
    } catch (err) {
      console.error('Failed to copy:', err);
    }
  }

  onMount(() => {
    return () => clearTimeout(copyTimeout);
  });
</script>

<div class="json-viewer">
  <div class="toolbar">
    <span class="label">JSON</span>
    <button class="copy-btn" on:click={copyToClipboard} title="Copy to clipboard">
      {#if copied}
        <span class="copied">Copied!</span>
      {:else}
        <span class="copy-icon">Copy</span>
      {/if}
    </button>
  </div>
  <pre class="json-content" style="max-height: {maxHeight};">{@html highlighted}</pre>
</div>

<style>
  .json-viewer {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    overflow: hidden;
  }

  .toolbar {
    display: flex;
    justify-content: space-between;
    align-items: center;
    padding: 8px 12px;
    background: rgba(255, 255, 255, 0.03);
    border-bottom: 1px solid rgba(255, 255, 255, 0.06);
  }

  .label {
    font-size: 0.75rem;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.05em;
    color: rgba(229, 231, 235, 0.5);
  }

  .copy-btn {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: all 0.15s ease;
  }

  .copy-btn:hover {
    background: rgba(255, 255, 255, 0.08);
    border-color: rgba(255, 255, 255, 0.18);
  }

  .copied {
    color: rgba(16, 185, 129, 0.95);
  }

  .json-content {
    margin: 0;
    padding: 12px;
    overflow: auto;
    color: rgba(229, 231, 235, 0.9);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
    font-size: 0.85rem;
    line-height: 1.5;
    tab-size: 2;
    white-space: pre;
  }

  /* Syntax highlighting */
  :global(.json-key) {
    color: rgba(147, 197, 253, 0.95);
  }

  :global(.json-string) {
    color: rgba(134, 239, 172, 0.95);
  }

  :global(.json-number) {
    color: rgba(253, 186, 116, 0.95);
  }

  :global(.json-boolean) {
    color: rgba(192, 132, 252, 0.95);
  }

  :global(.json-null) {
    color: rgba(156, 163, 175, 0.8);
    font-style: italic;
  }
</style>
