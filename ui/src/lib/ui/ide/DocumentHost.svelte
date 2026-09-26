<script lang="ts">
  import { EmptyState, Toolbar } from '$lib/ui/primitives';
  import type { WorkspaceDocument } from './types';
  import { DOCUMENT_ICONS, DOCUMENT_TYPE_LABELS } from './viewIcons';

  /**
   * Content surface for a non-route workspace document (trace, workflow
   * draft, debug session, event, profile). These document types have no
   * editor yet, so the surface says exactly that — a toolbar with the
   * document's title and one honest sentence, not a placeholder illustration.
   */

  export let document: WorkspaceDocument;

  $: type = document.type && document.type !== 'route' ? document.type : null;
  $: displayTitle = document.subtitle ? `${document.title} — ${document.subtitle}` : document.title;
</script>

{#if type}
  <div class="doc-host" data-document-type={type}>
    <Toolbar title={displayTitle} titleTag="h2">
      {#snippet actions()}
        {#if document.artifactId}
          <span class="doc-artifact-id" title={document.artifactId}>{document.artifactId}</span>
        {/if}
      {/snippet}
    </Toolbar>
    <EmptyState icon={DOCUMENT_ICONS[type]}>
      {DOCUMENT_TYPE_LABELS[type]} documents have no editor in this build.
    </EmptyState>
  </div>
{/if}

<style>
  .doc-host {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .doc-artifact-id {
    max-width: 280px;
    overflow: hidden;
    color: var(--color-text-tertiary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
