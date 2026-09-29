<!--
  StatisticsSeries — the per-bucket series as two rows of plain CSS bars:
  receipts (accepted over rejected) and delivery attempts (succeeded, failed,
  queued). Heights are relative to the busiest bucket of each row; every bar
  carries its exact counts in its label. No chart library.
-->
<script lang="ts">
  import type { AdmissionStatistics } from './verificationApi';
  import { bucketLabel } from './statisticsWindow';

  interface Props {
    series: AdmissionStatistics['series'];
    bucket: AdmissionStatistics['bucket'];
  }

  let { series, bucket }: Props = $props();

  const receiptPeak = $derived(Math.max(1, ...series.map((point) => point.accepted + point.rejected)));
  const attemptPeak = $derived(
    Math.max(1, ...series.map((point) => point.succeeded + point.failed + point.queued))
  );

  function share(count: number, peak: number): string {
    return `${(count / peak) * 100}%`;
  }

  // Label every bucket when there are few; otherwise every nth, so labels never collide.
  const labelEvery = $derived(Math.max(1, Math.ceil(series.length / 12)));
</script>

<div class="series" data-testid="statistics-series" data-buckets={series.length}>
  <div class="legend" aria-hidden="true">
    <span><i class="swatch accepted"></i>accepted</span>
    <span><i class="swatch rejected"></i>rejected</span>
    <span><i class="swatch succeeded"></i>succeeded</span>
    <span><i class="swatch failed"></i>failed</span>
    <span><i class="swatch queued"></i>queued</span>
  </div>
  {#each [{ key: 'receipts', title: 'Receipts' }, { key: 'attempts', title: 'Delivery attempts' }] as row (row.key)}
    <div class="row">
      <span class="row-title">{row.title}</span>
      <ol class="bars" aria-label={`${row.title} per ${bucket === 'DAY' ? 'day' : 'hour'}`}>
        {#each series as point, index (point.start)}
          {@const label = bucketLabel(point.start, bucket)}
          {#if row.key === 'receipts'}
            <li
              class="bucket"
              data-testid="series-bucket"
              data-start={point.start}
              data-accepted={point.accepted}
              data-rejected={point.rejected}
              aria-label={`${label}: ${point.accepted} accepted, ${point.rejected} rejected`}
              title={`${label} UTC: ${point.accepted} accepted, ${point.rejected} rejected`}
            >
              <span class="stack">
                <span class="seg rejected" style:height={share(point.rejected, receiptPeak)}></span>
                <span class="seg accepted" style:height={share(point.accepted, receiptPeak)}></span>
              </span>
              <span class="tick">{index % labelEvery === 0 ? label : ''}</span>
            </li>
          {:else}
            <li
              class="bucket"
              data-testid="series-attempt-bucket"
              data-start={point.start}
              data-succeeded={point.succeeded}
              data-failed={point.failed}
              data-queued={point.queued}
              aria-label={`${label}: ${point.succeeded} succeeded, ${point.failed} failed, ${point.queued} queued`}
              title={`${label} UTC: ${point.succeeded} succeeded, ${point.failed} failed, ${point.queued} queued`}
            >
              <span class="stack">
                <span class="seg queued" style:height={share(point.queued, attemptPeak)}></span>
                <span class="seg failed" style:height={share(point.failed, attemptPeak)}></span>
                <span class="seg succeeded" style:height={share(point.succeeded, attemptPeak)}></span>
              </span>
              <span class="tick">{index % labelEvery === 0 ? label : ''}</span>
            </li>
          {/if}
        {/each}
      </ol>
    </div>
  {/each}
</div>

<style>
  .series {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .legend {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .legend span {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .swatch {
    display: inline-block;
    width: 10px;
    height: 10px;
    border-radius: 2px;
  }

  .row {
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr);
    align-items: end;
    gap: var(--space-2);
  }

  .row-title {
    align-self: center;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .bars {
    display: flex;
    align-items: stretch;
    gap: 2px;
    height: 96px;
    margin: 0;
    padding: 0 0 18px;
    list-style: none;
    border-bottom: 1px solid var(--color-border-subtle);
    position: relative;
  }

  .bucket {
    position: relative;
    display: flex;
    flex: 1 1 0;
    min-width: 2px;
    flex-direction: column;
    justify-content: flex-end;
  }

  .stack {
    display: flex;
    flex-direction: column;
    justify-content: flex-end;
    height: 100%;
    background: var(--color-bg-subtle, transparent);
  }

  .seg {
    display: block;
    width: 100%;
  }

  .tick {
    position: absolute;
    bottom: -18px;
    left: 0;
    font-family: var(--font-mono);
    font-size: 10px;
    white-space: nowrap;
    color: var(--color-text-tertiary);
  }

  .accepted,
  .succeeded {
    background: var(--color-success);
  }

  .rejected,
  .failed {
    background: var(--color-danger);
  }

  .queued {
    background: var(--color-info);
  }
</style>
