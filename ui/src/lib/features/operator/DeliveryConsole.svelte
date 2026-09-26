<script lang="ts">
  /**
   * Delivery reliability console: the dead-letter queue, destination circuit
   * state, and the recovery actions the control plane exposes. Each dead letter
   * can open its Delivery block — what the destination provenance ledger
   * recorded for that attempt (Slice 4.2c) — before the operator decides.
   *
   * Every action opens the reason-required dialog. Actions that the server
   * would refuse are disabled with an explanatory title instead of firing and
   * being rejected (toast-budget B2).
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Inbox from '@lucide/svelte/icons/inbox';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import Send from '@lucide/svelte/icons/send';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import Zap from '@lucide/svelte/icons/zap';
  import {
    Badge,
    Button,
    EmptyState,
    IconButton,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type IconComponent
  } from '$lib/ui/primitives';
  import {
    badgeTone,
    circuitStateVariant,
    deadLetterStateLabel,
    deliveryActionBlockedReason,
    formatTimestamp,
    shortDigest,
    type DeliveryAction
  } from './attemptPresentation';
  import DestinationDeliveries from './DestinationDeliveries.svelte';
  import {
    fetchAttempt,
    fetchCircuits,
    fetchDeadLetters,
    type OperatorCircuit,
    type OperatorDeadLetter,
    type OperatorDestinationDelivery
  } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { deliveryControlBlock } from './operatorAccess';

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

  const recoveryActions: DeliveryAction[] = ['replay', 'resubmit', 'discard'];
  const actionIcons: Record<DeliveryAction, IconComponent> = {
    replay: RotateCcw,
    resubmit: Send,
    discard: Trash2
  };

  /**
   * A dead letter does not carry the provenance ledger, so each row's Delivery
   * block is fetched on demand — one bounded attempt read per row the operator
   * opens, never one per row rendered.
   */
  type DeliveryDetail = {
    loading: boolean;
    error: string | null;
    deliveries: OperatorDestinationDelivery[];
  };
  let expanded: Record<string, boolean> = {};
  let details: Record<string, DeliveryDetail> = {};

  async function toggleDeliveries(attemptId: string) {
    const open = !expanded[attemptId];
    expanded = { ...expanded, [attemptId]: open };
    if (open && !details[attemptId]) {
      await loadDeliveries(attemptId);
    }
  }

  async function loadDeliveries(attemptId: string) {
    details = { ...details, [attemptId]: { loading: true, error: null, deliveries: [] } };
    try {
      const attempt = await fetchAttempt(attemptId);
      details = {
        ...details,
        [attemptId]: attempt
          ? { loading: false, error: null, deliveries: attempt.deliveries }
          : {
              loading: false,
              error: `Delivery attempt ${attemptId} is not available in your tenant.`,
              deliveries: []
            }
      };
    } catch (err) {
      // Operator reads opt out of the global toast; the row is the only home
      // for the message (toast-budget B4).
      details = {
        ...details,
        [attemptId]: { loading: false, error: describeOperatorFailure(err).message, deliveries: [] }
      };
    }
  }

  export async function reload() {
    loading = true;
    error = null;
    circuitError = null;
    expanded = {};
    details = {};
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
   * attempt shape. An identity without the delivery role is blocked first.
   */
  function blockedReason(
    entry: OperatorDeadLetter,
    action: DeliveryAction,
    capabilityBlock: string | null
  ): string | null {
    if (capabilityBlock) return capabilityBlock;
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

<Panel title="Dead letters" flush>
  {#snippet actions()}
    <Button variant="ghost" onclick={toggleScope} disabled={loading}>
      {activeOnly ? 'Show resolved too' : 'Show open only'}
    </Button>
    <IconButton icon={RefreshCw} label="Refresh dead letters" {loading} onclick={reload} />
  {/snippet}

  {#if loading}
    <EmptyState align="start" message="Loading dead letters" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      align="start"
      role="alert"
      message={error}
      actionLabel="Retry"
      onaction={reload}
    />
  {:else if deadLetters.length === 0}
    <EmptyState
      icon={Inbox}
      align="start"
      message={activeOnly
        ? 'No open dead letters. Deliveries that exhaust their retries appear here.'
        : 'No dead letters recorded for this tenant.'}
    />
  {:else}
    <Table label="Durable dead-letter entries" layout="fixed">
      {#snippet head()}
        <tr>
          <Th width="330px">Attempt</Th>
          <Th width="204px">State</Th>
          <Th>Failure</Th>
          <Th width="76px" numeric>Replays</Th>
          <Th width="164px">Failed at</Th>
          <Th width="316px">Actions</Th>
        </tr>
      {/snippet}
      {#each deadLetters as entry, index (entry.attemptId)}
        {@const detail = details[entry.attemptId]}
        {@const open = expanded[entry.attemptId] ?? false}
        <Tr>
          <Td>
            <span class="attempt">
              <IconButton
                icon={open ? ChevronDown : ChevronRight}
                label={open ? 'Hide delivery' : 'Show delivery'}
                aria-expanded={open ? 'true' : 'false'}
                aria-controls={`dlq-deliveries-${index}`}
                onclick={() => toggleDeliveries(entry.attemptId)}
              />
              <button
                type="button"
                class="link"
                title={`Open the trace for ${entry.attemptId}`}
                on:click={() => dispatch('inspect', { attemptId: entry.attemptId })}
              >
                {entry.attemptId}
              </button>
            </span>
          </Td>
          <Td>
            <Badge tone={entry.active ? 'warning' : 'neutral'} dot>
              {deadLetterStateLabel(entry)}
            </Badge>
          </Td>
          <Td truncate title={`${entry.failureCode}: ${entry.failureDetail}`}>
            <code class="failure-code">{entry.failureCode}</code>
            <span class="detail">{entry.failureDetail}</span>
          </Td>
          <Td numeric value={entry.replayCount} />
          <Td mono muted value={formatTimestamp(entry.failedAt)} />
          <Td>
            <span class="row-actions">
              {#each recoveryActions as action (action)}
                {@const blocked = blockedReason(entry, action, $deliveryControlBlock)}
                <Button
                  variant={action === 'discard' ? 'danger' : 'secondary'}
                  icon={actionIcons[action]}
                  disabled={blocked !== null}
                  title={blocked ?? undefined}
                  onclick={() => dispatch('control', { action, attemptId: entry.attemptId })}
                >
                  {actionLabel(action)}
                </Button>
              {/each}
            </span>
          </Td>
        </Tr>
        {#if open}
          <tr class="detail-row" id={`dlq-deliveries-${index}`}>
            <td colspan="6">
              <h4 class="block-title">Delivery</h4>
              {#if !detail || detail.loading}
                <p class="detail" aria-busy="true" aria-live="polite">Loading destination deliveries</p>
              {:else if detail.error}
                <p class="error-message" role="alert">{detail.error}</p>
              {:else}
                <DestinationDeliveries
                  deliveries={detail.deliveries}
                  label={`Destination deliveries for ${entry.attemptId}, newest first`}
                />
              {/if}
            </td>
          </tr>
        {/if}
      {/each}
    </Table>
  {/if}
</Panel>

<Panel title="Destination circuits" flush>
  {#if loading}
    <EmptyState align="start" message="Loading circuit state" aria-busy="true" />
  {:else if circuitError}
    <EmptyState
      icon={CircleAlert}
      align="start"
      role="alert"
      message={circuitError}
      actionLabel="Retry"
      onaction={reload}
    />
  {:else if circuits.length === 0}
    <EmptyState
      icon={Zap}
      align="start"
      message="No circuit state recorded. A destination gets one after its first delivery."
    />
  {:else}
    <Table label="Destination circuit state" layout="fixed">
      {#snippet head()}
        <tr>
          <Th>Destination</Th>
          <Th width="96px">State</Th>
          <Th width="88px" numeric>Failures</Th>
          <Th width="164px">Open until</Th>
          <Th width="164px">Updated</Th>
        </tr>
      {/snippet}
      {#each circuits as circuit (circuit.destination.artifactId + circuit.destination.revisionId)}
        <Tr>
          <Td
            mono
            truncate
            title={circuit.destination.digest}
            value={`${circuit.destination.artifactId}@${circuit.destination.revisionId} · ${shortDigest(circuit.destination.digest)}`}
          />
          <Td>
            <Badge tone={badgeTone(circuitStateVariant(circuit.state))} dot>{circuit.state}</Badge>
          </Td>
          <Td numeric value={circuit.consecutiveFailures} />
          <Td mono muted value={formatTimestamp(circuit.openUntil)} />
          <Td mono muted value={formatTimestamp(circuit.updatedAt)} />
        </Tr>
      {/each}
    </Table>
  {/if}
</Panel>

<style>
  .attempt {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    overflow: hidden;
  }

  .link {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
    cursor: pointer;
  }

  .link:hover {
    text-decoration-color: currentColor;
  }

  .link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
    border-radius: var(--radius-sm);
  }

  .failure-code {
    margin-right: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-danger-text);
  }

  .detail {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .row-actions {
    display: inline-flex;
    gap: var(--space-1);
  }

  .detail-row td {
    padding: var(--space-2) var(--space-3) var(--space-3) 44px;
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-base);
  }

  .block-title {
    margin: 0 0 var(--space-2);
    font-family: var(--font-ui);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .error-message {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }
</style>
