<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Field, Input, Select } from '$lib/ui/primitives';
  import { TRANSFORM_FIELDS, TRANSFORM_TYPES, type TransformDraft, type TransformType } from '../workflowTypes';

  export let transform: TransformDraft;

  const dispatch = createEventDispatcher<{
    change: TransformDraft;
  }>();

  function handleTypeChange(e: Event) {
    const type = (e.target as HTMLSelectElement).value as TransformType;
    dispatch('change', { ...transform, type, config: {} });
  }

  function handleFieldChange(key: string, value: string) {
    dispatch('change', {
      ...transform,
      config: { ...transform.config, [key]: value }
    });
  }

  $: fields = TRANSFORM_FIELDS[transform.type] ?? [];
</script>

<div class="editor">
  <div class="editor-grid">
    <Field label="Transform type">
      <Select value={transform.type} onchange={handleTypeChange}>
        {#each TRANSFORM_TYPES as type (type)}
          <option value={type}>{type.replace(/_/g, ' ')}</option>
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
            value={transform.config[field.key] ?? ''}
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
