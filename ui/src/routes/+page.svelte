<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import Panel from '$lib/ui/Panel.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
  import WarningTrends from '$lib/features/dashboard/WarningTrends.svelte';
  import AlertsPanel from '$lib/features/dashboard/AlertsPanel.svelte';
  import UnmappedCodesWidget from '$lib/features/dashboard/UnmappedCodesWidget.svelte';
  import JourneyProgress from '$lib/ui/ide/JourneyProgress.svelte';
  import { recents } from '$lib/features/dashboard/recentsStore';
  import { hasRestoredLayout, restoreLayout } from '$lib/ui/ide/ideStore';
  import { platformState } from '$lib/platform/platformStore';
  import { getJourneyStages } from '$lib/ui/ide/journey';
  import type { JourneyStage } from '$lib/ui/ide/journey';

  // ── Stage definitions ──────────────────────────────────────────────────────
  const stages: JourneyStage[] = getJourneyStages();

  const stageColors: Record<string, string> = {
    'source-intake': 'var(--color-info)',
    normalization: '#8b5cf6',
    translation: 'var(--color-warning)',
    delivery: 'var(--color-primary)',
    verification: 'var(--color-success)',
  };

  const docTypeColors: Record<string, string> = {
    route: 'var(--color-text-muted)',
    'workflow-draft': 'var(--color-primary)',
    'debug-session': 'var(--color-warning)',
    trace: 'var(--color-info)',
    event: 'var(--color-success)',
    profile: '#8b5cf6',
  };

  // ── Launch pads (operator surfaces) ────────────────────────────────────────
  const launchPads: Array<{
    eyebrow: string;
    label: string;
    href: IDEAppRoute;
    hint: string;
    detail: string;
    stageId: string;
  }> = [
    {
      eyebrow: 'Inspect',
      label: 'Source intake lab',
      href: '/hl7',
      hint: 'Raw payloads, warnings, source profile',
      detail: 'Review inbound messages, parser drift, and feed-specific anomalies before they spill downstream.',
      stageId: 'source-intake',
    },
    {
      eyebrow: 'Refine',
      label: 'Normalization rules',
      href: '/profiles',
      hint: 'Identifiers, tolerances, profile rules',
      detail: 'Tighten assigning authorities, tolerance settings, and feed behavior in one editing surface.',
      stageId: 'normalization',
    },
    {
      eyebrow: 'Translate',
      label: 'Terminology queue',
      href: '/terminology',
      hint: 'Mappings, candidates, traceability',
      detail: 'Move from local codes to canonical semantics with enough traceability for review and correction.',
      stageId: 'translation',
    },
    {
      eyebrow: 'Deliver',
      label: 'Workflow workbench',
      href: '/workflows',
      hint: 'Routes, actions, destinations',
      detail: 'Author routes, dry-run changes, and inspect delivery behavior before traffic reaches live destinations.',
      stageId: 'delivery',
    },
    {
      eyebrow: 'Verify',
      label: 'Outcome timeline',
      href: '/events',
      hint: 'Timeline, outcomes, feedback',
      detail: 'Compare what landed against source intent and decide what to tune next.',
      stageId: 'verification',
    },
  ];

  // ── Reactive state ─────────────────────────────────────────────────────────
  let mounted = false;

  $: recentEntries = $recents.slice(0, 5);
  $: hasRecents = recentEntries.length > 0;
  $: connected = $platformState.connected;
  $: hasSavedLayout = $hasRestoredLayout;

  // ── Recommended action logic ───────────────────────────────────────────────
  $: recommendedAction = computeRecommendation($recents);

  function computeRecommendation(recent: typeof $recents): {
    label: string;
    href: IDEAppRoute;
    reason: string;
  } {
    // If there are recent entries, recommend continuing
    if (recent.length > 0) {
      const last = recent[0]!;
      const route = (last.route ?? '/hl7') as IDEAppRoute;
      return {
        label: `Continue: ${last.title}`,
        href: route,
        reason: `Pick up where you left off in ${last.stage}`,
      };
    }

    // Default: recommend starting source intake
    return {
      label: 'Start Source Intake',
      href: '/hl7',
      reason: 'Begin by loading and inspecting inbound interfaces',
    };
  }

  // ── Time formatting ────────────────────────────────────────────────────────
  function formatRelativeTime(ts: number): string {
    const diff = Date.now() - ts;
    if (diff < 60_000) return 'just now';
    if (diff < 3600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86400_000) return `${Math.floor(diff / 3600_000)}h ago`;
    if (diff < 604800_000) return `${Math.floor(diff / 86400_000)}d ago`;
    return new Date(ts).toLocaleDateString();
  }

  // ── Navigation ─────────────────────────────────────────────────────────────
  function navigateTo(path: string): void {
    // eslint-disable-next-line @typescript-eslint/no-explicit-any, svelte/no-navigation-without-resolve -- resolve() is called inside
    void (goto as any)((resolve as any)(path));
  }

  // ── Stage health placeholder (no diagnosticsStore exists yet) ──────────────
  type StageHealth = {
    id: string;
    label: string;
    color: string;
    errors: number;
    warnings: number;
  };

  // eslint-disable-next-line svelte/no-immutable-reactive-statements -- will become reactive when diagnosticsStore is wired in
  $: stageHealth = stages.map((s): StageHealth => ({
    id: s.id,
    label: s.label,
    color: stageColors[s.id] ?? 'var(--color-text-muted)',
    errors: 0,
    warnings: 0,
  }));

  // ── Hero greeting ──────────────────────────────────────────────────────────
  $: heroGreeting = hasRecents ? 'Welcome back' : 'Build the interface from source to destination';
  $: heroSubtext = hasRecents
    ? 'Your recent work is waiting. Pick up a thread or start something new.'
    : 'Keep intake, normalization, translation, delivery, and verification in one guided workspace so each decision stays traceable.';

  // ── Signal chips ───────────────────────────────────────────────────────────
  $: signalChips = buildSignalChips(connected, hasRecents);

  function buildSignalChips(isConnected: boolean, hasRecentWork: boolean): string[] {
    const chips: string[] = [];
    if (isConnected) chips.push('Platform connected');
    if (hasRecentWork) chips.push(`${$recents.length} recent items`);
    chips.push('Five-stage guided workspace');
    chips.push('Live runtime health');
    return chips;
  }

  // ── Mount ──────────────────────────────────────────────────────────────────
  onMount(() => {
    restoreLayout();
    mounted = true;
  });
