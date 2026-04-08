<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';

  const alerts: Array<{
    id: string;
    title: string;
    description: string;
    actionLabel: string;
    href: IDEAppRoute;
    type: 'warning' | 'error';
  }> = [
    {
      id: 'a1',
      title: 'New message type detected',
      description: 'Feed "epic_adt" sent ADT^A08 messages which have no active routing rules.',
      actionLabel: 'Create route',
      href: '/workflows',
      type: 'warning'
    },
    {
      id: 'a2',
      title: 'Parser drift detected',
      description: 'Unmapped segments (Z-segments) spiked to 12% in the last hour.',
      actionLabel: 'Review profile',
      href: '/profiles',
      type: 'error'
    }
  ];
</script>

<Panel title="Active Alerts" padding="md">
  {#if alerts.length === 0}
    <div class="empty">No active alerts.</div>
  {:else}
    <div class="alerts-list">
      {#each alerts as alert (alert.id)}
        <div class="alert-card">
          <div class="alert-content">
            <div class="alert-header">
              <Badge variant={alert.type === 'error' ? 'danger' : 'warning'} size="sm">
                {alert.type}
              </Badge>
              <h4 class="title">{alert.title}</h4>
            </div>
            <p class="description">{alert.description}</p>
          </div>
          <a class="action-link" href={resolve(alert.href)}>{alert.actionLabel}</a>
        </div>
      {/each}
    </div>
  {/if}
</Panel>

<style>
  .empty {
    color: var(--color-text-tertiary);
    font-size: var(--text-sm);
    padding: var(--space-4) 0;
    text-align: center;
  }

  .alerts-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .alert-card {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-default);
    border-left: 3px solid var(--color-warning);
    border-radius: var(--radius-md);
  }

  .alert-card:first-child {
    border-left-color: var(--color-danger);
  }

  .alert-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .title {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .description {
    margin: 0;
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    line-height: var(--leading-snug);
  }

  .action-link {
    align-self: flex-start;
    display: inline-flex;
    align-items: center;
    padding: 6px 12px;
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    color: var(--color-text-primary);
    text-decoration: none;
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    transition: var(--transition-colors);
  }

  .action-link:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-primary-border);
  }
</style>
