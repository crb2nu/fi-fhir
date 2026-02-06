<script lang="ts">
  import { createEventDispatcher } from 'svelte';
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

<div class="transform-editor">
  <div class="transform-type-row">
    <label class="field-label">
      Transform Type
      <select class="select" value={transform.type} on:change={handleTypeChange}>
        {#each TRANSFORM_TYPES as type (type)}
          <option value={type}>{type.replace(/_/g, ' ')}</option>
        {/each}
      </select>
    </label>
  </div>

  {#if fields.length > 0}
    <div class="config-fields">
      {#each fields as field (field.key)}
        <label class="field-label">
          {field.label}
          {#if field.required}<span class="required">*</span>{/if}
          <input
            type="text"
            class="input"
            value={transform.config[field.key] ?? ''}
            placeholder={field.placeholder ?? ''}
            on:input={(e) => handleFieldChange(field.key, (e.target as HTMLInputElement).value)}
          />
        </label>
      {/each}
    </div>
  {/if}
</div>

<style>
  .transform-editor {
    display: grid;
    gap: 10px;
  }

  .transform-type-row {
    display: grid;
  }

  .config-fields {
    display: grid;
    gap: 8px;
    grid-template-columns: repeat(auto-fill, minmax(200px, 1fr));
  }

  .field-label {
    display: grid;
    gap: 4px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .required {
    color: rgba(239, 68, 68, 0.7);
  }

  .select,
  .input {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    width: 100%;
    box-sizing: border-box;
  }

  .select:focus,
  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }
</style>
