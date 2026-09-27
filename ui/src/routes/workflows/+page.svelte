<script lang="ts">
  import RoleGatedPage from '$lib/features/access/RoleGatedPage.svelte';
  import { compatibilityGrantPreflight } from '$lib/features/access/rolePreflight';
  import WorkflowsPage from '$lib/features/workflows/WorkflowsPage.svelte';
</script>

<RoleGatedPage
  check={compatibilityGrantPreflight}
  title="Workflows"
  testid="workflows-preflight"
  lead="The workflow catalog, dry runs and the builder's server checks need"
>
  <WorkflowsPage />
</RoleGatedPage>
