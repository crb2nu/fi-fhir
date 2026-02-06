<script lang="ts">
  import { createEventDispatcher } from 'svelte';
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

<div class="action-editor">
  <div class="action-type-row">
    <label class="field-label">
      Action Type
      <select class="select" value={action.type} on:change={handleTypeChange}>
        {#each ACTION_TYPES as type (type)}
          <option value={type}>{type}</option>
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
            value={action.config[field.key] ?? ''}
            placeholder={field.placeholder ?? ''}
            on:input={(e) => handleFieldChange(field.key, (e.target as HTMLInputElement).value)}
          />
        </label>
      {/each}
    </div>
  {/if}
</div>

<style>
  .action-editor {
    display: grid;
    gap: 10px;
  }

  .action-type-row {
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

  @media (max-width: 640px) {
    .config-fields {
      grid-template-columns: 1fr;
    }
  }
</style>
