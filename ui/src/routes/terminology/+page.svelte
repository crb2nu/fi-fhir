<script lang="ts">
  import RoleGatedPage from '$lib/features/access/RoleGatedPage.svelte';
  import { compatibilityGrantPreflight } from '$lib/features/access/rolePreflight';
  import TerminologyPage from '$lib/features/terminology/TerminologyPage.svelte';
</script>

<svelte:head>
  <title>Terminology | fi-fhir</title>
</svelte:head>

<RoleGatedPage
  check={compatibilityGrantPreflight}
  title="Terminology"
  testid="terminology-preflight"
  lead="Terminology mappings and autoroute review need"
>
  <TerminologyPage />
</RoleGatedPage>
