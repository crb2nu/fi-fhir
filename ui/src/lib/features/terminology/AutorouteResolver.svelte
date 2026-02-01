<script lang="ts">
  /**
   * AutorouteResolver Component
   *
   * Resolves source codes to target terminology via persistent lookup
   * and LLM-powered autoroute suggestions.
   */

  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
  import Input from '$lib/ui/Input.svelte';
  import Select from '$lib/ui/Select.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import ConfidenceBadge from '$lib/ui/ConfidenceBadge.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import { toasts } from '$lib/ui/toastStore';
  import { resolveMapping, suggestMappings, createMapping } from './terminologyApi';
  import type {
    ResolveMappingQuery,
    SuggestMappingsQuery,
    AutorouteDecision,
    MappingEquivalence,
    MappingOrigin
  } from '$lib/gen/graphql';

  // Type aliases (internal use only, not exported)
  type ResolveResult = ResolveMappingQuery['resolveMapping'];
  type Candidate = SuggestMappingsQuery['suggestMappings'][number];

  // Props
  export let profileId: string | undefined = undefined;

  const dispatch = createEventDispatcher<{
    resolved: { result: ResolveResult };
    approved: { candidate: Candidate; mapping: unknown };
  }>();

  // Form state
  let sourceCode = '';
  let sourceSystem = '';
  let sourceDisplay = '';
  let targetSystem = 'http://loinc.org';
  let maxCandidates = 5;

  // Result state
  let loading = false;
  let result: ResolveResult | null = null;
  let candidates: Candidate[] = [];
  let error: string | null = null;

  // Approval state
  let approvingIndex: number | null = null;

  // Common target systems for quick selection
  const targetSystemOptions = [
    { value: 'http://loinc.org', label: 'LOINC' },
    { value: 'http://snomed.info/sct', label: 'SNOMED CT' },
    { value: 'http://hl7.org/fhir/sid/icd-10-cm', label: 'ICD-10-CM' },
    { value: 'http://www.nlm.nih.gov/research/umls/rxnorm', label: 'RxNorm' },
    { value: 'http://www.ama-assn.org/go/cpt', label: 'CPT' }
  ];

  async function handleResolve() {
    if (!sourceCode || !sourceSystem || !targetSystem) {
      toasts.error('Source code, source system, and target system are required');
      return;
    }

    loading = true;
    error = null;
    result = null;
    candidates = [];

    try {
      result = await resolveMapping({
        sourceCode,
        sourceSystem,
        sourceDisplay: sourceDisplay || null,
        targetSystem,
        profileId: profileId ?? null,
        allowAutoroute: null,
        minConfidence: null
      });

      // Extract candidates from result
      candidates = result.candidates;

      dispatch('resolved', { result });
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to resolve mapping';
    } finally {
      loading = false;
    }
  }

  async function handleSuggestOnly() {
    if (!sourceCode || !sourceSystem || !targetSystem) {
      toasts.error('Source code, source system, and target system are required');
      return;
    }

    loading = true;
    error = null;
    result = null;
    candidates = [];

    try {
      candidates = await suggestMappings({
        sourceCode,
        sourceSystem,
        sourceDisplay: sourceDisplay || null,
        targetSystem,
        maxCandidates
      });
    } catch (err) {
      error = err instanceof Error ? err.message : 'Failed to get suggestions';
    } finally {
      loading = false;
    }
  }

  async function approveCandidate(index: number) {
    const candidate = candidates[index];
    if (!candidate) return;

    approvingIndex = index;

    try {
      const mapping = await createMapping({
        sourceSystem,
        sourceCode,
        sourceDisplay: sourceDisplay || null,
        targetSystem: candidate.system,
        targetCode: candidate.code,
        targetDisplay: candidate.display,
        equivalence: (candidate.equivalence as MappingEquivalence) ?? 'EQUIVALENT',
        confidence: candidate.confidence,
        origin: 'APPROVED_AUTOROUTE' as MappingOrigin,
        profileId: profileId ?? null,
        comment: `Auto-approved from autoroute suggestion. Reasoning: ${candidate.reasoning ?? 'N/A'}`
      });

      toasts.success(`Mapping approved: ${sourceCode} → ${candidate.code}`);
      dispatch('approved', { candidate, mapping });
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Failed to approve mapping');
    } finally {
      approvingIndex = null;
    }
  }

  function getDecisionVariant(decision: AutorouteDecision): 'success' | 'warning' | 'danger' | 'default' {
    switch (decision) {
      case 'PERSISTENT_HIT':
      case 'AUTOROUTE_HIGH_CONF':
        return 'success';
      case 'AUTOROUTE_MED_CONF':
        return 'warning';
      case 'AUTOROUTE_LOW_CONF':
      case 'NO_MATCH':
        return 'danger';
      default:
        return 'default';
    }
  }

  function formatDecisionLabel(decision: AutorouteDecision): string {
    switch (decision) {
      case 'PERSISTENT_HIT':
        return 'Persistent Match';
      case 'AUTOROUTE_HIGH_CONF':
        return 'High Confidence';
      case 'AUTOROUTE_MED_CONF':
        return 'Medium Confidence';
      case 'AUTOROUTE_LOW_CONF':
        return 'Low Confidence';
      case 'NO_MATCH':
        return 'No Match';
      default:
        return decision;
    }
  }

  function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
