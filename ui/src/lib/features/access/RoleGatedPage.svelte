<!--
  RoleGatedPage — a route whose every surface needs one role. When the
  identity's reported capabilities say it lacks the role, the route renders its
  toolbar title and the pre-flight, and its content is never mounted, so none
  of its queries is issued. Otherwise (role held, or capabilities unknown) the
  content renders unchanged.
-->
<script lang="ts">
  import type { Snippet } from 'svelte';
  import { Toolbar } from '$lib/ui/primitives';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import RolePreflightNotice from './RolePreflightNotice.svelte';
  import type { RolePreflightCheck } from './rolePreflight';

  interface Props {
    check: RolePreflightCheck;
    /** The route's toolbar title. */
    title: string;
    testid: string;
    lead: string;
    children: Snippet;
  }

  let { check, title, testid, lead, children }: Props = $props();

  const preflight = $derived(check($accessCapabilities));
</script>

{#if preflight}
  <div class="gated-page">
    <Toolbar {title} />
    <RolePreflightNotice {preflight} {testid} {lead} />
  </div>
{:else}
  {@render children()}
{/if}

<style>
  .gated-page {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }
</style>
