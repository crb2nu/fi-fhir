<script lang="ts">
  /**
   * PresenceBar Component
   *
   * Compact horizontal bar showing who is working in the workspace.
   * Displays avatar circles with name, status dot, and current file/stage.
   */
  import { onMount } from 'svelte';
  import {
    collaborationState,
    activePresence,
    fetchPresence
  } from './collaborationStore';
  import type { AgentPresence } from './collaborationStore';

  export let compact = false;

  let hoveredAgent: string | null = null;
  let tooltipX = 0;
  let tooltipY = 0;

  onMount(() => {
    if ($collaborationState.presence.length === 0) {
      fetchPresence();
    }
  });

  function getInitial(name: string): string {
    return name.charAt(0).toUpperCase();
  }

  function isAiAgent(agent: AgentPresence): boolean {
    return agent.agentType !== 'human';
  }

  function statusLabel(status: AgentPresence['status']): string {
    if (status === 'active') return 'Active';
    if (status === 'idle') return 'Idle';
    return 'Away';
  }

  function agentTypeLabel(agentType: AgentPresence['agentType']): string {
    if (agentType === 'human') return 'Operator';
    if (agentType === 'claude-code') return 'Claude Code';
    if (agentType === 'codex') return 'Codex';
    if (agentType === 'gemini') return 'Gemini';
    return 'Kilocode';
  }

  function formatLastSeen(ts: number): string {
    const diff = Date.now() - ts;
    if (diff < 60_000) return 'just now';
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
    return `${Math.floor(diff / 86_400_000)}d ago`;
  }

  function handleMouseEnter(event: MouseEvent, agentId: string): void {
    hoveredAgent = agentId;
    const rect = (event.currentTarget as HTMLElement).getBoundingClientRect();
    tooltipX = rect.left + rect.width / 2;
    tooltipY = rect.bottom + 8;
  }

  function handleMouseLeave(): void {
    hoveredAgent = null;
  }

  $: agents = compact ? $activePresence : $collaborationState.presence;
  $: hoveredAgentData = $collaborationState.presence.find(
    (a) => a.agentId === hoveredAgent
  );
</script>

