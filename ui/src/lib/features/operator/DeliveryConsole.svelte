<script lang="ts">
  /**
   * Delivery reliability console: the dead-letter queue, destination circuit
   * state, and the recovery actions the control plane exposes.
   *
   * Every action opens the reason-required dialog. Actions that the server
   * would refuse are disabled with an explanatory title instead of firing and
   * being rejected (toast-budget B2).
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import {
    circuitStateVariant,
    deadLetterStateLabel,
    deliveryActionBlockedReason,
    formatTimestamp,
    shortDigest,
    type DeliveryAction
  } from './attemptPresentation';
  import {
    fetchCircuits,
    fetchDeadLetters,
    type OperatorCircuit,
    type OperatorDeadLetter
  } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';

  const dispatch = createEventDispatcher<{
    control: { action: DeliveryAction; attemptId: string };
    inspect: { attemptId: string };
  }>();

  export let activeOnly = true;

  let deadLetters: OperatorDeadLetter[] = [];
  let circuits: OperatorCircuit[] = [];
  let loading = true;
  let error: string | null = null;
  let circuitError: string | null = null;

  const actions: DeliveryAction[] = ['replay', 'resubmit', 'discard'];

  export async function reload() {
    loading = true;
    error = null;
    circuitError = null;
    try {
      const page = await fetchDeadLetters(activeOnly, { first: 50, after: null });
      deadLetters = page.nodes;
    } catch (err) {
      error = describeOperatorFailure(err).message;
      deadLetters = [];
    }
    try {
      circuits = await fetchCircuits();
    } catch (err) {
      circuitError = describeOperatorFailure(err).message;
      circuits = [];
    }
    loading = false;
  }

  function toggleScope() {
    activeOnly = !activeOnly;
    void reload();
  }

  /**
   * A DLQ row carries the durable dead-letter state directly, so the same
   * precondition helper the trace view uses applies here with a synthetic
   * attempt shape.
   */
  function blockedReason(entry: OperatorDeadLetter, action: DeliveryAction): string | null {
    return deliveryActionBlockedReason(
      {
        status: 'failed',
        outboxStatus: 'failed',
        deadLetter: { active: entry.active, resolution: entry.resolution }
      },
      action
    );
  }

  function actionLabel(action: DeliveryAction): string {
    return action.charAt(0).toUpperCase() + action.slice(1);
  }

  onMount(() => {
    void reload();
  });
</script>

<Panel title="Dead-letter queue" padding="md">
  <svelte:fragment slot="actions">
    <Button size="sm" variant="ghost" on:click={toggleScope} disabled={loading}>
      {activeOnly ? 'Show resolved too' : 'Show open only'}
    </Button>
    <Button size="sm" variant="secondary" on:click={reload} disabled={loading}>Refresh</Button>
  </svelte:fragment>

  {#if loading}
    <div aria-busy="true" aria-live="polite">
      <Skeleton lines={3} />
      <span class="sr-only">Loading dead letters</span>
    </div>
  {:else if error}
    <div class="error-state" role="alert">
      <p class="error-message">{error}</p>
      <Button size="sm" variant="secondary" on:click={reload}>Retry</Button>
    </div>
  {:else if deadLetters.length === 0}
    <EmptyState
      icon="inbox"
      title={activeOnly ? 'No open dead letters' : 'No dead letters recorded'}
      description={activeOnly
        ? 'Delivery is keeping up. Failed deliveries that exhaust their retries appear here.'
        : 'Nothing has entered the dead-letter queue for this tenant.'}
    />
  {:else}
    <div class="table-scroll">
      <table class="records">
        <caption class="sr-only">Durable dead-letter entries</caption>
        <thead>
          <tr>
            <th scope="col">Attempt</th>
            <th scope="col">State</th>
            <th scope="col">Failure</th>
            <th scope="col">Replays</th>
            <th scope="col">Failed at</th>
            <th scope="col">Actions</th>
          </tr>
        </thead>
        <tbody>
          {#each deadLetters as entry (entry.attemptId)}
            <tr>
              <th scope="row">
                <button
                  type="button"
                  class="link"
                  on:click={() => dispatch('inspect', { attemptId: entry.attemptId })}
                >
                  {entry.attemptId}
                </button>
              </th>
              <td>
                <Badge variant={entry.active ? 'warning' : 'default'} size="sm">
                  {deadLetterStateLabel(entry)}
                </Badge>
              </td>
              <td>
                <code>{entry.failureCode}</code>
                <span class="detail">{entry.failureDetail}</span>
              </td>
              <td class="numeric">{entry.replayCount}</td>
              <td class="mono">{formatTimestamp(entry.failedAt)}</td>
              <td>
                <div class="row-actions">
                  {#each actions as action (action)}
                    {@const blocked = blockedReason(entry, action)}
                    <Button
                      size="sm"
                      variant={action === 'discard' ? 'danger' : 'secondary'}
                      disabled={blocked !== null}
                      title={blocked ?? undefined}
                      on:click={() =>
                        dispatch('control', { action, attemptId: entry.attemptId })}
                    >
                      {actionLabel(action)}
                    </Button>
                  {/each}
                </div>
              </td>
            </tr>
          {/each}
        </tbody>
      </table>
    </div>
  {/if}
</Panel>

<Panel title="Destination circuits" padding="md">
  {#if loading}
    <Skeleton lines={2} />
  {:else if circuitError}
    <div class="error-state" role="alert">
      <p class="error-message">{circuitError}</p>
      <Button size="sm" variant="secondary" on:click={reload}>Retry</Button>
    </div>
  {:else if circuits.length === 0}
    <EmptyState
      icon="data"
      title="No circuit state recorded"
      description="A destination gets a circuit entry the first time a delivery to it succeeds or fails."
    />
  {:else}
    <ul class="circuits">
      {#each circuits as circuit (circuit.destination.artifactId + circuit.destination.revisionId)}
        <li>
          <Badge variant={circuitStateVariant(circuit.state)} size="sm">{circuit.state}</Badge>
          <span class="mono" title={circuit.destination.digest}>
            {circuit.destination.artifactId}@{circuit.destination.revisionId}
            <span class="detail">{shortDigest(circuit.destination.digest)}</span>
          </span>
          <span class="detail">{circuit.consecutiveFailures} consecutive failures</span>
          {#if circuit.openUntil}
            <span class="detail">open until {formatTimestamp(circuit.openUntil)}</span>
          {/if}
          <span class="detail">updated {formatTimestamp(circuit.updatedAt)}</span>
        </li>
      {/each}
    </ul>
  {/if}
</Panel>

<style>
  .error-state {
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .error-message {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  .table-scroll {
    overflow-x: auto;
  }

  .records {
    width: 100%;
    border-collapse: collapse;
    font-size: var(--text-sm);
  }

  .records th,
  .records td {
    text-align: left;
    padding: var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
    vertical-align: top;
  }

  .records thead th {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    white-space: nowrap;
  }

  .numeric {
    text-align: right;
  }

  .row-actions {
    display: flex;
    gap: var(--space-1);
    flex-wrap: wrap;
  }

  .link {
    background: none;
    border: none;
    padding: 0;
    color: var(--color-primary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    text-decoration: underline;
  }

  .link:hover {
    color: var(--color-primary-hover);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .detail {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .circuits {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .circuits li {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