</script>

<div class="resolver">
  <!-- Input Form -->
  <Panel title="Resolve Mapping">
    <div class="form-grid">
      <Input
        label="Source Code"
        bind:value={sourceCode}
        placeholder="e.g., LAB001"
        required
      />

      <Input
        label="Source System"
        bind:value={sourceSystem}
        placeholder="e.g., epic_custom_labs"
        required
      />

      <Input
        label="Source Display"
        bind:value={sourceDisplay}
        placeholder="e.g., Hemoglobin A1c Panel"
        hint="Optional - helps improve match quality"
      />

      <Select
        label="Target System"
        bind:value={targetSystem}
        options={targetSystemOptions}
        required
      />
    </div>

    <div class="form-actions">
      <Button on:click={handleResolve} {loading}>
        {loading ? 'Resolving...' : 'Resolve (Persistent + Autoroute)'}
      </Button>
      <Button variant="secondary" on:click={handleSuggestOnly} {loading}>
        {loading ? 'Loading...' : 'Suggest Only (Autoroute)'}
      </Button>
    </div>
  </Panel>

  <!-- Error -->
  {#if error}
    <Panel tone="error">
      <p class="error-text">{error}</p>
    </Panel>
  {/if}

  <!-- Result Summary -->
  {#if result}
    <Panel title="Resolution Result">
      <svelte:fragment slot="actions">
        <span class="duration">{formatDuration(result.durationMs)}</span>
      </svelte:fragment>

      <div class="result-header">
        <Badge variant={getDecisionVariant(result.decision)}>
          {formatDecisionLabel(result.decision)}
        </Badge>
        {#if result.confidence != null}
          <ConfidenceBadge confidence={result.confidence} />
        {/if}
      </div>

      {#if result.mapping}
        <div class="persistent-match">
          <div class="match-label">Persistent Mapping Found</div>
          <div class="match-codes">
            <span class="code">{result.mapping.sourceCode}</span>
            <span class="system">({result.mapping.sourceSystem})</span>
            <span class="arrow">→</span>
            <span class="code">{result.mapping.targetCode}</span>
            <span class="system">({result.mapping.targetSystem})</span>
          </div>
          {#if result.mapping.targetDisplay}
            <div class="match-display">{result.mapping.targetDisplay}</div>
          {/if}
        </div>
      {/if}

      {#if result.reasoning}
        <div class="reasoning">
          <div class="reasoning-label">Reasoning</div>
          <p class="reasoning-text">{result.reasoning}</p>
        </div>
      {/if}

      <!-- Decision Trace -->
      {#if result.trace && result.trace.steps.length > 0}
        <details class="trace">
          <summary class="trace-summary">
            Decision Trace ({result.trace.steps.length} steps)
          </summary>
          <div class="trace-steps">
            {#each result.trace.steps as step, i (i)}
              <div class="trace-step">
                <span class="step-name">{step.step}</span>
                <span class="step-arrow">→</span>
                <span class="step-result">{step.result}</span>
                <span class="step-duration">({step.durationMs}ms)</span>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    </Panel>
  {/if}

  <!-- Candidates -->
  {#if candidates.length > 0}
    <Panel title="Candidates ({candidates.length})">
      <div class="candidates">
        {#each candidates as candidate, index (candidate.code + candidate.system)}
          <div class="candidate">
            <div class="candidate-main">
              <div class="candidate-header">
                <span class="candidate-code">{candidate.code}</span>
                <ConfidenceBadge confidence={candidate.confidence} size="sm" />
                {#if candidate.equivalence}
                  <Badge variant="default" size="sm">{candidate.equivalence}</Badge>
                {/if}
              </div>
              <p class="candidate-display">{candidate.display}</p>
              <p class="candidate-system">{candidate.system}</p>
              {#if candidate.reasoning}
                <p class="candidate-reasoning">{candidate.reasoning}</p>
              {/if}
            </div>
            <Button
              size="sm"
              on:click={() => approveCandidate(index)}
              loading={approvingIndex === index}
            >
              {approvingIndex === index ? 'Saving...' : 'Approve'}
            </Button>
          </div>
        {/each}
      </div>
    </Panel>
  {:else if result && !result.mapping}
    <Panel tone="warning">
      <EmptyState
        icon="search"
        title="No candidates found"
        description="Try providing a more descriptive source display name to improve match quality."
        compact
      />
    </Panel>
  {/if}
</div>

<style>
  .resolver {
    display: flex;
    flex-direction: column;
    gap: var(--space-4);
  }

  /* Form */
  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, 1fr);
    gap: var(--space-4);
  }

  @media (max-width: 640px) {
    .form-grid {
      grid-template-columns: 1fr;
    }
  }

  .form-actions {
    display: flex;
    gap: var(--space-3);
    margin-top: var(--space-4);
    padding-top: var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }

  /* Error */
  .error-text {
    color: var(--color-danger-text);
    margin: 0;
  }

  /* Result */
  .result-header {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    margin-bottom: var(--space-4);
  }

  .duration {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  /* Persistent Match */
  .persistent-match {
    padding: var(--space-3);
    background: var(--color-success-bg);
    border: 1px solid var(--color-success-border);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-3);
  }

  .match-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-success-text);
    margin-bottom: var(--space-2);
  }

  .match-codes {
    display: flex;
    align-items: center;
    flex-wrap: wrap;
    gap: var(--space-1);
    font-size: var(--text-sm);
  }

  .code {
    font-family: var(--font-mono);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .system {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
  }

  .arrow {
    color: var(--color-text-muted);
    margin: 0 var(--space-1);
  }

  .match-display {
    font-size: var(--text-xs);
    color: var(--color-success-text);
    margin-top: var(--space-1);
  }

  /* Reasoning */
  .reasoning {
    padding: var(--space-3);
    background: var(--color-bg-elevated);
    border-radius: var(--radius-lg);
    margin-bottom: var(--space-3);
  }

  .reasoning-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-tertiary);
    margin-bottom: var(--space-1);
  }

  .reasoning-text {
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    margin: 0;
    line-height: var(--leading-relaxed);
  }

  /* Trace */
  .trace {
    margin-top: var(--space-3);
  }

  .trace-summary {
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    color: var(--color-text-tertiary);
    cursor: pointer;
    padding: var(--space-2);
    border-radius: var(--radius-md);
    transition: var(--transition-colors);
  }

  .trace-summary:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-secondary);
  }

  .trace-steps {
    margin-top: var(--space-2);
    padding-left: var(--space-4);
    border-left: 2px solid var(--color-border-default);
  }

  .trace-step {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-1) 0;
    font-size: var(--text-xs);
  }

  .step-name {
    font-family: var(--font-mono);
    color: var(--color-primary);
  }

  .step-arrow {
    color: var(--color-text-muted);
  }

  .step-result {
    color: var(--color-text-secondary);
  }

  .step-duration {
    color: var(--color-text-muted);
    margin-left: auto;
  }

  /* Candidates */
  .candidates {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .candidate {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-3);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    transition: var(--transition-all);
  }

  .candidate:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
    transform: translateY(-1px);
    box-shadow: var(--shadow-md);
  }

  .candidate-main {
    flex: 1;
    min-width: 0;
  }

  .candidate-header {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .candidate-code {
    font-family: var(--font-mono);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .candidate-display {
    font-size: var(--text-sm);
    color: var(--color-text-secondary);
    margin: 0 0 var(--space-1);
  }

  .candidate-system {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    margin: 0 0 var(--space-1);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .candidate-reasoning {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    font-style: italic;
    margin: var(--space-2) 0 0;
  }
</style>
