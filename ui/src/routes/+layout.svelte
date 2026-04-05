<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import { initTheme } from '$lib/theme/theme';
  import { IDEShell } from '$lib/ui/ide';
  import { connectionState, start, stop } from '$lib/stores/connectionStore';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  onMount(() => {
    initTheme();
    start();
  });

  onDestroy(() => {
    stop();
  });
</script>

<ToastContainer />
<IDEShell connectionState={$connectionState}>
  <slot />
</IDEShell>
