<script lang="ts">
  import type {
    ExtractEntitiesQuery,
    ExtractEntitiesQueryVariables
  } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';
  import { ExtractEntitiesDocument } from '$lib/gen/graphql';
  import Panel from './Panel.svelte';
  import Badge from './Badge.svelte';
  import { createEventDispatcher } from 'svelte';

  export let text: string = '';
  export let documentType: string | null = null;
  export let patientAge: number | null = null;
  export let patientGender: string | null = null;
  export let minConfidence: number = 0.7;
  export let includeNegated: boolean = false;

  const dispatch = createEventDispatcher<{
    extracted: ExtractEntitiesQuery['extractEntities'];
  }>();

  let isLoading = false;
  let error: string | null = null;
  let result: ExtractEntitiesQuery['extractEntities'] | null = null;
  let activeTab = 0;

  const tabs = ['Conditions', 'Medications', 'Vitals', 'Allergies', 'Procedures'];

  async function extractEntities() {
    if (!text.trim()) {
      error = 'No text provided for extraction';
      return;
    }

    isLoading = true;
    error = null;

    try {
      const variables: ExtractEntitiesQueryVariables = {
        input: {
          text,
          documentType,
          patientAge,
          patientGender,
          minConfidence,
          includeNegated
        }
      };

      const data = await graphqlFetch(ExtractEntitiesDocument, variables);
      result = data.extractEntities;
      dispatch('extracted', result);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Extraction failed';
    } finally {
      isLoading = false;
    }
  }

  function confidenceClass(confidence: number): string {
    if (confidence >= 0.9) return 'high';
    if (confidence >= 0.7) return 'medium';
    return 'low';
  }

  function formatConfidence(confidence: number): string {
    return `${Math.round(confidence * 100)}%`;
  }

  function severityVariant(severity: string): 'success' | 'warning' | 'danger' | 'default' {
    switch (severity.toLowerCase()) {
      case 'mild':
        return 'success';
      case 'moderate':
        return 'warning';
      case 'severe':
        return 'danger';
      default:
        return 'default';
    }
  }

  $: hasConditions = (result?.conditions?.length ?? 0) > 0;
  $: hasMedications = (result?.medications?.length ?? 0) > 0;
  $: hasVitalSigns = (result?.vitalSigns?.length ?? 0) > 0;
  $: hasAllergies = (result?.allergies?.length ?? 0) > 0;
  $: hasProcedures = (result?.procedures?.length ?? 0) > 0;
  $: totalEntities =
    (result?.conditions?.length ?? 0) +
    (result?.medications?.length ?? 0) +
    (result?.vitalSigns?.length ?? 0) +
    (result?.allergies?.length ?? 0) +
    (result?.procedures?.length ?? 0);
</script>

