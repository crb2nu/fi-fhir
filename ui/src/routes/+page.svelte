<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import type { IDEAppRoute } from '$lib/ui/ide/types';
  import Badge from '$lib/ui/Badge.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
  import DashboardStats from '$lib/features/dashboard/DashboardStats.svelte';
  import RecentEventsFeed from '$lib/features/dashboard/RecentEventsFeed.svelte';
  import WarningTrends from '$lib/features/dashboard/WarningTrends.svelte';
  import AlertsPanel from '$lib/features/dashboard/AlertsPanel.svelte';
  import UnmappedCodesWidget from '$lib/features/dashboard/UnmappedCodesWidget.svelte';
  import { recents } from '$lib/features/dashboard/recentsStore';
  import { hasRestoredLayout, restoreLayout } from '$lib/ui/ide/ideStore';
  import { platformState } from '$lib/platform/platformStore';
  import { getJourneyStages } from '$lib/ui/ide/journey';
  import type { JourneyStage } from '$lib/ui/ide/journey';

  // ── Stage definitions ──────────────────────────────────────────────────────
  const stages: JourneyStage[] = getJourneyStages();

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

  // ── Tier 1 journey shortcuts ───────────────────────────────────────────────
  // Deterministic entry points into the pipeline: start, mid-pipeline, and the
  // end-of-line review. These are stable so the dashboard always offers a clear
  // way in regardless of session state.
  const journeyLinks: { label: string; href: IDEAppRoute; hint: string }[] = [
    { label: 'Start Source Intake', href: '/hl7', hint: 'Load and inspect inbound interfaces' },
    { label: 'Continue to Normalization', href: '/profiles', hint: 'Tighten identifiers and profile rules' },
    { label: 'Review Verification', href: '/events', hint: 'Confirm outcomes against source intent' },
  ];

  // ── Launch pads (operator surfaces, Tier 3) ────────────────────────────────
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

  // ── Tier 3 progressive disclosure ──────────────────────────────────────────
  // Only the active panel mounts, keeping the default screen quiet and the
  // on-load element count low. Tab labels stay in the DOM as the disclosure.
  type OnDemandTab = 'surfaces' | 'telemetry' | 'events' | 'trends';
  const onDemandTabs: { id: OnDemandTab; label: string }[] = [
    { id: 'surfaces', label: 'Operator surfaces' },
    { id: 'telemetry', label: 'Operational telemetry' },
    { id: 'events', label: 'Recent events' },
    { id: 'trends', label: 'Signals & trends' },
  ];
  let activeTab: OnDemandTab = 'surfaces';

  // ── Reactive state ─────────────────────────────────────────────────────────
  let mounted = false;

  $: recentEntries = $recents.slice(0, 3);
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

  // ── Stage health placeholder (static; cross-stage diagnostics scaffolding retired) ──
  type StageHealth = {
    id: string;
    label: string;
    color: string;
    errors: number;
    warnings: number;
  };

  // eslint-disable-next-line svelte/no-immutable-reactive-statements -- static placeholder; no per-stage health signal exists yet
  $: stageHealth = stages.map((s): StageHealth => ({
    id: s.id,
    label: s.label,
    color: stageColors[s.id] ?? 'var(--color-text-muted)',
    errors: 0,
    warnings: 0,
  }));

  // ── Header greeting ────────────────────────────────────────────────────────
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
    return chips;
  }

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
        <div class="eyebrow">Mission control</div>
        <h1>{heroGreeting}</h1>
        <p>{heroSubtext}</p>
        <div class="signal-row" aria-label="Workspace signals">
          {#each signalChips as signal (signal)}
            <Badge variant="default" size="sm" pill>{signal}</Badge>
          {/each}
        </div>
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

    <nav class="journey-links" aria-label="Pipeline shortcuts">
      {#each journeyLinks as link (link.href)}
        <a class="journey-link" href={resolve(link.href)} aria-label={link.label}>
          <span class="journey-link-label">{link.label}</span>
          <span class="journey-link-hint">{link.hint}</span>
        </a>
      {/each}
    </nav>

    <div class="now-active">
      <section class="now-col" aria-label="Recent work">
        <h2 class="section-title">Continue where you left off</h2>
        {#if hasRecents}
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
            <p>No recent work — start from a pipeline shortcut above.</p>
          </div>
        {/if}
      </section>

      <section class="now-col" aria-label="Active investigations">
        <h2 class="section-title">Active investigations</h2>
        <div class="investigations-card">
          <div class="empty-investigations">
            <span class="inv-icon" aria-hidden="true">&#9678;</span>
            <p>No active investigations</p>
            <span class="inv-hint">Debug sessions and dry runs will appear here</span>
          </div>
        </div>
      </section>
    </div>
  </section>

  <!-- ── Tier 2 — Health at a glance ───────────────────────────────────────── -->
  <section class="health" aria-label="Health at a glance">
    <h2 class="section-title">Health at a glance</h2>
    <div class="health-grid">
      <div class="stage-health-grid">
        {#each stageHealth as stage (stage.id)}
          <div class="stage-health-card">
            <div class="stage-health-top">
              <span
                class="health-dot"
                class:healthy={stage.errors === 0 && stage.warnings === 0}
                class:warn={stage.errors === 0 && stage.warnings > 0}
                class:error={stage.errors > 0}
                style:--stage-color={stage.color}
              ></span>
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
      <div class="health-side">
        <SystemStatusPanel />
        <AlertsPanel />
      </div>
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
              <span class="launch-detail">{pad.detail}</span>
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
    font-size: clamp(var(--text-2xl), 3vw, 2.4rem);
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

  .signal-row {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-2);
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

  /* ── Journey shortcuts ── */

  .journey-links {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .journey-link {
    display: grid;
    gap: 4px;
    padding: var(--space-3) var(--space-4);
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    text-decoration: none;
    color: inherit;
    transition: var(--transition-all);
  }

  .journey-link:hover {
    border-color: var(--color-border-strong);
    background: var(--color-bg-hover);
  }

  .journey-link-label {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .journey-link-hint {
    font-size: var(--text-xs);
    color: var(--color-text-muted);
    line-height: var(--leading-snug);
  }

  /* ── Active now (recents + investigations) ── */

  .now-active {
    display: grid;
    grid-template-columns: minmax(0, 1.4fr) minmax(260px, 0.8fr);
    gap: var(--space-4);
    align-items: start;
  }

  .now-col {
    min-width: 0;
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

  .investigations-card {
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
    overflow: hidden;
  }

  .empty-investigations {
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    padding: var(--space-5) var(--space-4);
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
    grid-template-columns: minmax(0, 1.2fr) minmax(280px, 0.8fr);
    gap: var(--space-4);
    align-items: start;
  }

  .stage-health-grid {
    display: grid;
    grid-template-columns: repeat(5, minmax(0, 1fr));
    gap: var(--space-2);
    align-content: start;
  }

  .health-side {
    display: grid;
    gap: var(--space-4);
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
    min-height: 150px;
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

  .launch-detail {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  /* ═══════════════════════════════════════════════════════════════════════════
   * RESPONSIVE
   * ═══════════════════════════════════════════════════════════════════════════ */

  @media (max-width: 1080px) {
    .now-head,
    .now-active,
    .health-grid {
      grid-template-columns: 1fr;
    }

    .stage-health-grid {
      grid-template-columns: repeat(3, minmax(0, 1fr));
    }
  }

  @media (max-width: 760px) {
    .journey-links {
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
