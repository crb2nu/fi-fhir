<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import { workflowDraft } from '../workflowStore';
  import { draftToYaml } from '../workflowYaml';
  import { dryRunWorkflow } from '../workflowApi';
  import { toasts } from '$lib/ui/toastStore';
  import { debugSession } from '$lib/features/debug/debugStore';
  import { runtimeOutputState } from '$lib/ui/ide/panels/runtimeOutputStore';
  import type { DryRunWorkflowMutation } from '$lib/gen/graphql';

  type DryRunResult = DryRunWorkflowMutation['dryRunWorkflow'];
  type EventSource = 'presets' | 'debug' | 'recent' | 'custom';

  const presetEvents = [
    {
      label: 'Patient Admit',
      event: { type: 'PATIENT_ADMIT', source: 'epic', id: 'sample-1', isCritical: false }
    },
    {
      label: 'Lab Result (Critical)',
      event: { type: 'LAB_RESULT', source: 'epic', id: 'sample-2', isCritical: true }
    },
    {
      label: 'Patient Discharge',
      event: { type: 'PATIENT_DISCHARGE', source: 'cerner', id: 'sample-3', isCritical: false }
    },
    {
      label: 'Appointment Scheduled',
      event: { type: 'APPOINTMENT_SCHEDULED', source: 'epic', id: 'sample-4', isCritical: false }
    }
  ];

  const sourceOptions: { value: EventSource; label: string }[] = [
    { value: 'presets', label: 'Presets' },
    { value: 'debug', label: 'Debug Session' },
    { value: 'recent', label: 'Recent Output' },
    { value: 'custom', label: 'Custom JSON' },
  ];

  let eventSource: EventSource = 'presets';
  let selectedPresets = [0];
  let customEventJson = '';
  const customEventPlaceholder = '[{ "type": "PATIENT_ADMIT", "source": "epic" }]';
  let running = false;
  let result: DryRunResult | null = null;

  function togglePreset(index: number) {
    if (selectedPresets.includes(index)) {
      selectedPresets = selectedPresets.filter((i) => i !== index);
    } else {
      selectedPresets = [...selectedPresets, index];
    }
  }

  function toEventPayload(entry: { title: string; source: string; kind: string }): Record<string, unknown> {
    return { type: entry.title.toUpperCase().replace(/\s+/g, '_'), source: entry.source, kind: entry.kind };
  }

  function parseCustomJson(json: string): unknown[] {
    try {
      const parsed = JSON.parse(json);
      return Array.isArray(parsed) ? parsed : [parsed];
    } catch {
      return [];
    }
  }

  $: resolvedEvents = (() => {
    if (eventSource === 'debug') {
      const session = $debugSession;
      if (!session || session.steps.length === 0) return [];
      const firstStep = session.steps[0];
      const ev = firstStep?.variables?.['event'];
      if (ev && typeof ev === 'object') return [ev];
      return [firstStep?.variables ?? {}];
    }
    if (eventSource === 'recent') {
      return $runtimeOutputState.entries.slice(0, 5).map(toEventPayload);
    }
    if (eventSource === 'custom') {
      return parseCustomJson(customEventJson);
    }
    // presets
    return selectedPresets.map((i) => presetEvents[i]!.event);
  })();

  async function handleRun() {
    running = true;
    result = null;

    try {
      const yamlStr = draftToYaml($workflowDraft);

      if (resolvedEvents.length === 0) {
        toasts.error('No events available for dry run');
        running = false;
        return;
      }

      if (eventSource === 'custom' && customEventJson.trim()) {
        try {
          JSON.parse(customEventJson);
        } catch {
          toasts.error('Invalid JSON for custom events');
          running = false;
          return;
        }
      }

      const data = await dryRunWorkflow(yamlStr, resolvedEvents);
      result = data.dryRunWorkflow;
    } catch {
      toasts.error('Dry run failed');
    } finally {
      running = false;
    }
  }
</script>

