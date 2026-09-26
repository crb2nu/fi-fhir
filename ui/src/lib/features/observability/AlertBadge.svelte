<script lang="ts">
  /**
   * AlertBadge — Compact alert indicator for the StatusBar.
   * Shows firing alert count with severity-colored badge and dropdown popover.
   * Rendered only while the loom platform is connected (StatusBar); it lists
   * exactly what Alertmanager returned, never placeholder alerts.
   */
  import { onMount, onDestroy } from 'svelte';
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import { Icon } from '$lib/ui/primitives';
  import {
    observabilityState,
    activeAlertCount,
    alertSource,
    fetchAlerts,
    severityLabel,
  } from './observabilityStore';

  let showDropdown = false;
  let refreshInterval: ReturnType<typeof setInterval> | null = null;
  let badgeEl: HTMLElement | undefined = undefined;
  let triggerEl: HTMLButtonElement | undefined = undefined;
  let dropdownStyle = '';

  $: alerts = $observabilityState.alerts;
  $: firingCount = $activeAlertCount;
  $: hasCritical = alerts.some((a) => a.state === 'firing' && a.severity === 'critical');
  $: hasWarning = alerts.some((a) => a.state === 'firing' && a.severity === 'warning');

  function toggleDropdown() {
    showDropdown = !showDropdown;
    if (showDropdown && triggerEl) {
      const rect = triggerEl.getBoundingClientRect();
      const dropdownWidth = 340;
      const left = Math.max(8, rect.right - dropdownWidth);
      dropdownStyle = `position: fixed; bottom: ${window.innerHeight - rect.top + 6}px; left: ${left}px;`;
    }
  }

  function timeSince(ts: number): string {
    const diff = Math.floor((Date.now() - ts) / 1000);
    if (diff < 60) return `${diff}s ago`;
    if (diff < 3600) return `${Math.floor(diff / 60)}m ago`;
    return `${Math.floor(diff / 3600)}h ago`;
  }

  function handleClickOutside(event: MouseEvent) {
    if (badgeEl && !badgeEl.contains(event.target as Node)) {
      showDropdown = false;
    }
  }

  onMount(() => {
    fetchAlerts();
    refreshInterval = setInterval(() => fetchAlerts(), 30_000);
    document.addEventListener('click', handleClickOutside, true);
  });

  onDestroy(() => {
    if (refreshInterval) clearInterval(refreshInterval);
    document.removeEventListener('click', handleClickOutside, true);
  });
</script>

<div class="alert-badge-wrapper" bind:this={badgeEl}>
  <button
    class="badge-trigger"
    class:has-alerts={firingCount > 0}
    class:critical={hasCritical}
    class:warning={hasWarning && !hasCritical}
    on:click={toggleDropdown}
    bind:this={triggerEl}
    title="{firingCount} firing alert{firingCount === 1 ? '' : 's'}"
    aria-label="Alerts: {firingCount} firing"
    aria-expanded={showDropdown}
  >
    <Icon icon={TriangleAlert} size={12} />
    {#if firingCount > 0}
      <span class="badge-count" class:pulse={firingCount > 0}>{firingCount}</span>
    {/if}
  </button>

  {#if showDropdown}
    <div class="dropdown" role="dialog" aria-label="Alerts" style={dropdownStyle}>
      <div class="dropdown-header">
        <span class="dropdown-title">Alerts</span>
        <span class="dropdown-count">{alerts.length} total</span>
      </div>

      <div class="alert-list">
        {#each alerts as alert (alert.id)}
          <div class="alert-item" class:firing={alert.state === 'firing'} class:pending={alert.state === 'pending'} class:resolved={alert.state === 'resolved'}>
            <div class="alert-row-top">
              <span class="severity-tag severity-{alert.severity}">{severityLabel(alert.severity)}</span>
              <span class="alert-name">{alert.name}</span>
              <span class="alert-time">{timeSince(alert.startsAt)}</span>
            </div>
            <p class="alert-summary">{alert.summary}</p>
            <div class="alert-actions">
              <button class="action-btn" disabled>Silence</button>
              <button class="action-btn" disabled>Acknowledge</button>
            </div>
          </div>
        {/each}
      </div>

      {#if alerts.length === 0}
        <div class="empty-alerts">
          {$alertSource === 'unavailable' ? 'Alert source unavailable' : 'No alerts'}
        </div>
      {/if}
    </div>
  {/if}
</div>

<style>
  .alert-badge-wrapper {
    position: relative;
    display: inline-flex;
    align-items: center;
  }

  .badge-trigger {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 0 4px;
    height: 18px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-tertiary);
    cursor: pointer;
    transition: var(--transition-all);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    line-height: 1;
  }

  .badge-trigger:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .badge-trigger:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .badge-trigger.has-alerts {
    color: var(--color-text-primary);
  }

  .badge-trigger.critical {
    color: var(--color-danger-text);
  }

  .badge-trigger.warning {
    color: var(--color-warning-text);
  }

  .badge-count {
    font-family: var(--font-mono);
    min-width: 14px;
    text-align: center;
  }


  /* Dropdown — positioned via inline style (fixed) to escape overflow:hidden */
  .dropdown {
    width: 340px;
    max-height: 400px;
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-lg);
    overflow: hidden;
    z-index: var(--z-popover);
    animation: scaleIn var(--duration-normal) var(--ease-out);
  }

  .dropdown-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .dropdown-title {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .dropdown-count {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  .alert-list {
    overflow-y: auto;
    max-height: 320px;
  }

  .alert-item {
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    transition: var(--transition-colors);
  }

  .alert-item:last-child {
    border-bottom: none;
  }

  .alert-item:hover {
    background: var(--color-bg-hover);
  }

  .alert-item.firing {
    border-left: 2px solid var(--color-danger);
  }

  .alert-item.pending {
    border-left: 2px solid var(--color-warning);
    opacity: 0.85;
  }

  .alert-item.resolved {
    opacity: 0.5;
  }

  .alert-row-top {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .severity-tag {
    flex: 0 0 auto;
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
    border: 1px solid transparent;
    font-size: var(--text-2xs, 10px);
    font-weight: var(--font-semibold);
    letter-spacing: 0.02em;
    text-transform: uppercase;
    line-height: 1.4;
  }

  .severity-tag.severity-critical {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }

  .severity-tag.severity-warning {
    color: var(--color-warning-text);
    background: var(--color-warning-bg);
    border-color: var(--color-warning-border);
  }

  .severity-tag.severity-info {
    color: var(--color-info-text);
    background: var(--color-info-bg);
    border-color: var(--color-info-border);
  }


  .alert-name {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    flex: 1;
  }

  .alert-time {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    white-space: nowrap;
  }

  .alert-summary {
    margin: var(--space-1) 0 var(--space-1) 16px;
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    line-height: var(--leading-snug);
  }

  .alert-actions {
    display: flex;
    gap: var(--space-1);
    margin-left: 16px;
  }

  .action-btn {
    height: 20px;
    padding: 0 var(--space-2);
    font-size: 9px;
    font-weight: var(--font-medium);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: var(--color-bg-surface);
    color: var(--color-text-muted);
    cursor: not-allowed;
    opacity: 0.6;
  }

  .empty-alerts {
    padding: var(--space-6);
    text-align: center;
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  /* Animations */
  @keyframes scaleIn {
    from {
      opacity: 0;
      transform: scale(0.95) translateY(4px);
    }
    to {
      opacity: 1;
      transform: scale(1) translateY(0);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .badge-count.pulse {
      animation: none;
    }
    .dropdown {
      animation: none;
    }
  }
</style>
