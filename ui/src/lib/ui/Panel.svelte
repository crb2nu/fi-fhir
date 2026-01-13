<script lang="ts">
  export let title: string | null = null;
  export let tone: 'default' | 'error' | 'success' = 'default';
</script>

<section class="panel {tone}">
  {#if title}
    <header class="header">{title}</header>
  {/if}
  <div class="body">
    <slot />
  </div>
</section>

<style>
  .panel {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
  }

  .panel.error {
    border-color: rgba(239, 68, 68, 0.5);
  }

  .panel.success {
    border-color: rgba(16, 185, 129, 0.35);
  }

  .header {
    padding: 12px 14px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.08);
    color: #f3f4f6;
    font-weight: 700;
    font-size: 0.95rem;
  }

  .body {
    padding: 12px 14px;
    color: rgba(229, 231, 235, 0.9);
  }
</style>