<Panel title="Dry Run Simulation">
  <div class="dry-run">
    <div class="event-selector">
      <div class="selector-header">
        <span class="label">Event Source</span>
        <div class="source-tabs" role="tablist">
          {#each sourceOptions as opt (opt.value)}
            <button
              type="button"
              class="source-tab"
              class:active={eventSource === opt.value}
              role="tab"
              aria-selected={eventSource === opt.value}
              on:click={() => { eventSource = opt.value; }}
            >
              {opt.label}
            </button>
          {/each}
        </div>
      </div>

      {#if eventSource === 'custom'}
        <CodeEditor
          language="json"
          value={customEventJson}
          on:change={(e) => { customEventJson = e.detail; }}
          placeholder={customEventPlaceholder}
          height="150px"
        />
      {:else if eventSource === 'presets'}
        <div class="sample-list">
          {#each presetEvents as sample, i (i)}
            <label class="sample-item">
              <input
                type="checkbox"
                checked={selectedPresets.includes(i)}
                on:change={() => togglePreset(i)}
              />
              <span class="sample-label">{sample.label}</span>
              <span class="sample-type mono">{sample.event.type}</span>
            </label>
          {/each}
        </div>
      {:else if eventSource === 'debug'}
        <div class="source-info">
          {#if $debugSession}
            <span class="source-status active">Debug session active</span>
            <span class="source-detail mono">{$debugSession.id}</span>
          {:else}
            <span class="source-status">No active debug session</span>
          {/if}
        </div>
      {:else if eventSource === 'recent'}
        <div class="source-info">
          {#if $runtimeOutputState.entries.length > 0}
            <span class="source-status active">{Math.min(5, $runtimeOutputState.entries.length)} recent entries</span>
          {:else}
            <span class="source-status">No recent output entries</span>
          {/if}
        </div>
      {/if}

      <div class="run-row">
        <Button on:click={handleRun} loading={running} disabled={resolvedEvents.length === 0}>
          {running ? 'Running...' : 'Run Simulation'}
        </Button>
        <span class="event-count">{resolvedEvents.length} event{resolvedEvents.length === 1 ? '' : 's'}</span>
      </div>
    </div>

    {#if result}
      <div class="results">
        {#if result.validationErrors.length > 0}
          <div class="errors" role="alert">
            <h4 class="results-title">Validation Errors</h4>
            {#each result.validationErrors as err (err)}
              <div class="error-item">{err}</div>
            {/each}
          </div>
        {/if}

        {#if result.warnings.length > 0}
          <div class="warnings" role="alert">
            <h4 class="results-title">Warnings</h4>
            {#each result.warnings as warn (warn)}
              <div class="warning-item">{warn}</div>
            {/each}
          </div>
        {/if}

        <h4 class="results-title">Route Results</h4>
        <div class="results-table">
          <div class="table-header">
            <span>Route</span>
            <span>Matched</span>
            <span>Actions</span>
            <span>Skip Reason</span>
          </div>
          {#each result.routeResults as rr (rr.routeName)}
            <div class="table-row">
              <span class="mono">{rr.routeName}</span>
              <span>
                <Badge variant={rr.matched ? 'success' : 'default'} size="sm">
                  {rr.matched ? 'Yes' : 'No'}
                </Badge>
              </span>
              <span>{rr.actionsWouldRun}</span>
              <span class="muted">{rr.skipReason || '—'}</span>
            </div>
          {/each}
        </div>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .dry-run {
    display: grid;
    gap: 16px;
  }

  .event-selector {
    display: grid;
    gap: 10px;
  }

  .selector-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
  }

  .label {
    color: var(--color-text-tertiary);
    font-weight: 700;
    font-size: 0.9rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
  }

  .source-tabs {
    display: flex;
    gap: 2px;
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: 8px;
    padding: 2px;
  }

  .source-tab {
    padding: 4px 10px;
    border: none;
    border-radius: 6px;
    background: transparent;
    color: var(--color-text-tertiary);
    font-size: 0.8rem;
    font-weight: 600;
    cursor: pointer;
    transition: var(--transition-all);
  }

  .source-tab:hover {
    color: var(--color-text-secondary);
    background: var(--color-bg-hover);
  }

  .source-tab.active {
    color: var(--color-text-primary);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .source-info {
    display: grid;
    gap: 4px;
    padding: 8px 0;
  }

  .source-status {
    font-size: 0.85rem;
    color: var(--color-text-muted);
  }

  .source-status.active {
    color: var(--color-success-text, #10b981);
  }

  .source-detail {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
  }

  .run-row {
    display: flex;
    align-items: center;
    gap: 10px;
  }

  .event-count {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .sample-list {
    display: grid;
    gap: 4px;
  }

  .sample-item {
    display: flex;
    align-items: center;
    gap: 8px;
    cursor: pointer;
    padding: 4px 0;
  }

  .sample-item input {
    accent-color: rgba(59, 130, 246, 0.85);
  }

  .sample-label {
    color: var(--color-text-secondary);
    font-size: 0.9rem;
  }

  .sample-type {
    color: var(--color-text-muted);
    font-size: 0.8rem;
  }

  .results {
    display: grid;
    gap: 12px;
  }

  .results-title {
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    font-weight: 700;
    margin: 0;
  }

  .errors {
    display: grid;
    gap: 4px;
  }

  .error-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    color: var(--color-text-primary);
    font-size: 0.85rem;
  }

  .warnings {
    display: grid;
    gap: 4px;
  }

  .warning-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: var(--color-warning-bg);
    border: 1px solid var(--color-warning-border);
    color: var(--color-text-primary);
    font-size: 0.85rem;
  }

  .results-table {
    display: grid;
    gap: 4px;
  }

  .table-header {
    display: grid;
    grid-template-columns: 1fr 80px 80px 1fr;
    gap: 12px;
    padding: 6px 10px;
    color: var(--color-text-muted);
    font-size: 0.75rem;
    font-weight: 700;
    text-transform: uppercase;
    letter-spacing: 0.05em;
  }

  .table-row {
    display: grid;
    grid-template-columns: 1fr 80px 80px 1fr;
    gap: 12px;
    padding: 8px 10px;
    border-radius: 6px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    align-items: center;
    font-size: 0.9rem;
    color: var(--color-text-secondary);
  }

  .muted {
    color: var(--color-text-muted);
    font-size: 0.85rem;
  }

  @media (max-width: 640px) {
    .table-header,
    .table-row {
      grid-template-columns: 1fr 1fr;
    }

    .table-header span:nth-child(4),
    .table-row span:nth-child(4) {
      display: none;
    }
  }
</style>
