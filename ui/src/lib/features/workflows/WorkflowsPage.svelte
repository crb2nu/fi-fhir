<script lang="ts">
  import { resolve } from '$app/paths';
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import WorkflowList from './components/WorkflowList.svelte';
  import WorkflowBuilder from './components/WorkflowBuilder.svelte';
  import WorkflowMonitor from './components/WorkflowMonitor.svelte';

  const tabs: readonly TabItem[] = [
    { key: 'list', label: 'Inventory' },
    { key: 'builder', label: 'Design' },
    { key: 'monitor', label: 'Verification' }
  ];

  let activeTab = 'builder';
  let builderSelection:
    | {
        workflowId: string;
        name: string;
        description: string | null;
        versionId: string | null;
        versionNumber: number | null;
      }
    | null = null;
  let monitorWorkflowSelection: string | null = null;

  function handleOpenBuilder(
    event: CustomEvent<{
      workflowId: string;
      name: string;
      description: string | null;
      versionId: string | null;
      versionNumber: number | null;
    }>
  ) {
    builderSelection = event.detail;
    activeTab = 'builder';
  }

  function handleOpenMonitor(event: CustomEvent<{ workflowName: string }>) {
    monitorWorkflowSelection = event.detail.workflowName;
    activeTab = 'monitor';
  }
</script>

<section class="page">
  <header class="hero">
    <div class="hero-copy">
      <p class="eyebrow">Destination workbench</p>
      <h1>Design workflow destinations, then verify the handoff.</h1>
      <p class="sub">
        Start from mapped source behavior, shape the workflow that receives it, and confirm the
        downstream result with runtime output and event review.
      </p>

      <div class="hero-links">
        <a class="hero-link" href={resolve('/hl7')}>Open mapping studio</a>
        <span class="hero-note">Use the mapping workspace to prepare the source side before you refine routing here.</span>
      </div>
    </div>

    <div class="handoff-card" aria-label="Workflow handoff overview">
      <div class="handoff-step">
        <span class="step-number">1</span>
        <div>
          <div class="step-title">Map the source feed</div>
          <div class="step-copy">Normalize HL7, flatfile, or EDI inputs before they reach the destination workflow.</div>
        </div>
      </div>

      <div class="handoff-step">
        <span class="step-number">2</span>
        <div>
          <div class="step-title">Shape the destination</div>
          <div class="step-copy">Inventory keeps the catalog, Design edits routes and transforms, and Verification preserves the current behavior.</div>
        </div>
      </div>

      <div class="handoff-step">
        <span class="step-number">3</span>
        <div>
          <div class="step-title">Verify the handoff</div>
          <div class="step-copy">Watch runtime output and downstream events to confirm what actually executed.</div>
        </div>
      </div>
    </div>
  </header>

  <div class="workspace-frame">
    <div class="tabs-shell">
      <div class="tabs-copy">
        <p class="eyebrow">Workflow stages</p>
        <p class="tabs-note">
          Move from the workflow inventory to design changes, then finish in verification when you
          want to inspect the result.
        </p>
      </div>

      <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />
    </div>

    <div class="workspace">
      {#if activeTab === 'list'}
        <WorkflowList on:openBuilder={handleOpenBuilder} on:openMonitor={handleOpenMonitor} />
      {:else if activeTab === 'builder'}
        <WorkflowBuilder managedSelection={builderSelection} />
      {:else if activeTab === 'monitor'}
        <WorkflowMonitor initialWorkflowName={monitorWorkflowSelection} />
      {/if}
    </div>
  </div>
</section>

<style>
  .page {
    display: grid;
    gap: 20px;
  }

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.3fr) minmax(300px, 0.9fr);
    gap: 20px;
    align-items: stretch;
  }

  .hero-copy {
    padding: 24px;
    border-radius: 24px;
    border: 1px solid var(--color-border-subtle);
    background:
      radial-gradient(circle at top right, rgba(59, 130, 246, 0.14), transparent 36%),
      linear-gradient(180deg, rgba(15, 23, 42, 0.3), rgba(15, 23, 42, 0.18));
    box-shadow: var(--shadow-sm);
  }

  .eyebrow {
    margin: 0 0 10px;
    text-transform: uppercase;
    letter-spacing: 0.14em;
    font-size: 0.72rem;
    font-weight: 700;
    color: var(--color-text-tertiary);
  }

  h1 {
    margin: 0 0 10px;
    color: var(--color-text-primary);
    font-size: clamp(2rem, 4vw, 3.1rem);
    line-height: 1.05;
    letter-spacing: -0.04em;
    max-width: 11ch;
  }

  .sub {
    margin: 0;
    max-width: 66ch;
    color: var(--color-text-secondary);
    line-height: 1.6;
    font-size: 1rem;
  }

  .hero-links {
    display: flex;
    align-items: center;
    gap: 12px;
    flex-wrap: wrap;
    margin-top: 18px;
  }

  .hero-link {
    display: inline-flex;
    align-items: center;
    padding: 8px 12px;
    border-radius: 999px;
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    text-decoration: none;
    font-weight: 700;
  }

  .hero-link:hover,
  .hero-link:focus-visible {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
    outline: none;
  }

  .hero-note {
    color: var(--color-text-tertiary);
    font-size: 0.9rem;
    line-height: 1.45;
    max-width: 36ch;
  }

  .handoff-card {
    display: grid;
    gap: 14px;
    padding: 20px;
    border-radius: 24px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .handoff-step {
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 12px;
    align-items: start;
  }

  .step-number {
    width: 28px;
    height: 28px;
    border-radius: 999px;
    display: inline-flex;
    align-items: center;
    justify-content: center;
    font-size: 0.85rem;
    font-weight: 700;
    color: var(--color-primary);
    background: var(--color-primary-muted);
    border: 1px solid var(--color-primary-border);
  }

  .step-title {
    color: var(--color-text-primary);
    font-weight: 700;
    margin-bottom: 4px;
  }

  .step-copy {
    color: var(--color-text-secondary);
    font-size: 0.92rem;
    line-height: 1.5;
  }

  .workspace-frame {
    display: grid;
    gap: 16px;
    padding: 20px;
    border-radius: 24px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
  }

  .tabs-shell {
    display: grid;
    gap: 12px;
  }

  .tabs-copy {
    display: grid;
    gap: 4px;
  }

  .tabs-note {
    margin: 0;
    color: var(--color-text-tertiary);
    line-height: 1.5;
  }

  .workspace {
    min-width: 0;
  }

  @media (max-width: 960px) {
    .hero {
      grid-template-columns: 1fr;
    }
  }
</style>
