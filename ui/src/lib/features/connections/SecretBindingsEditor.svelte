<!--
  Secrets: the connection's secret bindings. A binding names where a secret
  lives — a provider and a key, optionally a version — and every `*_binding`
  field in Settings names one of them. There is no value column, ever: the
  catalog, this page and the API never hold, send or show a secret value.
-->
<script lang="ts">
  import KeyRound from '@lucide/svelte/icons/key-round';
  import Plus from '@lucide/svelte/icons/plus';
  import X from '@lucide/svelte/icons/x';
  import { Button, EmptyState, IconButton, Input, Select, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import type { BindingDraft } from './editBuffer';
  import { SECRET_PROVIDERS, isBlocking, type SpecProblem } from './specSchema';

  interface Props {
    bindings: BindingDraft[];
    /** Problems by binding index (`secret_bindings[i].<field>`). */
    problems?: Record<number, SpecProblem[]>;
    readOnly?: boolean;
  }

  let { bindings = $bindable(), problems = {}, readOnly = false }: Props = $props();

  const providerOptions = SECRET_PROVIDERS.map((provider) => ({ value: provider, label: provider }));

  function invalid(index: number, field: string): boolean {
    return (problems[index] ?? []).some(
      (problem) => isBlocking(problem) && problem.path.endsWith(`].${field}`)
    );
  }

  function add(): void {
    bindings.push({ name: '', provider: 'env', key: '', version: '' });
  }

  function remove(index: number): void {
    bindings.splice(index, 1);
  }

  const listed = $derived(
    Object.entries(problems).flatMap(([index, list]) =>
      list.map((problem) => ({ index: Number(index), problem }))
    )
  );
</script>

<div class="secrets" data-testid="connection-secrets">
  <div class="secrets-bar">
    <p class="secrets-note">
      A binding names where a secret lives. Values are resolved by the engine at startup and are never stored or shown here.
    </p>
    {#if !readOnly}
      <Button icon={Plus} onclick={add}>Add binding</Button>
    {/if}
  </div>

  {#if bindings.length === 0}
    <EmptyState icon={KeyRound} align="start" message="No secret bindings." />
  {:else}
    <Table label="Secret bindings" layout="fixed" class="bindings-table">
      {#snippet head()}
        <tr>
          <Th width="30%">Name</Th>
          <Th width="96px">Provider</Th>
          <Th>Key</Th>
          <Th width="72px">Version</Th>
          {#if !readOnly}<Th width="36px"><span class="sr-only">Remove</span></Th>{/if}
        </tr>
      {/snippet}
      {#each bindings as binding, index (index)}
        <Tr data-binding={index}>
          <Td>
            <Input
              mono
              aria-label={`Binding ${index + 1} name`}
              bind:value={binding.name}
              invalid={invalid(index, 'name')}
              readonly={readOnly}
            />
          </Td>
          <Td>
            {#if readOnly}
              <Input mono aria-label={`Binding ${index + 1} provider`} value={binding.provider} readonly />
            {:else}
              <Select
                mono
                aria-label={`Binding ${index + 1} provider`}
                bind:value={binding.provider}
                options={providerOptions}
                invalid={invalid(index, 'provider')}
              />
            {/if}
          </Td>
          <Td>
            <Input
              mono
              aria-label={`Binding ${index + 1} key`}
              bind:value={binding.key}
              invalid={invalid(index, 'key')}
              readonly={readOnly}
            />
          </Td>
          <Td>
            <Input
              mono
              aria-label={`Binding ${index + 1} version`}
              placeholder="—"
              bind:value={binding.version}
              invalid={invalid(index, 'version')}
              readonly={readOnly}
            />
          </Td>
          {#if !readOnly}
            <Td>
              <IconButton icon={X} label={`Remove binding ${index + 1}`} onclick={() => remove(index)} />
            </Td>
          {/if}
        </Tr>
      {/each}
    </Table>
  {/if}

  {#if listed.length > 0}
    <ul class="binding-problems" aria-label="Binding problems">
      {#each listed as entry, position (position)}
        <li data-tone={isBlocking(entry.problem) ? 'danger' : 'warning'}>
          <span class="which">{bindings[entry.index]?.name || `Binding ${entry.index + 1}`}</span>
          {entry.problem.message}
        </li>
      {/each}
    </ul>
  {/if}
</div>

<style>
  .secrets {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  .secrets-bar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .secrets-note {
    margin: 0;
    max-width: 60ch;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .secrets :global(.bindings-table td) {
    padding-left: var(--space-1);
    padding-right: var(--space-1);
  }

  .binding-problems {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .binding-problems li {
    padding: var(--space-1) var(--space-2);
    border-left: 2px solid var(--color-danger-border);
  }

  .binding-problems li[data-tone='warning'] {
    border-left-color: var(--color-warning-border);
  }

  .which {
    margin-right: var(--space-2);
    font-family: var(--font-mono);
    color: var(--color-text-primary);
  }
</style>
