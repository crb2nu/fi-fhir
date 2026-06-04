<script lang="ts">
  /**
   * CopilotPanel — inline LLM assistant for healthcare integration tasks.
   *
   * Docked in the IDE bottom panel. Supports Explain, Suggest, Generate,
   * and Review actions with streaming responses and context awareness.
   */
  import { afterUpdate } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import {
    copilotState,
    isAvailable,
    sendAction,
    cancelStream,
    clearMessages,
    type CopilotAction,
    type CopilotContext,
  } from './copilotStore';

  let selectedAction: CopilotAction = 'explain';
  let inputText = '';
  let messagesEl: HTMLDivElement | undefined;
  let textareaEl: HTMLTextAreaElement | undefined;

  // ── Derived ──
  $: streaming = $copilotState.isStreaming;
  $: messages = $copilotState.messages;
  $: context = $copilotState.context;
  $: canSend = inputText.trim().length > 0 && !streaming;
  $: available = $isAvailable;

  // ── Action definitions ──
  const actions: { key: CopilotAction; label: string; colorClass: string }[] = [
    { key: 'explain', label: 'Explain', colorClass: 'action-info' },
    { key: 'suggest', label: 'Suggest', colorClass: 'action-success' },
    { key: 'generate', label: 'Generate', colorClass: 'action-warning' },
    { key: 'review', label: 'Review', colorClass: 'action-primary' },
  ];

  const placeholders: Record<CopilotAction, string> = {
    explain: 'Paste an HL7 segment, EDI loop, or FHIR resource...',
    suggest: 'Enter source field and target FHIR element...',
    generate: 'Describe the filter condition in plain English...',
    review: 'Paste the mapping decision to review...',
  };

  const actionBadgeVariant: Record<
    CopilotAction,
    'info' | 'success' | 'warning' | 'primary'
  > = {
    explain: 'info',
    suggest: 'success',
    generate: 'warning',
    review: 'primary',
  };

  // ── Context chips ──
  $: contextChips = buildContextChips(context);

  function buildContextChips(ctx: CopilotContext): { label: string }[] {
    const chips: { label: string }[] = [];
    if (ctx.stage) chips.push({ label: ctx.stage });
    if (ctx.documentType) chips.push({ label: ctx.documentType });
    if (ctx.selection) chips.push({ label: 'Selection active' });
    if (ctx.artifactId) chips.push({ label: ctx.artifactId });
    return chips;
  }

  // ── Auto-scroll on messages change ──
  afterUpdate(() => {
    if (messagesEl) {
      messagesEl.scrollTop = messagesEl.scrollHeight;
    }
  });

  // ── Textarea auto-resize ──
  $: if (textareaEl) {
    resizeTextarea(inputText);
  }

  // eslint-disable-next-line @typescript-eslint/no-unused-vars
  function resizeTextarea(_value: string): void {
    if (!textareaEl) return;
    textareaEl.style.height = 'auto';
    const scrollH = textareaEl.scrollHeight;
    const maxH = 4 * 24; // ~4 lines
    textareaEl.style.height = `${Math.min(scrollH, maxH)}px`;
  }

  // ── Handlers ──
  function handleSend(): void {
    if (!canSend) return;
    const text = inputText.trim();
    inputText = '';
    sendAction(selectedAction, text);
  }

  function handleKeydown(e: KeyboardEvent): void {
    if (e.key === 'Enter' && !e.shiftKey) {
      e.preventDefault();
      handleSend();
    }
  }

  function handleCancel(): void {
    cancelStream();
  }

  function handleClear(): void {
    clearMessages();
    inputText = '';
  }

  function selectAction(key: CopilotAction): void {
    selectedAction = key;
    textareaEl?.focus();
  }

  // ── Simple markdown-ish formatting ──
  function formatContent(raw: string): string {
    let html = escapeHtml(raw);
    // Code blocks (```)
    html = html.replace(/```(\w*)\n([\s\S]*?)```/g, (_m, lang, code) => {
      const langLabel = lang ? ` data-lang="${lang}"` : '';
      return `<pre class="code-block"${langLabel}><code>${code.trim()}</code></pre>`;
    });
    // Inline code
    html = html.replace(/`([^`]+)`/g, '<code class="inline-code">$1</code>');
    // Bold
    html = html.replace(/\*\*(.+?)\*\*/g, '<strong>$1</strong>');
    // Tables
    html = html.replace(
      /^(\|.+\|)\n(\|[-| :]+\|)\n((?:\|.+\|\n?)+)/gm,
      (_m, header: string, _sep: string, body: string) => {
        const ths = header
          .split('|')
          .filter((c: string) => c.trim())
          .map((c: string) => `<th>${c.trim()}</th>`)
          .join('');
        const rows = body
          .trim()
          .split('\n')
          .map((row: string) => {
            const tds = row
              .split('|')
              .filter((c: string) => c.trim())
              .map((c: string) => `<td>${c.trim()}</td>`)
              .join('');
            return `<tr>${tds}</tr>`;
          })
          .join('');
        return `<table class="response-table"><thead><tr>${ths}</tr></thead><tbody>${rows}</tbody></table>`;
      }
    );
    // Bullet lists (lines starting with - )
    html = html.replace(/^- (.+)$/gm, '<li>$1</li>');
    html = html.replace(/(<li>[\s\S]*?<\/li>)/g, '<ul>$1</ul>');
    // Collapse adjacent <ul>s
    html = html.replace(/<\/ul>\s*<ul>/g, '');
    // Checkbox lists
    html = html.replace(
      /<li>\[ \] (.+?)<\/li>/g,
      '<li class="checklist"><span class="checkbox">&#9744;</span> $1</li>'
    );
    html = html.replace(
      /<li>\[x\] (.+?)<\/li>/g,
      '<li class="checklist"><span class="checkbox checked">&#9745;</span> $1</li>'
    );
    // Paragraphs (double newline)
    html = html.replace(/\n\n/g, '</p><p>');
    html = `<p>${html}</p>`;
    html = html.replace(/<p>\s*<\/p>/g, '');
    // Single newlines inside paragraphs
    html = html.replace(/([^>])\n([^<])/g, '$1<br>$2');
    return html;
  }

  function escapeHtml(text: string): string {
    return text
      .replace(/&/g, '&amp;')
      .replace(/</g, '&lt;')
      .replace(/>/g, '&gt;')
      .replace(/"/g, '&quot;');
  }

  function formatTime(ts: number): string {
    return new Date(ts).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  }

  function capitalize(value: string): string {
    return value.length === 0 ? value : value.charAt(0).toUpperCase() + value.slice(1);
  }
</script>

<div class="copilot-panel" class:disconnected={!available}>
  <!-- Header bar -->
  <div class="copilot-header">
    <div class="context-chips">
      {#each contextChips as chip, i (chip.label)}
        <span
          class="context-chip"
          style="--chip-delay: {i * 50}ms"
        >
          <span class="chip-label">{chip.label}</span>
        </span>
      {/each}
      {#if contextChips.length === 0}
        <span class="context-chip placeholder-chip">
          <span class="chip-label">No context</span>
        </span>
      {/if}
    </div>
    <div class="header-actions">
      <button
        type="button"
        class="header-btn"
        title="Clear conversation"
        disabled={streaming}
        on:click={handleClear}
      >
        Clear
      </button>
    </div>
  </div>

  <!-- Message area -->
  <div class="messages-area" bind:this={messagesEl}>
    {#each messages as msg (msg.id)}
      {#if msg.role === 'system'}
        <div class="msg msg-system">
          <span class="msg-system-text">{msg.content}</span>
        </div>
      {:else if msg.role === 'user'}
        <div class="msg msg-user">
          <div class="msg-meta-user">
            {#if msg.action}
              <Badge
                variant={actionBadgeVariant[msg.action]}
                size="sm"
                pill
              >
                {capitalize(msg.action)}
              </Badge>
            {/if}
            <span class="msg-time">{formatTime(msg.timestamp)}</span>
          </div>
          <div class="msg-bubble msg-bubble-user">
            {msg.content}
          </div>
        </div>
      {:else}
        <div class="msg msg-assistant">
          <div class="msg-bubble msg-bubble-assistant">
            {#if msg.content}
              <!-- eslint-disable-next-line svelte/no-at-html-tags -- markdown-like formatting from trusted LLM responses -->
              {@html formatContent(msg.content)}
            {/if}
            {#if msg.streaming}
              <span class="streaming-cursor">█</span>
            {/if}
          </div>
          <span class="msg-time">{formatTime(msg.timestamp)}</span>
        </div>
      {/if}
    {/each}

    {#if $copilotState.error}
      <div class="msg msg-system msg-error">
        <span class="msg-error-text">{$copilotState.error}</span>
      </div>
    {/if}
  </div>

  <!-- Action bar + input -->
  <div class="copilot-input-area">
    <div class="action-bar">
      {#each actions as action (action.key)}
        <button
          type="button"
          class="action-btn {action.colorClass}"
          class:active={selectedAction === action.key}
          disabled={streaming}
          on:click={() => selectAction(action.key)}
          title={action.label}
        >
          <span class="action-label">{action.label}</span>
        </button>
      {/each}
    </div>

    <div class="input-row">
      <textarea
        bind:this={textareaEl}
        bind:value={inputText}
        class="copilot-textarea"
        placeholder={placeholders[selectedAction]}
        rows="1"
        disabled={!available}
        on:keydown={handleKeydown}
      ></textarea>

      {#if streaming}
        <button
          type="button"
          class="send-btn cancel-btn"
          title="Cancel"
          on:click={handleCancel}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <rect x="6" y="6" width="12" height="12" rx="2" />
          </svg>
        </button>
      {:else}
        <button
          type="button"
          class="send-btn"
          title="Send (Enter)"
          disabled={!canSend}
          on:click={handleSend}
        >
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
            <path d="M22 2L11 13" />
            <path d="M22 2L15 22L11 13L2 9L22 2Z" />
          </svg>
        </button>
      {/if}
    </div>
  </div>

  <!-- Disconnected overlay -->
  {#if !available}
    <div class="disconnected-overlay">
      <div class="disconnected-card">
        <div class="disconnected-icon">
          <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.5" aria-hidden="true">
            <path d="M8.111 16.404a5.5 5.5 0 017.778 0M12 20h.01M1 1l22 22" />
            <path d="M2.05 12.05a12 12 0 011.06-1.49M5.636 8.364A9.97 9.97 0 0112 6c2.21 0 4.255.716 5.916 1.928" />
          </svg>
        </div>
        <h3 class="disconnected-title">Platform connection required</h3>
        <p class="disconnected-text">
          Connect to the platform to use the Copilot assistant for healthcare integration tasks.
        </p>
      </div>
    </div>
  {/if}
</div>

<style>
  /* ======================================================================
   * LAYOUT
   * ====================================================================== */

  .copilot-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    position: relative;
    overflow: hidden;
    font-family: var(--font-sans);
  }

  /* ── Header ── */
  .copilot-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: var(--space-1) var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
    min-height: 28px;
    flex-shrink: 0;
  }

  .context-chips {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-wrap: wrap;
  }

  .context-chip {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 1px var(--space-2);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-full);
    font-size: var(--text-2xs);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
    background: var(--color-bg-elevated);
    animation: fadeIn var(--duration-normal) var(--ease-out) both;
    animation-delay: var(--chip-delay, 0ms);
  }

  .placeholder-chip {
    opacity: 0.5;
  }

  .chip-label {
    line-height: 1;
  }

  .header-actions {
    display: flex;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  .header-btn {
    padding: 2px var(--space-2);
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .header-btn:hover:not(:disabled) {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  .header-btn:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  /* ── Messages ── */
  .messages-area {
    flex: 1;
    overflow-y: auto;
    overflow-x: hidden;
    padding: var(--space-2) var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-height: 0;
  }

  .msg {
    max-width: 100%;
    animation: slideInUp var(--duration-normal) var(--ease-out);
  }

  /* System message */
  .msg-system {
    text-align: center;
    padding: var(--space-1) 0;
  }

  .msg-system-text {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    font-style: italic;
  }

  .msg-error {
    text-align: center;
  }

  .msg-error-text {
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  /* User message */
  .msg-user {
    display: flex;
    flex-direction: column;
    align-items: flex-end;
    gap: 2px;
  }

  .msg-meta-user {
    display: flex;
    align-items: center;
    gap: var(--space-1);
  }

  .msg-bubble {
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-lg);
    font-size: var(--text-sm);
    line-height: var(--leading-normal);
    max-width: 85%;
    word-break: break-word;
  }

  .msg-bubble-user {
    background: rgba(99, 102, 241, 0.08);
    border: 1px solid rgba(99, 102, 241, 0.2);
    color: var(--color-text-primary);
    white-space: pre-wrap;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .msg-bubble-assistant {
    background: var(--color-bg-elevated);
    border: none;
    border-left: 3px solid var(--color-primary);
    color: var(--color-text-secondary);
  }

  /* Formatted content inside assistant bubbles */
  .msg-bubble-assistant :global(p) {
    margin: 0 0 var(--space-2);
  }

  .msg-bubble-assistant :global(p:last-child) {
    margin-bottom: 0;
  }

  .msg-bubble-assistant :global(strong) {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .msg-bubble-assistant :global(.code-block) {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    margin: var(--space-2) 0;
    overflow-x: auto;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    line-height: var(--leading-relaxed);
    color: var(--color-text-primary);
  }

  .msg-bubble-assistant :global(.inline-code) {
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    padding: 1px 4px;
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    color: var(--color-primary);
  }

  .msg-bubble-assistant :global(ul) {
    margin: var(--space-2) 0;
    padding-left: var(--space-4);
    list-style: none;
  }

  .msg-bubble-assistant :global(li) {
    position: relative;
    padding-left: var(--space-2);
    margin-bottom: var(--space-1);
  }

  .msg-bubble-assistant :global(li::before) {
    content: '\2022';
    position: absolute;
    left: calc(-1 * var(--space-2));
    color: var(--color-primary);
    font-weight: var(--font-bold);
  }

  .msg-bubble-assistant :global(li.checklist) {
    list-style: none;
    padding-left: 0;
  }

  .msg-bubble-assistant :global(li.checklist::before) {
    content: none;
  }

  .msg-bubble-assistant :global(.checkbox) {
    margin-right: var(--space-1);
    color: var(--color-text-muted);
  }

  .msg-bubble-assistant :global(.checkbox.checked) {
    color: var(--color-success);
  }

  .msg-bubble-assistant :global(.response-table) {
    width: 100%;
    border-collapse: collapse;
    margin: var(--space-2) 0;
    font-size: var(--text-xs);
  }

  .msg-bubble-assistant :global(.response-table th) {
    text-align: left;
    padding: var(--space-1) var(--space-2);
    border-bottom: 2px solid var(--color-border-default);
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
    white-space: nowrap;
  }

  .msg-bubble-assistant :global(.response-table td) {
    padding: var(--space-1) var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
  }

  .msg-time {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  /* Streaming cursor */
  .streaming-cursor {
    display: inline;
    color: var(--color-primary);
    font-weight: var(--font-bold);
  }

  /* Assistant message (left-aligned) */
  .msg-assistant {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 2px;
  }

  /* ── Action bar + Input ── */
  .copilot-input-area {
    flex-shrink: 0;
    border-top: 1px solid var(--color-border-subtle);
    padding: var(--space-2) var(--space-3);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
  }

  .action-bar {
    display: flex;
    gap: var(--space-1);
  }

  .action-btn {
    display: inline-flex;
    align-items: center;
    gap: 3px;
    padding: 3px var(--space-2);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    cursor: pointer;
    transition:
      color var(--duration-fast) var(--ease-out),
      background-color var(--duration-fast) var(--ease-out),
      border-color var(--duration-fast) var(--ease-out),
      transform var(--duration-normal) var(--ease-out),
      box-shadow var(--duration-normal) var(--ease-out);
    line-height: 1;
  }

  .action-btn:hover:not(:disabled) {
    transform: translateY(-1px);
    color: var(--color-text-primary);
  }

  .action-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  /* Action accent colors */
  .action-btn.action-info:hover:not(:disabled),
  .action-btn.action-info.active {
    color: var(--color-info);
    border-color: var(--color-info-border);
    background: var(--color-info-bg);
    box-shadow: 0 0 12px rgba(14, 165, 233, 0.15);
  }

  .action-btn.action-success:hover:not(:disabled),
  .action-btn.action-success.active {
    color: var(--color-success);
    border-color: var(--color-success-border);
    background: var(--color-success-bg);
    box-shadow: 0 0 12px rgba(16, 185, 129, 0.15);
  }

  .action-btn.action-warning:hover:not(:disabled),
  .action-btn.action-warning.active {
    color: var(--color-warning);
    border-color: var(--color-warning-border);
    background: var(--color-warning-bg);
    box-shadow: 0 0 12px rgba(245, 158, 11, 0.15);
  }

  .action-btn.action-primary:hover:not(:disabled),
  .action-btn.action-primary.active {
    color: var(--color-primary);
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
    box-shadow: 0 0 12px rgba(99, 102, 241, 0.15);
  }

  /* Input row */
  .input-row {
    display: flex;
    align-items: flex-end;
    gap: var(--space-2);
  }

  .copilot-textarea {
    flex: 1;
    resize: none;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: var(--font-sans);
    font-size: var(--text-sm);
    line-height: var(--leading-normal);
    transition: var(--transition-all);
    min-height: 36px;
    max-height: 96px;
    overflow-y: auto;
  }

  .copilot-textarea::placeholder {
    color: var(--color-text-muted);
  }

  .copilot-textarea:focus {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .copilot-textarea:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .send-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 36px;
    height: 36px;
    padding: 0;
    border: 1px solid var(--color-primary-border);
    border-radius: var(--radius-lg);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    cursor: pointer;
    transition: var(--transition-all);
    flex-shrink: 0;
  }

  .send-btn svg {
    width: 16px;
    height: 16px;
  }

  .send-btn:hover:not(:disabled) {
    background: var(--color-primary);
    color: var(--color-text-inverse);
    transform: translateY(-1px);
    box-shadow: var(--shadow-md);
  }

  .send-btn:disabled {
    opacity: 0.35;
    cursor: not-allowed;
  }

  .cancel-btn {
    border-color: var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .cancel-btn:hover:not(:disabled) {
    background: var(--color-danger);
    color: var(--color-text-inverse);
    box-shadow: 0 0 12px rgba(239, 68, 68, 0.3);
  }

  /* ======================================================================
   * DISCONNECTED OVERLAY
   * ====================================================================== */

  .disconnected-overlay {
    position: absolute;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(0, 0, 0, 0.3);
    backdrop-filter: blur(8px);
    -webkit-backdrop-filter: blur(8px);
    z-index: 10;
    animation: fadeIn var(--duration-slow) var(--ease-out);
  }

  .disconnected-card {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-6);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-xl);
    background: var(--color-bg-overlay);
    backdrop-filter: blur(12px);
    -webkit-backdrop-filter: blur(12px);
    box-shadow: var(--shadow-lg);
    text-align: center;
    max-width: 320px;
  }

  .disconnected-icon {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 48px;
    height: 48px;
    border-radius: var(--radius-full);
    background: var(--color-warning-bg);
    color: var(--color-warning);
  }

  .disconnected-icon svg {
    width: 24px;
    height: 24px;
  }

  .disconnected-title {
    margin: 0;
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .disconnected-text {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
  }

  /* ======================================================================
   * REDUCED MOTION
   * ====================================================================== */

  @media (prefers-reduced-motion: reduce) {
    .context-chip,
    .msg,
    .disconnected-overlay {
      animation: none !important;
    }

    .streaming-cursor {
      animation: none;
      opacity: 1;
    }

    .action-btn:hover:not(:disabled) {
      transform: none;
    }

    .send-btn:hover:not(:disabled) {
      transform: none;
    }
  }
</style>
