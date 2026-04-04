<script lang="ts">
  /**
   * HandoffDialog Component
   *
   * Modal dialog for creating and receiving integration handoffs.
   * Shows context summary (open documents, decisions, diagnostics)
   * and supports accept/reject/create flows.
   */
  import { createEventDispatcher } from 'svelte';
  import {
    collaborationState,
    createHandoff,
    acceptHandoff,
    rejectHandoff,
    CURRENT_AGENT_ID
  } from './collaborationStore';
  import type { Handoff, AgentPresence } from './collaborationStore';

  export let mode: 'create' | 'receive' = 'create';
  export let handoff: Handoff | null = null;
  export let open = false;

  const dispatch = createEventDispatcher<{ close: void }>();

  // Create-mode form state
  let toAgent = '';
  let summary = '';

  // Simulated current context
  const currentContext = {
    openDocuments: ['/workflows/adt-a01.yaml', '/profiles/us-core-patient.json', '/terminology/race-ethnicity.csv'],
    decisions: ['Use US Core R4 profiles', 'Map PID-10 to Patient.race extension'],
    diagnosticCount: 5,
    currentStage: 'Translation' as string | undefined
  };

  function close(): void {
    open = false;
    toAgent = '';
    summary = '';
    dispatch('close');
  }

  function handleBackdropClick(event: MouseEvent): void {
    if (event.target === event.currentTarget) {
      close();
    }
  }

  function handleKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      close();
    }
  }

  async function handleCreate(): Promise<void> {
    if (!summary.trim()) return;
    await createHandoff({
      fromAgent: CURRENT_AGENT_ID,
      toAgent: toAgent || undefined,
      summary: summary.trim(),
      context: currentContext
    });
    close();
  }

  async function handleAccept(): Promise<void> {
    if (!handoff) return;
    await acceptHandoff(handoff.id);
    close();
  }

  async function handleReject(): Promise<void> {
    if (!handoff) return;
    await rejectHandoff(handoff.id);
    close();
  }

  function agentDisplayName(agentId: string): string {
    if (agentId === CURRENT_AGENT_ID) return 'You';
    const agent = $collaborationState.presence.find(
      (a) => a.agentId === agentId
    );
    return agent?.displayName ?? agentId;
  }

  function isAvailableTarget(agent: AgentPresence): boolean {
    return agent.agentId !== CURRENT_AGENT_ID && agent.status !== 'away';
  }

  function formatTimestamp(ts: number): string {
    const diff = Date.now() - ts;
    if (diff < 60_000) return 'just now';
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    return `${Math.floor(diff / 3_600_000)}h ago`;
  }

  $: availableAgents = $collaborationState.presence.filter(isAvailableTarget);
</script>

<svelte:window on:keydown={handleKeydown} />

