<script lang="ts">
  /**
   * CopilotPanel — inline LLM assistant for healthcare integration tasks.
   *
   * Docked in the IDE bottom panel. Supports Explain, Suggest, Generate,
   * and Review actions with streaming responses and context awareness.
   *
   * Runs on the API's own LLM: availability comes from `llmCapability`,
   * probed each time the panel opens. The optional loom platform connection
   * plays no part (`.loom/36` R-B).
   */
  import { afterUpdate, onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Eraser from '@lucide/svelte/icons/eraser';
  import Info from '@lucide/svelte/icons/info';
  import Send from '@lucide/svelte/icons/send';
  import Square from '@lucide/svelte/icons/square';
  import { Badge, Button, Icon, IconButton, Textarea } from '$lib/ui/primitives';
  import {
    copilotState,
    sendAction,
    cancelStream,
    clearMessages,
    type CopilotAction,
    type CopilotContext,
  } from './copilotStore';
  import {
    llmCapabilityState,
    llmCapabilityChecking,
    refreshLlmCapability,
    actionBlockReason,
    copilotLlmState,
    type CopilotLlmState,
  } from './llmCapabilityStore';

  let selectedAction: CopilotAction = 'explain';
  let inputText = '';
  let messagesEl: HTMLDivElement | undefined;
  let inputRowEl: HTMLDivElement | undefined;
  let textareaEl: HTMLTextAreaElement | undefined;

  // ── Derived ──
  $: streaming = $copilotState.isStreaming;
  $: messages = $copilotState.messages;
  $: context = $copilotState.context;

  // ── LLM capability (honest availability state) ──
  onMount(() => {
    textareaEl = inputRowEl?.querySelector('textarea') ?? undefined;
    void refreshLlmCapability();
  });

  $: llmState = copilotLlmState($llmCapabilityState, $llmCapabilityChecking);
  $: llmWarnings = $llmCapabilityState.capability?.warnings ?? [];
  $: llmModel = $llmCapabilityState.capability?.defaultModel ?? null;
  $: degraded = llmState === 'ready' && $llmCapabilityState.status === 'degraded';
  $: inputLocked = llmState === 'not-configured' || llmState === 'unreachable';
  $: blockReason = actionBlockReason($llmCapabilityState, selectedAction);
  $: canSend = inputText.trim().length > 0 && !streaming && !blockReason && !inputLocked;

  const llmStateTitles: Record<CopilotLlmState, string> = {
    ready: 'Backend LLM ready',
    unreachable: "The deployment's LLM is not responding",
    'not-configured': 'No LLM is configured for this deployment',
    checking: "Checking the deployment's LLM…",
    unknown: 'LLM status unknown',
  };

  // ── Action definitions ──
  const actions: { key: CopilotAction; label: string }[] = [
    { key: 'explain', label: 'Explain' },
    { key: 'suggest', label: 'Suggest' },
    { key: 'generate', label: 'Generate' },
    { key: 'review', label: 'Review' },
  ];

  const placeholders: Record<CopilotAction, string> = {
    explain: 'Paste an HL7 segment, EDI loop, or FHIR resource',
    suggest: 'Enter a source field and the target FHIR element',
    generate: 'Describe the filter condition in plain English',
    review: 'Paste the mapping decision to review',
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
    const maxH = 4 * 20; // ~4 lines
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

<div class="copilot-panel">
  <!-- Context and panel actions -->
  <div class="copilot-header">
    <div class="context-chips" aria-label="Copilot context">
      {#each contextChips as chip (chip.label)}
        <Badge>{chip.label}</Badge>
      {:else}
        <span class="no-context">No context</span>
      {/each}
    </div>
    <div class="header-actions">
      {#if degraded}
        <Badge tone="warning" dot role="status" title={llmWarnings.join('; ') || 'LLM degraded'}>
          LLM degraded
        </Badge>
      {/if}
      <IconButton
        icon={Eraser}
        label="Clear conversation"
        disabled={streaming}
        onclick={handleClear}
      />
    </div>
  </div>

  <!-- The backend LLM's honest state (source of truth: llmCapability) -->
  <div
    class="llm-state llm-state-{llmState}"
    data-testid="copilot-llm-state"
    data-state={llmState}
    role={llmState === 'ready' ? undefined : 'status'}
  >
    <Icon
      icon={llmState === 'unreachable' ? CircleAlert : Info}
      size={14}
      class="llm-state-icon"
    />
    <div class="llm-state-copy">
      <span class="llm-state-title">
        {llmStateTitles[llmState]}{#if llmState === 'ready' && llmModel}<span class="llm-model">&nbsp;· {llmModel}</span>{/if}
      </span>
      {#if llmState === 'unknown'}
        <span class="llm-state-detail">
          The capability check did not answer; actions still run and report their own errors.
        </span>
      {/if}
      {#if inputLocked && llmWarnings.length > 0}
        <ul class="llm-state-warnings">
          {#each llmWarnings as warning (warning)}
            <li>{warning}</li>
          {/each}
        </ul>
      {/if}
    </div>
  </div>

  <!-- Message area -->
  <div class="messages-area" bind:this={messagesEl}>
    {#each messages as msg (msg.id)}
      {#if msg.role === 'system'}
        <div class="msg msg-system">{msg.content}</div>
      {:else if msg.role === 'user'}
        <div class="msg msg-user">
          <div class="msg-meta">
            {#if msg.action}
              <Badge>{capitalize(msg.action)}</Badge>
            {/if}
            <span class="msg-time">{formatTime(msg.timestamp)}</span>
          </div>
          <div class="msg-body msg-body-user">{msg.content}</div>
        </div>
      {:else}
        <div class="msg msg-assistant">
          <div class="msg-body msg-body-assistant">
            {#if msg.content}
              <!-- eslint-disable-next-line svelte/no-at-html-tags -- markdown-like formatting from trusted LLM responses -->
              {@html formatContent(msg.content)}
            {/if}
            {#if msg.streaming}
              <span class="streaming-cursor" aria-hidden="true">▍</span>
            {/if}
          </div>
          <div class="msg-meta">
            {#if msg.model}
              <span class="msg-model" title="Model that produced this response">{msg.model}</span>
            {/if}
            <span class="msg-time">{formatTime(msg.timestamp)}</span>
          </div>
        </div>
      {/if}
    {/each}

    {#if $copilotState.error}
      <div class="msg msg-error" role="alert">{$copilotState.error}</div>
    {/if}
  </div>

  <!-- Action + input: one row (wraps on narrow panels) -->
  <div class="copilot-input-area">
    <div class="action-bar" role="group" aria-label="Copilot action">
      {#each actions as action (action.key)}
        <Button
          variant="ghost"
          aria-pressed={selectedAction === action.key ? 'true' : 'false'}
          disabled={streaming}
          onclick={() => selectAction(action.key)}
        >
          {action.label}
        </Button>
      {/each}
    </div>

    <div class="input-row" bind:this={inputRowEl}>
      <Textarea
        bind:value={inputText}
        class="copilot-textarea"
        placeholder={placeholders[selectedAction]}
        aria-label="Copilot input"
        rows={1}
        disabled={inputLocked}
        onkeydown={handleKeydown}
      />

      {#if streaming}
        <IconButton icon={Square} label="Cancel" variant="danger" onclick={handleCancel} />
      {:else}
        <IconButton
          icon={Send}
          label={blockReason ?? 'Send (Enter)'}
          variant="primary"
          disabled={!canSend}
          onclick={handleSend}
        />
      {/if}
    </div>

    {#if blockReason && !inputLocked}
      <p class="llm-block-note" role="status">{blockReason}</p>
    {/if}
  </div>
</div>

<style>
  .copilot-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    font-size: var(--text-ui);
  }

  /* ── Header ── */
  .copilot-header {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    flex-shrink: 0;
    height: 28px;
    padding: 0 var(--space-1) 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .context-chips {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
    overflow: hidden;
  }

  .no-context {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  .header-actions {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-shrink: 0;
  }

  /* ── LLM state line ── */
  .llm-state {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    flex-shrink: 0;
    padding: 6px var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .llm-state :global(.llm-state-icon) {
    margin-top: 1px;
  }

  .llm-state-copy {
    display: flex;
    flex-wrap: wrap;
    align-items: baseline;
    column-gap: var(--space-3);
    row-gap: 2px;
    min-width: 0;
  }

  .llm-state-title {
    font-weight: var(--font-medium);
  }

  .llm-model {
    font-family: var(--font-mono);
    font-weight: var(--font-normal);
  }

  .llm-state-detail {
    color: var(--color-text-secondary);
  }

  .llm-state-not-configured,
  .llm-state-unreachable {
    color: var(--color-text-secondary);
  }

  .llm-state-not-configured .llm-state-title,
  .llm-state-unreachable .llm-state-title {
    color: var(--color-text-primary);
    font-size: var(--text-ui);
  }

  .llm-state-unreachable :global(.llm-state-icon) {
    color: var(--color-warning-text);
  }

  .llm-state-warnings {
    display: flex;
    flex-wrap: wrap;
    gap: 2px var(--space-3);
    margin: 0;
    padding: 0;
    list-style: none;
    font-family: var(--font-mono);
    font-size: var(--text-label);
  }

  /* ── Messages ── */
  .messages-area {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    overflow-x: hidden;
    overflow-y: auto;
    padding: var(--space-3);
  }

  .msg {
    max-width: 100%;
  }

  .msg-system {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    text-align: center;
  }

  .msg-error {
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .msg-user,
  .msg-assistant {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .msg-user {
    align-items: flex-end;
  }

  .msg-assistant {
    align-items: flex-start;
  }

  .msg-meta {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .msg-body {
    max-width: 85%;
    padding: var(--space-2) var(--space-3);
    border-radius: var(--radius-sm);
    line-height: var(--leading-ui);
    word-break: break-word;
  }

  .msg-body-user {
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    white-space: pre-wrap;
  }

  .msg-body-assistant {
    border-left: 2px solid var(--color-border-strong);
    border-radius: 0;
    color: var(--color-text-secondary);
  }

  .msg-body-assistant :global(p) {
    margin: 0 0 var(--space-2);
  }

  .msg-body-assistant :global(p:last-child) {
    margin-bottom: 0;
  }

  .msg-body-assistant :global(strong) {
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .msg-body-assistant :global(.code-block) {
    margin: var(--space-2) 0;
    padding: var(--space-2) var(--space-3);
    overflow-x: auto;
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: var(--leading-relaxed);
  }

  .msg-body-assistant :global(.inline-code) {
    padding: 0 4px;
    border-radius: var(--radius-sm);
    background: var(--color-bg-active);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .msg-body-assistant :global(ul) {
    margin: var(--space-2) 0;
    padding-left: var(--space-4);
  }

  .msg-body-assistant :global(li) {
    margin-bottom: var(--space-1);
  }

  .msg-body-assistant :global(li.checklist) {
    list-style: none;
    margin-left: calc(-1 * var(--space-4));
  }

  .msg-body-assistant :global(.checkbox) {
    margin-right: var(--space-1);
    color: var(--color-text-muted);
  }

  .msg-body-assistant :global(.checkbox.checked) {
    color: var(--color-success-text);
  }

  .msg-body-assistant :global(.response-table) {
    width: 100%;
    margin: var(--space-2) 0;
    border-collapse: collapse;
    font-size: var(--text-xs);
  }

  .msg-body-assistant :global(.response-table th) {
    padding: var(--space-1) var(--space-2);
    border-bottom: 1px solid var(--color-border-default);
    color: var(--color-text-tertiary);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-align: left;
    text-transform: uppercase;
    white-space: nowrap;
  }

  .msg-body-assistant :global(.response-table td) {
    padding: var(--space-1) var(--space-2);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .msg-time {
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-label);
  }

  .msg-model {
    color: var(--color-text-muted);
    font-family: var(--font-mono);
    font-size: var(--text-label);
  }

  .streaming-cursor {
    color: var(--color-text-tertiary);
  }

  /* ── Action + input row ── */
  .copilot-input-area {
    display: flex;
    flex-wrap: wrap;
    align-items: flex-end;
    column-gap: var(--space-3);
    row-gap: var(--space-1);
    flex-shrink: 0;
    padding: var(--space-2) var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
  }

  .action-bar {
    display: flex;
    align-items: center;
    gap: 2px;
    flex: 0 0 auto;
  }

  .input-row {
    display: flex;
    align-items: flex-end;
    gap: var(--space-2);
    flex: 1 1 320px;
    min-width: 0;
  }

  .input-row :global(.copilot-textarea) {
    flex: 1;
    min-height: var(--size-control-sm);
    height: var(--size-control-sm);
    max-height: 80px;
    padding-top: 5px;
    resize: none;
  }

  .llm-block-note {
    flex: 1 0 100%;
    margin: 0;
    overflow: hidden;
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    text-overflow: ellipsis;
    white-space: nowrap;
  }
</style>
