<!--
  RecentWork — the home "Recent" panel. Two real sources only:
  - documents open in the workspace (ideStore, restored from the saved layout),
    excluding Home itself; selecting one reopens it;
  - the tenant's integration sessions (HL7 intake previews run on the session
    engine), when this identity may read them.
  With neither, it says so and offers HL7 intake.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import FileInput from '@lucide/svelte/icons/file-input';
  import { Badge, EmptyState, Panel, Table, Td, Th, Tr, type BadgeTone } from '$lib/ui/primitives';
  import { ideState, setActiveTab } from '$lib/ui/ide/ideStore';
  import type { IDEAppRoute, WorkspaceDocument } from '$lib/ui/ide/types';
  import { integrationSessionsCapability } from '$lib/graphql/accessCapabilities';
  import { fetchRecentSessions, type RecentSession } from './dashboardApi';

  interface Row {
    key: string;
    name: string;
    kind: 'Document' | 'Session';
    reference: string;
    status: { label: string; tone: BadgeTone } | null;
    updatedAt: string | null;
    open: (() => void) | null;
  }

  let sessions = $state<RecentSession[]>([]);
  let sessionError = $state<string | null>(null);
  let sessionsLoaded = $state(false);

  function documentRoute(doc: WorkspaceDocument): string | undefined {
    return doc.path ?? doc.route;
  }

  function openDocument(doc: WorkspaceDocument): void {
    setActiveTab(doc.id);
    const route = documentRoute(doc);
    if ((doc.type ?? 'route') === 'route' && route) {
      void goto(resolve(route as IDEAppRoute));
    }
  }

  function runStatus(session: RecentSession): Row['status'] {
    const last = session.runs[session.runs.length - 1];
    if (!last) return { label: 'no runs', tone: 'neutral' };
    const tone: BadgeTone =
      last.status === 'completed'
        ? 'success'
        : last.status === 'failed'
          ? 'danger'
          : last.status === 'running'
            ? 'info'
            : 'neutral';
    return { label: last.status, tone };
  }

  const documentRows = $derived<Row[]>(
    $ideState.documents
      .filter((doc) => documentRoute(doc) !== '/')
      .map((doc) => ({
        key: `doc:${doc.id}`,
        name: doc.subtitle ? `${doc.title} — ${doc.subtitle}` : doc.title,
        kind: 'Document',
        reference: documentRoute(doc) ?? doc.artifactId ?? doc.type ?? '',
        status: doc.dirty ? { label: 'unsaved', tone: 'warning' } : null,
        updatedAt: null,
        open: () => openDocument(doc)
      }))
  );

  const sessionRows = $derived<Row[]>(
    sessions.map((session) => ({
      key: `session:${session.id}`,
      name: session.name,
      kind: 'Session',
      reference: session.id,
      status: runStatus(session),
      updatedAt: session.updatedAt,
      open: null
    }))
  );

  const rows = $derived([...documentRows, ...sessionRows]);
  const settled = $derived(sessionsLoaded || $integrationSessionsCapability !== true);

  function relative(iso: string | null): string | undefined {
    if (!iso) return undefined;
    const diff = Date.now() - Date.parse(iso);
    if (Number.isNaN(diff)) return iso;
    if (diff < 60_000) return 'just now';
    if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
    if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
    return `${Math.floor(diff / 86_400_000)}d ago`;
  }

  async function loadSessions(): Promise<void> {
    if ($integrationSessionsCapability !== true) return;
    try {
      sessions = await fetchRecentSessions();
      sessionError = null;
    } catch (err) {
      sessions = [];
      sessionError = err instanceof Error ? err.message : 'Sessions could not be loaded';
    } finally {
      sessionsLoaded = true;
    }
  }

  onMount(() => {
    void loadSessions();
  });
</script>

<Panel title="Recent" flush data-testid="recent-panel">
  {#if rows.length === 0 && settled}
    <EmptyState
      icon={FileInput}
      align="start"
      message="Nothing opened yet."
      actionLabel="Open HL7 intake"
      onaction={() => void goto(resolve('/hl7'))}
    />
  {:else if rows.length > 0}
    <Table label="Recent documents and sessions" layout="fixed">
      {#snippet head()}
        <tr>
          <Th>Name</Th>
          <Th width="96px">Kind</Th>
          <Th width="28%">Reference</Th>
          <Th width="112px">Status</Th>
          <Th width="88px" numeric>Updated</Th>
        </tr>
      {/snippet}
      {#each rows as row (row.key)}
        <Tr
          selectable={row.open !== null}
          onselect={row.open ?? undefined}
          title={row.open ? `Open ${row.name}` : undefined}
        >
          <Td truncate value={row.name} />
          <Td muted value={row.kind} />
          <Td mono truncate muted value={row.reference} />
          <Td>
            {#if row.status}
              <Badge tone={row.status.tone} dot>{row.status.label}</Badge>
            {/if}
          </Td>
          <Td numeric muted title={row.updatedAt ?? undefined} value={relative(row.updatedAt)} />
        </Tr>
      {/each}
    </Table>
  {/if}
  {#if sessionError}
    <p class="note" role="status">Integration sessions could not be loaded: {sessionError}</p>
  {/if}
</Panel>

<style>
  .note {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
