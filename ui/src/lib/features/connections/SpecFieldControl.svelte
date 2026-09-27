<!--
  One spec field: a Field (label, hint or error) around the control its
  schema entry names. Numbers are number inputs bounded by the checker's own
  limits, closed sets are Selects, lists are one entry per line, a fixed value
  is shown read only, and a `*_binding` field is a Select over the
  connection's declared bindings — never a text box, so there is nowhere to
  paste a secret value. A stored name that no binding declares stays visible
  as a read-only option, so the checker's UNBOUND_SECRET lands on this field.
-->
<script lang="ts">
  import { Field, Input, Select, Textarea, type SelectOption } from '$lib/ui/primitives';
  import { BINDING_DECLARE_FIRST, fieldText, type SpecField } from './specSchema';

  interface Props {
    field: SpecField;
    /** Full path of the value (a repeated item's field carries its index). */
    path: string;
    value: string | number | null | undefined;
    error?: string | undefined;
    bindingNames?: readonly string[] | undefined;
    readOnly?: boolean | undefined;
  }

  let {
    field,
    path,
    value = $bindable(),
    error,
    bindingNames = [],
    readOnly = false
  }: Props = $props();

  const text = $derived(fieldText(value));

  const range = $derived(
    field.min !== undefined && field.max !== undefined ? `${field.min}–${field.max}` : undefined
  );

  function labelled(option: string): SelectOption {
    return { value: option, label: field.optionLabels?.[option] ?? option };
  }

  const selectOptions = $derived.by((): SelectOption[] => {
    const options = (field.options ?? []).map(labelled);
    return field.required ? options : [{ value: '', label: 'Not set' }, ...options];
  });

  const booleanOptions = $derived.by((): SelectOption[] => {
    const options = [
      { value: 'true', label: 'true' },
      { value: 'false', label: 'false' }
    ];
    return field.required ? options : [{ value: '', label: 'Not set' }, ...options];
  });

  const noBindings = $derived(bindingNames.length === 0);
  const undeclared = $derived(text !== '' && !bindingNames.includes(text));

  const bindingOptions = $derived.by((): SelectOption[] => [
    // An optional binding (a CA bundle, a passphrase) can be set back to none.
    ...(field.required ? [] : [{ value: '', label: 'None' }]),
    // A stored name nothing declares: shown, not choosable.
    ...(undeclared ? [{ value: text, label: `${text} (not declared)`, disabled: true }] : []),
    ...bindingNames.map((name) => ({ value: name, label: name }))
  ]);

  const hint = $derived(field.control === 'binding' && noBindings && !readOnly ? BINDING_DECLARE_FIRST : field.hint);

  function pickBinding(event: Event): void {
    value = (event.currentTarget as HTMLSelectElement).value;
  }
</script>

<Field
  label={field.label}
  required={field.required ?? false}
  {hint}
  {error}
  class={['spec-field', { 'spec-field--wide': field.wide || field.control === 'list' }]}
  data-path={path}
  data-control={field.fixed ? 'fixed' : field.control}
>
  {#if field.fixed}
    <!-- The only value the document allows: shown, and always written. -->
    <Input mono value={field.defaultValue ?? ''} readonly />
  {:else if readOnly && field.control === 'list'}
    <Textarea mono rows={3} value={text} readonly />
  {:else if readOnly}
    <!-- Read only: every control is a read-only text box at full contrast. -->
    <Input mono value={field.control === 'select' && text ? labelled(text).label : text} placeholder="—" readonly />
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
    <Select
      mono
      value={text}
      options={bindingOptions}
      placeholder={field.required ? 'Choose a binding' : undefined}
      disabled={noBindings}
      title={noBindings ? BINDING_DECLARE_FIRST : undefined}
      onchange={pickBinding}
    />
  {:else}
    <Input mono placeholder={field.placeholder} bind:value />
  {/if}
</Field>

<style>
  :global(.spec-field--wide) {
    grid-column: 1 / -1;
  }
</style>
