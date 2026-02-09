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
            aria-required={field.required || undefined}
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
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 600;
  }

  .required {
    color: var(--color-danger);
  }

  .select,
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

  .select:hover:not(:disabled):not(:focus),
  .input:hover:not(:disabled):not(:focus) {
    border-color: var(--color-border-strong);
  }

  .select::placeholder,
  .input::placeholder {
    color: var(--color-text-muted);
  }

  .select:focus,
  .input:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  @media (max-width: 640px) {
    .config-fields {
      grid-template-columns: 1fr;
    }
  }
</style>
