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