<div class="presence-bar" class:compact>
  <div class="agents-row">
    {#each agents as agent (agent.agentId)}
      <div
        class="agent-chip"
        class:stacked={compact}
        role="status"
        aria-label="{agent.displayName}: {statusLabel(agent.status)}"
        on:mouseenter={(e) => handleMouseEnter(e, agent.agentId)}
        on:mouseleave={handleMouseLeave}
      >
        <div class="avatar" style="background-color: {agent.avatarColor}">
          <span class="avatar-initial">{getInitial(agent.displayName)}</span>
          {#if isAiAgent(agent)}
            <span class="ai-indicator" aria-hidden="true">
              <svg viewBox="0 0 12 12" fill="currentColor">
                <path d="M6 0l1.5 4.5L12 6l-4.5 1.5L6 12l-1.5-4.5L0 6l4.5-1.5z" />
              </svg>
            </span>
          {/if}
          <span
            class="status-dot"
            class:active={agent.status === 'active'}
            class:idle={agent.status === 'idle'}
            class:away={agent.status === 'away'}
            aria-hidden="true"
          ></span>
        </div>

        {#if !compact}
          <div class="agent-info">
            <span class="agent-name">{agent.displayName}</span>
            <span class="agent-detail">
              {#if agent.currentFile}
                {agent.currentFile}
              {:else if agent.status === 'idle'}
                idle
              {:else}
                {statusLabel(agent.status).toLowerCase()}
              {/if}
            </span>
          </div>
        {/if}
      </div>
    {/each}
  </div>

  {#if hoveredAgentData}
    <div
      class="tooltip"
      style="left: {tooltipX}px; top: {tooltipY}px"
      role="tooltip"
    >
      <div class="tooltip-header">
        <div
          class="tooltip-avatar"
          style="background-color: {hoveredAgentData.avatarColor}"
        >
          {getInitial(hoveredAgentData.displayName)}
        </div>
        <div class="tooltip-name-group">
          <span class="tooltip-name">{hoveredAgentData.displayName}</span>
          <span class="tooltip-type">{agentTypeLabel(hoveredAgentData.agentType)}</span>
        </div>
      </div>
      <div class="tooltip-details">
        <div class="tooltip-row">
          <span class="tooltip-label">Status</span>
          <span class="tooltip-value">
            <span
              class="tooltip-status-dot"
              class:active={hoveredAgentData.status === 'active'}
              class:idle={hoveredAgentData.status === 'idle'}
              class:away={hoveredAgentData.status === 'away'}
            ></span>
            {statusLabel(hoveredAgentData.status)}
          </span>
        </div>
        {#if hoveredAgentData.currentFile}
          <div class="tooltip-row">
            <span class="tooltip-label">File</span>
            <span class="tooltip-value mono">{hoveredAgentData.currentFile}</span>
          </div>
        {/if}
        {#if hoveredAgentData.currentStage}
          <div class="tooltip-row">
            <span class="tooltip-label">Stage</span>
            <span class="tooltip-value">{hoveredAgentData.currentStage}</span>
          </div>
        {/if}
        <div class="tooltip-row">
          <span class="tooltip-label">Seen</span>
          <span class="tooltip-value">{formatLastSeen(hoveredAgentData.lastSeen)}</span>
        </div>
      </div>
    </div>
  {/if}
</div>

<style>
  .presence-bar {
    position: relative;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-lg);
    background: var(--color-bg-elevated);
  }

  .presence-bar.compact {
    padding: var(--space-1) var(--space-2);
    border: none;
    background: transparent;
  }

  .agents-row {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  .compact .agents-row {
    gap: 0;
  }

  /* Agent chip */
  .agent-chip {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    cursor: default;
    animation: fadeIn var(--duration-slow) var(--ease-out) both;
  }

  .agent-chip.stacked {
    margin-left: -8px;
  }

  .agent-chip.stacked:first-child {
    margin-left: 0;
  }

  /* Avatar */
  .avatar {
    position: relative;
    width: 32px;
    height: 32px;
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    justify-content: center;
    flex-shrink: 0;
    border: 2px solid var(--color-bg-base);
    box-shadow: 0 0 0 1px var(--color-border-subtle);
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .agent-chip:hover .avatar {
    transform: scale(1.08);
    z-index: 1;
  }

  .avatar-initial {
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--palette-white);
    line-height: 1;
    user-select: none;
  }

  /* AI sparkle indicator */
  .ai-indicator {
    position: absolute;
    top: -3px;
    right: -3px;
    width: 12px;
    height: 12px;
    color: var(--palette-white);
    filter: drop-shadow(0 0 2px rgba(0, 0, 0, 0.4));
  }

  .ai-indicator svg {
    width: 100%;
    height: 100%;
  }

  /* Status dot */
  .status-dot {
    position: absolute;
    bottom: -1px;
    right: -1px;
    width: 10px;
    height: 10px;
    border-radius: var(--radius-full);
    border: 2px solid var(--color-bg-base);
  }

  .status-dot.active {
    background-color: var(--color-success);
    animation: statusPulse 2s ease-in-out infinite;
  }

  .status-dot.idle {
    background-color: var(--color-warning);
  }

  .status-dot.away {
    background-color: var(--color-text-muted);
  }

  /* Agent info text */
  .agent-info {
    display: flex;
    flex-direction: column;
    gap: 1px;
    min-width: 0;
  }

  .agent-name {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    line-height: var(--leading-tight);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  .agent-detail {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
    max-width: 140px;
  }

  /* Tooltip */
  .tooltip {
    position: fixed;
    z-index: var(--z-tooltip);
    transform: translateX(-50%);
    background: var(--color-bg-overlay);
    backdrop-filter: blur(16px);
    -webkit-backdrop-filter: blur(16px);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-xl);
    padding: var(--space-3);
    min-width: 200px;
    max-width: 280px;
    box-shadow: var(--shadow-lg);
    animation: scaleIn var(--duration-fast) var(--ease-out);
    pointer-events: none;
  }

  .tooltip-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
    padding-bottom: var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .tooltip-avatar {
    width: 28px;
    height: 28px;
    border-radius: var(--radius-full);
    display: flex;
    align-items: center;
    justify-content: center;
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--palette-white);
    flex-shrink: 0;
  }

  .tooltip-name-group {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .tooltip-name {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .tooltip-type {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
  }

  .tooltip-details {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .tooltip-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .tooltip-label {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    flex-shrink: 0;
  }

  .tooltip-value {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    text-align: right;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .tooltip-value.mono {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
  }

  .tooltip-status-dot {
    width: 6px;
    height: 6px;
    border-radius: var(--radius-full);
    flex-shrink: 0;
  }

  .tooltip-status-dot.active {
    background-color: var(--color-success);
  }

  .tooltip-status-dot.idle {
    background-color: var(--color-warning);
  }

  .tooltip-status-dot.away {
    background-color: var(--color-text-muted);
  }

  /* Animations */
  @keyframes statusPulse {
    0%, 100% { opacity: 1; }
    50% { opacity: 0.55; }
  }

  @keyframes fadeIn {
    from { opacity: 0; transform: translateY(4px); }
    to { opacity: 1; transform: translateY(0); }
  }

  @keyframes scaleIn {
    from { opacity: 0; transform: translateX(-50%) scale(0.95); }
    to { opacity: 1; transform: translateX(-50%) scale(1); }
  }

  @media (prefers-reduced-motion: reduce) {
    .status-dot.active {
      animation: none;
    }

    .agent-chip {
      animation: none;
    }

    .tooltip {
      animation: none;
    }
  }
</style>
