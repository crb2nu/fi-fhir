<script lang="ts">
  import { onMount, onDestroy } from 'svelte';
  import { page } from '$app/stores';
  import ToastContainer from '$lib/ui/ToastContainer.svelte';
  import { initTheme } from '$lib/theme/theme';
  import { IDEShell, getWorkspaceTabTitle } from '$lib/ui/ide';
  import { connectionState, start, stop } from '$lib/stores/connectionStore';
  import GraphQLCredentialGate, {
    type AccessSession
  } from '$lib/graphql/GraphQLCredentialGate.svelte';
  import { purgeLegacyHL7BrowserStorage } from '$lib/features/hl7/samples/legacyStorage';

  // Import global design tokens and base styles
  import '$lib/styles/tokens.css';
  import '$lib/styles/base.css';

  /**
   * Document titles for the routes whose page sets none (the others own a
   * `<svelte:head>`); same words as the editor tab. Drop a route from this
   * list when its page gains its own title.
   */
  const LAYOUT_TITLED_ROUTES = ['/hl7', '/profiles', '/workflows', '/operator'];

  function layoutTitle(pathname: string): string | null {
    const route = LAYOUT_TITLED_ROUTES.find((entry) => pathname === entry || pathname.startsWith(`${entry}/`));
    return route ? `${getWorkspaceTabTitle(route)} | fi-fhir` : null;
  }

  $: headTitle = layoutTitle($page.url.pathname);

  let credentialReady = false;
  let access: AccessSession | null = null;
  let gate: GraphQLCredentialGate | undefined;

  function clearAccess(): void {
    void gate?.clearCredential();
  }

  // Dev-only primitives gallery renders bare (no credential gate, no shell) so
  // it can be screenshotted; in production builds this is constant-false.
  $: bareDesignRoute = import.meta.env.DEV && $page.url.pathname === '/design';
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

<svelte:head>
  {#if headTitle}
    <title>{headTitle}</title>
  {/if}
</svelte:head>

<ToastContainer />
{#if bareDesignRoute}
  <slot />
{:else}
  <div class="credential-layout">
    <GraphQLCredentialGate bind:this={gate} bind:authenticated={credentialReady} bind:access />
    {#if credentialReady}
      <div class="ide-frame">
        <IDEShell connectionState={$connectionState} {access} onClearAccess={clearAccess}>
          <slot />
        </IDEShell>
      </div>
    {/if}
  </div>
{/if}

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
