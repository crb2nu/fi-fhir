<script lang="ts">
  import { onMount } from 'svelte';
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

  onMount(() => {
    refresh();
  });
</script>

<Panel title="System status">
  <div class="grid">
    <div class="row">
      <div class="label">HTTP</div>
      <div class="value">
        {#if http.state === 'idle' || http.state === 'loading'}
          <span class="pill muted">checking…</span>
        {:else if http.state === 'ok'}
          <span class="pill ok">{http.data.status ?? 'ok'}</span>
          <span class="mono">{http.data.service ?? 'service'}</span>
          <span class="meta">checked {new Date(http.checkedAt).toLocaleTimeString()}</span>
        {:else if http.state === 'error'}
          <span class="pill bad">unhealthy</span>
          <span class="mono">{http.message}</span>
          <span class="meta">checked {new Date(http.checkedAt).toLocaleTimeString()}</span>
        {:else}
          <span class="pill muted">unknown</span>
        {/if}
      </div>
      <div class="actions">
        <Button variant="secondary" on:click={refresh} disabled={refreshing}>
          Refresh
        </Button>
      </div>
    </div>

    <div class="row">
      <div class="label">GraphQL</div>
      <div class="value">
        {#if gql.state === 'idle' || gql.state === 'loading'}
          <span class="pill muted">checking…</span>
        {:else if gql.state === 'ok'}
          <span class="pill ok">{gql.data.status ?? 'ok'}</span>
          <span class="mono">{gql.data.version}</span>
          <span class="meta">checked {new Date(gql.checkedAt).toLocaleTimeString()}</span>
        {:else if gql.state === 'error'}
          <span class="pill bad">unhealthy</span>
          <span class="mono">{gql.message}</span>
          <span class="meta">checked {new Date(gql.checkedAt).toLocaleTimeString()}</span>
        {:else}
          <span class="pill muted">unknown</span>
        {/if}
      </div>
    </div>

    <div class="row">
      <div class="label">Profile</div>
      <div class="value">
        {#if $selectedProfile}
          <span class="pill profile">{$selectedProfile.id}</span>
          <span class="mono">v{$selectedProfile.version}</span>
        {:else}
          <span class="pill muted">none selected</span>
        {/if}
      </div>
    </div>

    <div class="row">
      <div class="label">UI build</div>
      <div class="value">
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
</Panel>

<style>
  .grid {
    display: grid;
    gap: 10px;
  }

  .row {
    display: grid;
    grid-template-columns: 90px 1fr auto;
    gap: 10px;
    align-items: center;
  }

  .row:nth-child(n + 2) {
    grid-template-columns: 90px 1fr;
  }

  .label {
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.9rem;
    font-weight: 800;
  }

  .value {
    display: flex;
    gap: 10px;
    align-items: baseline;
    flex-wrap: wrap;
  }

  .actions {
    justify-self: end;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    color: rgba(229, 231, 235, 0.88);
    font-size: 0.9rem;
  }

  .meta {
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.85rem;
  }

  .pill {
    padding: 3px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 700;
    font-size: 0.85rem;
  }

  .pill.ok {
    border-color: rgba(16, 185, 129, 0.35);
    background: rgba(16, 185, 129, 0.12);
  }

  .pill.bad {
    border-color: rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.12);
  }

  .pill.profile {
    border-color: rgba(59, 130, 246, 0.35);
    background: rgba(59, 130, 246, 0.12);
    color: rgba(147, 197, 253, 0.95);
  }

  .pill.muted {
    color: rgba(229, 231, 235, 0.55);
    border-color: rgba(255, 255, 255, 0.08);
  }

  @media (max-width: 640px) {
    .row {
      grid-template-columns: 1fr;
      gap: 8px;
      align-items: start;
    }

    .actions {
      justify-self: start;
    }
  }
</style>
