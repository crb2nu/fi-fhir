<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import Button from '$lib/ui/Button.svelte';
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
  const targetSystems = [
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

      toasts.success(`Mapping approved: ${sourceCode} -> ${candidate.code}`);
      dispatch('approved', { candidate, mapping });
    } catch (err) {
      toasts.error(err instanceof Error ? err.message : 'Failed to approve mapping');
    } finally {
      approvingIndex = null;
    }
  }

  function formatDecision(decision: AutorouteDecision): { label: string; color: string } {
    switch (decision) {
      case 'PERSISTENT_HIT':
        return { label: 'Persistent Match', color: 'text-green-600' };
      case 'AUTOROUTE_HIGH_CONF':
        return { label: 'High Confidence', color: 'text-green-500' };
      case 'AUTOROUTE_MED_CONF':
        return { label: 'Medium Confidence', color: 'text-yellow-600' };
      case 'AUTOROUTE_LOW_CONF':
        return { label: 'Low Confidence', color: 'text-orange-500' };
      case 'NO_MATCH':
        return { label: 'No Match', color: 'text-red-500' };
      default:
        return { label: decision, color: 'text-gray-500' };
    }
  }

  function formatConfidence(confidence: number): string {
    return `${(confidence * 100).toFixed(1)}%`;
  }

  function formatDuration(ms: number): string {
    if (ms < 1000) return `${ms}ms`;
    return `${(ms / 1000).toFixed(2)}s`;
  }
</script>

