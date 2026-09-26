<script lang="ts">
  import type {
    AnalyzeQualityQuery,
    AnalyzeQualityQueryVariables,
    EventType
  } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';
  import { AnalyzeQualityDocument } from '$lib/gen/graphql';
  import { Badge, Button, IconButton, KeyValue, Panel, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import type { BadgeTone } from '$lib/ui/primitives';
  import Gauge from '@lucide/svelte/icons/gauge';
  import X from '@lucide/svelte/icons/x';
  import { createEventDispatcher } from 'svelte';

  export let event: Record<string, unknown> | null = null;
  export let eventType: EventType | null = null;
  export let compact: boolean = false;

  const dispatch = createEventDispatcher<{
    analyzed: AnalyzeQualityQuery['analyzeQuality'];
  }>();

  type QualityResult = AnalyzeQualityQuery['analyzeQuality'];
  type ScoreTone = Extract<BadgeTone, 'success' | 'warning' | 'danger'>;

  let isLoading = false;
  let error: string | null = null;
  let result: QualityResult | null = null;
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

  function scoreTone(score: number): ScoreTone {
    if (score >= 0.8) return 'success';
    if (score >= 0.5) return 'warning';
    return 'danger';
  }

  function scoreLabel(score: number): string {
    if (score >= 0.8) return 'Good';
    if (score >= 0.5) return 'Fair';
    return 'Poor';
  }

  function formatScore(score: number): string {
    return `${Math.round(score * 100)}`;
  }

  function severityTone(severity: string): BadgeTone {
    switch (severity.toLowerCase()) {
      case 'critical':
      case 'high':
        return 'danger';
      case 'medium':
        return 'warning';
      default:
        return 'info';
    }
  }

  function issueValues(issue: QualityResult['issues'][number]): string {
    return [
      issue.actualValue ? `actual ${issue.actualValue}` : '',
      issue.expectedValue ? `expected ${issue.expectedValue}` : ''
    ]
      .filter(Boolean)
      .join(' · ');
  }

  function dimensionRows(dimensions: QualityResult['dimensions']): { name: string; score: number }[] {
    return [
      { name: 'Completeness', score: dimensions.completeness },
      { name: 'Accuracy', score: dimensions.accuracy },
      { name: 'Consistency', score: dimensions.consistency },
      { name: 'Conformance', score: dimensions.conformance },
      { name: 'Timeliness', score: dimensions.timeliness }
    ];
  }

  $: hasIssues = (result?.issues?.length ?? 0) > 0;
  $: hasRecommendations = (result?.recommendations?.length ?? 0) > 0;
  $: sortedRecommendations = result ? [...result.recommendations].sort((a, b) => a.priority - b.priority) : [];
</script>

{#if compact}
  <!-- Compact badge view -->
  <div class="quality-compact">
    {#if result}
      <button
        type="button"
        class="score-chip tone-{scoreTone(result.overallScore)}"
        onclick={() => (expanded = !expanded)}
        title="Data quality score (click for details)"
        aria-expanded={expanded}
      >
        {formatScore(result.overallScore)}%
      </button>
    {:else}
      <IconButton
        icon={Gauge}
        label="Analyze data quality"
        variant="secondary"
        onclick={analyzeQuality}
        loading={isLoading}
        disabled={!event || !eventType}
      />
    {/if}

    {#if expanded && result}
      <div class="popup" role="dialog" aria-label="Data quality details">
        <div class="popup-head">
          <span class="popup-title">Data quality</span>
          <IconButton icon={X} label="Close" onclick={() => (expanded = false)} />
        </div>
        {#if result.dimensions}
          <KeyValue
            columns={2}
            items={dimensionRows(result.dimensions).map((d) => ({
              key: d.name,
              value: `${formatScore(d.score)}%`,
              mono: true
            }))}
          />
        {/if}
        {#if hasIssues}
          <p class="popup-foot">
            {result.issues.length} issue{result.issues.length !== 1 ? 's' : ''} found
          </p>
        {/if}
      </div>
    {/if}
  </div>
{:else}
  <!-- Full panel view -->
  <Panel title="Data quality" titleTag="h3" flush>
    {#snippet actions()}
      <Button
        variant="ghost"
        icon={Gauge}
        onclick={analyzeQuality}
        loading={isLoading}
        disabled={!event || !eventType}
      >
        {isLoading ? 'Analyzing' : 'Analyze quality'}
      </Button>
    {/snippet}

    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    {#if result}
      <div class="overall">
        <span class="overall-label">Overall</span>
        <span class="overall-score">{formatScore(result.overallScore)}%</span>
        <Badge tone={scoreTone(result.overallScore)} dot>{scoreLabel(result.overallScore)}</Badge>
        <span class="overall-time">{result.processingTimeMs ?? 0} ms</span>
      </div>

      {#if result.dimensions}
        <Table label="Quality dimensions" layout="fixed">
          {#snippet head()}
            <tr>
              <Th width="136px">Dimension</Th>
              <Th width="64px" numeric>Score</Th>
              <Th><span class="sr-only">Score bar</span></Th>
            </tr>
          {/snippet}
          {#each dimensionRows(result.dimensions) as d (d.name)}
            <Tr>
              <Td value={d.name} />
              <Td numeric value={`${formatScore(d.score)}%`} />
              <Td>
                <span class="bar" aria-hidden="true">
                  <span
                    class="bar-fill tone-{scoreTone(d.score)}"
                    style:width="{Math.max(0, Math.min(1, d.score)) * 100}%"
                  ></span>
                </span>
              </Td>
            </Tr>
          {/each}
        </Table>
      {/if}

      {#if hasIssues}
        <h4 class="section-label">Issues <Badge mono>{result.issues.length}</Badge></h4>
        <Table label="Quality issues" layout="fixed" class="quality-issues">
          {#snippet head()}
            <tr>
              <Th width="88px">Severity</Th>
              <Th width="112px">Dimension</Th>
              <Th width="120px">Field</Th>
              <Th>Description</Th>
            </tr>
          {/snippet}
          {#each result.issues as issue, idx (issue.description + idx)}
            <Tr>
              <Td><Badge tone={severityTone(issue.severity)}>{issue.severity}</Badge></Td>
              <Td muted truncate value={issue.dimension} />
              <Td mono truncate value={issue.field ?? ''} />
              <Td
                truncate
                title={issueValues(issue) ? `${issue.description} (${issueValues(issue)})` : issue.description}
              >
                {issue.description}
                {#if issueValues(issue)}
                  <span class="issue-values">{issueValues(issue)}</span>
                {/if}
              </Td>
            </Tr>
          {/each}
        </Table>
      {/if}

      {#if hasRecommendations}
        <h4 class="section-label">Recommendations</h4>
        <ul class="recs">
          {#each sortedRecommendations as rec, idx (rec.title + idx)}
            <li class="rec">
              <div class="rec-head">
                <Badge mono>P{rec.priority}</Badge>
                <span class="rec-title">{rec.title}</span>
                {#if rec.category}
                  <span class="rec-category">{rec.category}</span>
                {/if}
              </div>
              <p class="rec-text">
                {rec.description}
                {#if rec.impact}
                  <span class="rec-impact">Impact: {rec.impact}</span>
                {/if}
              </p>
            </li>
          {/each}
        </ul>
      {/if}
    {:else if !error}
      <p class="idle">
        Scores completeness, accuracy, consistency, conformance and timeliness for the first event.
      </p>
    {/if}
  </Panel>
{/if}

<style>
  /* Compact view */
  .quality-compact {
    position: relative;
    display: inline-block;
  }

  .score-chip {
    display: inline-flex;
    align-items: center;
    height: var(--size-control-sm);
    padding: 0 var(--space-2);
    border: 1px solid transparent;
    border-radius: var(--radius-sm);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
    cursor: pointer;
  }

  .score-chip:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .score-chip.tone-success {
    background: var(--color-success-bg);
    color: var(--color-success-text);
  }

  .score-chip.tone-warning {
    background: var(--color-warning-bg);
    color: var(--color-warning-text);
  }

  .score-chip.tone-danger {
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
  }

  .popup {
    position: absolute;
    top: 100%;
    right: 0;
    z-index: var(--z-popover);
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    width: 280px;
    margin-top: var(--space-1);
    padding: var(--space-2) var(--space-3) var(--space-3);
    background: var(--color-bg-elevated);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-md);
  }

  .popup-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
  }

  .popup-title {
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .popup-foot {
    margin: 0;
    padding-top: var(--space-2);
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  /* Full panel view */
  .error {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-danger-border);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .idle {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .overall {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    height: 40px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .overall-label {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .overall-score {
    font-family: var(--font-mono);
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-primary);
  }

  .overall-time {
    margin-left: auto;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .bar {
    display: block;
    height: 4px;
    background: var(--color-bg-active);
    border-radius: var(--radius-sm);
    overflow: hidden;
  }

  .bar-fill {
    display: block;
    height: 100%;
  }

  .bar-fill.tone-success {
    background: var(--color-success);
  }

  .bar-fill.tone-warning {
    background: var(--color-warning);
  }

  .bar-fill.tone-danger {
    background: var(--color-danger);
  }

  .section-label {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 32px;
    margin: 0;
    padding: 0 var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-label);
    font-weight: var(--font-semibold);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  /* The Table renders the wrapper, so this class is not scoped to us. */
  :global(.ui-table-wrap.quality-issues) {
    max-height: 300px;
  }

  .issue-values {
    margin-left: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .recs {
    margin: 0;
    padding: 0;
    list-style: none;
    border-top: 1px solid var(--color-border-default);
  }

  .rec {
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 6px var(--space-3);
  }

  .rec + .rec {
    border-top: 1px solid var(--color-border-subtle);
  }

  .rec-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .rec-title {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }

  .rec-category {
    margin-left: auto;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .rec-text {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .rec-impact {
    margin-left: var(--space-1);
    color: var(--color-text-tertiary);
  }
</style>
