<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import { createEventDispatcher } from 'svelte';

  export let samples: readonly HL7Sample[];
  export let activeId: string | null;
  export let disabled = false;

  export let currentRaw: string;

  const dispatch = createEventDispatcher<{
    select: { id: string };
    remove: { id: string };
    saveCurrent: { name?: string };
    clear: Record<string, never>;
  }>();

  let name = '';

  function save() {
    const n = name.trim();
    dispatch('saveCurrent', n ? { name: n } : {});
    name = '';
  }
</script>

<Panel title="Samples (local)">
  <p class="note">
    Stored in <span class="mono">localStorage</span>. Don’t paste PHI unless you’re on an approved machine/profile.
  </p>

  <div class="save">
    <label class="label">
      Sample name (optional)
      <input class="input" type="text" bind:value={name} placeholder="ADT A01 - ICU admit" disabled={disabled} />
    </label>
    <div class="save-actions">
      <Button on:click={save} disabled={disabled || !currentRaw.trim()}>Save current</Button>
      <Button variant="secondary" on:click={() => dispatch('clear', {})} disabled={disabled || samples.length === 0}>
        Clear
      </Button>
    </div>
  </div>

  {#if samples.length === 0}
    <div class="empty">No saved samples yet.</div>
  {:else}
    <ul class="list">
      {#each samples as s (s.id)}
        <li class="li">
          <button
            type="button"
            class="item"
            class:active={activeId === s.id}
            on:click={() => dispatch('select', { id: s.id })}
            disabled={disabled}
          >
            <div class="top">
              <div class="title">{s.name}</div>
              <div class="meta">
                {#if s.messageType}<span class="pill mono">{s.messageType}</span>{/if}
                {#if s.version}<span class="pill mono">{s.version}</span>{/if}
              </div>
            </div>
            <div class="sub">
              <span class="mono">{s.source}</span>
              <span class="dot">•</span>
              <span class="mono">{new Date(s.createdAt).toLocaleString()}</span>
              {#if s.controlId}
                <span class="dot">•</span>
                <span class="mono">MSH-10={s.controlId}</span>
              {/if}
            </div>
          </button>
          <button
            type="button"
            class="trash"
            title="Remove"
            on:click={() => dispatch('remove', { id: s.id })}
            disabled={disabled}
          >
            Remove
          </button>
        </li>
      {/each}
    </ul>
  {/if}
</Panel>

<style>
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .note {
    margin: 0 0 12px;
    color: rgba(229, 231, 235, 0.78);
    line-height: 1.45;
  }

  .save {
    display: grid;
    gap: 10px;
    margin-bottom: 14px;
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .input {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .save-actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

  .empty {
    color: rgba(229, 231, 235, 0.7);
  }

  .list {
    padding: 0;
    margin: 0;
    display: grid;
    gap: 10px;
  }

  .li {
    list-style: none;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    align-items: start;
  }

  .item {
    width: 100%;
    text-align: left;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    padding: 12px;
    cursor: pointer;
    color: rgba(229, 231, 235, 0.9);
  }

  .item:hover:enabled {
    background: rgba(255, 255, 255, 0.04);
  }

  .item.active {
    border-color: rgba(59, 130, 246, 0.45);
    background: rgba(59, 130, 246, 0.12);
  }

  .top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
  }

  .title {
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .sub {
    margin-top: 8px;
    color: rgba(229, 231, 235, 0.72);
    font-size: 0.9rem;
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }

  .dot {
    color: rgba(229, 231, 235, 0.5);
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
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 650;
  }

  .trash {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.08);
    color: rgba(254, 226, 226, 0.9);
    cursor: pointer;
    font-weight: 700;
  }

  .trash:hover:enabled {
    background: rgba(239, 68, 68, 0.14);
  }
</style>
