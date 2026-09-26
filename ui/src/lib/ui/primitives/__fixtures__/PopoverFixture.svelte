<!-- Test fixture: Popover with an IconButton trigger and an outside target. -->
<script lang="ts">
  import CircleHelp from '@lucide/svelte/icons/circle-help';
  import { IconButton, Popover } from '../index';

  let { open = $bindable(false) }: { open?: boolean } = $props();
</script>

<button type="button" data-testid="outside">Outside</button>

<Popover bind:open label="About this view" data-testid="help-popover">
  {#snippet trigger(props)}
    <IconButton {...props} icon={CircleHelp} label="About this view" />
  {/snippet}
  <p>Events refresh every 10 seconds.</p>
  <button type="button">Inside action</button>
</Popover>
