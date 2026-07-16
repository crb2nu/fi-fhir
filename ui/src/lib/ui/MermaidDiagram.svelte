<script lang="ts" context="module">
  // Unique element-id prefix per component instance (mermaid.render requires one).
  let instanceCounter = 0;
</script>

<script lang="ts">
  /**
   * MermaidDiagram — renders mermaid diagram text to inline SVG.
   *
   * The mermaid library (~large) is imported lazily on first render, so it
   * never lands in the initial bundle. Source text is not guaranteed valid
   * mermaid (workflow explanations are LLM-generated), so a parse/render
   * failure falls back to showing the raw source rather than breaking the
   * surrounding panel. Renders follow the applied app theme.
   */
  import { onMount } from 'svelte';
  import { appliedTheme } from '$lib/theme/theme';

  export let source: string;

  const instanceId = ++instanceCounter;

  let svg = '';
  let error = '';
  let mounted = false;
  let renderSeq = 0;

  onMount(() => {
    mounted = true;
  });

  $: if (mounted) void renderDiagram(source, $appliedTheme);

  async function renderDiagram(src: string, theme: 'light' | 'dark'): Promise<void> {
    const seq = ++renderSeq;
    error = '';
    if (!src.trim()) {
      svg = '';
      return;
    }
    try {
      const mermaid = (await import('mermaid')).default;
      mermaid.initialize({
        startOnLoad: false,
        securityLevel: 'strict',
        theme: theme === 'dark' ? 'dark' : 'default'
      });
      const { svg: rendered } = await mermaid.render(`mermaid-${instanceId}-${seq}`, src);
      if (seq !== renderSeq) return; // superseded by a newer render
      svg = rendered;
    } catch (e) {
      if (seq !== renderSeq) return;
      svg = '';
      error = e instanceof Error ? e.message : 'Failed to render diagram';
    }
  }
</script>

{#if svg}
  <!-- eslint-disable-next-line svelte/no-at-html-tags -- mermaid output, sanitized via securityLevel: 'strict' -->
  <div class="mermaid-diagram">{@html svg}</div>
{:else if error}
  <div class="mermaid-fallback">
    <p class="fallback-note">Diagram could not be rendered</p>
    <pre class="fallback-source">{source}</pre>
  </div>
{:else if source.trim()}
  <div class="mermaid-loading">Rendering diagram…</div>
{/if}

<style>
  .mermaid-diagram {
    display: flex;
    justify-content: center;
    padding: 12px;
    border-radius: 8px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    overflow-x: auto;
  }

  .mermaid-diagram :global(svg) {
    max-width: 100%;
    height: auto;
  }

  .mermaid-fallback {
    padding: 8px 12px;
    border-radius: 8px;
    border: 1px dashed var(--color-border-default);
    background: var(--color-bg-surface);
  }

  .fallback-note {
    margin: 0 0 6px;
    color: var(--color-text-muted);
    font-size: 0.75rem;
  }

  .fallback-source {
    margin: 0;
    color: var(--color-text-secondary);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
    font-size: 0.8rem;
    line-height: 1.5;
    white-space: pre-wrap;
    word-break: break-word;
  }

  .mermaid-loading {
    padding: 8px 12px;
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }
</style>
