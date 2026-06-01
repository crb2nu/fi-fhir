<script lang="ts">
  import type {
    AnalyzeQualityQuery,
    AnalyzeQualityQueryVariables,
    EventType
  } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';
  import { AnalyzeQualityDocument } from '$lib/gen/graphql';
  import Panel from './Panel.svelte';
  import { createEventDispatcher } from 'svelte';

  export let event: Record<string, unknown> | null = null;
  export let eventType: EventType | null = null;
  export let compact: boolean = false;

  const dispatch = createEventDispatcher<{
    analyzed: AnalyzeQualityQuery['analyzeQuality'];
  }>();

  let isLoading = false;
  let error: string | null = null;
  let result: AnalyzeQualityQuery['analyzeQuality'] | null = null;
  let expanded = false;

  async function analyzeQuality() {
    if (!event || !eventType) {
      error = 'Event and event type are required';
      return;
    }

    isLoading = true;
    error = null;

    try {
      const variables: AnalyzeQualityQueryVariables = {
        input: {
          event,
          eventType
        }
      };

      const data = await graphqlFetch(AnalyzeQualityDocument, variables);
      result = data.analyzeQuality;
      dispatch('analyzed', result);
    } catch (e) {
      error = e instanceof Error ? e.message : 'Quality analysis failed';
    } finally {
      isLoading = false;
    }
  }

  function scoreClass(score: number): string {
    if (score >= 0.8) return 'good';
    if (score >= 0.5) return 'warning';
    return 'poor';
  }

  function formatScore(score: number): string {
    return `${Math.round(score * 100)}`;
  }

  function severityClass(severity: string): string {
    switch (severity.toLowerCase()) {
      case 'critical':
      case 'high':
        return 'high';
      case 'medium':
        return 'medium';
      default:
        return 'low';
    }
  }

  $: hasIssues = (result?.issues?.length ?? 0) > 0;
  $: hasRecommendations = (result?.recommendations?.length ?? 0) > 0;
</script>

