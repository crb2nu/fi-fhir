<!--
  The /hl7 session sidebar (.loom/42 E-3): the page's Integration Session as
  a piece of work. Its runs (sessionRuns) with the selected run's diagnostics
  (sessionDiagnostics) and, while that run is live, its stream
  (sessionRunEvents); what was published and simulated from it; its samples
  and drafts; and the two session-level acts, Export (audited, with a reason)
  and Archive.

  Every state is one the server reported. The shell's right sidebar is static
  per route (ui/src/lib/ui/ide/sidebar/sidebarContent.ts has no feature slot),
  so this rail lives inside the page.
-->
<script lang="ts">
  import X from '@lucide/svelte/icons/x';
  import Download from '@lucide/svelte/icons/download';
  import Archive from '@lucide/svelte/icons/archive';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import History from '@lucide/svelte/icons/history';
  import { Badge, Button, EmptyState, Icon, IconButton, KeyValue, Table, Td, Th, Tr, type BadgeTone } from '$lib/ui/primitives';
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
  import { streamStatus } from '$lib/graphql/streamAvailability';
  import SessionActionDialog from './SessionActionDialog.svelte';
  import { downloadSessionExport, sessionExportFile } from './sessionExport';
  import { rawPayloadBlockedSentence } from './sessionReason';
  import { isLiveRun, type SessionWorkspaceController, type SessionWorkspaceState } from './sessionWorkspace';

  interface Props {
    workspace: SessionWorkspaceController;
    view: SessionWorkspaceState;
    /** capabilities.phiExport and missingRoles.phiExport, as the status endpoint reported them. */
    phiExport: { allowed: boolean; missing: string[] };
    /** Why a deep-linked session cannot be opened here (the `unavailable` state). */
    unavailableReason?: string | undefined;
    /** The run the results pane shows, if it came from this sidebar or the last Preview. */
    shownRunId?: string | null | undefined;
    onshowrun: (runId: string) => void;
    oninspectpath: (path: string) => void;
    onclose: () => void;
  }

  let {
    workspace,
    view,
    phiExport,
    unavailableReason = 'Integration Sessions are not available on this deployment.',
    shownRunId = null,
    onshowrun,
    oninspectpath,
    onclose
  }: Props = $props();

  const runStream = streamStatus('sessionRunEvents');

  const session = $derived(view.session);
  const selectedRun = $derived(view.runs.find((run) => run.id === view.selectedRunId) ?? null);

  let dialog = $state<'export' | 'archive' | null>(null);
  let dialogBusy = $state(false);
  let dialogError = $state<string | null>(null);
  let lastExport = $state<{ fileName: string; reason: string; includeRawPayload: boolean } | null>(null);
  let acceptError = $state<string | null>(null);
  let accepting = $state<string | null>(null);

  function runTone(status: string): BadgeTone {
    if (status === 'completed') return 'success';
    if (status === 'failed') return 'danger';
    if (status === 'running' || status === 'pending') return 'info';
    return 'neutral';
  }

  function time(iso: string | null | undefined): string {
    if (!iso) return '';
    const parsed = Date.parse(iso);
    if (Number.isNaN(parsed)) return iso;
    return new Date(parsed).toLocaleString(undefined, {
      month: 'short',
      day: 'numeric',
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit'
    });
  }

  function duration(run: { createdAt: string; completedAt: string | null }): string {
    if (!run.completedAt) return '';
    const ms = Date.parse(run.completedAt) - Date.parse(run.createdAt);
    if (Number.isNaN(ms) || ms < 0) return '';
    return ms < 1000 ? `${ms} ms` : `${(ms / 1000).toFixed(1)} s`;
  }

  function short(id: string): string {
    return id.length > 14 ? `${id.slice(0, 14)}…` : id;
  }

  function openDialog(kind: 'export' | 'archive'): void {
    dialogError = null;
    dialogBusy = false;
    dialog = kind;
  }

  async function confirmExport(reason: string, includeRawPayload: boolean): Promise<void> {
    dialogBusy = true;
    dialogError = null;
    try {
      const bundle = await workspace.exportBundle(reason, includeRawPayload);
      const fileName = downloadSessionExport(sessionExportFile(bundle, reason, includeRawPayload));
      lastExport = { fileName, reason, includeRawPayload };
      dialog = null;
    } catch (error) {
      dialogError = error instanceof Error ? error.message : 'The export failed.';
    } finally {
      dialogBusy = false;
    }
  }

  async function confirmArchive(): Promise<void> {
    dialogBusy = true;
    dialogError = null;
    try {
      await workspace.archive();
      dialog = null;
    } catch (error) {
      dialogError = error instanceof Error ? error.message : 'Archiving failed.';
    } finally {
      dialogBusy = false;
    }
  }

  async function accept(diagnosticId: string): Promise<void> {
    accepting = diagnosticId;
    acceptError = null;
    try {
      await workspace.acceptFix(diagnosticId);
    } catch (error) {
      acceptError = error instanceof Error ? error.message : 'Accepting the fix failed.';
    } finally {
      accepting = null;
    }
  }
