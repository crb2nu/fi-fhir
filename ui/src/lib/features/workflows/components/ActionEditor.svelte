<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Field, Input, Select } from '$lib/ui/primitives';
  import { ACTION_FIELDS, ACTION_TYPES, type ActionDraft } from '../workflowTypes';

  export let action: ActionDraft;

  const dispatch = createEventDispatcher<{
    change: ActionDraft;
  }>();

  function handleTypeChange(e: Event) {
    const type = (e.target as HTMLSelectElement).value;
    dispatch('change', { ...action, type, config: {} });
  }

  function handleFieldChange(key: string, value: string) {
    dispatch('change', {
      ...action,
      config: { ...action.config, [key]: value }
    });
  }

  $: fields = ACTION_FIELDS[action.type] ?? [];
</script>

<div class="editor">
  <div class="editor-grid">
    <Field label="Action type">
      <Select value={action.type} onchange={handleTypeChange}>
        {#each ACTION_TYPES as type (type)}
          <option value={type}>{type}</option>
        {/each}
      </Select>
    </Field>
  </div>

  {#if fields.length > 0}
    <div class="editor-grid config-fields">
      {#each fields as field (field.key)}
        <Field label={field.label} required={field.required ?? false}>
          <Input
            mono
            value={action.config[field.key] ?? ''}
            placeholder={field.placeholder ?? ''}
            oninput={(e) => handleFieldChange(field.key, e.currentTarget.value)}
          />
        </Field>
      {/each}
    </div>
  {/if}
</div>

<style>
  .editor {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .editor-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  @media (max-width: 640px) {
    .editor-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