<Panel title="Clinical Entity Extraction">
  <div class="extraction-panel">
    <div class="controls">
      <button
        class="extract-btn"
        onclick={extractEntities}
        disabled={isLoading || !text.trim()}
      >
        {#if isLoading}
          <span class="spinner"></span>
          Extracting...
        {:else}
          Extract Entities
        {/if}
      </button>

      {#if result}
        <span class="stats">
          Found {totalEntities} entities
          <span class="confidence {confidenceClass(result.overallConfidence)}">
            ({formatConfidence(result.overallConfidence)} confidence)
          </span>
          in {result.processingTimeMs}ms
        </span>
      {/if}
    </div>

    {#if error}
      <div class="error">{error}</div>
    {/if}

    {#if result}
      <div class="results">
        <div class="tabs-container">
          {#each tabs as tab, i (tab)}
            <button
              class="tab"
              class:active={activeTab === i}
              onclick={() => activeTab = i}
            >
              {tab}
              <span class="count">
                {#if i === 0}{result.conditions?.length ?? 0}
                {:else if i === 1}{result.medications?.length ?? 0}
                {:else if i === 2}{result.vitalSigns?.length ?? 0}
                {:else if i === 3}{result.allergies?.length ?? 0}
                {:else}{result.procedures?.length ?? 0}
                {/if}
              </span>
            </button>
          {/each}
        </div>

        <div class="tab-content">
          {#if activeTab === 0}
            <!-- Conditions -->
            {#if hasConditions}
              <ul class="entity-list">
                {#each result.conditions as condition, idx (condition.name + idx)}
                  <li class="entity">
                    <div class="entity-main">
                      <span class="entity-name">{condition.name}</span>
                      {#if condition.negated}
                        <Badge variant="danger" size="sm">negated</Badge>
                      {/if}
                      {#if condition.status}
                        <Badge variant="info" size="sm">{condition.status}</Badge>
                      {/if}
                      <span class="confidence {confidenceClass(condition.confidence)}">
                        {formatConfidence(condition.confidence)}
                      </span>
                    </div>
                    {#if condition.code}
                      <div class="entity-code">{condition.codeSystem}: {condition.code}</div>
                    {/if}
                    {#if condition.textSpan}
                      <div class="text-span">"{condition.textSpan}"</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <div class="empty">No conditions found</div>
            {/if}

          {:else if activeTab === 1}
            <!-- Medications -->
            {#if hasMedications}
              <ul class="entity-list">
                {#each result.medications as med, idx (med.name + idx)}
                  <li class="entity">
                    <div class="entity-main">
                      <span class="entity-name">{med.name}</span>
                      {#if med.dose}
                        <Badge variant="default" size="sm">{med.dose}</Badge>
                      {/if}
                      {#if med.route}
                        <Badge variant="default" size="sm">{med.route}</Badge>
                      {/if}
                      <span class="confidence {confidenceClass(med.confidence)}">
                        {formatConfidence(med.confidence)}
                      </span>
                    </div>
                    {#if med.frequency}
                      <div class="entity-detail">Frequency: {med.frequency}</div>
                    {/if}
                    {#if med.code}
                      <div class="entity-code">{med.codeSystem}: {med.code}</div>
                    {/if}
                    {#if med.textSpan}
                      <div class="text-span">"{med.textSpan}"</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <div class="empty">No medications found</div>
            {/if}

          {:else if activeTab === 2}
            <!-- Vital Signs -->
            {#if hasVitalSigns}
              <ul class="entity-list">
                {#each result.vitalSigns as vital, idx (vital.name + idx)}
                  <li class="entity">
                    <div class="entity-main">
                      <span class="entity-name">{vital.name}</span>
                      <Badge variant="default" size="sm">{vital.value}{vital.unit ?? ''}</Badge>
                      {#if vital.interpretation}
                        <Badge variant="default" size="sm">{vital.interpretation}</Badge>
                      {/if}
                      <span class="confidence {confidenceClass(vital.confidence)}">
                        {formatConfidence(vital.confidence)}
                      </span>
                    </div>
                    {#if vital.loincCode}
                      <div class="entity-code">LOINC: {vital.loincCode}</div>
                    {/if}
                    {#if vital.textSpan}
                      <div class="text-span">"{vital.textSpan}"</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <div class="empty">No vital signs found</div>
            {/if}

          {:else if activeTab === 3}
            <!-- Allergies -->
            {#if hasAllergies}
              <ul class="entity-list">
                {#each result.allergies as allergy, idx (allergy.substance + idx)}
                  <li class="entity">
                    <div class="entity-main">
                      <span class="entity-name">{allergy.substance}</span>
                      {#if allergy.severity}
                        <Badge variant={severityVariant(allergy.severity)} size="sm">{allergy.severity}</Badge>
                      {/if}
                      <span class="confidence {confidenceClass(allergy.confidence)}">
                        {formatConfidence(allergy.confidence)}
                      </span>
                    </div>
                    {#if allergy.reaction}
                      <div class="entity-detail">Reaction: {allergy.reaction}</div>
                    {/if}
                    {#if allergy.code}
                      <div class="entity-code">{allergy.codeSystem}: {allergy.code}</div>
                    {/if}
                    {#if allergy.textSpan}
                      <div class="text-span">"{allergy.textSpan}"</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <div class="empty">No allergies found</div>
            {/if}

          {:else}
            <!-- Procedures -->
            {#if hasProcedures}
              <ul class="entity-list">
                {#each result.procedures as procedure, idx (procedure.name + idx)}
                  <li class="entity">
                    <div class="entity-main">
                      <span class="entity-name">{procedure.name}</span>
                      {#if procedure.status}
                        <Badge variant="info" size="sm">{procedure.status}</Badge>
                      {/if}
                      <span class="confidence {confidenceClass(procedure.confidence)}">
                        {formatConfidence(procedure.confidence)}
                      </span>
                    </div>
                    {#if procedure.code}
                      <div class="entity-code">{procedure.codeSystem}: {procedure.code}</div>
                    {/if}
                    {#if procedure.textSpan}
                      <div class="text-span">"{procedure.textSpan}"</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            {:else}
              <div class="empty">No procedures found</div>
            {/if}
          {/if}
        </div>
      </div>
    {/if}
  </div>
</Panel>

<style>
  .extraction-panel {
    display: flex;
    flex-direction: column;
    gap: 12px;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
  }

  .extract-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    background: linear-gradient(135deg, var(--palette-blue-500) 0%, var(--palette-blue-600) 100%);
    color: white;
    border: none;
    border-radius: 6px;
    font-weight: 500;
    cursor: pointer;
    transition: opacity 0.2s;
  }

  .extract-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .extract-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .extract-btn:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(255, 255, 255, 0.3);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .stats {
    font-size: 0.85rem;
    color: var(--color-text-tertiary);
  }

  .error {
    padding: 8px 12px;
    background: rgba(239, 68, 68, 0.15);
    border: 1px solid rgba(239, 68, 68, 0.3);
    border-radius: 6px;
    color: var(--color-danger-soft);
    font-size: 0.85rem;
  }

  .results {
    margin-top: 8px;
  }

  .tabs-container {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid var(--color-border-default);
    padding-bottom: 8px;
    margin-bottom: 12px;
    overflow-x: auto;
  }

  .tab {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 6px 12px;
    background: transparent;
    border: 1px solid transparent;
    border-radius: 6px;
    color: var(--color-text-tertiary);
    font-size: 0.85rem;
    cursor: pointer;
    transition: var(--transition-all);
    white-space: nowrap;
  }

  .tab:hover {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  .tab:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .tab.active {
    color: var(--color-text-primary);
    background: var(--color-primary-muted);
    border-color: var(--color-primary-border);
  }

  .count {
    padding: 2px 6px;
    background: var(--color-bg-surface);
    border-radius: 10px;
    font-size: 0.75rem;
  }

  .tab.active .count {
    background: var(--color-primary-border);
  }

  .tab-content {
    max-height: 400px;
    overflow-y: auto;
  }

  .entity-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }

  .entity {
    padding: 10px 12px;
    background: var(--color-bg-surface);
    border: 1px solid var(--color-border-subtle);
    border-radius: 6px;
  }

  .entity-main {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .entity-name {
    font-weight: 500;
    color: var(--color-text-primary);
  }

  .confidence {
    margin-left: auto;
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 600;
  }

  .confidence.high {
    background: rgba(16, 185, 129, 0.15);
    color: var(--color-success-soft);
  }

  .confidence.medium {
    background: rgba(234, 179, 8, 0.15);
    color: var(--color-warning-soft);
  }

  .confidence.low {
    background: rgba(239, 68, 68, 0.15);
    color: var(--color-danger-soft);
  }

  .entity-code,
  .entity-detail {
    margin-top: 4px;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .text-span {
    margin-top: 6px;
    padding: 6px 10px;
    background: var(--color-bg-elevated);
    border-radius: 4px;
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
    font-style: italic;
  }

  .empty {
    padding: 20px;
    text-align: center;
    color: var(--color-text-muted);
    font-size: 0.9rem;
  }
</style>