</script>

<aside class="rail" aria-label="Session" data-testid="hl7-session-rail" data-state={view.status.kind}>
  <header class="rail-head">
    <Icon icon={History} size={14} />
    <h2 class="rail-title">Session</h2>
    {#if view.status.kind === 'ready'}
      <IconButton icon={RefreshCw} label="Refresh session" onclick={() => void workspace.refresh()} />
    {/if}
    <IconButton icon={X} label="Close session sidebar" onclick={onclose} />
  </header>

  <div class="rail-body">
    {#if view.status.kind === 'idle'}
      <EmptyState
        align="start"
        data-testid="hl7-session-none"
        message="No session on this page yet. Preview starts one; Home › Recent reopens an earlier one."
      />
    {:else if view.status.kind === 'loading'}
      <EmptyState
        align="start"
        aria-busy="true"
        aria-live="polite"
        data-testid="hl7-session-loading"
        message={`Loading session ${view.status.sessionId}`}
      />
    {:else if view.status.kind === 'unavailable'}
      <EmptyState align="start" icon={CircleAlert} role="status" data-testid="hl7-session-unavailable">
        <span>Session <code>{view.status.sessionId}</code> cannot be opened here. {unavailableReason}</span>
      </EmptyState>
    {:else if view.status.kind === 'absent'}
      <EmptyState align="start" icon={CircleAlert} role="status" data-testid="hl7-session-absent">
        <span>
          Session <code>{view.status.sessionId}</code> is not in this deployment's session store. It may have been
          created on another deployment or tenant.
        </span>
      </EmptyState>
    {:else if view.status.kind === 'forbidden'}
      <EmptyState align="start" icon={CircleAlert} role="status" data-testid="hl7-session-forbidden">
        <span>The API refused to read session <code>{view.status.sessionId}</code> for this identity.</span>
      </EmptyState>
    {:else if view.status.kind === 'error'}
      <EmptyState
        align="start"
        icon={CircleAlert}
        role="alert"
        data-testid="hl7-session-error"
        message={`Session ${view.status.sessionId} could not be read: ${view.status.message}`}
        actionLabel="Retry"
        onaction={() => void workspace.open(view.status.kind === 'error' ? view.status.sessionId : '')}
      />
    {:else if session}
      <section class="block" data-testid="hl7-session-sidebar" data-session-id={session.id}>
        <div class="name-row">
          <span class="name" title={session.description ?? undefined}>{session.name}</span>
          {#if session.archived}
            <Badge tone="warning" data-testid="hl7-session-archived">Archived</Badge>
          {/if}
        </div>
        <KeyValue
          items={[
            { key: 'Id', value: session.id, mono: true, truncate: true },
            { key: 'Created', value: time(session.createdAt) },
            { key: 'Updated', value: time(session.updatedAt) }
          ]}
        />
        <div class="actions">
          <Button icon={Download} onclick={() => openDialog('export')} data-testid="hl7-session-export">
            Export…
          </Button>
          <Button
            icon={Archive}
            variant="ghost"
            disabled={session.archived}
            title={session.archived ? 'This session is already archived.' : undefined}
            onclick={() => openDialog('archive')}
            data-testid="hl7-session-archive"
          >
            Archive…
          </Button>
        </div>
        {#if lastExport}
          <p class="note" role="status" data-testid="hl7-session-export-done">
            Exported <span class="mono">{lastExport.fileName}</span>{lastExport.includeRawPayload
              ? ' with raw sample payloads'
              : ' without sample text'}. The reason is recorded with your identity on the export.
          </p>
        {/if}
      </section>

      <section class="block" aria-label="Runs">
        <h3 class="block-title">Runs <span class="count">{view.runs.length}</span></h3>
        {#if view.runsError}
          <p class="note error" role="alert">Runs could not be read: {view.runsError}</p>
        {:else if view.runs.length === 0}
          <p class="note" data-testid="hl7-session-runs-empty">No runs yet. Preview runs on this session.</p>
        {:else}
          <Table label="Session runs" layout="fixed">
            {#snippet head()}
              <tr>
                <Th width="96px">Status</Th>
                <Th>Started</Th>
                <Th width="64px" numeric>Took</Th>
              </tr>
            {/snippet}
            {#each view.runs as run (run.id)}
              <Tr
                selectable
                selected={run.id === view.selectedRunId}
                onselect={() => void workspace.selectRun(run.id)}
                title={run.id}
                data-testid="hl7-session-run"
                data-run-id={run.id}
                data-status={run.status}
              >
                <Td><Badge tone={runTone(run.status)} dot>{run.status}</Badge></Td>
                <Td muted truncate value={time(run.createdAt)} />
                <Td numeric muted value={duration(run)} />
              </Tr>
            {/each}
          </Table>
        {/if}
      </section>

      {#if selectedRun}
        <section class="block" aria-label="Selected run" data-testid="hl7-session-selected-run" data-run-id={selectedRun.id}>
          <div class="name-row">
            <h3 class="block-title">Run <span class="mono">{short(selectedRun.id)}</span></h3>
            {#if shownRunId === selectedRun.id}
              <Badge tone="info">In results</Badge>
            {:else}
              <Button variant="ghost" onclick={() => onshowrun(selectedRun.id)} data-testid="hl7-session-show-run">
                Show in results
              </Button>
            {/if}
          </div>

          {#if isLiveRun(selectedRun) || view.stream.state !== 'idle'}
            {#if view.stream.state === 'unavailable'}
              <StreamingUnavailable
                compact
                root="sessionRunEvents"
                subject="this run"
                reason={$runStream.availability === 'unavailable' ? $runStream.reason : 'streaming-off'}
                alternative="Refresh the session to see its progress."
              />
            {:else}
              <p class="note" role="status" data-testid="hl7-session-run-stream" data-state={view.stream.state}>
                {#if view.stream.state === 'connecting'}
                  Connecting to this run's stream.
                {:else if view.stream.state === 'open'}
                  Live · {view.stream.events} event{view.stream.events === 1 ? '' : 's'} received.
                {:else if view.stream.state === 'closed'}
                  The run finished; its stream is closed.
                {:else if view.stream.state === 'error'}
                  The run stream failed: {view.stream.message}
                {:else}
                  This run is {selectedRun.status}.
                {/if}
              </p>
            {/if}
          {/if}

          {#if view.diagnosticsLoading && !view.diagnostics}
            <p class="note" aria-busy="true">Reading diagnostics.</p>
          {:else if view.diagnosticsError}
            <p class="note error" role="alert">Diagnostics could not be read: {view.diagnosticsError}</p>
          {:else if view.diagnostics && view.diagnostics.length === 0}
            <p class="note" data-testid="hl7-session-diagnostics-empty">This run recorded no diagnostics.</p>
          {:else if view.diagnostics}
            <ul class="diagnostics" aria-label="Run diagnostics">
              {#each view.diagnostics as diagnostic (diagnostic.id)}
                <li class="diagnostic" data-testid="hl7-session-diagnostic" data-accepted={diagnostic.accepted}>
                  <div class="diag-head">
                    <Badge tone={diagnostic.severity === 'error' ? 'danger' : 'warning'} mono>{diagnostic.code}</Badge>
                    {#if diagnostic.path}
                      <button class="path-link mono" type="button" onclick={() => oninspectpath(diagnostic.path ?? '')}>
                        {diagnostic.path}
                      </button>
                    {/if}
                  </div>
                  <p class="diag-message">{diagnostic.message}</p>
                  {#if diagnostic.fixSuggestion}
                    <p class="diag-fix">{diagnostic.fixSuggestion}</p>
                  {/if}
                  {#if diagnostic.accepted}
                    <Badge tone="success" title={diagnostic.acceptedAt ?? undefined}>Fix accepted</Badge>
                  {:else if diagnostic.fixSuggestion}
                    <Button
                      variant="ghost"
                      loading={accepting === diagnostic.id}
                      disabled={accepting !== null}
                      onclick={() => void accept(diagnostic.id)}
                    >
                      Accept fix
                    </Button>
                  {/if}
                </li>
              {/each}
            </ul>
            {#if acceptError}
              <p class="note error" role="alert">{acceptError}</p>
            {/if}
          {/if}
        </section>
      {/if}

      <section class="block" aria-label="Publications">
        <h3 class="block-title">Publications <span class="count">{session.publications.length}</span></h3>
        {#if session.publications.length === 0}
          <p class="note" data-testid="hl7-session-publications-empty">Nothing has been published from this session.</p>
        {:else}
          <ul class="plain">
            {#each session.publications as publication (publication.id)}
              <li data-testid="hl7-session-publication">
                <span class="mono">v{publication.version}</span>
                <span class="mono" title={publication.definitionRevision.digest}>
                  {publication.definitionRevision.artifactId}@{publication.definitionRevision.revisionId}
                </span>
                <span class="muted">{publication.publishedBy} · {time(publication.createdAt)}</span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="block" aria-label="Simulations">
        <h3 class="block-title">Simulations <span class="count">{session.workflowSimulations.length}</span></h3>
        {#if session.workflowSimulations.length === 0}
          <p class="note" data-testid="hl7-session-simulations-empty">No workflow has been simulated against these runs.</p>
        {:else}
          <ul class="plain">
            {#each session.workflowSimulations as simulation (simulation.id)}
              <li data-testid="hl7-session-simulation">
                <span class="mono" title={simulation.workflowRevisionDigest}>
                  {simulation.workflowArtifactId}@{simulation.workflowRevisionId}
                </span>
                <span class="muted">
                  {simulation.sourceRunIds.length} run{simulation.sourceRunIds.length === 1 ? '' : 's'} · {time(simulation.createdAt)}
                </span>
              </li>
            {/each}
          </ul>
        {/if}
      </section>

      <section class="block" aria-label="Samples and drafts">
        <h3 class="block-title">Samples <span class="count">{session.samples.length}</span></h3>
        {#if session.samples.length > 0}
          <ul class="plain">
            {#each session.samples as sample (sample.id)}
              <li>
                <span>{sample.name}</span>
                <span class="muted mono">{sample.source ?? sample.format} · {time(sample.createdAt)}</span>
              </li>
            {/each}
          </ul>
          <p class="note">Sample text stays on the server. Captured and peeked samples load into Samples.</p>
        {/if}
        <h3 class="block-title">Drafts</h3>
        {#if !session.currentProfileDraft && !session.currentWorkflowDraft}
          <p class="note">No profile or workflow draft is saved in this session.</p>
        {:else}
          <ul class="plain">
            {#if session.currentProfileDraft}
              <li>
                <span>Profile · {session.currentProfileDraft.name}</span>
                <span class="muted mono" title={session.currentProfileDraft.digest}>
                  v{session.currentProfileDraft.version} · {session.currentProfileDraft.revisionId}
                </span>
              </li>
            {/if}
            {#if session.currentWorkflowDraft}
              <li>
                <span>Workflow · {session.currentWorkflowDraft.name}</span>
                <span class="muted mono" title={session.currentWorkflowDraft.digest}>
                  v{session.currentWorkflowDraft.version} · {session.currentWorkflowDraft.revisionId}
                </span>
              </li>
            {/if}
          </ul>
        {/if}
      </section>
    {/if}
  </div>
</aside>

<SessionActionDialog
  open={dialog === 'export'}
  testid="hl7-session-export-dialog"
  title="Export session"
  description="Downloads this session's runs, diagnostics, drafts, simulations and publications as JSON. An export is a PHI disclosure: the API records the reason and your identity on an append-only export record."
  confirmText="Export"
  reasonHint="Recorded with your verified identity on the export record."
  rawPayload={{ allowed: phiExport.allowed, blockedSentence: rawPayloadBlockedSentence(phiExport.missing) }}
  loading={dialogBusy}
  submitError={dialogError}
  onconfirm={(reason, raw) => void confirmExport(reason, raw)}
  oncancel={() => (dialog = null)}
/>

<SessionActionDialog
  open={dialog === 'archive'}
  testid="hl7-session-archive-dialog"
  title="Archive session"
  description="Archiving removes this session from Home › Recent. Its runs, publications and export records stay, and its link still opens it here. The API records no reason for an archive."
  confirmText="Archive"
  variant="danger"
  reasonRequired={false}
  loading={dialogBusy}
  submitError={dialogError}
  onconfirm={() => void confirmArchive()}
  oncancel={() => (dialog = null)}
/>

<style>
  .rail {
    display: flex;
    flex-direction: column;
    width: 300px;
    min-width: 300px;
    height: 100%;
    min-height: 0;
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-surface, var(--color-bg-elevated));
  }

  .rail-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 0 0 auto;
    height: 36px;
    padding: 0 var(--space-1) 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
    color: var(--color-text-secondary);
  }

  .rail-title {
    flex: 1 1 auto;
    margin: 0;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .rail-body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
  }

  .block {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .block-title {
    margin: 0;
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-tertiary);
  }

  .count {
    margin-left: var(--space-1);
    font-family: var(--font-mono);
    color: var(--color-text-secondary);
  }

  .name-row {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
    min-width: 0;
  }

  .name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .actions {
    display: flex;
    gap: var(--space-2);
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .note.error {
    color: var(--color-danger-text);
  }

  .mono,
  code {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .muted {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .plain,
  .diagnostics {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .plain li {
    display: grid;
    gap: 2px;
    min-width: 0;
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
  }

  .plain li span {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .diagnostic {
    display: grid;
    gap: var(--space-1);
    justify-items: start;
    padding-bottom: var(--space-2);
    border-bottom: 1px dashed var(--color-border-subtle);
  }

  .diag-head {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .diag-message,
  .diag-fix {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .diag-fix {
    color: var(--color-text-tertiary);
  }

  .path-link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-primary);
    cursor: pointer;
  }

  .path-link:hover {
    text-decoration: underline;
  }
</style>