<div class="space-y-6">
  <!-- Input Form -->
  <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4 space-y-4">
    <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Resolve Mapping</h3>

    <div class="grid grid-cols-1 md:grid-cols-2 gap-4">
      <div>
        <label for="sourceCode" class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >Source Code *</label
        >
        <input
          id="sourceCode"
          type="text"
          bind:value={sourceCode}
          placeholder="e.g., LAB001"
          class="mt-1 block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 shadow-sm focus:border-blue-500 focus:ring-blue-500"
        />
      </div>

      <div>
        <label for="sourceSystem" class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >Source System *</label
        >
        <input
          id="sourceSystem"
          type="text"
          bind:value={sourceSystem}
          placeholder="e.g., epic_custom_labs"
          class="mt-1 block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 shadow-sm focus:border-blue-500 focus:ring-blue-500"
        />
      </div>

      <div>
        <label
          for="sourceDisplay"
          class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >Source Display (optional)</label
        >
        <input
          id="sourceDisplay"
          type="text"
          bind:value={sourceDisplay}
          placeholder="e.g., Hemoglobin A1c Panel"
          class="mt-1 block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 shadow-sm focus:border-blue-500 focus:ring-blue-500"
        />
      </div>

      <div>
        <label for="targetSystem" class="block text-sm font-medium text-gray-700 dark:text-gray-300"
          >Target System *</label
        >
        <select
          id="targetSystem"
          bind:value={targetSystem}
          class="mt-1 block w-full rounded-md border-gray-300 dark:border-gray-600 dark:bg-gray-700 shadow-sm focus:border-blue-500 focus:ring-blue-500"
        >
          {#each targetSystems as ts (ts.value)}
            <option value={ts.value}>{ts.label}</option>
          {/each}
        </select>
      </div>
    </div>

    <div class="flex gap-3">
      <Button on:click={handleResolve} disabled={loading}>
        {loading ? 'Resolving...' : 'Resolve (Persistent + Autoroute)'}
      </Button>
      <Button variant="secondary" on:click={handleSuggestOnly} disabled={loading}>
        {loading ? 'Loading...' : 'Suggest Only (Autoroute)'}
      </Button>
    </div>
  </div>

  <!-- Error -->
  {#if error}
    <div class="bg-red-50 dark:bg-red-900/20 border border-red-200 dark:border-red-800 rounded-lg p-4">
      <p class="text-red-700 dark:text-red-300">{error}</p>
    </div>
  {/if}

  <!-- Result Summary -->
  {#if result}
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4 space-y-3">
      <div class="flex items-center justify-between">
        <h3 class="text-lg font-semibold text-gray-900 dark:text-white">Resolution Result</h3>
        <span class="text-sm text-gray-500">{formatDuration(result.durationMs)}</span>
      </div>

      <div class="flex items-center gap-4">
        <span class={`font-medium ${formatDecision(result.decision).color}`}>
          {formatDecision(result.decision).label}
        </span>
        {#if result.confidence != null}
          <span class="text-sm text-gray-600 dark:text-gray-400">
            Confidence: {formatConfidence(result.confidence)}
          </span>
        {/if}
      </div>

      {#if result.mapping}
        <div class="bg-green-50 dark:bg-green-900/20 rounded p-3">
          <p class="text-sm font-medium text-green-800 dark:text-green-300">Persistent Mapping Found</p>
          <p class="text-sm text-green-700 dark:text-green-400">
            {result.mapping.sourceCode} ({result.mapping.sourceSystem}) →
            {result.mapping.targetCode} ({result.mapping.targetSystem})
          </p>
          {#if result.mapping.targetDisplay}
            <p class="text-sm text-green-600 dark:text-green-500">{result.mapping.targetDisplay}</p>
          {/if}
        </div>
      {/if}

      {#if result.reasoning}
        <div class="bg-gray-50 dark:bg-gray-700 rounded p-3">
          <p class="text-sm font-medium text-gray-700 dark:text-gray-300">Reasoning</p>
          <p class="text-sm text-gray-600 dark:text-gray-400">{result.reasoning}</p>
        </div>
      {/if}

      <!-- Decision Trace -->
      {#if result.trace && result.trace.steps.length > 0}
        <details class="mt-2">
          <summary class="cursor-pointer text-sm font-medium text-gray-700 dark:text-gray-300">
            Decision Trace ({result.trace.steps.length} steps)
          </summary>
          <div class="mt-2 space-y-1 pl-4 border-l-2 border-gray-200 dark:border-gray-600">
            {#each result.trace.steps as step, i (i)}
              <div class="text-sm">
                <span class="font-mono text-blue-600 dark:text-blue-400">{step.step}</span>
                <span class="text-gray-500"> → </span>
                <span class="text-gray-700 dark:text-gray-300">{step.result}</span>
                <span class="text-gray-400 text-xs ml-2">({step.durationMs}ms)</span>
              </div>
            {/each}
          </div>
        </details>
      {/if}
    </div>
  {/if}

  <!-- Candidates -->
  {#if candidates.length > 0}
    <div class="bg-white dark:bg-gray-800 rounded-lg shadow p-4 space-y-3">
      <h3 class="text-lg font-semibold text-gray-900 dark:text-white">
        Candidates ({candidates.length})
      </h3>

      <div class="space-y-3">
        {#each candidates as candidate, index (candidate.code + candidate.system)}
          <div
            class="border border-gray-200 dark:border-gray-600 rounded-lg p-3 hover:bg-gray-50 dark:hover:bg-gray-700"
          >
            <div class="flex items-start justify-between">
              <div class="flex-1">
                <div class="flex items-center gap-2">
                  <span class="font-mono font-medium text-gray-900 dark:text-white">
                    {candidate.code}
                  </span>
                  <span
                    class="px-2 py-0.5 text-xs font-medium rounded-full
                      {candidate.confidence >= 0.9
                      ? 'bg-green-100 text-green-800 dark:bg-green-900 dark:text-green-200'
                      : candidate.confidence >= 0.7
                        ? 'bg-yellow-100 text-yellow-800 dark:bg-yellow-900 dark:text-yellow-200'
                        : 'bg-orange-100 text-orange-800 dark:bg-orange-900 dark:text-orange-200'}"
                  >
                    {formatConfidence(candidate.confidence)}
                  </span>
                  {#if candidate.equivalence}
                    <span class="text-xs text-gray-500">{candidate.equivalence}</span>
                  {/if}
                </div>
                <p class="text-sm text-gray-700 dark:text-gray-300 mt-1">{candidate.display}</p>
                <p class="text-xs text-gray-500 mt-0.5">{candidate.system}</p>
                {#if candidate.reasoning}
                  <p class="text-xs text-gray-500 mt-1 italic">{candidate.reasoning}</p>
                {/if}
              </div>
              <Button
                size="sm"
                on:click={() => approveCandidate(index)}
                disabled={approvingIndex === index}
              >
                {approvingIndex === index ? 'Saving...' : 'Approve'}
              </Button>
            </div>
          </div>
        {/each}
      </div>
    </div>
  {:else if result && !result.mapping}
    <div class="bg-yellow-50 dark:bg-yellow-900/20 border border-yellow-200 dark:border-yellow-800 rounded-lg p-4">
      <p class="text-yellow-700 dark:text-yellow-300">
        No candidates found. Try providing a more descriptive source display name.
      </p>
    </div>
  {/if}
</div>
