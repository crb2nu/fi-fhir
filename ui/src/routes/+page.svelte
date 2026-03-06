<script lang="ts">
  import { resolve } from "$app/paths";
  import Panel from "$lib/ui/Panel.svelte";
  import SystemStatusPanel from "$lib/features/system/SystemStatusPanel.svelte";
  import DashboardStats from "$lib/features/dashboard/DashboardStats.svelte";
  import RecentEventsFeed from "$lib/features/dashboard/RecentEventsFeed.svelte";
</script>

<h1 class="text-gradient">fi-fhir Mapping Studio</h1>
<p class="sub animate-slide-in-up">
  A workbench for turning messy interface data into typed, testable semantic
  events (and eventually Source Profiles).
</p>

<div
  class="animate-slide-in-up"
  style="animation-delay: 50ms; animation-fill-mode: both;"
>
  <DashboardStats />
</div>

<div
  class="grid animate-slide-in-up"
  style="animation-delay: 100ms; animation-fill-mode: both;"
>
  <Panel title="Recent Events">
    <RecentEventsFeed />
  </Panel>

  <div class="sidebar">
    <SystemStatusPanel />

    <Panel title="Tools">
      <div class="tools">
        <a class="cta" href={resolve("/events")}>Event Dashboard</a>
        <a class="cta" href={resolve("/hl7")}>HL7 Preview & Triage</a>
        <a class="cta" href={resolve("/profiles")}>Profiles (YAML)</a>
        <a class="cta" href={resolve("/terminology")}>Terminology Mapping</a>
        <a class="cta" href={resolve("/workflows")}>Workflow Builder</a>
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
    padding: 12px 16px;
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
    text-decoration: none;
    font-weight: 700;
    transition: var(--transition-all);
  }

  .cta:hover {
    background: var(--color-primary);
    color: var(--color-text-inverse);
    transform: translateY(-2px);
    box-shadow: var(--shadow-glow-primary);
  }
</style>
