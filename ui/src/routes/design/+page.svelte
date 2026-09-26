<script lang="ts">
  // Dev-only: in production `import.meta.env.DEV` is the literal `false`, so the
  // dynamic import is dead code and the gallery chunk is never emitted.
  const gallery = import.meta.env.DEV ? import('./Gallery.svelte') : null;
</script>

{#if gallery}
  {#await gallery then module}
    <module.default />
  {/await}
{/if}
