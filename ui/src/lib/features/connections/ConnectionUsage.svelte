<!--
  Usage: which lifecycle definition revisions name a revision of this
  connection, and whether this replica runs one. Both come from the catalog's
  own projection; nothing is inferred from names.
-->
<script lang="ts">
  import Link from '@lucide/svelte/icons/link';
  import { EmptyState, KeyValue, Panel, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import type { ConnectionRevisionRow, ConnectionRow } from './connectionsApi';
  import { shortHash } from './presentation';

  interface Props {
    row: ConnectionRow;
    /** The connection's revisions, when loaded, to name a referenced digest's revision. */
    revisions: ConnectionRevisionRow[] | null;
  }

  let { row, revisions }: Props = $props();

  const revisionByDigest = $derived(
    new Map((revisions ?? []).map((revision) => [revision.digest, revision.revisionId]))
  );

  function connectionRevision(digest: string): string {
    const revisionId =
      revisionByDigest.get(digest) ??
      (row.latestRevision?.digest === digest ? row.latestRevision.revisionId : undefined);
    return revisionId ? `r${revisionId}` : shortHash(digest);
  }
</script>

<div class="usage" data-testid="connection-usage">
  <Panel title="Referenced by" titleTag="h3" flush>
    {#if row.references.length === 0}
      <EmptyState
        icon={Link}
        align="start"
        message="No integration definition revision names a revision of this connection."
      />
    {:else}
      <Table label="Referencing definitions" layout="fixed">
        {#snippet head()}
          <tr>
            <Th>Definition</Th>
            <Th width="84px">Def. rev</Th>
            <Th width="96px">Connection</Th>
            <Th width="88px">State</Th>
            <Th width="80px">Health</Th>
          </tr>
        {/snippet}
        {#each row.references as reference (`${reference.definitionId}@${reference.revisionId}:${reference.digest}`)}
          <Tr>
            <Td mono truncate value={reference.definitionId} />
            <Td mono truncate value={reference.revisionId} />
            <Td mono title={reference.digest} value={connectionRevision(reference.digest)} />
            <Td value={reference.state} />
            <Td value={reference.health} />
          </Tr>
        {/each}
      </Table>
    {/if}
  </Panel>

  <Panel title="This replica" titleTag="h3">
    <KeyValue
      items={[
        { key: 'Mounted here', value: row.runtime.mounted ? 'Yes' : 'No' },
        { key: 'Role', value: row.runtime.role, mono: true },
        { key: 'Revision', value: row.runtime.revisionId, mono: true }
      ]}
    >
      <!-- Truncated with the full digest in title, as the Revisions tab does. -->
      <dt>Digest</dt>
      <dd class="digest" class:empty={!row.runtime.digest} title={row.runtime.digest ?? undefined}>
        {row.runtime.digest ? shortHash(row.runtime.digest, 16) : '—'}
      </dd>
      <dt>Detail</dt>
      <dd class:empty={!row.runtime.detail}>{row.runtime.detail || '—'}</dd>
    </KeyValue>
  </Panel>
</div>

<style>
  .usage {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .digest {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .empty {
    color: var(--color-text-muted);
  }
</style>
