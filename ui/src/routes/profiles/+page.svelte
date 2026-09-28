<script lang="ts">
  import RoleGatedPage from '$lib/features/access/RoleGatedPage.svelte';
  import { compatibilityGrantPreflight } from '$lib/features/access/rolePreflight';
  import ProfilesPage from '$lib/features/profiles/ProfilesPage.svelte';
</script>

<RoleGatedPage
  check={compatibilityGrantPreflight}
  title="Profiles"
  testid="profiles-preflight"
  lead="Source profiles need"
>
  <ProfilesPage />
</RoleGatedPage>
