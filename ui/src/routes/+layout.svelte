<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import { initTheme } from '$lib/theme/theme';
  import { IDEShell } from '$lib/ui/ide';
  import { connectionState, start, stop } from '$lib/stores/connectionStore';
  import GraphQLCredentialGate from '$lib/graphql/GraphQLCredentialGate.svelte';
  import { purgeLegacyHL7BrowserStorage } from '$lib/features/hl7/samples/legacyStorage';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  let credentialReady = false;
  let mounted = false;
  let connectionStarted = false;

  onMount(() => {
    purgeLegacyHL7BrowserStorage();
    initTheme();
    mounted = true;
  });

  $: if (mounted && credentialReady && !connectionStarted) {
    start();
    connectionStarted = true;
  }

  $: if (mounted && !credentialReady && connectionStarted) {
    stop();
    connectionStarted = false;
  }

  onDestroy(() => {
    stop();
  });
</script>

<ToastContainer />
<div class="credential-layout">
  <GraphQLCredentialGate bind:authenticated={credentialReady} />
  {#if credentialReady}
    <div class="ide-frame">
      <IDEShell connectionState={$connectionState}>
        <slot />
      </IDEShell>
    </div>
  {/if}
</div>

<style>
  .credential-layout {
    height: 100vh;
    display: flex;
    flex-direction: column;
    overflow: hidden;
  }

  .ide-frame {
    min-height: 0;
    flex: 1 1 auto;
  }

  .ide-frame :global(.ide-shell) {
    height: 100%;
  }
</style>
