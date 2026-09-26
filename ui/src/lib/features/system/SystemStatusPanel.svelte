<!--
  SystemStatusPanel — the home Health panel. Only real sources: the backend's
  `/health`, the GraphQL `health` query, and the shell's `/health` poll
  (connectionStore). Each check is a row with its own status and the time it
  was checked; nothing here is inferred or simulated.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import {
    Badge,
    IconButton,
    KeyValue,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type BadgeTone
  } from '$lib/ui/primitives';
  import { selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import { connectionState } from '$lib/stores/connectionStore';
  import { graphqlFetch } from '$lib/graphql/client';
  import { HealthDocument } from '$lib/gen/graphql';

  type HttpHealth = {
    status?: string;
    service?: string;
    version?: string;
  };

  type GraphQLHealth = {
    status: string;
    version: string;
  };

  type CheckState<TData> =
    | { state: 'idle' | 'loading' }
    | { state: 'ok'; checkedAt: string; data: TData }
    | { state: 'error'; checkedAt: string; message: string };

  const build = {
    tag: (import.meta.env.VITE_BUILD_TAG as string | undefined) ?? null,
    sha: (import.meta.env.VITE_BUILD_SHA as string | undefined) ?? null
  };

  let http = $state<CheckState<HttpHealth>>({ state: 'idle' });
  let gql = $state<CheckState<GraphQLHealth>>({ state: 'idle' });

  async function checkHttpHealth(): Promise<void> {
    http = { state: 'loading' };
    try {
      const res = await fetch('/health', {
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      const checkedAt = new Date().toISOString();
      if (!res.ok) {
        http = { state: 'error', checkedAt, message: `HTTP ${res.status}` };
        return;
      }
      http = { state: 'ok', checkedAt, data: (await res.json()) as HttpHealth };
    } catch (e) {
      http = {
        state: 'error',
        checkedAt: new Date().toISOString(),
        message: e instanceof Error ? e.message : String(e)
      };
    }
  }

  async function checkGraphQLHealth(): Promise<void> {
    gql = { state: 'loading' };
    try {
      const data = await graphqlFetch(HealthDocument, {}, { showErrorToast: false });
      gql = { state: 'ok', checkedAt: new Date().toISOString(), data: data.health };
    } catch (e) {
      gql = {
        state: 'error',
        checkedAt: new Date().toISOString(),
        message: e instanceof Error ? e.message : String(e)
      };
    }
  }

  function refresh(): void {
    void checkHttpHealth();
    void checkGraphQLHealth();
  }

  const refreshing = $derived(http.state === 'loading' || gql.state === 'loading');
  const pending = (check: CheckState<unknown>) =>
    check.state === 'idle' || check.state === 'loading';

  const summary = $derived.by((): { label: string; tone: BadgeTone } => {
    if (pending(http) || pending(gql)) return { label: 'Checking', tone: 'neutral' };
    if (http.state === 'error' || gql.state === 'error') return { label: 'Degraded', tone: 'warning' };
    return { label: 'Healthy', tone: 'success' };
  });

  function statusOf(check: CheckState<{ status?: string }>): { label: string; tone: BadgeTone } {
    if (check.state === 'ok') {
      const status = check.data.status ?? 'ok';
      return { label: status, tone: status === 'healthy' || status === 'ok' ? 'success' : 'warning' };
    }
    if (check.state === 'error') return { label: 'error', tone: 'danger' };
    return { label: 'checking', tone: 'neutral' };
  }

  function detailOf(check: CheckState<{ version?: string; service?: string }>): string | undefined {
    if (check.state === 'ok') return check.data.version || check.data.service || undefined;
    if (check.state === 'error') return check.message;
    return undefined;
  }

  function checkedOf(check: CheckState<unknown>): string | undefined {
    return check.state === 'ok' || check.state === 'error'
      ? new Date(check.checkedAt).toLocaleTimeString()
      : undefined;
  }

  const connection = $derived.by((): { label: string; tone: BadgeTone } => {
    switch ($connectionState) {
      case 'connected':
        return { label: 'connected', tone: 'success' };
      case 'connecting':
        return { label: 'connecting', tone: 'neutral' };
      default:
        return { label: 'disconnected', tone: 'danger' };
    }
  });

  const httpStatus = $derived(statusOf(http));
  const gqlStatus = $derived(statusOf(gql));

  onMount(refresh);
</script>

<Panel title="Health" flush data-testid="health-panel">
  {#snippet actions()}
    <Badge tone={summary.tone} dot data-testid="health-summary">{summary.label}</Badge>
    <IconButton
      icon={RefreshCw}
      label="Refresh health checks"
      loading={refreshing}
      onclick={refresh}
    />
  {/snippet}

  <Table label="Health checks" layout="fixed">
    {#snippet head()}
      <tr>
        <Th width="34%">Check</Th>
        <Th width="104px">Status</Th>
        <Th>Detail</Th>
        <Th width="92px" numeric>Checked</Th>
      </tr>
    {/snippet}
    <Tr>
      <Td>Backend <code class="path">/health</code></Td>
      <Td><Badge tone={httpStatus.tone} dot>{httpStatus.label}</Badge></Td>
      <Td mono truncate muted value={detailOf(http)} />
      <Td numeric muted value={checkedOf(http)} />
    </Tr>
    <Tr>
      <Td>GraphQL <code class="path">health</code></Td>
      <Td><Badge tone={gqlStatus.tone} dot>{gqlStatus.label}</Badge></Td>
      <Td mono truncate muted value={detailOf(gql)} />
      <Td numeric muted value={checkedOf(gql)} />
    </Tr>
    <Tr>
      <Td>Shell poll</Td>
      <Td><Badge tone={connection.tone} dot>{connection.label}</Badge></Td>
      <Td muted value="every 30 s" />
      <Td numeric muted />
    </Tr>
  </Table>

  <div class="meta">
    <KeyValue
      columns={2}
      items={[
        {
          key: 'Profile',
          value: $selectedProfile ? `${$selectedProfile.id} v${$selectedProfile.version}` : null,
          mono: true
        },
        {
          key: 'UI build',
          value: [build.tag, build.sha].filter(Boolean).join(' · ') || null,
          mono: true,
          truncate: true
        }
      ]}
    />
  </div>
</Panel>

<style>
  .path {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .meta {
    padding: var(--space-2) var(--space-3) var(--space-3);
  }
</style>
