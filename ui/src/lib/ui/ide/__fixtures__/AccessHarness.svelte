<!--
  Test harness: the credential gate wired to the status-bar access chip the
  way +layout.svelte and StatusBar wire them (bind the session out, clear
  through the gate).
-->
<script lang="ts">
  import GraphQLCredentialGate, {
    type AccessSession
  } from '$lib/graphql/GraphQLCredentialGate.svelte';
  import AccessChip from '../AccessChip.svelte';

  let authenticated = false;
  let access: AccessSession | null = null;
  let gate: GraphQLCredentialGate | undefined;
</script>

<GraphQLCredentialGate bind:this={gate} bind:authenticated bind:access />
{#if authenticated && access}
  <AccessChip {access} onclear={() => void gate?.clearCredential()} />
{/if}
