<!--
  One spec field: a Field (label, hint or error) around the control its
  schema entry names. Numbers are number inputs bounded by the checker's own
  limits, closed sets are Selects, lists are one entry per line, and a
  `*_binding` field picks one of the connection's declared bindings — or takes
  a typed name, which the checker reports if nothing declares it.
-->
<script lang="ts">
  import List from '@lucide/svelte/icons/list';
  import { Field, IconButton, Input, Select, Textarea, type SelectOption } from '$lib/ui/primitives';
  import { fieldText, type SpecField } from './specSchema';

  interface Props {
    field: SpecField;
    /** Full path of the value (a repeated item's field carries its index). */
    path: string;
    value: string | number | null | undefined;
    error?: string | undefined;
    bindingNames?: readonly string[];
    readOnly?: boolean;
  }

  let {
    field,
    path,
    value = $bindable(),
    error,
    bindingNames = [],
    readOnly = false
  }: Props = $props();

  const OTHER = '\u0000other';

  const text = $derived(fieldText(value));
  // A typed binding name that no binding declares stays a typed name.
  let typing = $state(false);
  const bindingInput = $derived(
    bindingNames.length === 0 || typing || (text !== '' && !bindingNames.includes(text))
  );

  const range = $derived(
    field.min !== undefined && field.max !== undefined ? `${field.min}–${field.max}` : undefined
  );

  const selectOptions = $derived.by((): SelectOption[] => {
    const options = (field.options ?? []).map((option) => ({ value: option, label: option }));
    return field.required ? options : [{ value: '', label: 'Not set' }, ...options];
  });

  const booleanOptions = $derived.by((): SelectOption[] => {
    const options = [
      { value: 'true', label: 'true' },
      { value: 'false', label: 'false' }
    ];
    return field.required ? options : [{ value: '', label: 'Not set' }, ...options];
  });

  // An optional binding (a CA bundle, a passphrase) can be set back to none.
  const bindingOptions = $derived([
    ...(field.required ? [] : [{ value: '', label: 'None' }]),
    ...bindingNames.map((name) => ({ value: name, label: name })),
    { value: OTHER, label: 'Other name…' }
  ]);

  function pickBinding(event: Event): void {
    const chosen = (event.currentTarget as HTMLSelectElement).value;
    if (chosen === OTHER) {
      typing = true;
      value = '';
      return;
    }
    value = chosen;
  }
</script>

<Field
  label={field.label}
  required={field.required ?? false}
  hint={field.hint}
  {error}
  class={['spec-field', { 'spec-field--wide': field.wide || field.control === 'list' }]}
  data-path={path}
  data-control={field.control}
>
  {#if readOnly && field.control === 'list'}
    <Textarea mono rows={3} value={text} readonly />
  {:else if readOnly}
    <!-- Read only: every control is a read-only text box at full contrast. -->
    <Input mono value={text} placeholder="—" readonly />
  {:else if field.control === 'number'}
    <Input
      type="number"
      mono
      step="1"
      min={field.min}
      max={field.max}
      placeholder={field.placeholder ?? range}
      bind:value
    />
  {:else if field.control === 'select'}
    <Select mono bind:value options={selectOptions} placeholder={field.required ? 'Choose' : undefined} />
  {:else if field.control === 'boolean'}
    <Select mono bind:value options={booleanOptions} placeholder={field.required ? 'Choose' : undefined} />
  {:else if field.control === 'list'}
    <Textarea mono rows={3} placeholder={field.placeholder} bind:value />
  {:else if field.control === 'binding'}
    <div class="binding-control">
      {#if bindingInput}
        <Input mono placeholder="Binding name" bind:value />
        {#if bindingNames.length > 0}
          <IconButton
            icon={List}
            label="Choose a declared binding"
            onclick={() => {
              typing = false;
              value = '';
            }}
          />
        {/if}
      {:else}
        <Select
          mono
          value={text}
          options={bindingOptions}
          placeholder={field.required ? 'Choose a binding' : undefined}
          onchange={pickBinding}
        />
      {/if}
    </div>
  {:else}
    <Input mono placeholder={field.placeholder} bind:value />
  {/if}
</Field>

<style>
  .binding-control {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
  }

  :global(.spec-field--wide) {
    grid-column: 1 / -1;
  }
</style>