{#if compact}
  <!-- Compact badge view -->
  <div class="quality-badge-compact">
    {#if result}
      <button
        class="score-badge {scoreClass(result.overallScore)}"
        onclick={() => expanded = !expanded}
        title="Data Quality Score (click for details)"
      >
        <span class="score-value">{formatScore(result.overallScore)}</span>
        <span class="score-label">%</span>
      </button>
    {:else}
      <button
        class="analyze-btn-compact"
        onclick={analyzeQuality}
        disabled={isLoading || !event || !eventType}
        title="Analyze data quality"
      >
        {#if isLoading}
          <span class="spinner-small"></span>
        {:else}
          Q
        {/if}
      </button>
    {/if}

    {#if expanded && result}
      <div class="expanded-popup">
        <div class="popup-header">
          <span>Data Quality Details</span>
          <button class="close-btn" onclick={() => expanded = false}>X</button>
        </div>
        <div class="dimensions-grid">
          {#if result.dimensions}
            <div class="dimension">
              <span class="dim-label">Completeness</span>
              <span class="dim-value {scoreClass(result.dimensions.completeness)}">
                {formatScore(result.dimensions.completeness)}%
              </span>
            </div>
            <div class="dimension">
              <span class="dim-label">Accuracy</span>
              <span class="dim-value {scoreClass(result.dimensions.accuracy)}">
                {formatScore(result.dimensions.accuracy)}%
              </span>
            </div>
            <div class="dimension">
              <span class="dim-label">Consistency</span>
              <span class="dim-value {scoreClass(result.dimensions.consistency)}">
                {formatScore(result.dimensions.consistency)}%
              </span>
            </div>
            <div class="dimension">
              <span class="dim-label">Conformance</span>
              <span class="dim-value {scoreClass(result.dimensions.conformance)}">
                {formatScore(result.dimensions.conformance)}%
              </span>
            </div>
            <div class="dimension">
              <span class="dim-label">Timeliness</span>
              <span class="dim-value {scoreClass(result.dimensions.timeliness)}">
                {formatScore(result.dimensions.timeliness)}%
              </span>
            </div>
          {/if}
        </div>
        {#if hasIssues}
          <div class="issues-summary">
            {result.issues.length} issue{result.issues.length !== 1 ? 's' : ''} found
          </div>
        {/if}
      </div>
    {/if}
  </div>
{:else}
  <!-- Full panel view -->
  <Panel title="Data Quality Analysis">
    <div class="quality-panel">
      <div class="controls">
        <button
          class="analyze-btn"
          onclick={analyzeQuality}
          disabled={isLoading || !event || !eventType}
        >
          {#if isLoading}
            <span class="spinner"></span>
            Analyzing...
          {:else}
            Analyze Quality
          {/if}
        </button>

        {#if result}
          <span class="stats">
            Analyzed in {result.processingTimeMs ?? 0}ms
          </span>
        {/if}
      </div>

      {#if error}
        <div class="error">{error}</div>
      {/if}

      {#if result}
        <div class="results">
          <!-- Overall Score Circle -->
          <div class="score-section">
            <div class="score-circle {scoreClass(result.overallScore)}">
              <span class="score-number">{formatScore(result.overallScore)}</span>
              <span class="score-percent">%</span>
            </div>
            <span class="score-title">Overall Quality</span>
          </div>

          <!-- Dimensions -->
          {#if result.dimensions}
            <div class="dimensions">
              <h4>Quality Dimensions</h4>
              <div class="dimension-bars">
                <div class="dimension-bar">
                  <span class="dim-name">Completeness</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill {scoreClass(result.dimensions.completeness)}"
                      style="width: {result.dimensions.completeness * 100}%"
                    ></div>
                  </div>
                  <span class="dim-score">{formatScore(result.dimensions.completeness)}%</span>
                </div>
                <div class="dimension-bar">
                  <span class="dim-name">Accuracy</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill {scoreClass(result.dimensions.accuracy)}"
                      style="width: {result.dimensions.accuracy * 100}%"
                    ></div>
                  </div>
                  <span class="dim-score">{formatScore(result.dimensions.accuracy)}%</span>
                </div>
                <div class="dimension-bar">
                  <span class="dim-name">Consistency</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill {scoreClass(result.dimensions.consistency)}"
                      style="width: {result.dimensions.consistency * 100}%"
                    ></div>
                  </div>
                  <span class="dim-score">{formatScore(result.dimensions.consistency)}%</span>
                </div>
                <div class="dimension-bar">
                  <span class="dim-name">Conformance</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill {scoreClass(result.dimensions.conformance)}"
                      style="width: {result.dimensions.conformance * 100}%"
                    ></div>
                  </div>
                  <span class="dim-score">{formatScore(result.dimensions.conformance)}%</span>
                </div>
                <div class="dimension-bar">
                  <span class="dim-name">Timeliness</span>
                  <div class="bar-track">
                    <div
                      class="bar-fill {scoreClass(result.dimensions.timeliness)}"
                      style="width: {result.dimensions.timeliness * 100}%"
                    ></div>
                  </div>
                  <span class="dim-score">{formatScore(result.dimensions.timeliness)}%</span>
                </div>
              </div>
            </div>
          {/if}

          <!-- Issues -->
          {#if hasIssues}
            <div class="issues">
              <h4>Issues ({result.issues.length})</h4>
              <ul class="issue-list">
                {#each result.issues as issue, idx (issue.description + idx)}
                  <li class="issue {severityClass(issue.severity)}">
                    <div class="issue-header">
                      <span class="issue-severity">{issue.severity}</span>
                      <span class="issue-dimension">{issue.dimension}</span>
                      {#if issue.field}
                        <span class="issue-field">{issue.field}</span>
                      {/if}
                    </div>
                    <div class="issue-description">{issue.description}</div>
                    {#if issue.actualValue || issue.expectedValue}
                      <div class="issue-values">
                        {#if issue.actualValue}
                          <span>Actual: <code>{issue.actualValue}</code></span>
                        {/if}
                        {#if issue.expectedValue}
                          <span>Expected: <code>{issue.expectedValue}</code></span>
                        {/if}
                      </div>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}

          <!-- Recommendations -->
          {#if hasRecommendations}
            <div class="recommendations">
              <h4>Recommendations</h4>
              <ul class="rec-list">
                {#each result.recommendations.sort((a, b) => a.priority - b.priority) as rec, idx (rec.title + idx)}
                  <li class="rec">
                    <div class="rec-header">
                      <span class="rec-priority">P{rec.priority}</span>
                      {#if rec.category}
                        <span class="rec-category">{rec.category}</span>
                      {/if}
                      <span class="rec-title">{rec.title}</span>
                    </div>
                    <div class="rec-description">{rec.description}</div>
                    {#if rec.impact}
                      <div class="rec-impact">Impact: {rec.impact}</div>
                    {/if}
                  </li>
                {/each}
              </ul>
            </div>
          {/if}
        </div>
      {/if}
    </div>
  </Panel>
{/if}

<style>
  .quality-badge-compact {
    position: relative;
    display: inline-block;
  }

  .score-badge {
    display: flex;
    align-items: baseline;
    gap: 1px;
    padding: 4px 8px;
    border-radius: 12px;
    border: none;
    cursor: pointer;
    font-weight: 600;
    transition: transform 0.2s;
  }

  .score-badge:hover {
    transform: scale(1.05);
  }

  .score-badge.good {
    background: linear-gradient(135deg, rgba(16, 185, 129, 0.3), rgba(16, 185, 129, 0.2));
    color: var(--color-success-soft);
  }

  .score-badge.warning {
    background: linear-gradient(135deg, rgba(234, 179, 8, 0.3), rgba(234, 179, 8, 0.2));
    color: var(--color-warning-soft);
  }

  .score-badge.poor {
    background: linear-gradient(135deg, rgba(239, 68, 68, 0.3), rgba(239, 68, 68, 0.2));
    color: var(--color-danger-soft);
  }

  .score-value {
    font-size: 1rem;
  }

  .score-label {
    font-size: 0.7rem;
  }

  .analyze-btn-compact {
    width: 28px;
    height: 28px;
    display: flex;
    align-items: center;
    justify-content: center;
    background: rgba(59, 130, 246, 0.2);
    border: 1px solid rgba(59, 130, 246, 0.3);
    border-radius: 6px;
    color: var(--color-info-soft);
    font-weight: 600;
    cursor: pointer;
    transition: all 0.2s;
  }

  .analyze-btn-compact:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .analyze-btn-compact:hover:not(:disabled) {
    background: rgba(59, 130, 246, 0.3);
  }

  .spinner-small {
    width: 12px;
    height: 12px;
    border: 2px solid rgba(147, 197, 253, 0.3);
    border-top-color: var(--color-info-soft);
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  .expanded-popup {
    position: absolute;
    top: 100%;
    right: 0;
    margin-top: 8px;
    width: 260px;
    background: var(--color-bg-base);
    border: 1px solid var(--color-border-default);
    border-radius: 8px;
    box-shadow: 0 10px 40px rgba(0, 0, 0, 0.4);
    z-index: 100;
    padding: 12px;
  }

  .popup-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 12px;
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .close-btn {
    background: none;
    border: none;
    color: var(--color-text-muted);
    cursor: pointer;
    font-size: 0.8rem;
  }

  .close-btn:hover {
    color: var(--color-text-primary);
  }

  .dimensions-grid {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 8px;
  }

  .dimension {
    display: flex;
    justify-content: space-between;
    font-size: 0.8rem;
  }

  .dim-label {
    color: var(--color-text-muted);
  }

  .dim-value {
    font-weight: 600;
  }

  .dim-value.good { color: var(--color-success-soft); }
  .dim-value.warning { color: var(--color-warning-soft); }
  .dim-value.poor { color: var(--color-danger-soft); }

  .issues-summary {
    margin-top: 12px;
    padding-top: 8px;
    border-top: 1px solid var(--color-border-default);
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  /* Full panel styles */
  .quality-panel {
    display: flex;
    flex-direction: column;
    gap: 16px;
  }

  .controls {
    display: flex;
    align-items: center;
    gap: 12px;
  }

  .analyze-btn {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 8px 16px;
    background: linear-gradient(135deg, var(--color-brand-gradient-end) 0%, var(--palette-violet-600) 100%);
    color: white;
    border: none;
    border-radius: 6px;
    font-weight: 500;
    cursor: pointer;
    transition: opacity 0.2s;
  }

  .analyze-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .analyze-btn:hover:not(:disabled) {
    opacity: 0.9;
  }

  .spinner {
    width: 14px;
    height: 14px;
    border: 2px solid rgba(59, 130, 246, 0.25);
    border-top-color: white;
    border-radius: 50%;
    animation: spin 0.8s linear infinite;
  }

  @keyframes spin {
    to { transform: rotate(360deg); }
  }

  .stats {
    font-size: 0.85rem;
    color: var(--color-text-muted);
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
    display: flex;
    flex-direction: column;
    gap: 20px;
  }

  .score-section {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 8px;
  }

  .score-circle {
    width: 80px;
    height: 80px;
    border-radius: 50%;
    display: flex;
    align-items: baseline;
    justify-content: center;
    gap: 2px;
  }

  .score-circle.good {
    background: linear-gradient(135deg, rgba(16, 185, 129, 0.3), rgba(16, 185, 129, 0.15));
    border: 3px solid rgba(16, 185, 129, 0.5);
    color: var(--color-success-soft);
  }

  .score-circle.warning {
    background: linear-gradient(135deg, rgba(234, 179, 8, 0.3), rgba(234, 179, 8, 0.15));
    border: 3px solid rgba(234, 179, 8, 0.5);
    color: var(--color-warning-soft);
  }

  .score-circle.poor {
    background: linear-gradient(135deg, rgba(239, 68, 68, 0.3), rgba(239, 68, 68, 0.15));
    border: 3px solid rgba(239, 68, 68, 0.5);
    color: var(--color-danger-soft);
  }

  .score-number {
    font-size: 1.75rem;
    font-weight: 700;
  }

  .score-percent {
    font-size: 0.9rem;
    font-weight: 500;
  }

  .score-title {
    font-size: 0.9rem;
    color: var(--color-text-muted);
  }

  .dimensions h4,
  .issues h4,
  .recommendations h4 {
    margin: 0 0 12px 0;
    font-size: 0.9rem;
    color: var(--color-text-primary);
  }

  .dimension-bars {
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .dimension-bar {
    display: grid;
    grid-template-columns: 100px 1fr 50px;
    align-items: center;
    gap: 10px;
  }

  .dim-name {
    font-size: 0.8rem;
    color: var(--color-text-tertiary);
  }

  .bar-track {
    height: 6px;
    background: var(--color-bg-active);
    border-radius: 3px;
    overflow: hidden;
  }

  .bar-fill {
    height: 100%;
    border-radius: 3px;
    transition: width 0.5s ease;
  }

  .bar-fill.good { background: var(--color-success); }
  .bar-fill.warning { background: var(--palette-yellow-500); }
  .bar-fill.poor { background: var(--color-danger); }

  .dim-score {
    font-size: 0.8rem;
    font-weight: 600;
    text-align: right;
    color: var(--color-text-secondary);
  }

  .issue-list,
  .rec-list {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
    max-height: 300px;
    overflow-y: auto;
  }

  .issue {
    padding: 10px 12px;
    border-radius: 6px;
    border-left: 3px solid;
  }

  .issue.high {
    background: rgba(239, 68, 68, 0.1);
    border-left-color: var(--color-danger);
  }

  .issue.medium {
    background: rgba(234, 179, 8, 0.1);
    border-left-color: var(--palette-yellow-500);
  }

  .issue.low {
    background: rgba(59, 130, 246, 0.1);
    border-left-color: var(--palette-blue-500);
  }

  .issue-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  .issue-severity {
    padding: 2px 6px;
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 600;
    text-transform: uppercase;
  }

  .issue.high .issue-severity { background: rgba(239, 68, 68, 0.3); color: var(--color-danger-soft); }
  .issue.medium .issue-severity { background: rgba(234, 179, 8, 0.3); color: var(--color-warning-soft); }
  .issue.low .issue-severity { background: rgba(59, 130, 246, 0.3); color: var(--color-info-soft); }

  .issue-dimension {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .issue-field {
    padding: 2px 6px;
    background: var(--color-bg-surface);
    border-radius: 4px;
    font-size: 0.75rem;
    font-family: monospace;
    color: var(--color-text-secondary);
  }

  .issue-description {
    font-size: 0.85rem;
    color: var(--color-text-secondary);
  }

  .issue-values {
    margin-top: 6px;
    display: flex;
    gap: 12px;
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .issue-values code {
    background: rgba(0, 0, 0, 0.3);
    padding: 2px 6px;
    border-radius: 4px;
  }

  .rec {
    padding: 10px 12px;
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-default);
    border-radius: 6px;
  }

  .rec-header {
    display: flex;
    align-items: center;
    gap: 8px;
    margin-bottom: 6px;
  }

  .rec-priority {
    padding: 2px 6px;
    background: rgba(168, 85, 247, 0.2);
    border-radius: 4px;
    font-size: 0.7rem;
    font-weight: 600;
    color: var(--color-accent-soft);
  }

  .rec-category {
    font-size: 0.8rem;
    color: var(--color-text-muted);
  }

  .rec-title {
    font-weight: 500;
    color: var(--color-text-primary);
  }

  .rec-description {
    font-size: 0.85rem;
    color: var(--color-text-tertiary);
  }

  .rec-impact {
    margin-top: 6px;
    font-size: 0.8rem;
    color: var(--color-text-muted);
    font-style: italic;
  }
</style>
