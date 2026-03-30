<script lang="ts">
  import { onMount } from 'svelte';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import { initTheme } from '$lib/theme/theme';
  import { IDEShell } from '$lib/ui/ide';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  onMount(() => {
    initTheme();
  });
</script>

<ToastContainer />
<IDEShell>
  <slot />
</IDEShell>