{#if open}
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <div
    class="backdrop"
    on:click={handleBackdropClick}
    role="dialog"
    aria-modal="true"
    aria-label={mode === 'create' ? 'Create handoff' : 'Incoming handoff'}
    tabindex="-1"
  >
    <div class="dialog">
      {#if mode === 'create'}
        <!-- Create handoff -->
        <header class="dialog-header">
          <h2 class="dialog-title">Hand Off Integration Work</h2>
          <button
            type="button"
            class="close-btn"
            on:click={close}
            aria-label="Close"
          >
            <svg viewBox="0 0 16 16" fill="currentColor">
              <path d="M4.646 4.646a.5.5 0 01.708 0L8 7.293l2.646-2.647a.5.5 0 01.708.708L8.707 8l2.647 2.646a.5.5 0 01-.708.708L8 8.707l-2.646 2.647a.5.5 0 01-.708-.708L7.293 8 4.646 5.354a.5.5 0 010-.708z" />
            </svg>
          </button>
        </header>

        <div class="dialog-body">
          <div class="field">
            <label class="field-label" for="handoff-to">To</label>
            <select id="handoff-to" class="field-select" bind:value={toAgent}>
              <option value="">Anyone available</option>
              {#each availableAgents as agent (agent.agentId)}
                <option value={agent.agentId}>
                  {agent.displayName}
                  ({agent.agentType === 'human' ? 'Operator' : agent.agentType})
                </option>
              {/each}
            </select>
          </div>

          <div class="field">
            <label class="field-label" for="handoff-summary">Summary</label>
            <textarea
              id="handoff-summary"
              class="field-textarea"
              rows="3"
              placeholder="Describe what you're handing off..."
              bind:value={summary}
            ></textarea>
          </div>

          <div class="context-section">
            <span class="context-heading">Context included</span>
            <ul class="context-list">
              <li class="context-item">
                <span class="check-icon" aria-hidden="true">
                  <svg viewBox="0 0 16 16" fill="currentColor">
                    <path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2.5-2.5a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z" />
                  </svg>
                </span>
                {currentContext.openDocuments.length} open document{currentContext.openDocuments.length === 1 ? '' : 's'}
              </li>
              <li class="context-item">
                <span class="check-icon" aria-hidden="true">
                  <svg viewBox="0 0 16 16" fill="currentColor">
                    <path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2.5-2.5a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z" />
                  </svg>
                </span>
                {currentContext.decisions.length} decision{currentContext.decisions.length === 1 ? '' : 's'} recorded
              </li>
              <li class="context-item">
                <span class="check-icon" aria-hidden="true">
                  <svg viewBox="0 0 16 16" fill="currentColor">
                    <path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2.5-2.5a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z" />
                  </svg>
                </span>
                {currentContext.diagnosticCount} diagnostic{currentContext.diagnosticCount === 1 ? '' : 's'}
              </li>
              {#if currentContext.currentStage}
                <li class="context-item">
                  <span class="check-icon" aria-hidden="true">
                    <svg viewBox="0 0 16 16" fill="currentColor">
                      <path d="M12.207 4.793a1 1 0 010 1.414l-5 5a1 1 0 01-1.414 0l-2.5-2.5a1 1 0 011.414-1.414L6.5 9.086l4.293-4.293a1 1 0 011.414 0z" />
                    </svg>
                  </span>
                  Current stage: {currentContext.currentStage}
                </li>
              {/if}
            </ul>
          </div>
        </div>

        <footer class="dialog-footer">
          <button type="button" class="dialog-btn cancel" on:click={close}>
            Cancel
          </button>
          <button
            type="button"
            class="dialog-btn create"
            disabled={!summary.trim()}
            on:click={handleCreate}
          >
            Create Handoff
          </button>
        </footer>

      {:else if handoff}
        <!-- Receive handoff -->
        <header class="dialog-header">
          <h2 class="dialog-title">
            Incoming Handoff from {agentDisplayName(handoff.fromAgent)}
          </h2>
          <button
            type="button"
            class="close-btn"
            on:click={close}
            aria-label="Close"
          >
            <svg viewBox="0 0 16 16" fill="currentColor">
              <path d="M4.646 4.646a.5.5 0 01.708 0L8 7.293l2.646-2.647a.5.5 0 01.708.708L8.707 8l2.647 2.646a.5.5 0 01-.708.708L8 8.707l-2.646 2.647a.5.5 0 01-.708-.708L7.293 8 4.646 5.354a.5.5 0 010-.708z" />
            </svg>
          </button>
        </header>

        <div class="dialog-body">
          <blockquote class="handoff-summary">
            {handoff.summary}
          </blockquote>

          <div class="context-section">
            <span class="context-heading">Context</span>
            <ul class="context-list receive">
              {#each handoff.context.openDocuments as doc}
                <li class="context-item">
                  <span class="bullet" aria-hidden="true">
                    <svg viewBox="0 0 6 6" fill="currentColor">
                      <circle cx="3" cy="3" r="3" />
                    </svg>
                  </span>
                  <span class="context-file">{doc}</span>
                </li>
              {/each}

              {#each handoff.context.decisions as decision}
                <li class="context-item decision">
                  <span class="bullet" aria-hidden="true">
                    <svg viewBox="0 0 6 6" fill="currentColor">
                      <circle cx="3" cy="3" r="3" />
                    </svg>
                  </span>
                  Decision: {decision}
                </li>
              {/each}

              <li class="context-item">
                <span class="bullet" aria-hidden="true">
                  <svg viewBox="0 0 6 6" fill="currentColor">
                    <circle cx="3" cy="3" r="3" />
                  </svg>
                </span>
                {handoff.context.diagnosticCount} parser warning{handoff.context.diagnosticCount === 1 ? '' : 's'}
              </li>

              {#if handoff.context.currentStage}
                <li class="context-item">
                  <span class="bullet" aria-hidden="true">
                    <svg viewBox="0 0 6 6" fill="currentColor">
                      <circle cx="3" cy="3" r="3" />
                    </svg>
                  </span>
                  Stage: {handoff.context.currentStage}
                </li>
              {/if}
            </ul>
          </div>

          <span class="handoff-time">Received {formatTimestamp(handoff.createdAt)}</span>
        </div>

        <footer class="dialog-footer">
          <button type="button" class="dialog-btn reject" on:click={handleReject}>
            Reject
          </button>
          <button type="button" class="dialog-btn accept" on:click={handleAccept}>
            Accept & Load Context
          </button>
        </footer>
      {/if}
    </div>
  </div>
{/if}

<style>
  /* Backdrop */
  .backdrop {
    position: fixed;
    inset: 0;
    z-index: var(--z-modal-backdrop);
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(6px);
    -webkit-backdrop-filter: blur(6px);
    animation: backdropFadeIn var(--duration-normal) var(--ease-out);
  }

  /* Dialog */
  .dialog {
    position: relative;
    z-index: var(--z-modal);
    width: 100%;
    max-width: 520px;
    margin: var(--space-4);
    background: var(--color-bg-overlay);
    backdrop-filter: blur(20px);
    -webkit-backdrop-filter: blur(20px);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    box-shadow: var(--shadow-xl);
    overflow: hidden;
    animation: dialogScaleIn var(--duration-slow) var(--ease-out);
  }

  /* Header */
  .dialog-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-4) var(--space-5);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .dialog-title {
    margin: 0;
    font-size: var(--text-base);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
    line-height: var(--leading-tight);
  }

  .close-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border: none;
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: var(--transition-colors);
    flex-shrink: 0;
  }

  .close-btn:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .close-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .close-btn svg {
    width: 16px;
    height: 16px;
  }

  /* Body */
  .dialog-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
    padding: var(--space-4) var(--space-5);
  }

  /* Form fields */
  .field {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .field-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
  }

  .field-select,
  .field-textarea {
    font-family: inherit;
    font-size: var(--text-sm);
    color: var(--color-text-primary);
    background: var(--color-bg-input);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    padding: var(--space-2) var(--space-3);
    transition: var(--transition-colors);
  }

  .field-select:focus,
  .field-textarea:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .field-textarea {
    resize: vertical;
    min-height: 72px;
  }

  /* Context section */
  .context-section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .context-heading {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-secondary);
  }

  .context-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .context-item {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
  }

  .check-icon {
    display: flex;
    width: 16px;
    height: 16px;
    color: var(--color-success);
    flex-shrink: 0;
  }

  .check-icon svg {
    width: 100%;
    height: 100%;
  }

  .bullet {
    display: flex;
    width: 6px;
    height: 6px;
    color: var(--color-text-muted);
    flex-shrink: 0;
    margin-left: 5px;
    margin-right: 5px;
  }

  .bullet svg {
    width: 100%;
    height: 100%;
  }

  .context-file {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .context-item.decision {
    color: var(--color-primary);
  }

  /* Handoff summary (receive mode) */
  .handoff-summary {
    margin: 0;
    padding: var(--space-3) var(--space-4);
    background: var(--color-bg-surface);
    border-left: 3px solid var(--color-primary);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
    font-size: var(--text-sm);
    color: var(--color-text-primary);
    line-height: var(--leading-relaxed);
    font-style: italic;
  }

  .handoff-time {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  /* Footer */
  .dialog-footer {
    display: flex;
    align-items: center;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-5);
    border-top: 1px solid var(--color-border-subtle);
  }

  .dialog-btn {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-2) var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid transparent;
    font-size: var(--text-sm);
    font-family: inherit;
    font-weight: var(--font-semibold);
    cursor: pointer;
    transition: var(--transition-all);
  }

  .dialog-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .dialog-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .dialog-btn.cancel {
    color: var(--color-text-secondary);
    background: var(--color-bg-surface);
    border-color: var(--color-border-default);
  }

  .dialog-btn.cancel:hover {
    background: var(--color-bg-hover);
  }

  .dialog-btn.create,
  .dialog-btn.accept {
    color: #fff;
    background: linear-gradient(135deg, var(--color-brand-gradient-start), var(--color-brand-gradient-end));
    border-color: transparent;
  }

  .dialog-btn.create:hover:not(:disabled),
  .dialog-btn.accept:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: var(--shadow-glow-primary);
  }

  .dialog-btn.create:active:not(:disabled),
  .dialog-btn.accept:active:not(:disabled) {
    transform: translateY(0);
    box-shadow: none;
  }

  .dialog-btn.reject {
    color: var(--color-danger-text);
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
  }

  .dialog-btn.reject:hover {
    background: rgba(239, 68, 68, 0.18);
  }

  /* Animations */
  @keyframes backdropFadeIn {
    from { opacity: 0; }
    to { opacity: 1; }
  }

  @keyframes dialogScaleIn {
    from {
      opacity: 0;
      transform: scale(0.95);
    }
    to {
      opacity: 1;
      transform: scale(1);
    }
  }

  @media (prefers-reduced-motion: reduce) {
    .backdrop {
      animation: none;
    }

    .dialog {
      animation: none;
    }
  }
</style>
