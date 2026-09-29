<!-- Test fixture: a trigger that opens a Dialog with two fields and a footer. -->
<script lang="ts">
  import { Button, Dialog } from '../index';

  let {
    open = $bindable(false),
    dismissible = true,
    layout = 'default',
    onclose
  }: {
    open?: boolean;
    dismissible?: boolean;
    layout?: 'default' | 'bare';
    onclose?: (() => void) | undefined;
  } = $props();
</script>

<button type="button" data-testid="opener" onclick={() => (open = true)}>Open</button>

<Dialog
  bind:open
  title="Rename definition"
  description="The new name is recorded in the audit trail."
  {dismissible}
  {layout}
  {onclose}
  initialFocus="[data-testid='second']"
  data-testid="fixture-dialog"
>
  <input aria-label="First" data-testid="first" />
  <input aria-label="Second" data-testid="second" />
  {#snippet footer()}
    <Button size="md" onclick={() => (open = false)}>Done</Button>
  {/snippet}
</Dialog>
