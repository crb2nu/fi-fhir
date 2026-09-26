<script lang="ts">
  import type {
    ExtractEntitiesQuery,
    ExtractEntitiesQueryVariables
  } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';
  import { ExtractEntitiesDocument } from '$lib/gen/graphql';
  import { Badge, Button, EmptyState, Panel, Table, Tabs, Td, Th, Tr } from '$lib/ui/primitives';
  import type { BadgeTone, TabItem } from '$lib/ui/primitives';
  import ScanText from '@lucide/svelte/icons/scan-text';
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

  type Category = 'conditions' | 'medications' | 'vitals' | 'allergies' | 'procedures';

  let isLoading = false;
  let error: string | null = null;
  let result: ExtractEntitiesQuery['extractEntities'] | null = null;
  let activeTab: Category = 'conditions';

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

  function severityTone(severity: string): BadgeTone {
    switch (severity.toLowerCase()) {
      case 'moderate':
        return 'warning';
      case 'severe':
        return 'danger';
      default:
        return 'neutral';
    }
  }

  function code(system: string | null | undefined, value: string | null | undefined): string {
    if (!value) return '';
    return system ? `${system}: ${value}` : value;
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
  let categoryTabs: TabItem[] = [];
  $: categoryTabs = [
    { id: 'conditions', label: 'Conditions', count: result?.conditions?.length ?? 0, controls: 'extraction-category' },
    { id: 'medications', label: 'Medications', count: result?.medications?.length ?? 0, controls: 'extraction-category' },
    { id: 'vitals', label: 'Vitals', count: result?.vitalSigns?.length ?? 0, controls: 'extraction-category' },
    { id: 'allergies', label: 'Allergies', count: result?.allergies?.length ?? 0, controls: 'extraction-category' },
    { id: 'procedures', label: 'Procedures', count: result?.procedures?.length ?? 0, controls: 'extraction-category' }
  ];
</script>

<Panel title="Clinical entity extraction" titleTag="h3" flush>
  {#snippet actions()}
    <Button icon={ScanText} onclick={extractEntities} loading={isLoading} disabled={!text.trim()}>
      {isLoading ? 'Extracting' : 'Extract entities'}
    </Button>
  {/snippet}

  <div class="extraction">
    {#if error}
      <p class="error" role="alert">{error}</p>
    {/if}

    {#if result}
      <p class="summary">
        <span><span class="num">{totalEntities}</span> entities</span>
        <span class="sep" aria-hidden="true">·</span>
        <span>
          <span class="num conf-{confidenceClass(result.overallConfidence)}">
            {formatConfidence(result.overallConfidence)}
          </span>
          confidence
        </span>
        <span class="sep" aria-hidden="true">·</span>
        <span><span class="num">{result.processingTimeMs}</span> ms</span>
      </p>

      <div class="category-tabs">
        <Tabs
          label="Entity categories"
          items={categoryTabs}
          value={activeTab}
          onchange={(id) => (activeTab = id as Category)}
        />
      </div>

      <div class="category" id="extraction-category" role="tabpanel">
        {#if activeTab === 'conditions'}
          {#if hasConditions}
            <Table label="Conditions" layout="fixed" class="extraction-table">
              {#snippet head()}
                <tr>
                  <Th>Name</Th>
                  <Th width="132px">Code</Th>
                  <Th width="120px">Status</Th>
                  <Th width="60px" numeric>Conf</Th>
                  <Th>Text</Th>
                </tr>
              {/snippet}
              {#each result.conditions as condition, idx (condition.name + idx)}
                <Tr>
                  <Td truncate value={condition.name} />
                  <Td mono truncate value={code(condition.codeSystem, condition.code)} />
                  <Td>
                    {#if condition.negated}
                      <Badge tone="warning">negated</Badge>
                    {/if}
                    {#if condition.status}
                      <Badge>{condition.status}</Badge>
                    {/if}
                  </Td>
                  <Td
                    numeric
                    class="conf-{confidenceClass(condition.confidence)}"
                    value={formatConfidence(condition.confidence)}
                  />
                  <Td mono muted truncate value={condition.textSpan ?? ''} />
                </Tr>
              {/each}
            </Table>
          {:else}
            <EmptyState align="start" message="No conditions found." />
          {/if}
        {:else if activeTab === 'medications'}
          {#if hasMedications}
            <Table label="Medications" layout="fixed" class="extraction-table">
              {#snippet head()}
                <tr>
                  <Th>Name</Th>
                  <Th width="108px">Dose</Th>
                  <Th width="96px">Frequency</Th>
                  <Th width="120px">Code</Th>
                  <Th width="60px" numeric>Conf</Th>
                  <Th>Text</Th>
                </tr>
              {/snippet}
              {#each result.medications as med, idx (med.name + idx)}
                <Tr>
                  <Td truncate value={med.name} />
                  <Td mono truncate value={[med.dose, med.route].filter(Boolean).join(' ')} />
                  <Td muted truncate value={med.frequency ?? ''} />
                  <Td mono truncate value={code(med.codeSystem, med.code)} />
                  <Td numeric class="conf-{confidenceClass(med.confidence)}" value={formatConfidence(med.confidence)} />
                  <Td mono muted truncate value={med.textSpan ?? ''} />
                </Tr>
              {/each}
            </Table>
          {:else}
            <EmptyState align="start" message="No medications found." />
          {/if}
        {:else if activeTab === 'vitals'}
          {#if hasVitalSigns}
            <Table label="Vital signs" layout="fixed" class="extraction-table">
              {#snippet head()}
                <tr>
                  <Th>Name</Th>
                  <Th width="108px">Value</Th>
                  <Th width="108px">Interpretation</Th>
                  <Th width="96px">LOINC</Th>
                  <Th width="60px" numeric>Conf</Th>
                  <Th>Text</Th>
                </tr>
              {/snippet}
              {#each result.vitalSigns as vital, idx (vital.name + idx)}
                <Tr>
                  <Td truncate value={vital.name} />
                  <Td mono truncate value={vital.unit ? `${vital.value} ${vital.unit}` : vital.value} />
                  <Td>
                    {#if vital.interpretation}
                      <Badge>{vital.interpretation}</Badge>
                    {/if}
                  </Td>
                  <Td mono truncate value={vital.loincCode ?? ''} />
                  <Td numeric class="conf-{confidenceClass(vital.confidence)}" value={formatConfidence(vital.confidence)} />
                  <Td mono muted truncate value={vital.textSpan ?? ''} />
                </Tr>
              {/each}
            </Table>
          {:else}
            <EmptyState align="start" message="No vital signs found." />
          {/if}
        {:else if activeTab === 'allergies'}
          {#if hasAllergies}
            <Table label="Allergies" layout="fixed" class="extraction-table">
              {#snippet head()}
                <tr>
                  <Th>Substance</Th>
                  <Th width="96px">Severity</Th>
                  <Th width="108px">Reaction</Th>
                  <Th width="120px">Code</Th>
                  <Th width="60px" numeric>Conf</Th>
                  <Th>Text</Th>
                </tr>
              {/snippet}
              {#each result.allergies as allergy, idx (allergy.substance + idx)}
                <Tr>
                  <Td truncate value={allergy.substance} />
                  <Td>
                    {#if allergy.severity}
                      <Badge tone={severityTone(allergy.severity)}>{allergy.severity}</Badge>
                    {/if}
                  </Td>
                  <Td muted truncate value={allergy.reaction ?? ''} />
                  <Td mono truncate value={code(allergy.codeSystem, allergy.code)} />
                  <Td
                    numeric
                    class="conf-{confidenceClass(allergy.confidence)}"
                    value={formatConfidence(allergy.confidence)}
                  />
                  <Td mono muted truncate value={allergy.textSpan ?? ''} />
                </Tr>
              {/each}
            </Table>
          {:else}
            <EmptyState align="start" message="No allergies found." />
          {/if}
        {:else if hasProcedures}
          <Table label="Procedures" layout="fixed" class="extraction-table">
            {#snippet head()}
              <tr>
                <Th>Name</Th>
                <Th width="108px">Status</Th>
                <Th width="132px">Code</Th>
                <Th width="60px" numeric>Conf</Th>
                <Th>Text</Th>
              </tr>
            {/snippet}
            {#each result.procedures as procedure, idx (procedure.name + idx)}
              <Tr>
                <Td truncate value={procedure.name} />
                <Td>
                  {#if procedure.status}
                    <Badge>{procedure.status}</Badge>
                  {/if}
                </Td>
                <Td mono truncate value={code(procedure.codeSystem, procedure.code)} />
                <Td
                  numeric
                  class="conf-{confidenceClass(procedure.confidence)}"
                  value={formatConfidence(procedure.confidence)}
                />
                <Td mono muted truncate value={procedure.textSpan ?? ''} />
              </Tr>
            {/each}
          </Table>
        {:else}
          <EmptyState align="start" message="No procedures found." />
        {/if}
      </div>
    {:else if !error}
      <p class="idle">
        Extracts conditions, medications, vitals, allergies and procedures from the message text at
        {formatConfidence(minConfidence)} confidence or higher.
      </p>
    {/if}
  </div>
</Panel>

<style>
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

  .summary {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2) var(--space-3) 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .num {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    font-variant-numeric: tabular-nums;
    color: var(--color-text-primary);
  }

  .sep {
    color: var(--color-text-muted);
  }

  .category-tabs {
    display: flex;
    height: 32px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .extraction :global(.extraction-table) {
    max-height: 400px;
  }

  /* Low confidence reads as a state; high confidence stays neutral. */
  .num.conf-medium,
  .extraction :global(.conf-medium) {
    color: var(--color-warning-text);
  }

  .num.conf-low,
  .extraction :global(.conf-low) {
    color: var(--color-danger-text);
  }

  .extraction :global(td .ui-badge + .ui-badge) {
    margin-left: var(--space-1);
  }
</style>
