<script lang="ts">
  import { resolve } from '$app/paths';
  import Panel from '$lib/ui/Panel.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
</script>

<h1>fi-fhir Mapping Studio</h1>
<p class="sub">
  A workbench for turning messy interface data into typed, testable semantic events (and eventually Source Profiles).
</p>

<DashboardStats />

<div class="grid">
  <Panel title="Recent Events">
    <RecentEventsFeed />
  </Panel>

  <div class="sidebar">
    <SystemStatusPanel />

    <Panel title="Tools">
      <div class="tools">
        <a class="cta" href={resolve('/events')}>Event Dashboard</a>
        <a class="cta" href={resolve('/hl7')}>HL7 Preview & Triage</a>
        <a class="cta" href={resolve('/profiles')}>Profiles (YAML)</a>
        <a class="cta" href={resolve('/terminology')}>Terminology Mapping</a>
        <a class="cta" href={resolve('/workflows')}>Workflow Builder</a>
      </div>
    </Panel>
  </div>
</div>

<style>
  h1 {
    color: var(--color-text-primary);
    margin: 0 0 8px;
  }

  .sub {
    color: var(--color-text-secondary);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 70ch;
  }

  .grid {
    display: grid;
    gap: 14px;
    grid-template-columns: 1fr;
    margin-top: 14px;
  }

  @media (min-width: 980px) {
    .grid {
      grid-template-columns: 1.3fr 0.7fr;
      align-items: start;
    }
  }

  .sidebar {
    display: grid;
    gap: 14px;
  }

  .tools {
    display: grid;
    gap: 8px;
  }

  .cta {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    text-decoration: none;
    font-weight: 700;
  }

  .cta:hover {
    background: rgba(59, 130, 246, 0.22);
  }
</style>
