<script lang="ts">
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import Panel from '$lib/ui/Panel.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
  import JourneyProgress from '$lib/ui/ide/JourneyProgress.svelte';

  const missionSignals = [
    'Five-stage guided workspace',
    'Live runtime health',
    'Recent downstream verification'
  ];

  const recommendation = {
    label: 'Continue to Normalization',
    href: '/profiles' satisfies IDEAppRoute,
    summary: 'Open source profiles and tighten identifier rules before the next delivery pass.'
  };

  const launchPads: Array<{
    eyebrow: string;
    label: string;
    href: IDEAppRoute;
    hint: string;
    detail: string;
  }> = [
    {
      eyebrow: 'Inspect',
      label: 'Source intake lab',
      href: '/hl7',
      hint: 'Raw payloads, warnings, source profile',
      detail: 'Review inbound messages, parser drift, and feed-specific anomalies before they spill downstream.'
    },
    {
      eyebrow: 'Refine',
      label: 'Normalization rules',
      href: '/profiles',
      hint: 'Identifiers, tolerances, profile rules',
      detail: 'Tighten assigning authorities, tolerance settings, and feed behavior in one editing surface.'
    },
    {
      eyebrow: 'Translate',
      label: 'Terminology queue',
      href: '/terminology',
      hint: 'Mappings, candidates, traceability',
      detail: 'Move from local codes to canonical semantics with enough traceability for review and correction.'
    },
    {
      eyebrow: 'Deliver',
      label: 'Workflow workbench',
      href: '/workflows',
      hint: 'Routes, actions, destinations',
      detail: 'Author routes, dry-run changes, and inspect delivery behavior before traffic reaches live destinations.'
    },
    {
      eyebrow: 'Verify',
      label: 'Outcome timeline',
      href: '/events',
      hint: 'Timeline, outcomes, feedback',
      detail: 'Compare what landed against source intent and decide what to tune next.'
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

    <div class="hero-signals" aria-label="Mission control signals">
      {#each missionSignals as signal (signal)}
        <span class="signal-chip">{signal}</span>
      {/each}
    </div>
  </div>

  <aside class="hero-aside">
    <div class="hero-rail">
      <div class="rail-copy">
        <span class="rail-label">Recommended move</span>
        <a class="primary-action" href={resolve(recommendation.href as '/')}>{recommendation.label}</a>
        <p>{recommendation.summary}</p>
      </div>

      <div class="hero-actions">
        <a class="secondary-action" href={resolve('/hl7')}>Start Source Intake</a>
        <a class="tertiary-action" href={resolve('/events')}>Review Verification</a>
      </div>
    </div>
  </aside>
</section>

<div class="stack">
  <JourneyProgress pathname="/" variant="full" showAction={false} />

  <div class="workspace-grid">
    <Panel title="Operator surfaces" padding="lg">
      <div class="launch-grid">
        {#each launchPads as pad (pad.label)}
          <a class="launch-card" href={resolve(pad.href)}>
            <span class="launch-eyebrow">{pad.eyebrow}</span>
            <span class="launch-label">{pad.label}</span>
            <span class="launch-hint">{pad.hint}</span>
            <span class="launch-detail">{pad.detail}</span>
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
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(280px, 0.7fr);
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

  .hero-signals {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
  }

  .signal-chip {
    padding: 6px 10px;
    border-radius: var(--radius-full);
    border: 1px solid rgba(148, 163, 184, 0.2);
    background: rgba(15, 23, 42, 0.32);
    color: var(--color-text-secondary);
    font-size: var(--text-2xs);
    font-weight: var(--font-semibold);
    letter-spacing: 0.06em;
    text-transform: uppercase;
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

  .hero-aside {
    display: grid;
    align-items: stretch;
  }

  .hero-rail {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid rgba(99, 102, 241, 0.18);
    background:
      linear-gradient(180deg, rgba(99, 102, 241, 0.18), rgba(99, 102, 241, 0.05)),
      rgba(15, 23, 42, 0.5);
    box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.04);
  }

  .rail-copy {
    display: grid;
    gap: 10px;
  }

  .rail-label {
    color: rgba(199, 210, 254, 0.9);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .rail-copy p {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  .hero-actions {
    display: grid;
    gap: var(--space-2);
  }

  .primary-action,
  .secondary-action,
  .tertiary-action {
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
    min-height: 44px;
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

  .tertiary-action {
    border-color: rgba(148, 163, 184, 0.16);
    background: transparent;
    color: var(--color-text-secondary);
  }

  .tertiary-action:hover {
    background: rgba(148, 163, 184, 0.08);
    border-color: rgba(148, 163, 184, 0.28);
    color: var(--color-text-primary);
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
    gap: 8px;
    padding: var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    text-decoration: none;
    color: inherit;
    transition: var(--transition-all);
    min-height: 158px;
  }

  .launch-card:hover {
    transform: translateY(-2px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-md);
  }

  .launch-eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
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
    font-weight: var(--font-medium);
  }

  .launch-detail {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  @media (max-width: 1080px) {
    .hero {
      grid-template-columns: 1fr;
    }

    .workspace-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 760px) {
    .hero {
      padding: var(--space-5);
    }

    .hero-rail {
      padding: var(--space-3);
    }

    .launch-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
