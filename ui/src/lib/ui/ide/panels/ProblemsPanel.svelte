<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import { SvelteSet } from 'svelte/reactivity';
  import {
    diagnostics,
    diagnosticCounts,
    clearAll,
    type Diagnostic,
    type DiagnosticSeverity,
    type JourneyStage,
  } from './diagnosticsStore';

  const dispatch = createEventDispatcher<{
    navigate: { panel: string; target?: Diagnostic['target'] };
  }>();

  // ── Filter state ──────────────────────────────────────────────────────

  let severityFilters = new SvelteSet<DiagnosticSeverity>(['error', 'warning', 'info']);
  let stageFilters = new SvelteSet<JourneyStage>(
    ['intake', 'normalization', 'translation', 'delivery', 'verification'],
  );

  // ── Section collapse state ────────────────────────────────────────────

  let collapsedSections = new SvelteSet<DiagnosticSeverity>();

  // ── Detail expansion state ────────────────────────────────────────────

  let expandedIds = new SvelteSet<string>();

  // ── Derived data ──────────────────────────────────────────────────────

  const SEVERITY_ORDER: DiagnosticSeverity[] = ['error', 'warning', 'info'];

  const ALL_STAGES: JourneyStage[] = [
    'intake',
    'normalization',
    'translation',
    'delivery',
    'verification',
  ];

  const STAGE_LABELS: Record<JourneyStage, string> = {
    intake: 'Intake',
    normalization: 'Normalization',
    translation: 'Translation',
    delivery: 'Delivery',
    verification: 'Verification',
  };

  const SEVERITY_LABELS: Record<DiagnosticSeverity, string> = {
    error: 'Error',
    warning: 'Warning',
    info: 'Info',
  };

  let filtered = $derived(
    $diagnostics.filter(
      (d) => severityFilters.has(d.severity) && stageFilters.has(d.stage),
    ),
  );

  let groupedFiltered = $derived(groupBySeverity(filtered));

  function groupBySeverity(
    items: Diagnostic[],
  ): { severity: DiagnosticSeverity; items: Diagnostic[] }[] {
    const groups: { severity: DiagnosticSeverity; items: Diagnostic[] }[] = [];
    for (const sev of SEVERITY_ORDER) {
      const matching = items
        .filter((d) => d.severity === sev)
        .sort((a, b) => b.timestamp - a.timestamp);
      if (matching.length > 0) {
        groups.push({ severity: sev, items: matching });
      }
    }
    return groups;
  }

  // ── Helpers ───────────────────────────────────────────────────────────

  function toggleSeverityFilter(sev: DiagnosticSeverity): void {
    if (severityFilters.has(sev)) {
      severityFilters.delete(sev);
    } else {
      severityFilters.add(sev);
    }
  }

  function toggleStageFilter(stage: JourneyStage): void {
    if (stageFilters.has(stage)) {
      stageFilters.delete(stage);
    } else {
      stageFilters.add(stage);
    }
  }

  function toggleSection(sev: DiagnosticSeverity): void {
    if (collapsedSections.has(sev)) {
      collapsedSections.delete(sev);
    } else {
      collapsedSections.add(sev);
    }
  }

  function toggleDetail(id: string): void {
    if (expandedIds.has(id)) {
      expandedIds.delete(id);
    } else {
      expandedIds.add(id);
    }
  }

  function handleRowClick(d: Diagnostic): void {
    toggleDetail(d.id);
  }

  function handleRowNavigate(d: Diagnostic): void {
    dispatch('navigate', { panel: 'problems', target: d.target });
  }

  function relativeTime(ts: number): string {
    const delta = Math.max(0, Math.floor((Date.now() - ts) / 1000));
    if (delta < 5) return 'just now';
    if (delta < 60) return `${delta}s ago`;
    const minutes = Math.floor(delta / 60);
    if (minutes < 60) return `${minutes}m ago`;
    const hours = Math.floor(minutes / 60);
    if (hours < 24) return `${hours}h ago`;
    return `${Math.floor(hours / 24)}d ago`;
  }

  function handleClearAll(): void {
    clearAll();
  }
</script>

