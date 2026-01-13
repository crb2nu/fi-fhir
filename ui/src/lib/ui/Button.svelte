<script lang="ts">
  export let variant: 'primary' | 'secondary' | 'danger' = 'primary';
  export let disabled = false;
</script>

<button class="btn {variant}" {disabled} {...$$restProps}>
  <slot />
</button>

<style>
  .btn {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.14);
    cursor: pointer;
    font-weight: 600;
    color: #f3f4f6;
    background: rgba(255, 255, 255, 0.06);
  }

  .btn:hover:enabled {
    background: rgba(255, 255, 255, 0.1);
  }

  .btn:disabled {
    opacity: 0.6;
    cursor: not-allowed;
  }

  .btn.secondary {
    background: rgba(255, 255, 255, 0.03);
  }

  .btn.danger {
    border-color: rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.08);
  }

  .btn.danger:hover:enabled {
    background: rgba(239, 68, 68, 0.14);
  }
</style>

