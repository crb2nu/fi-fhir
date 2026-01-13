<script lang="ts" context="module">
  export type TabItem = { key: string; label: string; disabled?: boolean };
</script>

<script lang="ts">
  export let tabs: readonly TabItem[];
  export let active: string;
  export let onChange: (key: string) => void;
</script>

<div class="tabs" role="tablist">
  {#each tabs as tab (tab.key)}
    <button
      class="tab"
      class:active={tab.key === active}
      disabled={tab.disabled}
      role="tab"
      aria-selected={tab.key === active}
      on:click={() => onChange(tab.key)}
    >
      {tab.label}
    </button>
  {/each}
</div>

<style>
  .tabs {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .tab {
    padding: 8px 10px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.88);
    cursor: pointer;
    font-weight: 600;
  }

  .tab.active {
    background: rgba(59, 130, 246, 0.18);
    border-color: rgba(59, 130, 246, 0.35);
    color: #f3f4f6;
  }

  .tab:disabled {
    opacity: 0.55;
    cursor: not-allowed;
  }
</style>
