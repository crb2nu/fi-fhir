<script lang="ts">
  export let title: string;
  export let subtitle: string | undefined = undefined;
</script>

<header class="page-header">
  <h1>{title}</h1>
  {#if subtitle}
    <p class="subtitle">{subtitle}</p>
  {/if}
</header>

<style>
  .page-header {
    display: grid;
    gap: 6px;
    margin-bottom: 16px;
  }

  h1 {
    margin: 0;
    color: var(--color-text-primary);
  }

  .subtitle {
    margin: 0;
    color: var(--color-text-secondary);
    line-height: 1.55;
    max-width: 72ch;
  }
</style>