<div class="problems-panel">
  <!-- Toolbar -->
  <div class="toolbar">
    <div class="filter-row">
      <div class="filter-group">
        <span class="filter-label">Severity</span>
        {#each SEVERITY_ORDER as sev (sev)}
          <button
            type="button"
            class="filter-chip severity-{sev}"
            class:active={severityFilters.has(sev)}
            onclick={() => toggleSeverityFilter(sev)}
          >
            {SEVERITY_LABELS[sev]}
            {#if $diagnosticCounts[sev] > 0}
              <span class="chip-count">{$diagnosticCounts[sev]}</span>
            {/if}
          </button>
        {/each}
      </div>

      <div class="filter-group">
        <span class="filter-label">Stage</span>
        {#each ALL_STAGES as stage (stage)}
          <button
            type="button"
            class="filter-chip stage-{stage}"
            class:active={stageFilters.has(stage)}
            onclick={() => toggleStageFilter(stage)}
          >
            {STAGE_LABELS[stage]}
          </button>
        {/each}
      </div>
    </div>

    <button
      type="button"
      class="clear-btn"
      onclick={handleClearAll}
      disabled={$diagnosticCounts.total === 0}
    >
      Clear All
    </button>
  </div>

  <!-- Content area -->
  {#if filtered.length === 0}
    <div class="empty-state">
      <div class="empty-icon">
        <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
          <path d="M9 12l2 2 4-4" />
          <circle cx="12" cy="12" r="10" />
        </svg>
      </div>
      <div class="empty-title">No problems detected</div>
      <div class="empty-body">
        {#if $diagnosticCounts.total === 0}
          All stages are clear. The integration pipeline has no reported issues.
        {:else}
          No diagnostics match the current filters. Adjust severity or stage filters above.
        {/if}
      </div>
    </div>
  {:else}
    <div class="diagnostic-list" role="list" aria-label="Cross-stage diagnostics">
      {#each groupedFiltered as group (group.severity)}
        <div class="severity-section">
          <button
            type="button"
            class="section-header severity-{group.severity}"
            onclick={() => toggleSection(group.severity)}
            aria-expanded={!collapsedSections.has(group.severity)}
          >
            <span class="chevron" class:collapsed={collapsedSections.has(group.severity)}>
              <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                <path
                  fill-rule="evenodd"
                  d="M5.293 7.293a1 1 0 011.414 0L10 10.586l3.293-3.293a1 1 0 111.414 1.414l-4 4a1 1 0 01-1.414 0l-4-4a1 1 0 010-1.414z"
                  clip-rule="evenodd"
                />
              </svg>
            </span>
            <span class="section-label">
              {group.severity === 'error' ? 'Errors' : group.severity === 'warning' ? 'Warnings' : 'Info'}
            </span>
            <span class="section-count severity-{group.severity}">{group.items.length}</span>
          </button>

          {#if !collapsedSections.has(group.severity)}
            <div class="section-items">
              {#each group.items as diag (diag.id)}
                <article
                  class="diagnostic-row"
                  role="listitem"
                  onclick={() => handleRowClick(diag)}
                >
                  <!-- Severity strip -->
                  <div class="severity-strip severity-{diag.severity}"></div>

                  <div class="row-content">
                    <div class="row-top">
                      <span class="stage-pill stage-{diag.stage}">
                        {STAGE_LABELS[diag.stage]}
                      </span>
                      <span class="scope-label">{diag.scope}</span>
                      <span class="row-message">{diag.message}</span>
                      <span class="row-right">
                        <span class="source-label">{diag.source}</span>
                        <span class="timestamp">{relativeTime(diag.timestamp)}</span>
                        {#if diag.target}
                          <button
                            type="button"
                            class="nav-btn"
                            title="Go to source"
                            onclick={(e) => { e.stopPropagation(); handleRowNavigate(diag); }}
                          >
                            <svg viewBox="0 0 20 20" fill="currentColor" aria-hidden="true">
                              <path fill-rule="evenodd" d="M10.293 3.293a1 1 0 011.414 0l6 6a1 1 0 010 1.414l-6 6a1 1 0 01-1.414-1.414L14.586 11H3a1 1 0 110-2h11.586l-4.293-4.293a1 1 0 010-1.414z" clip-rule="evenodd" />
                            </svg>
                          </button>
                        {/if}
                      </span>
                    </div>

                    {#if expandedIds.has(diag.id) && diag.detail}
                      <div class="row-detail">{diag.detail}</div>
                    {/if}
                  </div>
                </article>
              {/each}
            </div>
          {/if}
        </div>
      {/each}
    </div>
  {/if}
</div>

<style>
  /* ── Panel container ──────────────────────────────────────────────── */
  .problems-panel {
    display: flex;
    flex-direction: column;
    height: 100%;
    overflow: hidden;
    font-family: var(--font-sans);
  }

  /* ── Toolbar ──────────────────────────────────────────────────────── */
  .toolbar {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-3);
    padding: var(--space-2) 0;
    border-bottom: 1px solid var(--color-border-subtle);
    flex-shrink: 0;
  }

  .filter-row {
    display: flex;
    gap: var(--space-4);
    flex-wrap: wrap;
  }

  .filter-group {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    flex-wrap: wrap;
  }

  .filter-label {
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    color: var(--color-text-muted);
    margin-right: var(--space-1);
  }

  .filter-chip {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    padding: 2px 8px;
    border-radius: var(--radius-full);
    border: 1px solid var(--color-border-default);
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
    white-space: nowrap;
  }

  .filter-chip:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-secondary);
  }

  .filter-chip:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .filter-chip.active.severity-error {
    background: var(--color-danger-bg);
    border-color: var(--color-danger-border);
    color: var(--color-danger-text);
  }

  .filter-chip.active.severity-warning {
    background: var(--color-warning-bg);
    border-color: var(--color-warning-border);
    color: var(--color-warning-text);
  }

  .filter-chip.active.severity-info {
    background: var(--color-info-bg);
    border-color: var(--color-info-border);
    color: var(--color-info-text);
  }

  /* Stage filter chip active colors */
  .filter-chip.active.stage-intake {
    background: rgba(14, 165, 233, 0.12);
    border-color: rgba(14, 165, 233, 0.35);
    color: var(--color-info-text);
  }

  .filter-chip.active.stage-normalization {
    background: rgba(139, 92, 246, 0.12);
    border-color: rgba(139, 92, 246, 0.35);
    color: rgba(139, 92, 246, 0.9);
  }

  .filter-chip.active.stage-translation {
    background: rgba(245, 158, 11, 0.12);
    border-color: rgba(245, 158, 11, 0.35);
    color: var(--color-warning-text);
  }

  .filter-chip.active.stage-delivery {
    background: rgba(99, 102, 241, 0.12);
    border-color: rgba(99, 102, 241, 0.35);
    color: rgba(99, 102, 241, 0.9);
  }

  .filter-chip.active.stage-verification {
    background: rgba(16, 185, 129, 0.12);
    border-color: rgba(16, 185, 129, 0.35);
    color: var(--color-success-text);
  }

  .chip-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 14px;
    height: 14px;
    padding: 0 3px;
    border-radius: var(--radius-full);
    background: rgba(255, 255, 255, 0.12);
    font-size: 9px;
    font-weight: var(--font-bold);
    line-height: 1;
  }

  .clear-btn {
    padding: 4px 10px;
    border-radius: var(--radius-md);
    border: 1px solid var(--color-border-default);
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: var(--text-2xs);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .clear-btn:hover:not(:disabled) {
    background: var(--color-bg-hover);
    color: var(--color-text-secondary);
  }

  .clear-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .clear-btn:disabled {
    opacity: 0.4;
    cursor: default;
  }

  /* ── Empty state ──────────────────────────────────────────────────── */
  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: var(--space-2);
    padding: var(--space-8) var(--space-4);
    text-align: center;
    flex: 1;
  }

  .empty-icon {
    width: 40px;
    height: 40px;
    color: var(--color-success);
    opacity: 0.7;
  }

  .empty-icon svg {
    width: 100%;
    height: 100%;
  }

  .empty-title {
    font-family: var(--font-heading);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .empty-body {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    line-height: 1.5;
    max-width: 340px;
  }

  /* ── Diagnostic list ──────────────────────────────────────────────── */
  .diagnostic-list {
    flex: 1;
    overflow-y: auto;
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding-top: var(--space-2);
  }

  /* ── Severity sections ────────────────────────────────────────────── */
  .severity-section {
    display: flex;
    flex-direction: column;
  }

  .section-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) var(--space-2);
    border: none;
    border-radius: var(--radius-md);
    background: transparent;
    cursor: pointer;
    transition: var(--transition-colors);
    width: 100%;
    text-align: left;
  }

  .section-header:hover {
    background: var(--color-bg-hover);
  }

  .section-header:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .section-label {
    font-family: var(--font-heading);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    color: var(--color-text-primary);
  }

  .section-header.severity-error .section-label {
    color: var(--color-danger-text);
  }

  .section-header.severity-warning .section-label {
    color: var(--color-warning-text);
  }

  .section-header.severity-info .section-label {
    color: var(--color-info-text);
  }

  .section-count {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    min-width: 18px;
    height: 18px;
    padding: 0 5px;
    border-radius: var(--radius-full);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    line-height: 1;
  }

  .section-count.severity-error {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .section-count.severity-warning {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }

  .section-count.severity-info {
    background: var(--color-info-bg);
    color: var(--color-info-text);
  }

  .chevron {
    display: flex;
    width: 14px;
    height: 14px;
    color: var(--color-text-muted);
    transition: transform var(--duration-slow) var(--ease-in-out);
  }

  .chevron svg {
    width: 100%;
    height: 100%;
  }

  .chevron.collapsed {
    transform: rotate(-90deg);
  }

  .section-items {
    display: flex;
    flex-direction: column;
    gap: 1px;
    padding-left: var(--space-2);
  }

  /* ── Diagnostic row ───────────────────────────────────────────────── */
  .diagnostic-row {
    display: flex;
    align-items: stretch;
    border-radius: var(--radius-md);
    cursor: pointer;
    transition:
      background-color var(--duration-fast) var(--ease-out),
      box-shadow var(--duration-fast) var(--ease-out);
    animation: slideInUp var(--duration-normal) var(--ease-out);
    overflow: hidden;
  }

  .diagnostic-row:hover {
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .diagnostic-row:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .severity-strip {
    width: 3px;
    flex-shrink: 0;
    border-radius: 2px 0 0 2px;
  }

  .severity-strip.severity-error {
    background: var(--color-danger);
  }

  .severity-strip.severity-warning {
    background: var(--color-warning);
  }

  .severity-strip.severity-info {
    background: var(--color-info);
  }

  .row-content {
    flex: 1;
    min-width: 0;
    padding: var(--space-1) var(--space-2);
    display: flex;
    flex-direction: column;
    gap: 2px;
  }

  .row-top {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  /* ── Stage pill ───────────────────────────────────────────────────── */
  .stage-pill {
    flex-shrink: 0;
    padding: 1px 6px;
    border-radius: var(--radius-full);
    font-size: 9px;
    font-weight: var(--font-bold);
    text-transform: uppercase;
    letter-spacing: var(--tracking-wider);
    line-height: 1.6;
    white-space: nowrap;
  }

  .stage-pill.stage-intake {
    background: rgba(14, 165, 233, 0.12);
    color: var(--color-info-text);
  }

  .stage-pill.stage-normalization {
    background: rgba(139, 92, 246, 0.12);
    color: rgba(139, 92, 246, 0.9);
  }

  .stage-pill.stage-translation {
    background: rgba(245, 158, 11, 0.12);
    color: var(--color-warning-text);
  }

  .stage-pill.stage-delivery {
    background: rgba(99, 102, 241, 0.12);
    color: rgba(99, 102, 241, 0.9);
  }

  .stage-pill.stage-verification {
    background: rgba(16, 185, 129, 0.12);
    color: var(--color-success-text);
  }

  /* ── Scope label ──────────────────────────────────────────────────── */
  .scope-label {
    flex-shrink: 0;
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  /* ── Message ──────────────────────────────────────────────────────── */
  .row-message {
    flex: 1;
    min-width: 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }

  /* ── Right-side metadata ──────────────────────────────────────────── */
  .row-right {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex-shrink: 0;
    margin-left: auto;
  }

  .source-label {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
  }

  .timestamp {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  .nav-btn {
    display: flex;
    align-items: center;
    justify-content: center;
    width: 18px;
    height: 18px;
    padding: 0;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-muted);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .nav-btn:hover {
    background: var(--color-bg-active);
    color: var(--color-primary);
  }

  .nav-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .nav-btn svg {
    width: 12px;
    height: 12px;
  }

  /* ── Detail row ───────────────────────────────────────────────────── */
  .row-detail {
    font-family: var(--font-mono);
    font-size: var(--text-2xs);
    color: var(--color-text-tertiary);
    padding: 2px 0;
    line-height: 1.5;
    animation: slideInUp var(--duration-fast) var(--ease-out);
  }

  /* ── Keyframes ────────────────────────────────────────────────────── */
  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  /* ── Reduced motion ───────────────────────────────────────────────── */
  @media (prefers-reduced-motion: reduce) {
    .diagnostic-row,
    .row-detail,
    .chevron {
      animation: none;
      transition: none;
    }
  }
</style>
