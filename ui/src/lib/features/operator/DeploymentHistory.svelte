<!--
  DeploymentHistory — the append-only lifecycle history of one definition
  revision (`operatorDeploymentEvents`): every transition with its version,
  from → to, health, release, actor and reason. Read only.
-->
<script lang="ts">
  import { Badge, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { badgeTone, deploymentStateVariant, formatTimestamp } from './attemptPresentation';
  import { fetchDeploymentEvents, type OperatorDeploymentEvent } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';

  interface Props {
    definitionId: string;
    revisionId: string;
  }

  let { definitionId, revisionId }: Props = $props();

  let events = $state<OperatorDeploymentEvent[]>([]);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let seq = 0;

  async function load(definition: string, revision: string): Promise<void> {
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const answer = await fetchDeploymentEvents(definition, revision);
      if (current !== seq) return;
      // Newest first: the operator is usually asking what happened last.
      events = [...answer].sort((a, b) => b.version - a.version);
    } catch (err) {
      if (current !== seq) return;
      events = [];
      error = describeOperatorFailure(err).message;
    } finally {
      if (current === seq) loading = false;
    }
  }

  $effect(() => {
    void load(definitionId, revisionId);
  });
</script>

<div class="history" data-testid="deployment-history" data-definition-id={definitionId} data-revision-id={revisionId}>
  <h4 class="title">Lifecycle history of {definitionId}@{revisionId}</h4>
  {#if loading}
    <p class="note" aria-busy="true">Loading lifecycle history</p>
  {:else if error}
    <p class="error" role="alert">{error}</p>
  {:else if events.length === 0}
    <p class="note">No lifecycle transition is recorded for this revision.</p>
  {:else}
    <Table label={`Lifecycle history of ${definitionId}@${revisionId}`} layout="fixed" class="history-table">
      {#snippet head()}
        <tr>
          <Th width="44px" numeric>Ver</Th>
          <Th width="164px">Action</Th>
          <Th width="200px">From → to</Th>
          <Th width="84px">Health</Th>
          <Th>Release</Th>
          <Th>Actor and reason</Th>
          <Th width="152px">Occurred</Th>
        </tr>
      {/snippet}
      {#each events as event (event.eventId)}
        <Tr data-testid="history-row" data-action={event.action}>
          <Td numeric value={event.version} />
          <Td mono value={event.action} />
          <Td>
            <span class="states">
              {#if event.fromState}
                <Badge tone={badgeTone(deploymentStateVariant(event.fromState))}>{event.fromState}</Badge> →
              {/if}
              <Badge tone={badgeTone(deploymentStateVariant(event.toState))} dot>{event.toState}</Badge>
            </span>
          </Td>
          <Td muted value={event.health} />
          <Td mono truncate muted value={event.releaseId ?? '—'} />
          <Td truncate value={`${event.actor.id || 'unknown'}${event.reason ? ` — “${event.reason}”` : ''}`} />
          <Td mono muted value={formatTimestamp(event.occurredAt)} />
        </Tr>
      {/each}
    </Table>
  {/if}
</div>

<style>
  .history {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .title {
    margin: 0;
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .states {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    color: var(--color-text-tertiary);
  }

  .note,
  .error {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .error {
    color: var(--color-danger-text);
  }

  .history :global(.history-table) {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }
</style>
