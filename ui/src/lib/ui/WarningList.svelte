<script lang="ts">
  import type { WarningGroup } from '$lib/domain/warnings';

  export let groups: readonly WarningGroup[];
</script>

{#if groups.length === 0}
  <div class="empty">No warnings</div>
{:else}
  <div class="groups">
    {#each groups as g (g.phase)}
      <div class="group">
        <div class="group-title">
          <span class="phase">{g.phase}</span>
          <span class="count">{g.items.length}</span>
        </div>
        <ul class="list">
          {#each g.items as w, idx (w.phase + ':' + w.code + ':' + idx)}
            <li class="item">
              <div class="top">
                <span class="code">{w.code}</span>
                {#if w.path}
                  <span class="path">{w.path}</span>
                {/if}
              </div>
              <div class="msg">{w.message}</div>
            </li>
          {/each}
        </ul>
      </div>
    {/each}
  </div>
{/if}

<style>
  .empty {
    color: rgba(229, 231, 235, 0.7);
  }

  .groups {
    display: grid;
    gap: 12px;
  }

  .group {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    padding: 10px 12px;
  }

  .group-title {
    display: flex;
    align-items: center;
    justify-content: space-between;
    margin-bottom: 8px;
    color: rgba(243, 244, 246, 0.95);
    font-weight: 700;
  }

  .phase {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.95rem;
  }

  .count {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.88);
    font-size: 0.85rem;
  }

  .list {
    list-style: none;
    padding: 0;
    margin: 0;
    display: grid;
    gap: 10px;
  }

  .item {
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
    padding: 10px 10px;
  }

  .top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
  }

  .code {
    font-weight: 800;
    color: rgba(229, 231, 235, 0.92);
  }

  .path {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.7);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 60%;
  }

  .msg {
    margin-top: 6px;
    color: rgba(229, 231, 235, 0.82);
    line-height: 1.4;
  }
</style>

