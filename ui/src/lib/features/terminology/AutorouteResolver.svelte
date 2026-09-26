<script lang="ts">
  /**
   * AutorouteResolver Component
   *
   * Resolves source codes to target terminology via persistent lookup
   * and LLM-powered autoroute suggestions.
   */

  import { createEventDispatcher } from "svelte";
  import Check from "@lucide/svelte/icons/check";
  import CircleAlert from "@lucide/svelte/icons/circle-alert";
  import Search from "@lucide/svelte/icons/search";
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    Icon,
    Input,
    KeyValue,
    Panel,
    Select,
    Table,
    Td,
    Th,
    Tr,
    type KeyValueItem,
  } from "$lib/ui/primitives";
  import { toasts } from "$lib/ui/toastStore";
  import { isErrorToasted } from "$lib/graphql/client";
  import {
    resolveMapping,
    suggestMappings,
    createMapping,
  } from "./terminologyApi";
  import { validateResolveInputs } from "./resolveValidation";
  import {
    confidenceTone,
    decisionLabel,
    decisionTone,
    equivalenceLabel,
    formatDurationMs,
    formatPercent,
  } from "./terminologyFormat";
  import type {
    ResolveMappingQuery,
    SuggestMappingsQuery,
    MappingEquivalence,
    MappingOrigin,
  } from "$lib/gen/graphql";

  // Type aliases (internal use only, not exported)
  type ResolveResult = ResolveMappingQuery["resolveMapping"];
  type Candidate = SuggestMappingsQuery["suggestMappings"][number];

  // Props
  export let profileId: string | undefined = undefined;

  const dispatch = createEventDispatcher<{
    resolved: { result: ResolveResult };
    approved: { candidate: Candidate; mapping: unknown };
  }>();

  // Form state
  let sourceCode = "";
  let sourceSystem = "";
  let sourceDisplay = "";
  let targetSystem = "http://loinc.org";
  let maxCandidates = 5;

  // Result state
  let loading = false;
  let result: ResolveResult | null = null;
  let candidates: Candidate[] = [];
  let error: string | null = null;
  // Which request produced the current results (null until one succeeds), so an
  // empty suggestion list reads as "no candidates", not as "nothing run yet".
  let lastAction: "resolve" | "suggest" | null = null;

  // Approval state
  let approvingIndex: number | null = null;

  // Common target systems for quick selection
  const targetSystemOptions = [
    { value: "http://loinc.org", label: "LOINC" },
    { value: "http://snomed.info/sct", label: "SNOMED CT" },
    { value: "http://hl7.org/fhir/sid/icd-10-cm", label: "ICD-10-CM" },
    { value: "http://www.nlm.nih.gov/research/umls/rxnorm", label: "RxNorm" },
    { value: "http://www.ama-assn.org/go/cpt", label: "CPT" },
  ];

  $: resultItems = result ? summaryItems(result) : [];

  async function handleResolve() {
    const validationError = validateResolveInputs({
      sourceCode,
      sourceSystem,
      targetSystem,
    });
    if (validationError) {
      error = validationError;
      return;
    }

    loading = true;
    error = null;
    result = null;
    candidates = [];
    lastAction = null;

    try {
      result = await resolveMapping({
        sourceCode,
        sourceSystem,
        sourceDisplay: sourceDisplay || null,
        targetSystem,
        profileId: profileId ?? null,
        allowAutoroute: null,
        minConfidence: null,
      });

      // Extract candidates from result
      candidates = result.candidates;
      lastAction = "resolve";

      dispatch("resolved", { result });
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to resolve mapping";
    } finally {
      loading = false;
    }
  }

  async function handleSuggestOnly() {
    const validationError = validateResolveInputs({
      sourceCode,
      sourceSystem,
      targetSystem,
    });
    if (validationError) {
      error = validationError;
      return;
    }

    loading = true;
    error = null;
    result = null;
    candidates = [];
    lastAction = null;

    try {
      candidates = await suggestMappings({
        sourceCode,
        sourceSystem,
        sourceDisplay: sourceDisplay || null,
        targetSystem,
        maxCandidates,
      });
      lastAction = "suggest";
    } catch (err) {
      error = err instanceof Error ? err.message : "Failed to get suggestions";
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
        equivalence:
          (candidate.equivalence as MappingEquivalence) ?? "EQUIVALENT",
        confidence: candidate.confidence,
        origin: "APPROVED_AUTOROUTE" as MappingOrigin,
        profileId: profileId ?? null,
        comment: `Auto-approved from autoroute suggestion. Reasoning: ${candidate.reasoning ?? "N/A"}`,
      });

      toasts.success(`Mapping approved: ${sourceCode} → ${candidate.code}`);
      dispatch("approved", { candidate, mapping });
    } catch (err) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(err)) {
        toasts.error(
          err instanceof Error ? err.message : "Failed to approve mapping",
        );
      }
    } finally {
      approvingIndex = null;
    }
  }

  function summaryItems(res: ResolveResult): KeyValueItem[] {
    const items: KeyValueItem[] = [
      { key: "Confidence", value: formatPercent(res.confidence), mono: true },
    ];
    if (res.mapping) {
      items.push(
        {
          key: "Persistent mapping",
          value: `${res.mapping.sourceCode} → ${res.mapping.targetCode}`,
          mono: true,
        },
        { key: "Source system", value: res.mapping.sourceSystem, mono: true },
        { key: "Target system", value: res.mapping.targetSystem, mono: true },
        { key: "Target display", value: res.mapping.targetDisplay },
      );
    }
    if (res.reasoning) {
      items.push({ key: "Reasoning", value: res.reasoning });
    }
    return items;
  }
