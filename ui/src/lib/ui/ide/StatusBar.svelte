<script lang="ts">
  /**
   * 24px status bar at the bottom of the IDE shell.
   * Displays connection state, active profile, parser status, and branding.
   */

  export let connectionState: 'connected' | 'disconnected' | 'connecting' = 'disconnected';
  export let activeProfile: string = '';
  export let parserStatus: string = '';

  function connectionLabel(state: typeof connectionState): string {
    if (state === 'connected') return 'Connected';
    if (state === 'connecting') return 'Connecting';
    return 'Disconnected';
  }
</script>

<footer class="status-bar" role="status">
  <div class="status-left">
    <span
      class="connection"
      class:connected={connectionState === 'connected'}
      class:connecting={connectionState === 'connecting'}
      class:disconnected={connectionState === 'disconnected'}
      title={connectionLabel(connectionState)}
    >
      <span class="dot" aria-hidden="true"></span>
      <span class="connection-text">{connectionLabel(connectionState)}</span>
    </span>

    {#if activeProfile}
      <span class="separator" aria-hidden="true"></span>
      <span class="profile" title="Active profile">{activeProfile}</span>
    {/if}

    {#if parserStatus}
      <span class="separator" aria-hidden="true"></span>
      <span class="parser" title="Parser status">{parserStatus}</span>
    {/if}
  </div>

  <div class="status-right">
    <span class="branding">fi-fhir</span>
  </div>
</footer>

<style>
  .status-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    height: var(--ide-status-bar-height, 24px);
    min-height: var(--ide-status-bar-height, 24px);
    padding: 0 var(--space-3);
    background: var(--ide-status-bar-bg, var(--color-primary));
    color: var(--ide-status-bar-text, var(--color-text-inverse));
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    user-select: none;
    -webkit-user-select: none;
    overflow: hidden;
  }

  .status-left,
  .status-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .connection {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    white-space: nowrap;
  }

  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: 0 0 auto;
  }

  .connected .dot {
    background: #86efac;
    box-shadow: 0 0 4px rgba(134, 239, 172, 0.6);
  }

  .connecting .dot {
    background: #fde68a;
    animation: pulse 1.5s ease-in-out infinite;
  }

  .disconnected .dot {
    background: rgba(255, 255, 255, 0.4);
  }

  .connection-text {
    white-space: nowrap;
  }

  .separator {
    width: 1px;
    height: 12px;
    background: rgba(255, 255, 255, 0.3);
    flex: 0 0 auto;
  }

  .profile,
  .parser {
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .branding {
    font-family: var(--font-heading);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wide);
    opacity: 0.8;
  }

  @keyframes pulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.4; }
  }
</style>
