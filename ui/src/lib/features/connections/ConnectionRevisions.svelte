<!--
  Revisions: every compile of this connection, newest first. A revision is
  immutable and content-addressed; its JSON is the exact byte sequence `serve`
  mounts, so "Copy JSON" and "Download" hand over `revisionJson` verbatim
  (`<id>-r<N>.json`), never a re-serialisation.
-->
<script lang="ts">
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Copy from '@lucide/svelte/icons/copy';
  import Download from '@lucide/svelte/icons/download';
  import GitCommit from '@lucide/svelte/icons/git-commit-horizontal';
  import { CodeEditor } from '$lib/ui/editor';
  import { Button, EmptyState, KeyValue, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import { formatTimestamp } from '$lib/features/operator/attemptPresentation';
  import type { ConnectionRevisionRow } from './connectionsApi';
  import { formatMinute, revisionFileName, shortHash } from './presentation';

  interface Props {
    revisions: ConnectionRevisionRow[] | null;
    loading: boolean;
    error: string | null;
    selectedRevisionId: string | null;
    onselect: (revisionId: string) => void;
    onretry: () => void;
  }

  let { revisions, loading, error, selectedRevisionId, onselect, onretry }: Props = $props();

  const selected = $derived(
    revisions?.find((revision) => revision.revisionId === selectedRevisionId) ?? revisions?.[0] ?? null
  );

  // The stored bytes are compact JSON; the viewer indents them for reading.
  // Copy JSON and Download always hand over the stored bytes, unchanged.
  const readable = $derived.by(() => {
    if (!selected) return '';
    try {
      return JSON.stringify(JSON.parse(selected.revisionJson), null, 2);
    } catch {
      return selected.revisionJson;
    }
  });
  const byteCount = $derived(selected ? new TextEncoder().encode(selected.revisionJson).length : 0);

  let copyError = $state<string | null>(null);

  async function copyJson(revision: ConnectionRevisionRow): Promise<void> {
    copyError = null;
    try {
      await navigator.clipboard.writeText(revision.revisionJson);
      toasts.success('Copied');
    } catch {
      copyError = 'The browser refused clipboard access. Use Download instead.';
    }
  }

  /** Writes `revisionJson` byte for byte (UTF-8, no trailing newline added). */
  function download(revision: ConnectionRevisionRow): void {
    const blob = new Blob([revision.revisionJson], { type: 'application/json' });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement('a');
    anchor.href = url;
    anchor.download = revisionFileName(revision.artifactId, revision.revisionId);
    anchor.rel = 'noopener';
    document.body.append(anchor);
    anchor.click();
    anchor.remove();
    setTimeout(() => URL.revokeObjectURL(url), 1_000);
  }
</script>

<div class="revisions" data-testid="connection-revisions">
  {#if loading && !revisions}
    <EmptyState message="Loading revisions" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={onretry} />
  {:else if !revisions || revisions.length === 0}
    <EmptyState icon={GitCommit} align="start" message="No revision has been compiled." />
  {:else}
    <Table label="Connection revisions" layout="fixed" class="revision-table">
      {#snippet head()}
        <tr>
          <Th width="48px">Rev</Th>
          <Th>Digest</Th>
          <Th width="56px" numeric>From v</Th>
          <Th width="104px">By</Th>
          <Th width="124px">Created</Th>
        </tr>
      {/snippet}
      {#each revisions as revision (revision.revisionId)}
        <Tr
          selectable
          selected={revision.revisionId === selected?.revisionId}
          onselect={() => onselect(revision.revisionId)}
          data-revision={revision.revisionId}
        >
          <Td mono value={`r${revision.revisionId}`} />
          <Td mono truncate title={revision.digest} value={shortHash(revision.digest, 16)} />
          <Td numeric value={revision.compiledFromVersion} />
          <Td mono truncate muted value={revision.createdBy.id} />
          <Td mono muted title={revision.createdAt} value={formatMinute(revision.createdAt)} />
        </Tr>
      {/each}
    </Table>

    {#if selected}
      <section
        class="revision"
        aria-label={`Revision r${selected.revisionId}`}
        data-testid="revision-detail"
        data-digest={selected.digest}
      >
        <div class="revision-head">
          <KeyValue
            class="revision-facts"
            items={[
              { key: 'Revision', value: `r${selected.revisionId}`, mono: true },
              { key: 'Digest', value: selected.digest, mono: true },
              { key: 'From draft version', value: selected.compiledFromVersion, mono: true },
              { key: 'Created by', value: selected.createdBy.id, mono: true },
              { key: 'Created', value: formatTimestamp(selected.createdAt), mono: true },
              { key: 'Reason', value: selected.createdReason }
            ]}
          />
          <div class="revision-actions">
            <Button icon={Copy} onclick={() => copyJson(selected)}>Copy JSON</Button>
            <Button icon={Download} onclick={() => download(selected)}>Download</Button>
          </div>
        </div>
        {#if copyError}
          <p class="copy-error" role="alert">{copyError}</p>
        {/if}
        <p class="bytes-note">
          Indented for reading. Copy JSON and Download give the stored {byteCount.toLocaleString()} bytes exactly.
        </p>
        <div class="revision-json">
          <CodeEditor language="json" value={readable} readOnly lineNumbers={false} height="280px" />
        </div>
      </section>
    {/if}
  {/if}
</div>

<style>
  .revisions {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .revision {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .revision-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .revision-head :global(.revision-facts) {
    flex: 1 1 auto;
  }

  .revision-actions {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .copy-error,
  .bytes-note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .bytes-note {
    color: var(--color-text-tertiary);
  }

  .revision-json {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }
</style>
