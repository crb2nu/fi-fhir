<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Badge, Button, Field, Input } from '$lib/ui/primitives';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
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

  function handleConditionChange(e: CustomEvent<string>) {
    dispatch('change', { ...filter, condition: e.detail });
  }
</script>

<div class="filter-editor">
  <div class="block">
    <div class="block-head">
      <span class="block-label">Event types</span>
      {#if filter.eventTypes.length > 0}
        <Badge mono>{filter.eventTypes.length} selected</Badge>
      {/if}
      <div class="preset-bar">
        {#each EVENT_TYPE_PRESETS as preset (preset.label)}
          <Button variant="ghost" onclick={() => applyPreset(preset.types)}>
            {preset.label}
          </Button>
        {/each}
        {#if filter.eventTypes.length > 0}
          <Button variant="ghost" onclick={clearEventTypes}>Clear</Button>
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
                <span class="checkbox-label">{type}</span>
              </label>
            {/each}
          </div>
        </div>
      {/each}
    </div>
  </div>

  <div class="sources">
    <Field label="Sources" hint="Comma-separated source system names.">
      <Input
        mono
        bind:value={sourcesText}
        onblur={handleSourcesBlur}
        placeholder="e.g. epic, cerner"
      />
    </Field>
  </div>

  <div class="block">
    <div class="block-head">
      <span class="block-label">CEL condition</span>
      <Button variant="ghost" onclick={() => (showCel = !showCel)} aria-expanded={showCel}>
        {showCel ? 'Hide' : 'Show'} expert mode
      </Button>
    </div>
    {#if showCel}
      <div class="cel-editor">
        <CodeEditor
          language="cel"
          value={filter.condition}
          on:change={handleConditionChange}
          placeholder="e.g. event.isCritical == true"
          height="60px"
          lineNumbers={false}
        />
      </div>
      <p class="hint">CEL expression evaluated against the event. Returns true to match.</p>
    {/if}
  </div>
</div>

<style>
  .filter-editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .block {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .block-head {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .block-label {
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .preset-bar {
    display: flex;
    flex-wrap: wrap;
    gap: 2px;
    margin-left: auto;
  }

  .checkbox-groups {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
  }

  .checkbox-group {
    display: grid;
    grid-template-columns: 136px minmax(0, 1fr);
    align-items: baseline;
    gap: var(--space-3);
  }

  .group-label {
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-muted);
  }

  .checkboxes {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1) var(--space-4);
  }

  .checkbox {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-height: 22px;
    cursor: pointer;
  }

  .checkbox input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  .checkbox-label {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-secondary);
  }

  .sources {
    max-width: 360px;
  }

  .cel-editor {
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .hint {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  @media (max-width: 640px) {
    .checkbox-group {
      grid-template-columns: 1fr;
      gap: var(--space-1);
    }
  }
</style>
