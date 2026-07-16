<script lang="ts">
  import type { HL7Message } from '$lib/domain/hl7v2';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import { getHL7Value } from '$lib/domain/hl7Access';
  import type { ParsePreviewQuery } from '$lib/gen/graphql';
  import { createEventDispatcher } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import type { IntegrationSessionLineage } from '$lib/features/integration-session';

  export let events: ParsePreviewQuery['parsePreview']['events'];
  export let message: HL7Message;
  export let lineage: IntegrationSessionLineage[] = [];

  const dispatch = createEventDispatcher<{ inspectPath: { path: string } }>();

  function hl7(path: string): string | null {
    return getHL7Value(message, parseHL7Path(path));
  }

  function compact(s: string | null, max = 56): string {
    const v = (s ?? '').trim();
    if (!v) return '∅';
    if (v.length <= max) return v;
    return v.slice(0, max - 1) + '…';
  }

  $: obxSegments = message.segments.filter((s) => s.id === 'OBX');
  $: obrSegments = message.segments.filter((s) => s.id === 'OBR');
  let obxOccurrence = 0;
  let obrOccurrence = 0;

  $: if (obxSegments.length && obxOccurrence >= obxSegments.length) obxOccurrence = 0;
  $: if (obrSegments.length && obrOccurrence >= obrSegments.length) obrOccurrence = 0;

  function obxPath(field: number, component?: number): string {
    return component
      ? `OBX[${obxOccurrence}]-${field}.${component}`
      : `OBX[${obxOccurrence}]-${field}`;
  }

  function obrPath(field: number, component?: number): string {
    return component
      ? `OBR[${obrOccurrence}]-${field}.${component}`
      : `OBR[${obrOccurrence}]-${field}`;
  }

  function obxLabel(idx: number): string {
    const key = `OBX[${idx}]-3`;
    const id = compact(hl7(key), 44);
    const value = compact(hl7(`OBX[${idx}]-5`), 24);
    return `#${idx} ${id} → ${value}`;
  }

  function obrLabel(idx: number): string {
    const key = `OBR[${idx}]-4`;
    return `#${idx} ${compact(hl7(key), 56)}`;
  }
</script>

{#if events.length === 0}
  <div class="empty">No semantic events extracted.</div>
{:else}
  <div class="stack">
    {#if lineage.length > 0}
      <section class="server-lineage" aria-label="Server lineage">
        <div class="server-lineage-head">
          <div>
            <div class="section-title">Server lineage</div>
            <div class="note">Select a mapping to inspect the exact source field used by this run.</div>
          </div>
          <Badge variant="info">{lineage.length} links</Badge>
        </div>
        <div class="grid">
          {#each lineage as link (`${link.sourcePath}:${link.targetPath ?? ''}`)}
            <button
              class="row server-row"
              type="button"
              on:click={() => dispatch('inspectPath', { path: link.sourcePath })}
            >
              <span class="k mono">{link.sourcePath}</span>
              <span class="v">{link.targetPath ?? 'canonical event'}</span>
              <span class="hint">{link.description ?? 'inspect'}</span>
            </button>
          {/each}
        </div>
      </section>
    {/if}
    {#each events as ev (ev.id)}
      <div class="card">
        <div class="head">
          <div class="title">{ev.__typename}</div>
          <div class="meta">
            <Badge mono>{String(ev.type)}</Badge>
            <Badge mono>{new Date(String(ev.timestamp)).toLocaleString()}</Badge>
          </div>
        </div>

        <div class="body">
          {#if ev.__typename === 'PatientAdmitEvent' || ev.__typename === 'PatientDischargeEvent'}
            <div class="section">
              <div class="section-title">Common HL7 pointers (ADT)</div>
              <div class="grid">
                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-3[0].1' })}>
                  <span class="k mono">PID-3[0].1</span>
                  <span class="v">{hl7('PID-3[0].1') ?? '∅'}</span>
                  <span class="hint">MRN (first repetition)</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-5.1' })}>
                  <span class="k mono">PID-5.1</span>
                  <span class="v">{hl7('PID-5.1') ?? '∅'}</span>
                  <span class="hint">family name</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-5.2' })}>
                  <span class="k mono">PID-5.2</span>
                  <span class="v">{hl7('PID-5.2') ?? '∅'}</span>
                  <span class="hint">given name</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-7' })}>
                  <span class="k mono">PID-7</span>
                  <span class="v">{hl7('PID-7') ?? '∅'}</span>
                  <span class="hint">DOB</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-8' })}>
                  <span class="k mono">PID-8</span>
                  <span class="v">{hl7('PID-8') ?? '∅'}</span>
                  <span class="hint">sex</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PV1-2' })}>
                  <span class="k mono">PV1-2</span>
                  <span class="v">{hl7('PV1-2') ?? '∅'}</span>
                  <span class="hint">patient class</span>
                </button>

                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PV1-3' })}>
                  <span class="k mono">PV1-3</span>
                  <span class="v">{hl7('PV1-3') ?? '∅'}</span>
                  <span class="hint">assigned location</span>
                </button>
              </div>
            </div>
          {:else if ev.__typename === 'LabResultEvent'}
            <div class="section">
              <div class="section-title">Common HL7 pointers (ORU)</div>
              {#if obxSegments.length > 0}
                <div class="pickers">
                  <label class="picker">
                    OBR
                    <select class="select" bind:value={obrOccurrence}>
                      {#if obrSegments.length === 0}
                        <option value={0}>no OBR</option>
                      {:else}
                        {#each obrSegments as s (s.index)}
                          <option value={s.occurrence}>{obrLabel(s.occurrence)}</option>
                        {/each}
                      {/if}
                    </select>
                  </label>

                  <label class="picker">
                    OBX
                    <select class="select" bind:value={obxOccurrence}>
                      {#each obxSegments as s (s.index)}
                        <option value={s.occurrence}>{obxLabel(s.occurrence)}</option>
                      {/each}
                    </select>
                  </label>
                </div>
              {:else}
                <div class="note">No <span class="mono">OBX</span> segments found in this message.</div>
              {/if}

              <div class="grid">
                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-3[0].1' })}>
                  <span class="k mono">PID-3[0].1</span>
                  <span class="v">{hl7('PID-3[0].1') ?? '∅'}</span>
                  <span class="hint">MRN</span>
                </button>

                {#if obrSegments.length > 0}
                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obrPath(4) })}>
                    <span class="k mono">{obrPath(4)}</span>
                    <span class="v">{compact(hl7(obrPath(4)))}</span>
                    <span class="hint">order / test</span>
                  </button>
                {/if}

                {#if obxSegments.length > 0}
                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: `OBX[${obxOccurrence}]` })}>
                    <span class="k mono">OBX[{obxOccurrence}]</span>
                    <span class="v">{compact(hl7(`OBX[${obxOccurrence}]`), 72)}</span>
                    <span class="hint">segment</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(2) })}>
                    <span class="k mono">{obxPath(2)}</span>
                    <span class="v">{compact(hl7(obxPath(2)))}</span>
                    <span class="hint">value type</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(3) })}>
                    <span class="k mono">{obxPath(3)}</span>
                    <span class="v">{compact(hl7(obxPath(3)))}</span>
                    <span class="hint">obs id</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(5) })}>
                    <span class="k mono">{obxPath(5)}</span>
                    <span class="v">{compact(hl7(obxPath(5)))}</span>
                    <span class="hint">value</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(6) })}>
                    <span class="k mono">{obxPath(6)}</span>
                    <span class="v">{compact(hl7(obxPath(6)))}</span>
                    <span class="hint">units</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(11) })}>
                    <span class="k mono">{obxPath(11)}</span>
                    <span class="v">{compact(hl7(obxPath(11)))}</span>
                    <span class="hint">status</span>
                  </button>

                  <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: obxPath(14) })}>
                    <span class="k mono">{obxPath(14)}</span>
                    <span class="v">{compact(hl7(obxPath(14)))}</span>
                    <span class="hint">obs time</span>
                  </button>
                {/if}
              </div>
            </div>
          {:else}
            <div class="note">No lineage view for {ev.__typename} yet.</div>
          {/if}
        </div>
      </div>
    {/each}
  </div>
{/if}

