<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Field, Input, Select } from '$lib/ui/primitives';
  import { ACTION_FIELDS, ACTION_TYPES, type ActionDraft } from '../workflowTypes';

  export let action: ActionDraft;
  /** Field messages keyed by config key (or `type`), shown under the field. */
  export let errors: Record<string, string> = {};

  const dispatch = createEventDispatcher<{
    change: ActionDraft;
  }>();

  function handleTypeChange(e: Event) {
    const type = (e.target as HTMLSelectElement).value;
    // A new type starts clean: its settings, nested YAML values included, no
    // longer apply.
    dispatch('change', { _key: action._key, type, config: {} });
  }

  function handleFieldChange(key: string, value: string) {
    dispatch('change', {
      ...action,
      config: { ...action.config, [key]: value }
    });
  }

  $: fields = ACTION_FIELDS[action.type] ?? [];
  $: nestedKeys = Object.keys(action.yamlOnly ?? {});
</script>

<div class="editor">
  {#if nestedKeys.length > 0}
    <p class="yaml-only-note" data-testid="action-yaml-only">
      Nested values kept from YAML: <code>{nestedKeys.join(', ')}</code>. The builder cannot edit them and the
      engine reads action settings as flat values, so they have no effect at runtime.
    </p>
  {/if}
  <div class="editor-grid">
    <Field label="Action type" error={errors['type']}>
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
        <Field label={field.label} required={field.required ?? false} error={errors[field.key]}>
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

  .yaml-only-note {
    margin: 0;
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
  }

  @media (max-width: 640px) {
    .editor-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
