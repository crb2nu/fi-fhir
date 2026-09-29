<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { Field, Input, Select } from '$lib/ui/primitives';
  import { TRANSFORM_FIELDS, TRANSFORM_TYPES, type TransformDraft, type TransformType } from '../workflowTypes';

  export let transform: TransformDraft;
  /** Field messages keyed by config key, shown under the field. */
  export let errors: Record<string, string> = {};

  const dispatch = createEventDispatcher<{
    change: TransformDraft;
  }>();

  function handleTypeChange(e: Event) {
    const type = (e.target as HTMLSelectElement).value as TransformType;
    // Choosing a type replaces the transform, including one kept from YAML.
    dispatch('change', { _key: transform._key, type, config: {} });
  }

  function handleFieldChange(key: string, value: string) {
    dispatch('change', {
      ...transform,
      config: { ...transform.config, [key]: value }
    });
  }

  $: fields = transform.raw ? [] : (TRANSFORM_FIELDS[transform.type] ?? []);
  $: rawKeys = transform.raw ? Object.keys(transform.raw) : [];
</script>

<div class="editor">
  <div class="editor-grid">
    <Field label="Transform type">
      <Select value={transform.raw ? '' : transform.type} onchange={handleTypeChange}>
        {#if transform.raw}
          <option value="" disabled>Kept from YAML</option>
        {/if}
        {#each TRANSFORM_TYPES as type (type)}
          <option value={type}>{type.replace(/_/g, ' ')}</option>
        {/each}
      </Select>
    </Field>
  </div>

  {#if transform.raw}
    <p class="yaml-only-note" data-testid="transform-yaml-only">
      The builder cannot edit this transform (<code>{rawKeys.join(', ')}</code>). It is saved exactly as
      written in YAML; choose a type to replace it.
    </p>
  {/if}

  {#if fields.length > 0}
    <div class="editor-grid config-fields">
      {#each fields as field (field.key)}
        <Field label={field.label} required={field.required ?? false} error={errors[field.key]}>
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
