<script lang="ts">
  import { Dialog } from '$lib/ui/primitives';
  import Explorer from './sidebar/Explorer.svelte';

  export let open = false;
  export let drawer = false;
  export let width = 280;
  export let onclose: () => void;
  export let onnavigate: (path: string) => void;
</script>

{#if drawer}
  <Dialog {open} title="Explorer" placement="left" layout="bare" initialFocus="input" {onclose}>
    <Explorer {onclose} {onnavigate} {drawer} />
  </Dialog>
{:else if open}
  <aside class="sidebar" style="--sidebar-w: {Math.min(400, Math.max(240, width))}px" aria-label="Explorer">
    <Explorer {onclose} {onnavigate} />
  </aside>
{/if}

<style>
  .sidebar {
    flex: 0 0 var(--sidebar-w, 280px);
    width: var(--sidebar-w, 280px);
    min-height: 0;
    background: var(--ide-sidebar-bg, var(--color-bg-elevated));
    border-right: 1px solid var(--color-border-subtle);
    overflow: hidden;
  }
</style>
