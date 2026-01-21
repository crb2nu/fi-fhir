<script lang="ts">
  import type { HL7Message } from '$lib/domain/hl7v2';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import { getHL7Value } from '$lib/domain/hl7Access';
  import type { ParsePreviewQuery } from '$lib/gen/graphql';
  import { createEventDispatcher } from 'svelte';

  export let events: ParsePreviewQuery['parsePreview']['events'];
  export let message: HL7Message;

  const dispatch = createEventDispatcher<{ inspectPath: { path: string } }>();

  function hl7(path: string): string | null {
    return getHL7Value(message, parseHL7Path(path));
  }
</script>

{#if events.length === 0}
  <div class="empty">No semantic events extracted.</div>
{:else}
  <div class="stack">
    {#each events as ev (ev.id)}
      <div class="card">
        <div class="head">
          <div class="title">{ev.__typename}</div>
          <div class="meta">
            <span class="pill mono">{String(ev.type)}</span>
            <span class="pill mono">{new Date(String(ev.timestamp)).toLocaleString()}</span>
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
              <div class="note">
                ORU messages usually contain multiple <span class="mono">OBX</span> segments; this view currently shows
                only general pointers.
              </div>
              <div class="grid">
                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'PID-3[0].1' })}>
                  <span class="k mono">PID-3[0].1</span>
                  <span class="v">{hl7('PID-3[0].1') ?? '∅'}</span>
                  <span class="hint">MRN</span>
                </button>
                <button class="row" type="button" on:click={() => dispatch('inspectPath', { path: 'OBR-4' })}>
                  <span class="k mono">OBR-4</span>
                  <span class="v">{hl7('OBR-4') ?? '∅'}</span>
                  <span class="hint">order / test code</span>
                </button>
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
    color: rgba(229, 231, 235, 0.7);
  }

  .stack {
    display: grid;
    gap: 12px;
  }

  .card {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
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
    color: rgba(243, 244, 246, 0.95);
    font-weight: 850;
  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .pill {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.88);
    font-size: 0.85rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .section {
    margin-top: 10px;
  }

  .section-title {
    color: rgba(243, 244, 246, 0.9);
    font-weight: 800;
    margin-bottom: 10px;
  }

  .grid {
    display: grid;
    gap: 8px;
  }

  .row {
    width: 100%;
    text-align: left;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
    padding: 10px;
    cursor: pointer;
    display: grid;
    grid-template-columns: 120px 1fr auto;
    gap: 10px;
    align-items: baseline;
  }

  .row:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  .k {
    color: rgba(229, 231, 235, 0.92);
    font-weight: 850;
  }

  .v {
    color: rgba(229, 231, 235, 0.85);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .hint {
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.85rem;
    font-weight: 700;
    white-space: nowrap;
  }

  .note {
    color: rgba(229, 231, 235, 0.72);
    line-height: 1.45;
    margin-bottom: 10px;
  }
</style>
