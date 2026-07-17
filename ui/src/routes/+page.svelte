<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
  import WarningTrends from '$lib/features/dashboard/WarningTrends.svelte';
  import AlertsPanel from '$lib/features/dashboard/AlertsPanel.svelte';
  import UnmappedCodesWidget from '$lib/features/dashboard/UnmappedCodesWidget.svelte';
  import { recents } from '$lib/features/dashboard/recentsStore';
  import { hasRestoredLayout, restoreLayout } from '$lib/ui/ide/ideStore';
  const stageColors: Record<string, string> = {
    'source-intake': 'var(--color-info)',
    normalization: 'var(--palette-violet-600)',
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
    profile: 'var(--palette-violet-600)',
  };

  // ── Launch pads (operator surfaces, Tier 3) ────────────────────────────────
  const launchPads: Array<{
    eyebrow: string;
    label: string;
    href: IDEAppRoute;
    hint: string;
    stageId: string;
  }> = [
    {
      eyebrow: 'Inspect',
      label: 'Source intake lab',
      href: '/hl7',
      hint: 'Raw payloads, warnings, source profile',
      stageId: 'source-intake',
    },
    {
      eyebrow: 'Refine',
      label: 'Normalization rules',
      href: '/profiles',
      hint: 'Identifiers, tolerances, profile rules',
      stageId: 'normalization',
    },
    {
      eyebrow: 'Translate',
      label: 'Terminology queue',
      href: '/terminology',
      hint: 'Mappings, candidates, traceability',
      stageId: 'translation',
    },
    {
      eyebrow: 'Deliver',
      label: 'Workflow workbench',
      href: '/workflows',
      hint: 'Routes, actions, destinations',
      stageId: 'delivery',
    },
    {
      eyebrow: 'Verify',
      label: 'Outcome timeline',
      href: '/events',
      hint: 'Timeline, outcomes, feedback',
      stageId: 'verification',
    },
  ];

  // ── Tier 3 progressive disclosure ──────────────────────────────────────────
  // Only the active panel mounts, keeping the default screen quiet and the
  // on-load element count low. Live data leads; navigation cards come last —
  // the shell banner and sidebar already cover navigation.
  type OnDemandTab = 'surfaces' | 'telemetry' | 'events' | 'trends';
  const onDemandTabs: { id: OnDemandTab; label: string }[] = [
    { id: 'events', label: 'Recent events' },
    { id: 'telemetry', label: 'Operational telemetry' },
    { id: 'trends', label: 'Signals & trends' },
    { id: 'surfaces', label: 'Operator surfaces' },
  ];
  let activeTab: OnDemandTab = 'events';

  // ── Reactive state ─────────────────────────────────────────────────────────
  let mounted = false;

  $: recentEntries = $recents.slice(0, 3);
  $: hasRecents = recentEntries.length > 0;
  $: hasSavedLayout = $hasRestoredLayout;

  // ── Recommended action logic ───────────────────────────────────────────────
  $: recommendedAction = computeRecommendation($recents);

  function computeRecommendation(recent: typeof $recents): {
    label: string;
    href: IDEAppRoute;
    reason: string;
  } {
    if (recent.length > 0) {
      const last = recent[0]!;
      const route = (last.route ?? '/hl7') as IDEAppRoute;
      return {
        label: `Continue: ${last.title}`,
        href: route,
        reason: `Pick up where you left off in ${last.stage}`,
      };
    }

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

  // ── Tier 3 tab keyboard navigation ─────────────────────────────────────────
  function onTabKeydown(e: KeyboardEvent, id: OnDemandTab): void {
    const idx = onDemandTabs.findIndex((t) => t.id === id);
    let next = idx;
    if (e.key === 'ArrowRight') next = (idx + 1) % onDemandTabs.length;
    else if (e.key === 'ArrowLeft') next = (idx - 1 + onDemandTabs.length) % onDemandTabs.length;
    else return;
    e.preventDefault();
    activeTab = onDemandTabs[next]!.id;
    const tabs = (e.currentTarget as HTMLElement).parentElement?.querySelectorAll('[role="tab"]');
    (tabs?.[next] as HTMLElement | undefined)?.focus();
  }

  // ── Header greeting ────────────────────────────────────────────────────────
  $: heroGreeting = hasRecents ? 'Welcome back' : 'Build the interface from source to destination';
  $: heroSubtext = hasRecents
    ? 'Pick up recent work or start something new.'
    : 'Intake to verification in one workspace, every decision traceable.';

  // ── Mount ──────────────────────────────────────────────────────────────────
  onMount(() => {
    restoreLayout();
    mounted = true;
  });
</script>

<svelte:head>
  <title>Dashboard | fi-fhir</title>
</svelte:head>

<div class="dashboard" class:mounted>
  <!-- ── Tier 1 — Now ──────────────────────────────────────────────────────── -->
  <section class="now" aria-label="Now">
    <div class="now-head">
      <div class="now-intro">
        <h1>{heroGreeting}</h1>
        <p>{heroSubtext}</p>
      </div>

      <aside class="recommended" aria-label="Recommended move">
        <span class="rail-label">Recommended move</span>
        <button class="primary-action" on:click={() => navigateTo(recommendedAction.href)}>
          {recommendedAction.label}
        </button>
        <p class="rec-reason">{recommendedAction.reason}</p>
        {#if hasSavedLayout && hasRecents}
          <button class="secondary-action" on:click={() => navigateTo(recommendedAction.href)}>
            Resume session
          </button>
        {/if}
      </aside>
    </div>

    <section aria-label="Recent work">
      {#if hasRecents}
        <h2 class="section-title">Continue where you left off</h2>
        <div class="recents-grid">
          {#each recentEntries as entry (entry.id)}
              <button class="recent-card" on:click={() => navigateTo(entry.route ?? '/hl7')}>
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
          <p>No recent work yet.</p>
        </div>
      {/if}
    </section>
  </section>

  <!-- ── Tier 2 — Health at a glance (live signals only) ───────────────────── -->
  <section class="health" aria-label="Health at a glance">
    <h2 class="section-title">Health at a glance</h2>
    <div class="health-grid">
      <SystemStatusPanel />
      <AlertsPanel />
    </div>
  </section>

  <!-- ── Tier 3 — On demand (progressive disclosure) ───────────────────────── -->
  <section class="on-demand" aria-label="On demand">
    <div class="tablist" role="tablist" aria-label="On-demand panels">
      {#each onDemandTabs as tab (tab.id)}
        <button
          type="button"
          role="tab"
          class="od-tab"
          class:active={activeTab === tab.id}
          id={`od-tab-${tab.id}`}
          aria-selected={activeTab === tab.id}
          aria-controls={`od-panel-${tab.id}`}
          tabindex={activeTab === tab.id ? 0 : -1}
          on:click={() => (activeTab = tab.id)}
          on:keydown={(e) => onTabKeydown(e, tab.id)}
        >{tab.label}</button>
      {/each}
    </div>

    <div
      class="od-panel"
      role="tabpanel"
      id={`od-panel-${activeTab}`}
      aria-labelledby={`od-tab-${activeTab}`}
      tabindex="0"
    >
      {#if activeTab === 'surfaces'}
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
            </a>
          {/each}
        </div>
      {:else if activeTab === 'telemetry'}
        <DashboardStats />
      {:else if activeTab === 'events'}
        <RecentEventsFeed />
      {:else if activeTab === 'trends'}
        <div class="trends-stack">
          <WarningTrends />
          <UnmappedCodesWidget />
        </div>
      {/if}
    </div>
  </section>
</div>

<style>
  /* ═══════════════════════════════════════════════════════════════════════════
   * ENTRY MOTION — single quiet one-shot (<=200ms), reduced-motion safe.
   * ═══════════════════════════════════════════════════════════════════════════ */

  @keyframes fadeIn {
    from {
      opacity: 0;
      transform: translateY(6px);
    }
    to {
      opacity: 1;
      transform: translateY(0);
    }
  }

  .dashboard {
    display: grid;
    gap: var(--space-5);
    opacity: 0;
  }

  .dashboard.mounted {
    animation: fadeIn 0.2s ease-out forwards;
  }

  @media (prefers-reduced-motion: reduce) {
    .dashboard,
    .dashboard.mounted {
      opacity: 1;
      transform: none;
      animation: none;
    }
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * TIER 1 — NOW
   * ═══════════════════════════════════════════════════════════════════════════ */

  .now {
    display: grid;
    gap: var(--space-4);
  }

  .now-head {
    display: grid;
    grid-template-columns: minmax(0, 1.5fr) minmax(260px, 0.7fr);
    gap: var(--space-4);
    align-items: start;
  }

  .now-intro {
    display: grid;
    gap: var(--space-3);
    max-width: 70ch;
  }

  h1 {
    margin: 0;
    font-family: var(--font-heading);
    color: var(--color-text-primary);
    font-size: clamp(var(--text-xl), 2.4vw, 2rem);
    line-height: var(--leading-tight);
    letter-spacing: var(--tracking-tight);
  }

  .now-intro p {
    margin: 0;
    color: var(--color-text-secondary);
    max-width: 64ch;
    font-size: var(--text-base);
    line-height: var(--leading-relaxed);
  }

  .recommended {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-4);
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-primary-border);
    background: var(--color-bg-surface);
    align-content: start;
  }

  .rail-label {
    color: var(--color-text-muted);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .rec-reason {
    margin: 0;
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  .primary-action,
  .secondary-action {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 10px 16px;
    border-radius: var(--radius-lg);
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
    box-shadow: var(--shadow-sm);
  }

  .secondary-action {
    border-color: var(--color-border-default);
    background: var(--color-bg-elevated);
    color: var(--color-text-primary);
  }

  .secondary-action:hover {
    background: var(--color-bg-hover);
    border-color: var(--color-border-strong);
  }

  .section-title {
    margin: 0 0 var(--space-3) 0;
    font-family: var(--font-heading);
    font-size: var(--text-base);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
    letter-spacing: var(--tracking-tight);
  }

  .recents-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
    gap: var(--space-3);
  }

  .recent-card {
    position: relative;
    display: grid;
    grid-template-columns: 3px 1fr;
    padding: 0;
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    text-align: left;
    cursor: pointer;
    font-family: inherit;
    transition: var(--transition-all);
    overflow: hidden;
  }

  .recent-card:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
  }

  .recent-type-strip {
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

  .empty-section {
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-5) var(--space-4);
    border: 1px dashed var(--color-border-strong);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
  }

  .empty-section p {
    margin: 0;
    color: var(--color-text-muted);
    font-size: var(--text-sm);
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * TIER 2 — HEALTH AT A GLANCE
   * ═══════════════════════════════════════════════════════════════════════════ */

  .health-grid {
    display: grid;
    grid-template-columns: minmax(0, 1.1fr) minmax(0, 0.9fr);
    gap: var(--space-4);
    align-items: start;
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * TIER 3 — ON DEMAND
   * ═══════════════════════════════════════════════════════════════════════════ */

  .on-demand {
    display: grid;
    gap: var(--space-3);
  }

  .tablist {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .od-tab {
    padding: var(--space-2) var(--space-3);
    border: none;
    border-bottom: 2px solid transparent;
    background: transparent;
    color: var(--color-text-tertiary);
    font-family: inherit;
    font-size: var(--text-sm);
    font-weight: var(--font-medium);
    cursor: pointer;
    transition: var(--transition-colors);
    margin-bottom: -1px;
  }

  .od-tab:hover {
    color: var(--color-text-primary);
  }

  .od-tab.active {
    color: var(--color-text-primary);
    border-bottom-color: var(--color-primary);
    font-weight: var(--font-semibold);
  }

  .od-tab:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
    border-radius: var(--radius-sm);
  }

  .od-panel {
    padding-top: var(--space-2);
  }

  .od-panel:focus-visible {
    outline: none;
  }

  .trends-stack {
    display: grid;
    gap: var(--space-4);
  }

  /* ── Launch cards (operator surfaces) ── */

  .launch-grid {
    display: grid;
    gap: var(--space-3);
    grid-template-columns: repeat(auto-fill, minmax(260px, 1fr));
  }

  .launch-card {
    display: grid;
    gap: 8px;
    padding: var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface);
    text-decoration: none;
    color: inherit;
    transition: var(--transition-all);
  }

  .launch-card:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
    box-shadow: var(--shadow-sm);
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

  /* ═══════════════════════════════════════════════════════════════════════════
   * RESPONSIVE
   * ═══════════════════════════════════════════════════════════════════════════ */

  @media (max-width: 1080px) {
    .now-head,
    .health-grid {
      grid-template-columns: 1fr;
    }
  }

  @media (max-width: 760px) {
    .recents-grid {
      grid-template-columns: 1fr;
    }
  }
</style>
