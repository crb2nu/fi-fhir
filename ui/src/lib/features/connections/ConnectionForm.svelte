<!--
  Settings: the connection's labels and its kind's spec, generated from the
  schema (specSchema.ts) on a two-column grid of 28 px Fields. Problems the
  server reported land on the field their path names; the rest — the
  document, a whole group, a binding, a key the form does not know — are
  listed under the form, labelled.
-->
<script lang="ts">
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { Button, Field, IconButton, Input, Textarea } from '$lib/ui/primitives';
  import SpecFieldControl from './SpecFieldControl.svelte';
  import type { EditBuffer } from './editBuffer';
  import {
    emptyRepeatItem,
    fieldVisible,
    isBlocking,
    type KindSchema,
    type PlacedProblems,
    type SpecRepeat
  } from './specSchema';

  interface Props {
    schema: KindSchema;
    buffer: EditBuffer;
    placed: PlacedProblems;
    readOnly?: boolean;
    /** Client-side checks of the connection's own fields (the server re-checks them). */
    identityErrors: { id: string | null; name: string | null; description: string | null };
    /** One line about the last check of the spec, or null. */
    checkStatus?: { text: string; tone: 'neutral' | 'warning' | 'danger' | 'success' } | null;
  }

  let {
    schema,
    buffer = $bindable(),
    placed,
    readOnly = false,
    identityErrors,
    checkStatus = null
  }: Props = $props();

  const bindingNames = $derived(
    buffer.bindings.map((binding) => binding.name.trim()).filter((name) => name.length > 0)
  );
  const idEditable = $derived(buffer.mode === 'create' && !readOnly);

  function fieldError(path: string): string | undefined {
    const messages = placed.byField[path];
    return messages && messages.length > 0 ? messages.join('; ') : undefined;
  }

  function addItem(section: SpecRepeat): void {
    (buffer.repeats[section.path] ??= []).push(emptyRepeatItem(section));
  }

  function removeItem(section: SpecRepeat, index: number): void {
    buffer.repeats[section.path]?.splice(index, 1);
  }
</script>

<form class="connection-form" data-testid="connection-form" onsubmit={(event) => event.preventDefault()}>
  {#if checkStatus}
    <p class="check-status" data-tone={checkStatus.tone} role="status">{checkStatus.text}</p>
  {/if}

  <section class="form-group" data-group="connection">
    <h3 class="group-label">Connection</h3>
    <div class="grid">
      <Field
        label="ID"
        required
        hint={idEditable ? 'Letters, digits, ".", "_" and "-"; at most 128. Fixed once created.' : undefined}
        error={identityErrors.id}
        data-path="id"
      >
        <Input mono bind:value={buffer.id} readonly={!idEditable} placeholder="adt-east-mllp" />
      </Field>
      <Field label="Name" required error={identityErrors.name} data-path="name">
        <Input bind:value={buffer.name} readonly={readOnly} />
      </Field>
      <Field label="Description" error={identityErrors.description} class="spec-field--wide" data-path="description">
        <Textarea rows={2} bind:value={buffer.description} readonly={readOnly} />
      </Field>
    </div>
  </section>

  {#each schema.sections as section (section.id)}
    {#if section.type === 'group'}
      {@const fields = section.fields.filter((field) => fieldVisible(field, buffer.values))}
      {#if fields.length > 0}
        <section class="form-group" data-group={section.id}>
          <h3 class="group-label">{section.label}</h3>
          <div class="grid">
            {#each fields as field (field.path)}
              <SpecFieldControl
                {field}
                path={field.path}
                bind:value={buffer.values[field.path]}
                error={fieldError(field.path)}
                {bindingNames}
                {readOnly}
              />
            {/each}
          </div>
        </section>
      {/if}
    {:else}
      {@const items = buffer.repeats[section.path] ?? []}
      <section class="form-group" data-group={section.id}>
        <div class="group-head">
          <h3 class="group-label">{section.label}</h3>
          {#if !readOnly}
            <Button variant="ghost" icon={Plus} onclick={() => addItem(section)}>
              Add {section.itemLabel.toLowerCase()}
            </Button>
          {/if}
        </div>
        {#each items as item, index (index)}
          <div class="repeat-item" data-item={`${section.path}[${index}]`}>
            <div class="item-head">
              <span class="item-label">{section.itemLabel} {index + 1}</span>
              {#if !readOnly}
                <IconButton
                  icon={X}
                  label={`Remove ${section.itemLabel.toLowerCase()} ${index + 1}`}
                  onclick={() => removeItem(section, index)}
                />
              {/if}
            </div>
            <div class="grid">
              {#each section.fields as field (field.path)}
                <SpecFieldControl
                  {field}
                  path={`${section.path}[${index}].${field.path}`}
                  bind:value={item[field.path]}
                  error={fieldError(`${section.path}[${index}].${field.path}`)}
                  {bindingNames}
                  {readOnly}
                />
              {/each}
            </div>
          </div>
        {:else}
          <p class="none">None.</p>
        {/each}
      </section>
    {/if}
  {/each}

  {#if placed.unplaced.length > 0}
    <section class="form-group" aria-label="Other problems">
      <h3 class="group-label">Other problems</h3>
      <ul class="problems" data-testid="connection-problems">
        {#each placed.unplaced as problem, index (index)}
          <li class="problem" data-tone={isBlocking(problem) ? 'danger' : 'warning'} data-code={problem.code}>
            <span class="problem-label">{problem.label}</span>
            <span class="problem-message">{problem.message}</span>
          </li>
        {/each}
      </ul>
    </section>
  {/if}
</form>

<style>
  .connection-form {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-3);
  }

  .check-status {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .check-status[data-tone='warning'] {
    color: var(--color-warning-text);
  }

  .check-status[data-tone='danger'] {
    color: var(--color-danger-text);
  }

  .check-status[data-tone='success'] {
    color: var(--color-success-text);
  }

  .form-group {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .group-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .group-label {
    margin: 0;
    padding-bottom: var(--space-1);
    border-bottom: 1px solid var(--color-border-subtle);
    font-family: var(--font-ui);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .group-head .group-label {
    flex: 1 1 auto;
  }

  .grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-2) var(--space-3);
  }

  .repeat-item {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
  }

  .item-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .item-label {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .none {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .problems {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .problem {
    display: grid;
    grid-template-columns: minmax(120px, max-content) minmax(0, 1fr);
    gap: var(--space-3);
    padding: var(--space-1) var(--space-2);
    border-left: 2px solid var(--color-danger-border);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .problem[data-tone='warning'] {
    border-left-color: var(--color-warning-border);
  }

  .problem-label {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }

  .problem-message {
    color: var(--color-text-secondary);
  }
</style>
