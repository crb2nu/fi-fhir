<!-- Test fixture: Field wrapping each control kind, with bound values. -->
<script lang="ts">
  import { Field, Input, Select, Textarea } from '../index';

  interface Props {
    kind: 'input' | 'select' | 'textarea';
    label?: string;
    hint?: string | undefined;
    error?: string | undefined;
    required?: boolean;
    onvalue?: (value: unknown) => void;
  }

  let { kind, label = 'Endpoint', hint, error, required = false, onvalue }: Props = $props();

  let text = $state('initial');
  let choice = $state('b');

  $effect(() => {
    onvalue?.(kind === 'select' ? choice : text);
  });
</script>

<Field {label} {hint} {error} {required} data-testid="field">
  {#if kind === 'input'}
    <Input bind:value={text} mono data-testid="control" />
  {:else if kind === 'select'}
    <Select
      bind:value={choice}
      data-testid="control"
      options={[
        { value: 'a', label: 'Alpha' },
        { value: 'b', label: 'Bravo' },
        { value: 'c', label: 'Charlie', disabled: true }
      ]}
    />
  {:else}
    <Textarea bind:value={text} rows={4} data-testid="control" />
  {/if}
</Field>