</script>

<div class="resolver">
  <Panel title="Resolve mapping">
    <form class="form" on:submit|preventDefault={handleResolve} novalidate>
      <div class="form-grid">
        <Field label="Source code" required>
          <Input bind:value={sourceCode} placeholder="e.g., LAB001" mono />
        </Field>

        <Field label="Source system" required>
          <Input
            bind:value={sourceSystem}
            placeholder="e.g., epic_custom_labs"
            mono
          />
        </Field>

        <Field label="Source display" hint="Optional; improves match quality.">
          <Input
            bind:value={sourceDisplay}
            placeholder="e.g., Hemoglobin A1c Panel"
          />
        </Field>

        <Field label="Target system" required>
          <Select bind:value={targetSystem} options={targetSystemOptions} />
        </Field>
      </div>

      {#if error}
        <p class="form-error" role="alert">
          <Icon icon={CircleAlert} />
          <span>{error}</span>
        </p>
      {/if}

      <div class="form-actions">
        <Button
          type="submit"
          variant="primary"
          {loading}
          title="Persistent lookup, then autoroute"
        >
          Resolve
        </Button>
        <Button
          onclick={handleSuggestOnly}
          disabled={loading}
          title="Autoroute suggestions only"
        >
          Suggest only
        </Button>
      </div>
    </form>
  </Panel>

  <div class="results">
    {#if result}
      <Panel title="Resolution">
        {#snippet actions()}
          <span class="panel-meta text-mono"
            >{formatDurationMs(result?.durationMs)}</span
          >
        {/snippet}
        <div class="result-head">
          <Badge tone={decisionTone(result.decision)} dot>
            {decisionLabel(result.decision)}
          </Badge>
        </div>
        <KeyValue items={resultItems} />
      </Panel>

      {#if result.trace && result.trace.steps.length > 0}
        <Panel title="Decision trace ({result.trace.steps.length} steps)" flush>
          <Table label="Decision trace" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="200px">Step</Th>
                <Th>Result</Th>
                <Th width="80px" numeric>ms</Th>
              </tr>
            {/snippet}
            {#each result.trace.steps as step, i (i)}
              <Tr>
                <Td mono truncate value={step.step} />
                <Td truncate value={step.result} />
                <Td numeric value={step.durationMs} />
              </Tr>
            {/each}
          </Table>
        </Panel>
      {/if}
    {/if}

    {#if candidates.length > 0}
      <Panel title="Candidates ({candidates.length})" flush>
        <Table label="Candidates" layout="fixed">
          {#snippet head()}
            <tr>
              <Th width="112px">Code</Th>
              <Th>Display</Th>
              <Th>System</Th>
              <Th width="88px" numeric>Confidence</Th>
              <Th width="104px">Equivalence</Th>
              <Th width="104px"><span class="sr-only">Actions</span></Th>
            </tr>
          {/snippet}
          {#each candidates as candidate, index (candidate.code + candidate.system)}
            <Tr>
              <Td mono truncate value={candidate.code} />
              <Td
                truncate
                value={candidate.display}
                title={candidate.reasoning
                  ? `${candidate.display} — ${candidate.reasoning}`
                  : candidate.display}
              />
              <Td muted truncate value={candidate.system} />
              <Td numeric>
                <Badge tone={confidenceTone(candidate.confidence)} mono
                  >{formatPercent(candidate.confidence)}</Badge
                >
              </Td>
              <Td truncate value={equivalenceLabel(candidate.equivalence) || "—"} />
              <Td class="cell-action">
                <Button
                  icon={Check}
                  onclick={() => approveCandidate(index)}
                  loading={approvingIndex === index}
                  aria-label="Approve {candidate.code}"
                >
                  Approve
                </Button>
              </Td>
            </Tr>
          {/each}
        </Table>
      </Panel>
    {:else if (result && !result.mapping) || lastAction === "suggest"}
      <Panel title="Candidates">
        <EmptyState
          icon={Search}
          align="start"
          message="No candidates found for this code."
        />
      </Panel>
    {/if}

    {#if lastAction === null && !loading}
      <EmptyState
        align="start"
        message="Resolve a source code to see the decision, its trace and the candidates."
      />
    {/if}
  </div>
</div>

<style>
  .resolver {
    display: grid;
    grid-template-columns: minmax(0, 520px) minmax(0, 1fr);
    align-items: start;
    gap: var(--space-3);
    padding: var(--space-3);
  }

  @media (max-width: 1100px) {
    .resolver {
      grid-template-columns: minmax(0, 1fr);
    }
  }

  .form {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .form-error {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .form-error :global(.ui-icon) {
    margin-top: 1px;
  }

  .form-actions {
    display: flex;
    gap: var(--space-2);
  }

  .results {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-width: 0;
  }

  .panel-meta {
    padding-right: var(--space-2);
    color: var(--color-text-tertiary);
  }

  .result-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
  }

  .results :global(.cell-action) {
    text-align: right;
  }
</style>
