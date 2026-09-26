<script lang="ts">
  /**
   * 24 px status bar: a neutral strip where colour appears only as state dots.
   * Left: API connection, access chip (credential state + popover), optional
   * profile/parser, and the loom platform indicator. Right: `Next:` stage and
   * the build.
   *
   * The loom-platform indicator and its AlertBadge are optional chrome: they
   * render only when the build configured a platform endpoint
   * (`platformEnabled`, from PLATFORM_CONFIG.enabled). Otherwise there is no
   * platform to be connected to, and a permanently grey dot would only
   * suggest something is broken.
   */
  import { resolve } from '$app/paths';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import AlertBadge from '$lib/features/observability/AlertBadge.svelte';
  import type { AccessSession } from '$lib/graphql/GraphQLCredentialGate.svelte';
  import { Icon } from '$lib/ui/primitives';
  import AccessChip from './AccessChip.svelte';
  import { getJourneyState } from './journey';
  import { connectionLabel, type ConnectionState } from './connection';

  export let connectionState: ConnectionState = 'disconnected';
  export let activeProfile: string = '';
  export let parserStatus: string = '';
  export let platformEnabled: boolean = false;
  export let platformConnected: boolean = false;
  export let access: AccessSession | null = null;
  export let onClearAccess: (() => void) | undefined = undefined;
  export let pathname: string = '/';

  const buildTag = (import.meta.env.VITE_BUILD_TAG as string | undefined) || '';
  const buildSha = ((import.meta.env.VITE_BUILD_SHA as string | undefined) || '').slice(0, 8);
  const buildLabel = buildTag || buildSha;

  $: nextStage = getJourneyState(pathname).nextStage;
</script>

<footer class="status-bar" role="status">
  <div class="status-left">
    <span
      class="status-item connection"
      data-state={connectionState}
      title="API health (/health, checked every 30 s): {connectionLabel(connectionState)}"
    >
      <span class="dot" aria-hidden="true"></span>
      <span class="connection-text">{connectionLabel(connectionState)}</span>
    </span>

    {#if access}
      <AccessChip {access} onclear={onClearAccess} />
    {/if}

    {#if activeProfile}
      <span class="status-item profile" title="Active profile">{activeProfile}</span>
    {/if}

    {#if parserStatus}
      <span class="status-item parser" title="Parser status">{parserStatus}</span>
    {/if}

    {#if platformEnabled}
      <span
        class="status-item platform"
        class:platform-connected={platformConnected}
        data-testid="platform-indicator"
        title={platformConnected ? 'Platform connected' : 'Platform disconnected'}
      >
        <span class="dot" aria-hidden="true"></span>
        <span>Platform</span>
      </span>

      {#if platformConnected}
        <AlertBadge />
      {/if}
    {/if}
  </div>

  <div class="status-right">
    {#if nextStage}
      <a
        class="status-item status-next"
        href={resolve(nextStage.route)}
        data-testid="status-next"
        title="Stage {nextStage.order} of 5: {nextStage.label}"
      >
        <span class="next-key">Next:</span>
        <span>{nextStage.label}</span>
        <Icon icon={ArrowRight} size={12} />
      </a>
    {/if}
    <span class="status-item build" title={buildSha ? `Build ${buildSha}` : undefined}>
      <span class="wordmark">fi-fhir</span>
      {#if buildLabel}
        <span class="build-version">{buildLabel}</span>
      {/if}
    </span>
  </div>
</footer>

<style>
  .status-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-4);
    height: var(--statusbar-height, 24px);
    min-height: var(--statusbar-height, 24px);
    padding: 0 var(--space-1);
    background: var(--ide-status-bar-bg, var(--color-bg-elevated));
    color: var(--ide-status-bar-text, var(--color-text-secondary));
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    line-height: 1;
    user-select: none;
    -webkit-user-select: none;
    overflow: hidden;
  }

  .status-left,
  .status-right {
    display: flex;
    align-items: center;
    gap: 2px;
    min-width: 0;
  }

  .status-item {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 20px;
    padding: 0 6px;
    border-radius: var(--radius-sm);
    white-space: nowrap;
  }

  .dot {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    background: var(--color-text-muted);
    flex: 0 0 auto;
  }

  .connection[data-state='connected'] .dot {
    background: var(--color-success);
  }

  .connection[data-state='connecting'] .dot {
    background: var(--color-warning);
  }

  .connection[data-state='disconnected'] .dot {
    background: var(--color-danger);
  }

  .profile,
  .parser {
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 240px;
  }

  .platform {
    color: var(--color-text-muted);
  }

  .platform.platform-connected {
    color: inherit;
  }

  .platform.platform-connected .dot {
    background: var(--color-success);
  }

  .status-next {
    color: inherit;
    text-decoration: none;
    transition: var(--transition-colors);
  }

  .status-next:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .status-next:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .next-key {
    color: var(--color-text-tertiary);
  }

  .build {
    color: var(--color-text-tertiary);
  }

  .wordmark {
    font-family: var(--font-heading);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-tight);
  }

  .build-version {
    font-family: var(--font-mono);
    font-size: var(--text-label);
    font-variant-numeric: tabular-nums;
  }
</style>
