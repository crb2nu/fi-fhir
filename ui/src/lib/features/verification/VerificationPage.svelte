<!--
  Verification (/events, journey stage 5) — .loom/42 E-2.

  What the engine actually admitted, read from the durable tables through the
  operator control plane: Admissions (canonical events joined to their
  receipts), Statistics (counts and a per-bucket series over a window) and
  Retention (tombstones and the purge posture). Before any query the page
  pre-flights `controlPlane` and `operatorRead` in the Connections precedence
  and, when either is missing, says so and mounts no view.

  `/events?receipt=<id>` opens Admissions filtered to that receipt.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { afterNavigate, goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import { operatorPreflight } from '$lib/features/operator/operatorAccess';
  import AdmissionsBrowser from './AdmissionsBrowser.svelte';
  import RetentionView from './RetentionView.svelte';
  import StatisticsView from './StatisticsView.svelte';
  import VerificationPreflight from './VerificationPreflight.svelte';
  import { defaultWindowChoice, type WindowChoice } from './statisticsWindow';
  import { parseVerificationDeepLink } from './verificationLinks';

  const views: TabItem[] = [
    { id: 'admissions', label: 'Admissions', controls: 'verification-view' },
    { id: 'statistics', label: 'Statistics', controls: 'verification-view' },
    { id: 'retention', label: 'Retention', controls: 'verification-view' }
  ];

  const preflight = $derived(operatorPreflight($accessCapabilities));

  let view = $state('admissions');
  let ready = $state(false);
  let receiptLink = $state<string | null>(null);
  let windowChoice = $state<WindowChoice>(defaultWindowChoice());
  let selector: string | null | undefined;

  function syncSelector(url: URL): void {
    const receiptId = parseVerificationDeepLink(url.search)?.receiptId ?? null;
    if (selector === receiptId) {
      if (receiptId) view = 'admissions';
      return;
    }
    selector = receiptId;
    receiptLink = receiptId;
    view = 'admissions';
  }

  function rememberReceipt(receiptId: string | null): void {
    selector = receiptId;
    receiptLink = receiptId;
    const url = new URL(window.location.href);
    if (receiptId) url.searchParams.set('receipt', receiptId);
    else url.searchParams.delete('receipt');
    // Complete a Kit navigation so the shell remembers this receipt's URL.
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- only changes the receipt on the already-resolved current URL
    void goto(url, { replaceState: true, noScroll: true, keepFocus: true });
  }

  onMount(() => {
    syncSelector(new URL(window.location.href));
    ready = true;
  });
  afterNavigate(({ to }) => {
    if (to) syncSelector(to.url);
  });
</script>

<svelte:head>
  <title>Verification | fi-fhir</title>
</svelte:head>

<div class="verification">
  {#if preflight}
    <Toolbar title="Verification" />
    <VerificationPreflight {preflight} />
  {:else}
    <Toolbar title="Verification">
      {#snippet tabs()}
        <Tabs label="Verification views" items={views} bind:value={view} />
      {/snippet}
    </Toolbar>

    <p class="notes" data-testid="verification-notes">
      <span data-testid="verification-not-streamed">
        Admissions are read from what the engine committed and are not streamed; a session's run stream is on
        <a href={resolve('/hl7')}>HL7 intake</a>.
      </span>
      <span data-testid="verification-no-timeline">
        There is no patient timeline: payload values are never read back, by design, so nothing here is arranged by
        patient.
      </span>
    </p>

    <div
      class="view"
      id="verification-view"
      role="tabpanel"
      aria-label={views.find((item) => item.id === view)?.label}
    >
      {#if ready}
        {#if view === 'admissions'}
          <AdmissionsBrowser receiptId={receiptLink} onreceiptchange={rememberReceipt} />
        {:else if view === 'statistics'}
          <StatisticsView bind:choice={windowChoice} />
        {:else}
          <RetentionView bind:choice={windowChoice} />
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .verification {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .notes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-3);
    margin: 0;
    padding: var(--space-2) var(--space-3) 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .notes a {
    color: var(--color-text-secondary);
    text-decoration: underline;
    text-underline-offset: 2px;
  }

  .view {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
  }
</style>
