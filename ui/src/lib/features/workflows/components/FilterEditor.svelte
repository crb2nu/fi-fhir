<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import {
    EVENT_TYPE_CATEGORIES,
    EVENT_TYPE_PRESETS,
    type FilterDraft
  } from '../workflowTypes';

  export let filter: FilterDraft;

  const dispatch = createEventDispatcher<{
    change: FilterDraft;
  }>();

  let showCel = !!filter.condition;
  let sourcesText = filter.sources.join(', ');

  function toggleEventType(type: string) {
    const types = filter.eventTypes.includes(type)
      ? filter.eventTypes.filter((t) => t !== type)
      : [...filter.eventTypes, type];
    dispatch('change', { ...filter, eventTypes: types });
  }

  function applyPreset(types: string[]) {
    dispatch('change', { ...filter, eventTypes: [...types] });
  }

  function clearEventTypes() {
    dispatch('change', { ...filter, eventTypes: [] });
  }

  function handleSourcesBlur() {
    const sources = sourcesText
      .split(',')
      .map((s) => s.trim())
      .filter(Boolean);
    dispatch('change', { ...filter, sources });
  }

  function handleConditionInput(e: Event) {
    const target = e.target as HTMLInputElement;
    dispatch('change', { ...filter, condition: target.value });
  }
</script>

<div class="filter-editor">
  <div class="section">
    <div class="section-header">
      <span class="section-label">Event Types</span>
      <div class="preset-bar">
        {#each EVENT_TYPE_PRESETS as preset (preset.label)}
          <Button variant="secondary" size="sm" on:click={() => applyPreset(preset.types)}>
            {preset.label}
          </Button>
        {/each}
        {#if filter.eventTypes.length > 0}
          <Button variant="danger" size="sm" on:click={clearEventTypes}>Clear</Button>
        {/if}
      </div>
    </div>

    <div class="checkbox-groups">
      {#each Object.entries(EVENT_TYPE_CATEGORIES) as [category, types] (category)}
        <div class="checkbox-group">
          <span class="group-label">{category}</span>
          <div class="checkboxes">
            {#each types as type (type)}
              <label class="checkbox">
                <input
                  type="checkbox"
                  checked={filter.eventTypes.includes(type)}
                  on:change={() => toggleEventType(type)}
                />
                <span class="checkbox-label">{type.replace(/_/g, ' ')}</span>
              </label>
            {/each}
          </div>
        </div>
      {/each}
    </div>

    {#if filter.eventTypes.length > 0}
      <div class="selected-count">
        {filter.eventTypes.length} event type{filter.eventTypes.length > 1 ? 's' : ''} selected
      </div>
    {/if}
  </div>

  <div class="section">
    <label class="section-label">
      Sources (comma-separated)
      <input
        type="text"
        class="input"
        bind:value={sourcesText}
        on:blur={handleSourcesBlur}
        placeholder="e.g. epic, cerner"
      />
    </label>
  </div>

  <div class="section">
    <div class="section-header">
      <span class="section-label">CEL Condition</span>
      <Button variant="ghost" size="sm" on:click={() => (showCel = !showCel)}>
        {showCel ? 'Hide' : 'Show'} Expert Mode
      </Button>
    </div>
    {#if showCel}
      <input
        type="text"
        class="input mono"
        value={filter.condition}
        on:input={handleConditionInput}
        placeholder='e.g. event.isCritical == true'
      />
      <div class="hint">
        CEL expression evaluated against the event. Returns true to match.
      </div>
    {/if}
  </div>
</div>

<style>
  .filter-editor {
    display: grid;
    gap: 16px;
  }

  .section {
    display: grid;
    gap: 8px;
  }

  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    flex-wrap: wrap;
    gap: 8px;
  }

  .section-label {
    color: var(--color-text-tertiary);
    font-size: 0.9rem;
    font-weight: 700;
    display: grid;
    gap: 6px;
  }

  .preset-bar {
    display: flex;
    gap: 4px;
    flex-wrap: wrap;
  }

  .checkbox-groups {
    display: grid;
    gap: 10px;
  }

  .checkbox-group {
    display: grid;
    gap: 4px;
  }

  .group-label {
    color: var(--color-text-muted);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .checkboxes {
    display: flex;
    flex-wrap: wrap;
    gap: 4px 12px;
  }

  .checkbox {
    display: flex;
    align-items: center;
    gap: 6px;
    cursor: pointer;
  }

  .checkbox input {
    accent-color: rgba(59, 130, 246, 0.85);
  }

  .checkbox-label {
    color: var(--color-text-secondary);
    font-size: 0.85rem;
  }

  .selected-count {
    color: rgba(147, 197, 253, 0.8);
    font-size: 0.8rem;
    font-weight: 600;
  }

  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    outline: none;
    width: 100%;
    box-sizing: border-box;
    transition: var(--transition-all);
  }

  .input::placeholder {
    color: var(--color-text-muted);
  }

  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .hint {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  @media (max-width: 640px) {
    .section-header {
      flex-direction: column;
      align-items: flex-start;
    }

    .checkboxes {
      gap: 6px 16px;
    }
  }
</style>
