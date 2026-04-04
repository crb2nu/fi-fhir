<script lang="ts">
  import type { WorkspaceDocument, DocumentType } from './types';

  /**
   * Renders the content surface for a non-route workspace document.
   * Each artifact type gets a polished placeholder that fits the IDE aesthetic.
   */

  export let document: WorkspaceDocument;

  type DocMeta = {
    label: string;
    color: string;
    iconPath: string;
    description: string;
  };

  const DOC_META: Record<Exclude<DocumentType, 'route'>, DocMeta> = {
    'workflow-draft': {
      label: 'Workflow Draft',
      color: 'var(--color-primary)',
      iconPath: 'M6 3v12 M18 9a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M6 21a3 3 0 1 0 0-6 3 3 0 0 0 0 6z M18 9a9 9 0 0 1-9 9',
      description: 'Build and iterate on your integration workflow. Define source mappings, transforms, and delivery targets.',
    },
    'debug-session': {
      label: 'Debug Session',
      color: 'var(--color-warning)',
      iconPath: 'M12 2L2 7l10 5 10-5-10-5z M2 17l10 5 10-5 M2 12l10 5 10-5',
      description: 'Step through execution traces, inspect variable state, and diagnose mapping failures.',
    },
    trace: {
      label: 'Trace View',
      color: 'var(--color-info)',
      iconPath: 'M22 12h-4l-3 9L9 3l-3 9H2',
      description: 'Visualize execution spans, timing waterfall, and resource consumption across the pipeline.',
    },
    event: {
      label: 'Event',
      color: 'var(--color-success)',
      iconPath: 'M13 2L3 14h9l-1 8 10-12h-9l1-8z',
      description: 'Inspect event payloads, compare schemas, and validate structural conformance.',
    },
    profile: {
      label: 'Profile',
      color: '#8b5cf6',
      iconPath: 'M20 21v-2a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v2 M12 3a4 4 0 1 0 0 8 4 4 0 0 0 0-8z',
      description: 'Review source profile definitions, field constraints, and revision history.',
    },
  };

  $: meta = document.type && document.type !== 'route'
    ? DOC_META[document.type] ?? null
    : null;
  $: displayTitle = document.subtitle
    ? `${document.title} — ${document.subtitle}`
    : document.title;
</script>

{#if meta}
  <div class="doc-host" style="--doc-color: {meta.color}">
    <div class="doc-hero">
      <div class="doc-icon-ring">
        <svg
          viewBox="0 0 24 24"
          fill="none"
          stroke="currentColor"
          stroke-width="1.5"
          stroke-linecap="round"
          stroke-linejoin="round"
          aria-hidden="true"
          class="doc-icon"
        >
          {#each meta.iconPath.split(' M') as segment, i (i)}
            <path d={i === 0 ? segment : `M${segment}`} />
          {/each}
        </svg>
      </div>

      <div class="doc-eyebrow">{meta.label}</div>
      <h2 class="doc-title">{displayTitle}</h2>
      {#if document.artifactId}
        <div class="doc-artifact-id">{document.artifactId}</div>
      {/if}
      <p class="doc-description">{meta.description}</p>

      <div class="doc-badge">
        <span class="badge-dot" style="background: {meta.color}"></span>
        <span class="badge-text">Coming soon</span>
      </div>
    </div>

    <div class="doc-surface">
      <div class="surface-lines">
        <!-- eslint-disable-next-line @typescript-eslint/no-unused-vars -->
        {#each {length: 5} as _, i (i)}
          <div
            class="surface-line"
            style="width: {60 + Math.sin(i * 1.7) * 25}%; animation-delay: {i * 120}ms"
          ></div>
        {/each}
      </div>
    </div>
  </div>
{/if}

<style>
  .doc-host {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-6);
    height: 100%;
    padding: var(--space-8);
    overflow: auto;
    animation: fadeIn var(--duration-slow) var(--ease-out);
  }

  .doc-hero {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
    max-width: 420px;
    text-align: center;
  }

  .doc-icon-ring {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 64px;
    height: 64px;
    border-radius: var(--radius-2xl);
    background: color-mix(in srgb, var(--doc-color) 10%, transparent);
    border: 1px solid color-mix(in srgb, var(--doc-color) 25%, transparent);
    color: var(--doc-color);
    box-shadow: 0 0 24px color-mix(in srgb, var(--doc-color) 15%, transparent);
  }

  .doc-icon {
    width: 28px;
    height: 28px;
  }

  .doc-eyebrow {
    color: var(--doc-color);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  .doc-title {
    margin: 0;
    font-family: var(--font-heading);
    font-size: var(--text-xl);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-tight);
    color: var(--color-text-primary);
    line-height: var(--leading-tight);
  }

  .doc-artifact-id {
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-full);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  .doc-description {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  .doc-badge {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-3);
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    margin-top: var(--space-2);
  }

  .badge-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
  }

  .badge-text {
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .doc-surface {
    width: 100%;
    max-width: 480px;
    padding: var(--space-5);
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background:
      radial-gradient(ellipse at top, color-mix(in srgb, var(--doc-color) 4%, transparent), transparent 60%),
      var(--color-bg-surface);
    box-shadow:
      inset 0 1px 0 rgba(255, 255, 255, 0.05),
      var(--shadow-sm);
  }

  .surface-lines {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .surface-line {
    height: 8px;
    border-radius: var(--radius-sm);
    background: linear-gradient(
      90deg,
      color-mix(in srgb, var(--doc-color) 8%, transparent),
      color-mix(in srgb, var(--doc-color) 3%, transparent)
    );
    animation: fadeIn var(--duration-slow) var(--ease-out) both;
  }

  @media (prefers-reduced-motion: reduce) {
    .doc-host,
    .surface-line {
      animation: none;
    }
  }
</style>
