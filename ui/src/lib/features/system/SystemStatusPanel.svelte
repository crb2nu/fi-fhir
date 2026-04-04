<script lang="ts">
  import { onMount } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import { selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import { graphqlFetch } from '$lib/graphql/client';
  import { HealthDocument } from '$lib/gen/graphql';

  type HttpHealth = {
    status?: string;
    service?: string;
  };

  type CheckState<TData> =
    | { state: 'idle' | 'loading' }
    | { state: 'ok'; checkedAt: string; data: TData }
    | { state: 'error'; checkedAt: string; message: string };

  type GraphQLHealth = {
    status: string;
    version: string;
  };

  const build = {
    tag: (import.meta.env.VITE_BUILD_TAG as string | undefined) ?? null,
    sha: (import.meta.env.VITE_BUILD_SHA as string | undefined) ?? null
  };

  let http: CheckState<HttpHealth> = { state: 'idle' };
  let gql: CheckState<GraphQLHealth> = { state: 'idle' };
  let summaryState: 'healthy' | 'degraded' | 'checking' = 'checking';
  let summaryVariant: 'success' | 'warning' | 'info' = 'info';
  let summaryCopy = 'Checking backend and GraphQL surfaces.';
  let lastCheckedAt: string | null = null;

  function nowIso(): string {
    return new Date().toISOString();
  }

  async function checkHttpHealth(): Promise<void> {
    http = { state: 'loading' };
    try {
      const res = await fetch('/health', {
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      const checkedAt = nowIso();
      if (!res.ok) {
        http = { state: 'error', checkedAt, message: `HTTP ${res.status}` };
        return;
      }
      const data = (await res.json()) as HttpHealth;
      http = { state: 'ok', checkedAt, data };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      http = { state: 'error', checkedAt: nowIso(), message: msg };
    }
  }

  async function checkGraphQLHealth(): Promise<void> {
    gql = { state: 'loading' };
    try {
      const checkedAt = nowIso();
      const data = await graphqlFetch(HealthDocument);
      gql = { state: 'ok', checkedAt, data: data.health };
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      gql = { state: 'error', checkedAt: nowIso(), message: msg };
    }
  }

  function refresh(): void {
    void checkHttpHealth();
    void checkGraphQLHealth();
  }

  $: refreshing = http.state === 'loading' || gql.state === 'loading';
  $: lastCheckedAt =
    http.state === 'ok' || http.state === 'error'
      ? http.checkedAt
      : gql.state === 'ok' || gql.state === 'error'
        ? gql.checkedAt
        : null;
  $: summaryState =
    http.state === 'loading' || gql.state === 'loading' || http.state === 'idle' || gql.state === 'idle'
      ? 'checking'
      : http.state === 'error' || gql.state === 'error'
        ? 'degraded'
        : 'healthy';
  $: summaryVariant =
    summaryState === 'healthy' ? 'success' : summaryState === 'degraded' ? 'warning' : 'info';
  $: summaryCopy =
    summaryState === 'healthy'
      ? 'Backend surfaces look ready for operator work.'
      : summaryState === 'degraded'
        ? 'One or more service surfaces need attention before deeper runtime work.'
        : 'Checking backend and GraphQL surfaces.';

  onMount(() => {
    refresh();
  });
</script>

<Panel title="System status">
  <svelte:fragment slot="actions">
    <Button variant="secondary" size="sm" on:click={refresh} disabled={refreshing}>
      {refreshing ? 'Checking…' : 'Refresh'}
    </Button>
  </svelte:fragment>

  <div class="status-panel">
    <div class="summary-card">
      <div class="summary-copy">
        <div class="summary-label">Operator signal</div>
        <div class="summary-title">Service readiness</div>
        <div class="summary-body">{summaryCopy}</div>
      </div>

      <div class="summary-meta">
        <Badge variant={summaryVariant} size="sm" pill>{summaryState}</Badge>
        <span class="meta">
          {#if lastCheckedAt}
            checked {new Date(lastCheckedAt).toLocaleTimeString()}
          {:else}
            awaiting first response
          {/if}
        </span>
      </div>
    </div>

    <div class="surface-grid">
      <article class="surface-card">
        <div class="surface-top">
          <span class="surface-label">HTTP</span>
          {#if http.state === 'idle' || http.state === 'loading'}
            <Badge variant="info" size="sm" pill>checking</Badge>
          {:else if http.state === 'ok'}
            <Badge variant="success" size="sm" pill>{http.data.status ?? 'ok'}</Badge>
          {:else if http.state === 'error'}
            <Badge variant="warning" size="sm" pill>degraded</Badge>
          {:else}
            <Badge variant="default" size="sm" pill>unknown</Badge>
          {/if}
        </div>

        <div class="surface-value mono">
          {#if http.state === 'ok'}
            {http.data.service ?? 'service'}
          {:else if http.state === 'error'}
            {http.message}
          {:else}
            waiting for response
          {/if}
        </div>

        <div class="surface-detail">
          {#if http.state === 'ok' || http.state === 'error'}
            checked {new Date(http.checkedAt).toLocaleTimeString()}
          {:else}
            health endpoint
          {/if}
        </div>
      </article>

      <article class="surface-card">
        <div class="surface-top">
          <span class="surface-label">GraphQL</span>
          {#if gql.state === 'idle' || gql.state === 'loading'}
            <Badge variant="info" size="sm" pill>checking</Badge>
          {:else if gql.state === 'ok'}
            <Badge variant="success" size="sm" pill>{gql.data.status ?? 'ok'}</Badge>
          {:else if gql.state === 'error'}
            <Badge variant="warning" size="sm" pill>degraded</Badge>
          {:else}
            <Badge variant="default" size="sm" pill>unknown</Badge>
          {/if}
        </div>

        <div class="surface-value mono">
          {#if gql.state === 'ok'}
            {gql.data.version}
          {:else if gql.state === 'error'}
            {gql.message}
          {:else}
            waiting for response
          {/if}
        </div>

        <div class="surface-detail">
          {#if gql.state === 'ok' || gql.state === 'error'}
            checked {new Date(gql.checkedAt).toLocaleTimeString()}
          {:else}
            schema health
          {/if}
        </div>
      </article>
    </div>

    <div class="metadata-grid">
      <div class="meta-item">
        <span class="meta-label">Profile</span>
        <div class="meta-value">
          {#if $selectedProfile}
            <span class="pill profile">{$selectedProfile.id}</span>
            <span class="mono">v{$selectedProfile.version}</span>
          {:else}
            <span class="pill muted">none selected</span>
          {/if}
        </div>
      </div>

      <div class="meta-item">
        <span class="meta-label">UI build</span>
        <div class="meta-value">
          {#if build.tag}
            <span class="pill">{build.tag}</span>
          {:else}
            <span class="pill muted">unknown</span>
          {/if}
          {#if build.sha}
            <span class="mono">{build.sha}</span>
          {/if}
        </div>
      </div>
    </div>
  </div>
</Panel>

<style>
  .status-panel {
    display: grid;
    gap: 12px;
  }

  .summary-card {
    display: grid;
    gap: 10px;
    padding: 14px;
    border-radius: var(--radius-xl);
    border: 1px solid rgba(99, 102, 241, 0.18);
    background:
      linear-gradient(180deg, rgba(99, 102, 241, 0.16), rgba(99, 102, 241, 0.04)),
      rgba(15, 23, 42, 0.45);
  }

  .summary-copy {
    display: grid;
    gap: 4px;
  }

  .summary-label,
  .meta-label,
  .surface-label {
    color: var(--color-text-tertiary);
    font-size: var(--text-2xs);
    font-weight: var(--font-bold);
    letter-spacing: 0.12em;
    text-transform: uppercase;
  }

  .summary-title {
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
  }

  .summary-body {
    color: var(--color-text-secondary);
    font-size: var(--text-sm);
    line-height: 1.5;
  }

  .summary-meta {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .surface-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: 10px;
  }

  .surface-card,
  .meta-item {
    display: grid;
    gap: 8px;
    padding: 12px;
    border-radius: var(--radius-lg);
    border: 1px solid var(--color-border-subtle);
    background: rgba(255, 255, 255, 0.02);
  }

  .surface-top {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: 8px;
  }

  .surface-value {
    color: var(--color-text-primary);
    font-size: var(--text-sm);
    line-height: 1.4;
    word-break: break-word;
  }

  .surface-detail {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  .metadata-grid {
    display: grid;
    gap: 10px;
  }

  .mono {
    font-family: var(--font-mono);
    color: var(--color-text-secondary);
    font-size: var(--text-xs);
  }

  .meta {
    color: var(--color-text-muted);
    font-size: var(--text-xs);
  }

  .meta-value {
    display: flex;
    gap: 8px;
    align-items: center;
    flex-wrap: wrap;
  }

  .pill {
    padding: 3px 10px;
    border-radius: 999px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-surface);
    color: var(--color-text-secondary);
    font-weight: 700;
    font-size: 0.85rem;
  }

  .pill.profile {
    border-color: rgba(59, 130, 246, 0.35);
    background: rgba(59, 130, 246, 0.12);
    color: rgba(147, 197, 253, 0.95);
  }

  .pill.muted {
    color: var(--color-text-muted);
    border-color: var(--color-border-default);
  }

  @media (max-width: 760px) {
    .surface-grid {
      grid-template-columns: 1fr;
    }

    .summary-meta {
      align-items: flex-start;
    }

    .meta-value {
      grid-template-columns: 1fr;
    }
  }
</style>