</script>

<svelte:head>
  <title>Mission Control | fi-fhir</title>
</svelte:head>

<!-- Hero Section -->
<section class="hero" class:mounted>
  <div class="hero-copy">
    <div class="eyebrow">Mission control</div>
    <h1>{heroGreeting}</h1>
    <p>{heroSubtext}</p>

    <div class="hero-signals" aria-label="Mission control signals">
      {#each signalChips as signal (signal)}
        <Badge variant="default" size="sm" pill>{signal}</Badge>
      {/each}
    </div>
  </div>

  <aside class="hero-aside">
    <div class="hero-rail">
      <div class="rail-copy">
        <span class="rail-label">Recommended move</span>
        <button
          class="primary-action"
          on:click={() => navigateTo(recommendedAction.href)}
        >{recommendedAction.label}</button>
        <p>{recommendedAction.reason}</p>
      </div>

      <div class="hero-actions">
        {#if hasSavedLayout && hasRecents}
          <button class="secondary-action" on:click={() => navigateTo(recommendedAction.href)}>
            Resume session
          </button>
        {/if}
        <a class="tertiary-action" href={resolve('/hl7')}>Start Source Intake</a>
        <a class="tertiary-action" href={resolve('/events')}>Review Verification</a>
      </div>
    </div>
  </aside>
</section>

<div class="stack">
  <!-- Continue Where You Left Off -->
  <section class="section stagger-1" class:mounted aria-label="Recent work">
    <h2 class="section-title">Continue where you left off</h2>

    {#if hasRecents}
      <div class="recents-grid">
        {#each recentEntries as entry (entry.id)}
          <button
            class="recent-card"
            on:click={() => navigateTo(entry.route ?? '/hl7')}
          >
            <span
              class="recent-type-strip"
              style:background={docTypeColors[entry.documentType] ?? 'var(--color-text-muted)'}
            ></span>
            <div class="recent-body">
              <div class="recent-top">
                <span
                  class="recent-dot"
                  style:background={docTypeColors[entry.documentType] ?? 'var(--color-text-muted)'}
                ></span>
                <span class="recent-doc-type">{entry.documentType.replace(/-/g, ' ')}</span>
                <span class="recent-time">{formatRelativeTime(entry.timestamp)}</span>
              </div>
              <span class="recent-title">{entry.title}</span>
              {#if entry.subtitle}
                <span class="recent-subtitle">{entry.subtitle}</span>
              {/if}
              <span class="recent-stage">
                <span
                  class="stage-dot-sm"
                  style:background={stageColors[entry.stage] ?? 'var(--color-text-muted)'}
                ></span>
                {entry.stage.replace(/-/g, ' ')}
              </span>
            </div>
          </button>
        {/each}
      </div>
    {:else}
      <div class="empty-section">
        <p>No recent work -- start by exploring a stage above</p>
      </div>
    {/if}
  </section>

  <!-- Stage Health + Active Investigations row -->
  <div class="health-row stagger-2" class:mounted>
    <!-- Stage Health -->
    <section class="health-section" aria-label="Stage health">
      <h2 class="section-title">Stage health</h2>
      <div class="stage-health-grid">
        {#each stageHealth as stage (stage.id)}
          <div class="stage-health-card">
            <div class="stage-health-top">
              <span class="health-dot" class:healthy={stage.errors === 0 && stage.warnings === 0} class:warn={stage.errors === 0 && stage.warnings > 0} class:error={stage.errors > 0} style:--stage-color={stage.color}></span>
              <span class="stage-health-label">{stage.label}</span>
            </div>
            <div class="stage-health-counts">
              {#if stage.errors > 0}
                <Badge variant="danger" size="sm">{stage.errors} err</Badge>
              {/if}
              {#if stage.warnings > 0}
                <Badge variant="warning" size="sm">{stage.warnings} warn</Badge>
              {/if}
              {#if stage.errors === 0 && stage.warnings === 0}
                <span class="stage-clean">Clean</span>
              {/if}
            </div>
          </div>
        {/each}
      </div>
    </section>

    <!-- Active Investigations -->
    <section class="investigations-section" aria-label="Active investigations">
      <h2 class="section-title">Active investigations</h2>
      <div class="investigations-card">
        <div class="empty-investigations">
          <span class="inv-icon">&#9678;</span>
          <p>No active investigations</p>
          <span class="inv-hint">Debug sessions and dry runs will appear here</span>
        </div>
      </div>
    </section>
  </div>

  <!-- Journey Progress -->
  <div class="stagger-3" class:mounted>
    <JourneyProgress pathname="/" variant="full" showAction={false} />
  </div>

  <!-- Operator Surfaces (existing launch cards) -->
  <section class="stagger-4" class:mounted aria-label="Operator surfaces">
    <Panel title="Operator surfaces" padding="lg">
      <div class="launch-grid">
        {#each launchPads as pad (pad.label)}
          <a class="launch-card" href={resolve(pad.href)}>
            <div class="launch-card-head">
              <span
                class="launch-stage-dot"
                style:background={stageColors[pad.stageId] ?? 'var(--color-text-muted)'}
              ></span>
              <span class="launch-eyebrow">{pad.eyebrow}</span>
            </div>
            <span class="launch-label">{pad.label}</span>
            <span class="launch-hint">{pad.hint}</span>
            <span class="launch-detail">{pad.detail}</span>
          </a>
        {/each}
      </div>
    </Panel>
  </section>

  <!-- Telemetry + System Status -->
  <div class="workspace-grid stagger-5" class:mounted>
    <Panel title="Operational telemetry" padding="md">
      <DashboardStats />
    </Panel>

    <div class="side-stack">
      <SystemStatusPanel />
      <AlertsPanel />
      <UnmappedCodesWidget />
    </div>
  </div>
<!-- Recent Events -->
<div class="workspace-grid stagger-6" class:mounted>
  <WarningTrends />
  <Panel title="Recent events" padding="md">
    <RecentEventsFeed />
  </Panel>
</div>
</div>

<style>
  /* ═══════════════════════════════════════════════════════════════════════════
   * ANIMATIONS
   * ═══════════════════════════════════════════════════════════════════════════ */

  @keyframes slideInUp {
    from {
      opacity: 0;
      transform: translateY(12px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  /* Staggered entry: elements start invisible and slide in on mount */
  .stagger-1,
  .stagger-2,
  .stagger-3,
  .stagger-4,
  .stagger-5,
  .stagger-6 {
    opacity: 0;
    transform: translateY(12px);
  }

  .stagger-1.mounted { animation: slideInUp 0.4s ease-out 0.1s forwards; }
  .stagger-2.mounted { animation: slideInUp 0.4s ease-out 0.2s forwards; }
  .stagger-3.mounted { animation: slideInUp 0.4s ease-out 0.3s forwards; }
  .stagger-4.mounted { animation: slideInUp 0.4s ease-out 0.4s forwards; }
  .stagger-5.mounted { animation: slideInUp 0.4s ease-out 0.5s forwards; }
  .stagger-6.mounted { animation: slideInUp 0.4s ease-out 0.6s forwards; }

  .hero {
    opacity: 0;
    transform: translateY(12px);
  }
  .hero.mounted {
    animation: slideInUp 0.5s ease-out forwards;
  }

  @media (prefers-reduced-motion: reduce) {
    .stagger-1,
    .stagger-2,
    .stagger-3,
    .stagger-4,
    .stagger-5,
    .stagger-6,
    .hero {
      opacity: 1;
      transform: none;
      animation: none;
    }

    .stagger-1.mounted,
    .stagger-2.mounted,
    .stagger-3.mounted,
    .stagger-4.mounted,
    .stagger-5.mounted,
    .stagger-6.mounted,
    .hero.mounted {
      animation: none;
    }
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * HERO SECTION
   * ═══════════════════════════════════════════════════════════════════════════ */

  .hero {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(280px, 0.7fr);
    gap: var(--space-4);
    padding: var(--space-6);
    border: 1px solid var(--color-border-subtle);
    border-radius: calc(var(--radius-xl) + 4px);
    background: var(--color-bg-elevated);
    box-shadow: var(--shadow-sm);
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

  .eyebrow {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  h1 {
    margin: 0;
    font-family: var(--font-heading);
    color: var(--color-text-primary);
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
    border: 1px solid var(--color-primary-border);
    background: var(--color-bg-surface);
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
    cursor: pointer;
    font-family: inherit;
    font-size: inherit;
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
    box-shadow: var(--shadow-md);
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

  /* ═══════════════════════════════════════════════════════════════════════════
   * LAYOUT
   * ═══════════════════════════════════════════════════════════════════════════ */

  .stack {
    display: grid;
    gap: var(--space-4);
  }

  .section-title {
    margin: 0 0 var(--space-3) 0;
    font-family: var(--font-heading);
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    letter-spacing: var(--tracking-tight);
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

  /* ═══════════════════════════════════════════════════════════════════════════
   * RECENT WORK CARDS
   * ═══════════════════════════════════════════════════════════════════════════ */

  .recents-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(240px, 1fr));
    gap: var(--space-3);
  }

  .recent-card {
    position: relative;
    display: grid;
    grid-template-columns: 3px 1fr;
    gap: 0;
    padding: 0;
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    text-decoration: none;
    color: inherit;
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition: var(--transition-all);
    overflow: hidden;
  }

  .recent-card:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-md);
  }

  .recent-type-strip {
    border-radius: var(--radius-xl) 0 0 var(--radius-xl);
    min-height: 100%;
  }

  .recent-body {
    display: grid;
    gap: 6px;
    padding: var(--space-3);
  }

  .recent-top {
    display: flex;
    align-items: center;
    gap: var(--space-2);
  }

  .recent-dot {
    width: 6px;
    height: 6px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  .recent-doc-type {
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    color: var(--color-text-muted);
    text-transform: capitalize;
    letter-spacing: 0.06em;
  }

  .recent-time {
    margin-left: auto;
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    white-space: nowrap;
  }

  .recent-title {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    line-height: var(--leading-snug);
  }

  .recent-subtitle {
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    line-height: var(--leading-snug);
  }

  .recent-stage {
    display: flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    text-transform: capitalize;
  }

  .stage-dot-sm {
    width: 5px;
    height: 5px;
    border-radius: 50%;
    flex-shrink: 0;
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * STAGE HEALTH
   * ═══════════════════════════════════════════════════════════════════════════ */

  .health-row {
    display: grid;
    grid-template-columns: minmax(0, 1.2fr) minmax(280px, 0.8fr);
    gap: var(--space-4);
    align-items: start;
  }

  .health-section,
  .investigations-section {
    min-width: 0;
  }

  .stage-health-grid {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--space-2);
  }

  .stage-health-card {
    display: grid;
    gap: 8px;
    padding: var(--space-3);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    transition: var(--transition-all);
  }

  .stage-health-card:hover {
    transform: translateY(-1px);
    border-color: var(--color-border-strong);
    box-shadow: var(--shadow-sm);
  }

  .stage-health-top {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .health-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
    background: var(--stage-color, var(--color-text-muted));
    transition: var(--transition-all);
  }

  /* Solid state dots — color carries state, no glow or pulsing (Slice 2). */
  .health-dot.healthy {
    background: var(--color-success);
  }

  .health-dot.warn {
    background: var(--color-warning);
  }

  .health-dot.error {
    background: var(--color-danger);
  }

  .stage-health-label {
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .stage-health-counts {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
  }

  .stage-clean {
    font-size: var(--text-2xs);
    color: var(--color-text-muted);
    font-weight: var(--font-medium);
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * ACTIVE INVESTIGATIONS
   * ═══════════════════════════════════════════════════════════════════════════ */

  .investigations-card {
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    overflow: hidden;
  }

  .empty-investigations {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--space-6) var(--space-4);
    text-align: center;
    gap: 4px;
  }

  .inv-icon {
    font-size: 1.5rem;
    color: var(--color-text-muted);
    opacity: 0.5;
    margin-bottom: var(--space-1);
  }

  .empty-investigations p {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
  }

  .inv-hint {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * EMPTY STATE
   * ═══════════════════════════════════════════════════════════════════════════ */

  .empty-section {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-5) var(--space-4);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-xl);
    background: var(--color-bg-surface);
  }

  .empty-section p {
    margin: 0;
    color: var(--color-text-muted);
    font-size: var(--text-sm);
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * LAUNCH CARDS
   * ═══════════════════════════════════════════════════════════════════════════ */

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

  .launch-card-head {
    display: flex;
    align-items: center;
    gap: 8px;
  }

  .launch-stage-dot {
    width: 8px;
    height: 8px;
    border-radius: 50%;
    flex-shrink: 0;
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

  /* ═══════════════════════════════════════════════════════════════════════════
   * RESPONSIVE
   * ═══════════════════════════════════════════════════════════════════════════ */

  @media (max-width: 1080px) {
    .hero {
      grid-template-columns: 1fr;
    }

    .workspace-grid {
      grid-template-columns: 1fr;
    }

    .health-row {
      grid-template-columns: 1fr;
    }

    .stage-health-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
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

    .stage-health-grid {
      grid-template-columns: repeat(2, minmax(0, 1fr));
    }

    .recents-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
