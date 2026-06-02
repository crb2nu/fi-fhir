<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';

  export let source: string = 'unknown';
  export let profile: string | null = null;
  export let workflow: string | null = null;
  export let eventCount: number = 0;
  export let warningCount: number = 0;
  export let success: boolean = false;
  export let destinations: string[] = [];

  const dispatch = createEventDispatcher<{
    navigate: { step: string };
  }>();

  function onStepClick(step: string) {
    dispatch('navigate', { step });
  }

  function handleKeydown(event: KeyboardEvent, step: string) {
    if (event.key === 'Enter' || event.key === ' ') {
      event.preventDefault();
      onStepClick(step);
    }
  }
</script>

<div class="trace-container">
  <div
    class="step"
    role="button"
    tabindex="0"
    on:click={() => onStepClick('samples')}
    on:keydown={(e) => handleKeydown(e, 'samples')}
    aria-label="Navigate to Source samples"
  >
    <div class="node source">
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M4 7V4a2 2 0 0 1 2-2h12a2 2 0 0 1 2 2v3M4 17v3a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-3M4 7h16M4 17h16M4 7v10M20 7v10" />
        </svg>
      </div>
      <div class="label">Source</div>
      <div class="value mono">{source}</div>
    </div>
    <div class="connector">
      <svg width="40" height="20" viewBox="0 0 40 20">
        <path d="M0 10 H35" stroke="var(--color-border-strong)" stroke-width="2" stroke-dasharray="4 2" />
        <path d="M35 10 L30 5 M35 10 L30 15" stroke="var(--color-border-strong)" stroke-width="2" />
      </svg>
    </div>
  </div>

  <div
    class="step"
    role="button"
    tabindex="0"
    on:click={() => onStepClick('profile')}
    on:keydown={(e) => handleKeydown(e, 'profile')}
    aria-label="Navigate to Source Profile"
  >
    <div class="node profile" class:active={!!profile}>
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M14.7 6.3a1 1 0 0 0 0 1.4l1.6 1.6a1 1 0 0 0 1.4 0l3.77-3.7a6 6 0 0 1-7.94 7.94l-6.91 6.91a2.12 2.12 0 0 1-3-3l6.91-6.91a6 6 0 0 1 7.94-7.94l-3.76 3.7z" />
        </svg>
      </div>
      <div class="label">Source Profile</div>
      <div class="value mono">{profile || 'Default (Strict)'}</div>
      <div class="badges">
        {#if warningCount > 0}
          <Badge variant="warning" size="sm">{warningCount} warnings</Badge>
        {/if}
      </div>
    </div>
    <div class="connector">
      <svg width="40" height="20" viewBox="0 0 40 20">
        <path d="M0 10 H35" stroke="var(--color-border-strong)" stroke-width="2" />
        <path d="M35 10 L30 5 M35 10 L30 15" stroke="var(--color-border-strong)" stroke-width="2" />
      </svg>
    </div>
  </div>

  <div
    class="step"
    role="button"
    tabindex="0"
    on:click={() => onStepClick('events')}
    on:keydown={(e) => handleKeydown(e, 'events')}
    aria-label="Navigate to Extracted Events"
  >
    <div class="node workflow" class:active={eventCount > 0}>
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <polyline points="22 12 18 12 15 21 9 3 6 12 2 12" />
        </svg>
      </div>
      <div class="label">Workflow</div>
      <div class="value mono">{workflow || 'Auto-Route'}</div>
      <div class="badges">
        {#if eventCount > 0}
          <Badge variant="success" size="sm">{eventCount} events</Badge>
        {/if}
      </div>
    </div>
    <div class="connector">
      <svg width="40" height="20" viewBox="0 0 40 20">
        <path d="M0 10 H35" stroke="var(--color-border-strong)" stroke-width="2" />
        <path d="M35 10 L30 5 M35 10 L30 15" stroke="var(--color-border-strong)" stroke-width="2" />
      </svg>
    </div>
  </div>

  <div
    class="step"
    role="button"
    tabindex="0"
    on:click={() => onStepClick('process')}
    on:keydown={(e) => handleKeydown(e, 'process')}
    aria-label="Navigate to Process and Destination"
  >
    <div class="node destination" class:active={success}>
      <div class="icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2">
          <path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4" />
          <polyline points="17 8 12 3 7 8" />
          <line x1="12" y1="3" x2="12" y2="15" />
        </svg>
      </div>
      <div class="label">Destination</div>
      <div class="value mono">
        {#if destinations.length > 0}
          {destinations.join(', ')}
        {:else if success}
          FHIR Storage
        {:else}
          -
        {/if}
      </div>
      {#if success}
        <div class="badges">
          <Badge variant="success" size="sm">Delivered</Badge>
        </div>
      {/if}
    </div>
  </div>
</div>

<style>
  .trace-container {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-6);
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-2xl);
    overflow-x: auto;
    gap: 0;
  }

  .step {
    display: flex;
    align-items: center;
    cursor: pointer;
    transition: transform 0.2s ease;
    outline: none;
  }

  .step:focus-visible {
    transform: translateY(-2px);
  }

  .step:focus-visible .node {
    border-color: var(--color-primary);
    box-shadow: 0 0 0 2px var(--color-bg-elevated), 0 0 0 4px var(--color-primary);
  }

  .step:hover {
    transform: translateY(-2px);
  }

  .node {
    display: flex;
    flex-direction: column;
    align-items: center;
    text-align: center;
    padding: var(--space-4);
    min-width: 140px;
    background: var(--color-bg-surface);
    border: 2px solid var(--color-border-default);
    border-radius: var(--radius-xl);
    position: relative;
    transition: all 0.2s ease;
  }

  .node.active {
    border-color: var(--color-primary-border);
    box-shadow: var(--shadow-md);
  }

  .icon {
    width: 32px;
    height: 32px;
    margin-bottom: var(--space-2);
    color: var(--color-text-muted);
  }

  .node.active .icon {
    color: var(--color-primary);
  }

  .label {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-tertiary);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    margin-bottom: 4px;
  }

  .value {
    font-size: var(--text-sm);
    color: var(--color-text-primary);
    max-width: 120px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .badges {
    position: absolute;
    top: -10px;
    right: -10px;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }

  .connector {
    display: flex;
    align-items: center;
    color: var(--color-border-strong);
  }

  .mono {
    font-family: var(--font-mono);
  }

  @media (max-width: 768px) {
    .trace-container {
      flex-direction: column;
      gap: var(--space-4);
    }

    .connector {
      transform: rotate(90deg);
      height: 40px;
    }
  }
</style>
