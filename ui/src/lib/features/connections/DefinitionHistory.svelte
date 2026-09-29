<!--
  A definition revision's append-only lifecycle history (operatorDeploymentEvents):
  every transition with its actor and reason. A local component; the Operator
  page grows its own History (E-0) and the two may later share one.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import { EmptyState, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { fetchDeploymentEvents } from '$lib/features/operator/operatorApi';
  import { describeDefinitionFailure } from './definitionsErrors';
  import { formatSecond } from './definitionPresentation';

  interface Props {
    definitionId: string;
    revisionId: string;
    /** Bumped by the parent after a write so the history reloads. */
    version: number;
  }

  let { definitionId, revisionId, version }: Props = $props();

  type HistoryEvent = Awaited<ReturnType<typeof fetchDeploymentEvents>>[number];
  let events = $state<HistoryEvent[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let seq = 0;

  async function load(): Promise<void> {
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const list = await fetchDeploymentEvents(definitionId, revisionId);
      if (current === seq) events = list;
    } catch (err) {
      if (current === seq) error = describeDefinitionFailure(err).message;
    } finally {
      if (current === seq) loading = false;
    }
  }

  onMount(() => void load());

  let loadedVersion = 0;
  $effect(() => {
    if (version !== loadedVersion) {
      const first = loadedVersion === 0;
      loadedVersion = version;
      if (!first) void load();
    }
  });
</script>

{#if loading && events.length === 0}
  <EmptyState message="Loading history" aria-busy="true" />
{:else if error}
  <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={load} />
{:else if events.length === 0}
  <EmptyState message="No lifecycle events are recorded for this revision." />
{:else}
  <Table label="Lifecycle history" layout="fixed" data-testid="definition-history">
    {#snippet head()}
      <tr>
        <Th width="40px">v</Th>
        <Th width="150px">Action</Th>
        <Th width="150px">State</Th>
        <Th>Actor · reason</Th>
        <Th width="150px">At</Th>
      </tr>
    {/snippet}
    {#each events as event (event.eventId)}
      <Tr data-action={event.action}>
        <Td mono value={String(event.version)} />
        <Td mono truncate value={event.action} />
        <Td mono truncate value={event.fromState ? `${event.fromState} → ${event.toState}` : event.toState} />
        <Td truncate title={event.reason} value={`${event.actor.id} · ${event.reason || '—'}`} />
        <Td mono muted value={formatSecond(event.occurredAt)} />
      </Tr>
    {/each}
  </Table>
{/if}
