<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import { workflowDraft } from '../workflowStore';
  import { draftToYaml } from '../workflowYaml';
  import { dryRunWorkflow } from '../workflowApi';
  import { toasts } from '$lib/ui/toastStore';
  import type { DryRunWorkflowMutation } from '$lib/gen/graphql';

  type DryRunResult = DryRunWorkflowMutation['dryRunWorkflow'];

  const sampleEvents = [
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

  let selectedSamples = [0];
  let customEventJson = '';
  let useCustom = false;
  let running = false;
  let result: DryRunResult | null = null;

  function toggleSample(index: number) {
    if (selectedSamples.includes(index)) {
      selectedSamples = selectedSamples.filter((i) => i !== index);
    } else {
      selectedSamples = [...selectedSamples, index];
    }
  }

  async function handleRun() {
    running = true;
    result = null;

    try {
      const yamlStr = draftToYaml($workflowDraft);
      let events: unknown[];

      if (useCustom) {
        try {
          const parsed = JSON.parse(customEventJson);
          events = Array.isArray(parsed) ? parsed : [parsed];
        } catch {
          toasts.error('Invalid JSON for custom events');
          running = false;
          return;
        }
      } else {
        events = selectedSamples.map((i) => sampleEvents[i]!.event);
      }

      if (events.length === 0) {
        toasts.error('Select at least one sample event');
        running = false;
        return;
      }

      const data = await dryRunWorkflow(yamlStr, events);
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
        <span class="label">Sample Events</span>
        <Button variant="ghost" size="sm" on:click={() => (useCustom = !useCustom)}>
          {useCustom ? 'Use Presets' : 'Custom JSON'}
        </Button>
      </div>

      {#if useCustom}
        <textarea
          class="textarea mono"
          bind:value={customEventJson}
          placeholder={'[\n  { "type": "PATIENT_ADMIT", "source": "epic" }\n]'}
          rows="5"
        ></textarea>
      {:else}
        <div class="sample-list">
          {#each sampleEvents as sample, i (i)}
            <label class="sample-item">
              <input
                type="checkbox"
                checked={selectedSamples.includes(i)}
                on:change={() => toggleSample(i)}
              />
              <span class="sample-label">{sample.label}</span>
              <span class="sample-type mono">{sample.event.type}</span>
            </label>
          {/each}
        </div>
      {/if}

      <Button on:click={handleRun} loading={running}>
        {running ? 'Running...' : 'Run Simulation'}
      </Button>
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
    color: rgba(229, 231, 235, 0.8);
    font-weight: 700;
    font-size: 0.9rem;
  }

  .textarea {
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    resize: vertical;
    width: 100%;
    box-sizing: border-box;
  }

  .textarea:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, monospace;
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
    color: rgba(229, 231, 235, 0.85);
    font-size: 0.9rem;
  }

  .sample-type {
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.8rem;
  }

  .results {
    display: grid;
    gap: 12px;
  }

  .results-title {
    color: rgba(229, 231, 235, 0.8);
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
    background: rgba(239, 68, 68, 0.1);
    border: 1px solid rgba(239, 68, 68, 0.25);
    color: rgba(254, 202, 202, 0.9);
    font-size: 0.85rem;
  }

  .warnings {
    display: grid;
    gap: 4px;
  }

  .warning-item {
    padding: 6px 10px;
    border-radius: 6px;
    background: rgba(245, 158, 11, 0.1);
    border: 1px solid rgba(245, 158, 11, 0.25);
    color: rgba(253, 230, 138, 0.9);
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
    color: rgba(229, 231, 235, 0.55);
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
    border: 1px solid rgba(255, 255, 255, 0.06);
    background: rgba(255, 255, 255, 0.02);
    align-items: center;
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.85);
  }

  .muted {
    color: rgba(229, 231, 235, 0.5);
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
