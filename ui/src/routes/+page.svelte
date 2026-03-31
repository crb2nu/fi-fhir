<script lang="ts">
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import Panel from '$lib/ui/Panel.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
  import JourneyProgress from '$lib/ui/ide/JourneyProgress.svelte';

  const launchPads: Array<{
    order: string;
    label: string;
    href: IDEAppRoute;
    hint: string;
  }> = [
    {
      order: '1',
      label: 'Source Intake',
      href: '/hl7',
      hint: 'Inspect raw HL7, preserve payloads, and surface warnings.',
    },
    {
      order: '2',
      label: 'Normalization',
      href: '/profiles',
      hint: 'Tune identifiers, tolerances, and source profile rules.',
    },
    {
      order: '3',
      label: 'Translation',
      href: '/terminology',
      hint: 'Verify semantic mappings and canonical code paths.',
    },
    {
      order: '4',
      label: 'Delivery',
      href: '/workflows',
      hint: 'Route normalized events into downstream actions.',
    },
    {
      order: '5',
      label: 'Verification',
      href: '/events',
      hint: 'Review the downstream event trail and timeline.',
    },
  ];
</script>

<svelte:head>
  <title>Mission Control | fi-fhir</title>
</svelte:head>

<section class="hero">
  <div class="hero-copy">
    <div class="eyebrow">Mission control</div>
    <h1 class="text-gradient">Build the interface from source to destination</h1>
    <p>
      Keep intake, normalization, translation, delivery, and verification in one guided workspace so
      each decision stays traceable.
    </p>
  </div>

  <div class="hero-actions">
    <a class="primary-action" href={resolve('/hl7')}>Start Source Intake</a>
    <a class="secondary-action" href={resolve('/events')}>Review Verification</a>
  </div>
</section>

<div class="stack">
  <JourneyProgress pathname="/" variant="full" />

  <div class="workspace-grid">
    <Panel title="Launch deck" padding="lg">
      <div class="launch-grid">
        {#each launchPads as pad (pad.order)}
          <a class="launch-card" href={resolve(pad.href)}>
            <span class="launch-order">{pad.order}</span>
            <span class="launch-label">{pad.label}</span>
            <span class="launch-hint">{pad.hint}</span>
          </a>
        {/each}
      </div>
    </Panel>

    <div class="side-stack">
      <SystemStatusPanel />
      <Panel title="Operational telemetry" padding="md">
        <DashboardStats />
      </Panel>
    </div>
  </div>

  <Panel title="Recent events" padding="md">
    <RecentEventsFeed />
  </Panel>
</div>

<style>
  .hero {
    display: flex;
    align-items: flex-end;
    justify-content: space-between;
    gap: var(--space-4);
    padding: var(--space-6);
    border: 1px solid var(--color-border-subtle);
    border-radius: calc(var(--radius-xl) + 4px);
    background:
      radial-gradient(circle at top left, rgba(96, 165, 250, 0.18), transparent 36%),
      radial-gradient(circle at 80% 0%, rgba(52, 211, 153, 0.14), transparent 28%),
      linear-gradient(180deg, rgba(255, 255, 255, 0.03), transparent),
      var(--color-bg-elevated);
    box-shadow: var(--shadow-lg);
  }

  .hero-copy {
    display: grid;
    gap: var(--space-3);
    max-width: 66ch;
  }

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  h1 {
    margin: 0;
    font-size: clamp(var(--text-3xl), 5vw, 3.8rem);
    line-height: 0.95;
    letter-spacing: var(--tracking-tight);
  }

  .hero-copy p {
    margin: 0;
    color: var(--color-text-secondary);
    max-width: 62ch;
    font-size: var(--text-lg);
    line-height: var(--leading-relaxed);
  }

  .hero-actions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-3);
    justify-content: flex-end;
  }

  .primary-action,
  .secondary-action {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 12px 16px;
    border-radius: var(--radius-xl);
    border: 1px solid transparent;
    font-weight: var(--font-semibold);
    text-decoration: none;
    transition: var(--transition-all);
  }

  .primary-action {
    border-color: var(--color-primary-border);
    background: var(--color-primary-muted);
    color: var(--color-primary);
  }

  .primary-action:hover {
    background: var(--color-primary);
    color: var(--color-text-inverse);
    transform: translateY(-2px);
    box-shadow: var(--shadow-glow-primary);
  }

  .secondary-action {
    border-color: var(--color-border-default);
    background: var(--color-bg-surface);
    color: var(--color-text-primary);
  }

  .secondary-action:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
    transform: translateY(-2px);
  }

  .stack {
    display: grid;
    gap: var(--space-4);
  }

  .workspace-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(320px, 0.8fr);
    gap: var(--space-4);
    align-items: start;
  }

  .side-stack {
    display: grid;
    gap: var(--space-4);
  }

  .launch-grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(2, minmax(0, 1fr));
  }

  .launch-card {
    display: grid;
    gap: 6px;
    padding: var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    text-decoration: none;
    color: inherit;
    transition: var(--transition-all);
    min-height: 128px;
  }

  .launch-card:hover {
    transform: translateY(-2px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-md);
  }

  .launch-order {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    width: 28px;
    height: 28px;
    border-radius: 999px;
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-base);
    color: var(--color-text-primary);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
  }

  .launch-label {
    font-size: var(--text-base);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .launch-hint {
    color: var(--color-text-muted);
    font-size: var(--text-sm);
    line-height: var(--leading-snug);
  }

  @media (max-width: 1080px) {
    .hero {
      flex-direction: column;
      align-items: flex-start;
    }

    .hero-actions {
      justify-content: flex-start;
    }

    .workspace-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 760px) {
    .hero {
      padding: var(--space-5);
    }

    .launch-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