<style>
	  .empty {
	    color: var(--color-text-tertiary);
	  }

  .stack {
    display: grid;
    gap: 12px;
  }

  .server-lineage {
    display: grid;
    gap: 10px;
    padding: 12px;
    border: 1px solid var(--color-info-border);
    border-radius: 12px;
    background: var(--color-info-bg);
  }

  .server-lineage-head {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 12px;
  }

  .server-lineage .note {
    margin-bottom: 0;
  }

	  .card {
	    border-radius: 12px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
	    padding: 10px 12px;
	  }

  .head {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    margin-bottom: 8px;
  }

	  .title {
	    color: var(--color-text-primary);
	    font-weight: 850;
	  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

	  .mono {
	    font-family: var(--font-mono);
	  }

  .section {
    margin-top: 10px;
  }

	  .section-title {
	    color: var(--color-text-primary);
	    font-weight: 800;
	    margin-bottom: 10px;
	  }

  .grid {
    display: grid;
    gap: 8px;
  }

  .pickers {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 10px;
  }

	  .picker {
    display: grid;
    gap: 6px;
	    color: var(--color-text-secondary);
	    font-size: 0.9rem;
	    font-weight: 750;
	    min-width: 260px;
	  }

	  .select {
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	  }

	  .select:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
	  }

	  .row {
    width: 100%;
    text-align: left;
    border-radius: 10px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
	    padding: 10px;
	    cursor: pointer;
    display: grid;
    grid-template-columns: 120px 1fr auto;
    gap: 10px;
    align-items: baseline;
  }

  .row:hover {
	    background: var(--color-bg-hover);
  }

  .row:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  @media (max-width: 640px) {
    .server-lineage-head {
      flex-direction: column;
    }

    .row {
      grid-template-columns: 1fr;
    }

    .v {
      white-space: normal;
    }
  }

	  .k {
	    color: var(--color-text-primary);
	    font-weight: 850;
	  }

	  .v {
	    color: var(--color-text-secondary);
	    overflow: hidden;
	    text-overflow: ellipsis;
	    white-space: nowrap;
	  }

	  .hint {
	    color: var(--color-text-muted);
	    font-size: 0.85rem;
	    font-weight: 700;
	    white-space: nowrap;
	  }

	  .note {
	    color: var(--color-text-tertiary);
	    line-height: 1.45;
	    margin-bottom: 10px;
	  }
</style>
