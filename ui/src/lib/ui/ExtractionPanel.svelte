<script lang="ts">
  import type {
    ExtractEntitiesQuery,
    ExtractEntitiesQueryVariables
  } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';
  import { ExtractEntitiesDocument } from '$lib/gen/graphql';
  import Panel from './Panel.svelte';
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
                        <span class="tag negated">negated</span>
                      {/if}
                      {#if condition.status}
                        <span class="tag status">{condition.status}</span>
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
                        <span class="tag dose">{med.dose}</span>
                      {/if}
                      {#if med.route}
                        <span class="tag route">{med.route}</span>
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
                      <span class="tag value">{vital.value}{vital.unit ?? ''}</span>
                      {#if vital.interpretation}
                        <span class="tag interpretation">{vital.interpretation}</span>
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
                        <span class="tag severity-{allergy.severity.toLowerCase()}">{allergy.severity}</span>
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
                        <span class="tag status">{procedure.status}</span>
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
    background: linear-gradient(135deg, #3b82f6 0%, #2563eb 100%);
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
    color: rgba(255, 255, 255, 0.7);
  }

  .error {
    padding: 8px 12px;
    background: rgba(239, 68, 68, 0.15);
    border: 1px solid rgba(239, 68, 68, 0.3);
    border-radius: 6px;
    color: #f87171;
    font-size: 0.85rem;
  }

  .results {
    margin-top: 8px;
  }

  .tabs-container {
    display: flex;
    gap: 4px;
    border-bottom: 1px solid rgba(255, 255, 255, 0.1);
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
    color: rgba(255, 255, 255, 0.6);
    font-size: 0.85rem;
    cursor: pointer;
    transition: all 0.2s;
    white-space: nowrap;
  }

  .tab:hover {
    color: rgba(255, 255, 255, 0.9);
    background: rgba(255, 255, 255, 0.05);
  }

  .tab.active {
    color: white;
    background: rgba(59, 130, 246, 0.2);
    border-color: rgba(59, 130, 246, 0.3);
  }

  .count {
    padding: 2px 6px;
    background: rgba(255, 255, 255, 0.1);
    border-radius: 10px;
    font-size: 0.75rem;
  }

  .tab.active .count {
    background: rgba(59, 130, 246, 0.3);
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
    background: rgba(255, 255, 255, 0.03);
    border: 1px solid rgba(255, 255, 255, 0.06);
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
    color: #f3f4f6;
  }

  .tag {
    padding: 2px 8px;
    border-radius: 4px;
    font-size: 0.75rem;
    font-weight: 500;
  }

  .tag.negated {
    background: rgba(239, 68, 68, 0.2);
    color: #f87171;
  }

  .tag.status,
  .tag.dose,
  .tag.route,
  .tag.value {
    background: rgba(59, 130, 246, 0.15);
    color: #93c5fd;
  }

  .tag.interpretation {
    background: rgba(168, 85, 247, 0.15);
    color: #c4b5fd;
  }

  .tag.severity-mild {
    background: rgba(34, 197, 94, 0.15);
    color: #86efac;
  }

  .tag.severity-moderate {
    background: rgba(234, 179, 8, 0.15);
    color: #fde047;
  }

  .tag.severity-severe {
    background: rgba(239, 68, 68, 0.15);
    color: #f87171;
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
    color: #6ee7b7;
  }

  .confidence.medium {
    background: rgba(234, 179, 8, 0.15);
    color: #fde047;
  }

  .confidence.low {
    background: rgba(239, 68, 68, 0.15);
    color: #f87171;
  }

  .entity-code,
  .entity-detail {
    margin-top: 4px;
    font-size: 0.8rem;
    color: rgba(255, 255, 255, 0.5);
  }

  .text-span {
    margin-top: 6px;
    padding: 6px 10px;
    background: rgba(0, 0, 0, 0.2);
    border-radius: 4px;
    font-size: 0.8rem;
    color: rgba(255, 255, 255, 0.6);
    font-style: italic;
  }

  .empty {
    padding: 20px;
    text-align: center;
    color: rgba(255, 255, 255, 0.4);
    font-size: 0.9rem;
  }
</style>
