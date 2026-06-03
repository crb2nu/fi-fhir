<script lang="ts">
  /**
   * AlertBadge — Compact alert indicator for the StatusBar.
   * Shows firing alert count with severity-colored badge and dropdown popover.
   */
  import { onMount, onDestroy } from 'svelte';
  import {
    observabilityState,
    activeAlertCount,
    fetchAlerts,
    type Alert,
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

  function severityIcon(severity: Alert['severity']): string {
    switch (severity) {
      case 'critical': return 'crit';
      case 'warning': return 'warn';
      case 'info': return 'info';
      default: return 'info';
    }
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
    <svg class="alert-icon" viewBox="0 0 16 16" fill="currentColor">
      <path d="M8 1.5a.5.5 0 01.424.235l6.5 10.5A.5.5 0 0114.5 13h-13a.5.5 0 01-.424-.765l6.5-10.5A.5.5 0 018 1.5zM7.5 10v1h1v-1h-1zm0-4v3h1V6h-1z" />
    </svg>
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
              <span class="severity-dot {severityIcon(alert.severity)}" aria-hidden="true"></span>
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
        <div class="empty-alerts">No alerts</div>
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
    color: rgba(255, 255, 255, 0.5);
    cursor: pointer;
    transition: var(--transition-all);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    line-height: 1;
  }

  .badge-trigger:hover {
    background: rgba(255, 255, 255, 0.1);
    color: rgba(255, 255, 255, 0.8);
  }

  .badge-trigger.has-alerts {
    color: rgba(255, 255, 255, 0.9);
  }

  .badge-trigger.critical {
    color: var(--palette-red-300);
  }

  .badge-trigger.warning {
    color: var(--palette-yellow-200);
  }

  .alert-icon {
    width: 12px;
    height: 12px;
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
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
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

  .severity-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex: 0 0 auto;
  }

  .severity-dot.crit {
    background: var(--color-danger);
    box-shadow: 0 0 6px rgba(239, 68, 68, 0.5);
  }

  .severity-dot.warn {
    background: var(--color-warning);
    box-shadow: 0 0 6px rgba(245, 158, 11, 0.4);
  }

  .severity-dot.info {
    background: var(--color-info);
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
    .badge-count.pulse,
    .alert-item.firing .severity-dot {
      animation: none;
    }
    .dropdown {
      animation: none;
    }
  }
</style>
