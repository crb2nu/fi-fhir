<!--
  RetentionView — Verification › Retention: whether the retention purge runs
  on the replica that answered (`engineRuntime.retentionPurge`), and what the
  canonical events of one window carry: tombstoned (purged_at), scheduled
  (a purge_after deadline, payload intact), or no deadline. A purge replaces
  the payload with a tombstone and keeps the row, so every count here is a
  count of rows that still exist.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, EmptyState, IconButton, Panel } from '$lib/ui/primitives';
  import { describeOperatorFailure } from '$lib/features/operator/operatorErrors';
  import WindowPicker from './WindowPicker.svelte';
  import {
    fetchAdmissionStatistics,
    fetchRetentionPosture,
    type AdmissionStatistics,
    type RetentionPosture
  } from './verificationApi';
  import { describeWindow, resolveWindow, type WindowChoice } from './statisticsWindow';

  interface Props {
    choice: WindowChoice;
  }

  let { choice = $bindable() }: Props = $props();

  const POLICY_ENV = 'FI_FHIR_RETENTION_POLICY_PATH';

  let posture = $state<RetentionPosture | null>(null);
  let postureError = $state<string | null>(null);
  let stats = $state<AdmissionStatistics | null>(null);
  let statsError = $state<string | null>(null);
  let loading = $state(true);
  let seq = 0;

  const intact = $derived(stats ? stats.canonicalEvents - stats.purgedEvents - stats.scheduledForPurge : 0);

  async function loadPosture(): Promise<void> {
    try {
      posture = await fetchRetentionPosture();
      postureError = null;
    } catch (err) {
      posture = null;
      postureError = describeOperatorFailure(err).message;
    }
  }

  async function loadCounts(next: WindowChoice): Promise<void> {
    const resolved = resolveWindow(next);
    if (!resolved.ok) {
      statsError = resolved.message;
      return;
    }
    const current = ++seq;
    loading = true;
    statsError = null;
    try {
      const result = await fetchAdmissionStatistics(
        { from: resolved.window.from, to: resolved.window.to },
        resolved.window.bucket
      );
      if (current !== seq) return;
      stats = result;
    } catch (err) {
      if (current !== seq) return;
      statsError = describeOperatorFailure(err).message;
      stats = null;
    } finally {
      if (current === seq) loading = false;
    }
  }

  function reload(): void {
    void loadPosture();
    void loadCounts(choice);
  }

  onMount(reload);
</script>

<div class="retention" data-testid="verification-retention">
  <Panel title="Retention" flush>
    {#snippet actions()}
      <IconButton icon={RefreshCw} label="Refresh retention" {loading} onclick={reload} />
    {/snippet}

    <div class="posture" data-testid="retention-posture" data-purge={posture ? String(posture.retentionPurge) : 'unknown'}>
      {#if posture?.retentionPurge}
        <Badge tone="success" dot>purge on</Badge>
        <p>
          The retention purge runs on replica <code>{posture.replicaId}</code>: an event whose deadline has passed
          has its payload replaced by a tombstone. The row, its identifiers and its receipt stay.
        </p>
      {:else if posture}
        <Badge tone="neutral" dot>purge off</Badge>
        <p>
          The retention purge is off on replica <code>{posture.replicaId}</code>, so no payload is tombstoned by
          it. It runs when <code>{POLICY_ENV}</code> names a retention policy document.
        </p>
      {:else if postureError}
        <Badge tone="warning" dot>unknown</Badge>
        <p>Whether the retention purge runs could not be read: {postureError}</p>
      {:else}
        <p class="muted">Reading the engine's retention posture</p>
      {/if}
    </div>

    <div class="bar">
      <WindowPicker bind:choice disabled={loading} onapply={(next) => void loadCounts(next)} />
      {#if stats}
        <span class="window">{describeWindow(stats)}</span>
      {/if}
    </div>

    {#if loading && !stats}
      <EmptyState message="Counting retention marks" aria-busy="true" aria-live="polite" />
    {:else if statsError}
      <EmptyState
        icon={CircleAlert}
        role="alert"
        message={statsError}
        actionLabel="Retry"
        onaction={() => void loadCounts(choice)}
      />
    {:else if stats}
      <dl class="counts">
        <div data-testid="retention-purged" data-value={stats.purgedEvents}>
          <dt>Tombstoned</dt>
          <dd>{stats.purgedEvents.toLocaleString()}</dd>
        </div>
        <div data-testid="retention-scheduled" data-value={stats.scheduledForPurge}>
          <dt>Scheduled for purge</dt>
          <dd>{stats.scheduledForPurge.toLocaleString()}</dd>
        </div>
        <div data-testid="retention-unscheduled" data-value={intact}>
          <dt>No deadline</dt>
          <dd>{intact.toLocaleString()}</dd>
        </div>
      </dl>
      <p class="muted">
        Of {stats.canonicalEvents.toLocaleString()} canonical events recorded in this window. Admissions lists
        tombstoned events when Include tombstoned is checked.
      </p>
    {/if}
  </Panel>
</div>

<style>
  .posture {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .posture p {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
  }

  .bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .window {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .counts {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(160px, 1fr));
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-3);
  }

  .counts div {
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
  }

  .counts dt {
    font-size: var(--text-label);
    letter-spacing: 0.04em;
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .counts dd {
    margin: var(--space-1) 0 0;
    font-family: var(--font-mono);
    font-size: var(--text-lg);
    color: var(--color-text-primary);
  }

  .muted {
    margin: 0;
    padding: 0 var(--space-3) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }
</style>
