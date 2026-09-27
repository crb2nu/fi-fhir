<!--
  The details pane of one connection (or of a new one): its honest status, the
  write actions — each through the reason dialog, each carrying the draft
  version it started from — and four tabs: Settings, Secrets, Revisions, Usage.

  The spec is checked with validateConnectionSpec a moment after every change
  (no write, no toast); its problems land on their fields. A save refused for
  secret material lands the refused paths on their fields too, and a refused
  compile lands its problems the same way.
-->
<script lang="ts">
  import Archive from '@lucide/svelte/icons/archive';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, Button, IconButton, Tabs, type TabItem } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import ConnectionForm from './ConnectionForm.svelte';
  import ConnectionReasonDialog from './ConnectionReasonDialog.svelte';
  import ConnectionRevisions from './ConnectionRevisions.svelte';
  import ConnectionUsage from './ConnectionUsage.svelte';
  import SecretBindingsEditor from './SecretBindingsEditor.svelte';
  import {
    archiveConnection,
    compileConnection,
    createConnection,
    fetchConnection,
    fetchConnectionRevisions,
    updateConnection,
    validateConnectionSpec,
    type ConnectionRevisionRow,
    type ConnectionRow
  } from './connectionsApi';
  import { writeBlockedReason } from './connectionsAccess';
  import {
    describeConnectionFailure,
    describeRefusal,
    refusedAtWrite,
    specRejectionProblems
  } from './connectionsErrors';
  import { connectionStatus } from './connectionStatus';
  import {
    bindingsOf,
    descriptionProblem,
    idProblem,
    isDirty,
    nameProblem,
    specOf,
    validationKey,
    type EditBuffer
  } from './editBuffer';
  import { isBlocking, kindSchema, placeProblems, type KindSchema } from './specSchema';

  type DetailsTab = 'settings' | 'secrets' | 'revisions' | 'usage';
  type WriteAction = 'create' | 'save' | 'compile' | 'archive';


  interface Props {
    buffer: EditBuffer;
    /** The stored connection; null while creating. */
    row: ConnectionRow | null;
    /** Roles this identity lacks to change connections, or null. */
    writeBlocked: string[] | null;
    /** A write succeeded (or a reload answered): the catalog's current row. */
    onchanged: (row: ConnectionRow) => void;
    /** Reload found nothing: the connection is gone from this tenant. */
    onmissing: () => void;
    /** Discard the edits (or cancel the creation). */
    ondiscard: () => void;
  }

  let { buffer = $bindable(), row, writeBlocked, onchanged, onmissing, ondiscard }: Props = $props();

  const schema = $derived(kindSchema(buffer.kind) as KindSchema);
  const creating = $derived(buffer.mode === 'create' || row === null);
  const archived = $derived(row?.archived ?? false);
  const readOnly = $derived(writeBlocked !== null || archived);
  const dirty = $derived(isDirty(buffer));
  const key = $derived(validationKey(buffer));
  const bindingNames = $derived(buffer.bindings.map((binding) => binding.name.trim()));
  const placed = $derived(placeProblems(schema, buffer.problems, buffer.values, bindingNames));
  const fresh = $derived(buffer.problemsKey === key);
  const blocking = $derived(buffer.problems.filter(isBlocking).length);
  const warnings = $derived(buffer.problems.length - blocking);
  // What a draft write is refused for, whatever else the spec lacks (C-0 writeProblems).
  const writeRefusal = $derived(fresh ? buffer.problems.find(refusedAtWrite) : undefined);
  const tokens = $derived(row ? connectionStatus(row) : []);

  const identityErrors = $derived({
    id: creating ? idProblem(buffer.id.trim()) : null,
    name: nameProblem(buffer.name),
    description: descriptionProblem(buffer.description)
  });
  const identityError = $derived(identityErrors.id ?? identityErrors.name ?? identityErrors.description);

  const title = $derived(
    row?.name || buffer.name.trim() || `New ${schema.label} ${schema.direction === 'source' ? 'source' : 'destination'}`
  );

  let tab = $state<DetailsTab>('settings');
  // Unique per instance: Sources and Destinations each keep a details pane mounted.
  const uid = $props.id();
  const panelId = `${uid}-details-panel`;

  const tabs = $derived<TabItem[]>([
    { id: 'settings', label: 'Settings', controls: panelId },
    {
      id: 'secrets',
      label: 'Secrets',
      count: buffer.bindings.length > 0 ? buffer.bindings.length : undefined,
      controls: panelId
    },
    { id: 'revisions', label: 'Revisions', disabled: creating, controls: panelId },
    { id: 'usage', label: 'Usage', disabled: creating, controls: panelId }
  ]);

  // ── validation: debounced, no write, no toast ──

  let validating = $state(false);
  let validationError = $state<string | null>(null);
  let validationSeq = 0;

  // A stored draft is checked as soon as it is shown (what it lacks before it
  // compiles); a new one only once something was entered, so an untouched form
  // does not open covered in "is required".
  const checking = $derived(writeBlocked === null && !archived && !(creating && !dirty));

  $effect(() => {
    const current = key;
    if (!checking || buffer.problemsKey === current) return;
    const kind = schema.graphqlKind;
    const spec = specOf(buffer);
    const secretBindings = bindingsOf(buffer);
    const target = buffer;
    const timer = setTimeout(() => {
      const seq = ++validationSeq;
      validating = true;
      validateConnectionSpec({ kind, spec, secretBindings })
        .then((problems) => {
          if (seq !== validationSeq) return;
          target.problems = problems;
          target.problemsKey = current;
          validationError = null;
        })
        .catch((err: unknown) => {
          if (seq !== validationSeq) return;
          validationError = describeConnectionFailure(err).message;
        })
        .finally(() => {
          if (seq === validationSeq) validating = false;
        });
    }, 400);
    return () => clearTimeout(timer);
  });

  const checkStatus = $derived.by(() => {
    if (!checking) return null;
    if (validationError) return { text: `The spec could not be checked. ${validationError}`, tone: 'danger' as const };
    if (validating || !fresh) return { text: 'Checking the spec…', tone: 'neutral' as const };
    if (blocking > 0) {
      const counted = `${blocking} problem${blocking === 1 ? '' : 's'} block compile`;
      return {
        text: warnings > 0 ? `${counted}; ${warnings} warning${warnings === 1 ? '' : 's'}.` : `${counted}.`,
        tone: 'danger' as const
      };
    }
    if (warnings > 0) return { text: `No blocking problems; ${warnings} warning${warnings === 1 ? '' : 's'}.`, tone: 'warning' as const };
    return { text: 'No problems: this spec compiles.', tone: 'success' as const };
  });

  // ── revisions (Revisions and Usage) ──

  let revisions = $state<ConnectionRevisionRow[] | null>(null);
  let revisionsLoading = $state(false);
  let revisionsError = $state<string | null>(null);
  let selectedRevisionId = $state<string | null>(null);
  let revisionsFor = '';
  let revisionSeq = 0;

  async function loadRevisions(): Promise<void> {
    if (!row) return;
    const id = row.id;
    const seq = ++revisionSeq;
    revisionsLoading = true;
    revisionsError = null;
    try {
      const list = await fetchConnectionRevisions(id);
      if (seq !== revisionSeq) return;
      revisions = list;
      if (!list.some((revision) => revision.revisionId === selectedRevisionId)) {
        selectedRevisionId = list[0]?.revisionId ?? null;
      }
    } catch (err) {
      if (seq !== revisionSeq) return;
      revisionsError = describeConnectionFailure(err).message;
    } finally {
      if (seq === revisionSeq) revisionsLoading = false;
    }
  }

  $effect(() => {
    // Refetch when the tab needs them and whenever a compile moved the latest revision.
    const wanted = tab === 'revisions' || tab === 'usage';
    const marker = row ? `${row.id}:${row.latestRevision?.revisionId ?? ''}` : '';
    if (!wanted || !row) return;
    if (marker === revisionsFor && (revisions !== null || revisionsLoading || revisionsError !== null)) return;
    revisionsFor = marker;
    void loadRevisions();
  });

  // ── writes ──

  let dialogAction = $state<WriteAction | null>(null);
  let dialogBusy = $state(false);
  let dialogError = $state<string | null>(null);
  let dialogStale = $state(false);
  let reloading = $state(false);

  const writeReason = $derived(writeBlockedReason(writeBlocked));

  const saveBlocked = $derived.by((): string | null => {
    if (writeReason) return writeReason;
    if (archived) return 'Archived connections accept no change.';
    if (!creating && !dirty) return 'No unsaved changes.';
    if (identityError) return identityError;
    if (writeRefusal?.code === 'SECRET_VALUE_FORBIDDEN') {
      return 'Remove the secret value first: a spec names a binding, never a value.';
    }
    if (writeRefusal?.path.startsWith('secret_bindings')) {
      return 'Complete or remove the secret binding marked in Secrets first.';
    }
    if (writeRefusal?.code === 'UNBOUND_SECRET') {
      return 'Choose a declared binding in every binding field first.';
    }
    if (writeRefusal) return 'Remove the key this connection kind does not have first.';
    return null;
  });

  // The catalog moved past the version this form was made from (a list
  // reload kept the edits). Even with the edits undone the form is not
  // showing the saved draft, so nothing may act on row.version until reload.
  const stale = $derived(row !== null && buffer.version !== row.version);
  const STALE_REASON = 'Reload first: this connection changed since you opened it.';

  const compileBlocked = $derived.by((): string | null => {
    if (writeReason) return writeReason;
    if (!row) return 'Create the connection first.';
    if (archived) return 'Archived connections accept no change.';
    if (stale) return STALE_REASON;
    if (dirty) return 'Save the draft first: compile reads the saved version.';
    const latest = row.latestRevision;
    if (latest && latest.compiledFromVersion === row.version) {
      return `r${latest.revisionId} was already compiled from this draft version.`;
    }
    if (fresh && blocking > 0) return `${blocking} problem${blocking === 1 ? '' : 's'} block compile.`;
    return null;
  });

  const archiveBlocked = $derived.by((): string | null => {
    if (writeReason) return writeReason;
    if (archived) return 'This connection is already archived.';
    if (stale) return STALE_REASON;
    if (dirty) return 'Save or discard your edits first.';
    return null;
  });

  function openDialog(action: WriteAction): void {
    dialogAction = action;
    dialogError = null;
    dialogStale = false;
  }

  function closeDialog(): void {
    if (dialogBusy) return;
    dialogAction = null;
    dialogError = null;
    dialogStale = false;
  }

  const dialog = $derived.by(() => {
    const id = row?.id ?? buffer.id.trim();
    switch (dialogAction) {
      case 'create':
        return {
          title: `Create ${schema.label} ${schema.direction}`,
          confirm: 'Create',
          variant: 'primary' as const,
          description: `Creates ${id} at draft version 1. A draft may be incomplete; compile reports what is missing. Nothing is mounted by creating it.`
        };
      case 'save':
        return {
          title: `Save ${id}`,
          confirm: 'Save',
          variant: 'primary' as const,
          description: `Saves your edits over draft version ${buffer.version}. If someone saved a newer version first, this is refused rather than overwriting it.`
        };
      case 'compile':
        return {
          title: `Compile ${id}`,
          confirm: 'Compile',
          variant: 'primary' as const,
          description: `Compiles draft version ${row?.version ?? buffer.version} with the ${schema.label} document constructor into a new immutable revision. Nothing is mounted until a deployment references its digest.`
        };
      case 'archive':
        return {
          title: `Archive ${id}`,
          confirm: 'Archive',
          variant: 'danger' as const,
          description: `The draft accepts no further change. Its revisions stay readable, and definitions that reference them keep resolving.`
        };
      default:
        return { title: '', confirm: 'Confirm', variant: 'primary' as const, description: '' };
    }
  });

  function failed(err: unknown): void {
    const refused = specRejectionProblems(err);
    if (refused !== null && refused.length > 0) {
      // Every refused path lands on its field (or in the problem list), whatever its code.
      buffer.problems = refused;
      buffer.problemsKey = key;
      tab = 'settings';
      dialogError = describeRefusal(refused);
      dialogStale = false;
      return;
    }
    const failure = describeConnectionFailure(err);
    dialogError = failure.message;
    dialogStale = failure.staleView;
  }

  async function confirm(reason: string): Promise<void> {
    const action = dialogAction;
    if (!action) return;
    dialogBusy = true;
    dialogError = null;
    dialogStale = false;
    try {
      if (action === 'create') {
        const created = await createConnection({
          id: buffer.id.trim(),
          direction: schema.direction === 'source' ? 'SOURCE' : 'DESTINATION',
          kind: schema.graphqlKind,
          name: buffer.name,
          description: buffer.description === '' ? null : buffer.description,
          spec: specOf(buffer),
          secretBindings: bindingsOf(buffer),
          reason
        });
        dialogAction = null;
        onchanged(created);
      } else if (action === 'save' && row) {
        const saved = await updateConnection({
          id: row.id,
          expectedVersion: buffer.version,
          name: buffer.name,
          description: buffer.description,
          spec: specOf(buffer),
          secretBindings: bindingsOf(buffer),
          reason
        });
        dialogAction = null;
        onchanged(saved);
      } else if (action === 'compile' && row) {
        const result = await compileConnection({ id: row.id, expectedVersion: row.version, reason });
        if (result.revision) {
          dialogAction = null;
          toasts.success(`Compiled ${row.id} r${result.revision.revisionId}`);
          onchanged(result.connection);
          tab = 'revisions';
        } else {
          buffer.problems = result.problems;
          buffer.problemsKey = key;
          const count = result.problems.filter(isBlocking).length;
          dialogError = `Compile found ${count} problem${count === 1 ? '' : 's'}, so nothing was written. They are marked on the form.`;
          tab = 'settings';
        }
      } else if (action === 'archive' && row) {
        const archivedRow = await archiveConnection({ id: row.id, expectedVersion: row.version, reason });
        dialogAction = null;
        onchanged(archivedRow);
      }
    } catch (err) {
      failed(err);
    } finally {
      dialogBusy = false;
    }
  }

  async function reload(): Promise<void> {
    if (!row) return;
    reloading = true;
    try {
      const current = await fetchConnection(row.id);
      dialogAction = null;
      if (current) onchanged(current);
      else onmissing();
    } catch (err) {
      dialogError = describeConnectionFailure(err).message;
    } finally {
      reloading = false;
    }
  }
</script>

<div class="details" data-testid="connection-details" data-mode={creating ? 'create' : 'edit'}>
  <header class="details-head">
    <div class="head-line">
      <h2 class="details-title" title={title}>{title}</h2>
      {#if row}
        <span class="details-id" title={row.id}>{row.id}</span>
      {/if}
      <span class="spacer"></span>
      {#if row}
        <IconButton
          icon={RefreshCw}
          label={dirty ? 'Reload the saved connection (unsaved edits are discarded)' : 'Reload connection'}
          loading={reloading}
          onclick={reload}
        />
        <IconButton
          icon={Archive}
          label="Archive connection"
          disabled={archiveBlocked !== null}
          title={archiveBlocked ?? 'Archive connection'}
          onclick={() => openDialog('archive')}
        />
      {/if}
    </div>
    <div class="head-line">
      <div class="status" data-testid="connection-status">
        {#each tokens as token (token.key)}
          <Badge tone={token.tone} dot={token.key === 'mounted'} title={token.title}>{token.label}</Badge>
        {/each}
        {#if creating}
          <Badge tone="warning">Not created</Badge>
        {:else if dirty}
          <Badge tone="warning">Unsaved</Badge>
        {/if}
      </div>
      <span class="spacer"></span>
      {#if !readOnly}
        <Button variant="ghost" onclick={ondiscard} disabled={!creating && !dirty}>
          {creating ? 'Cancel' : 'Discard'}
        </Button>
        {#if !creating}
          <Button
            onclick={() => openDialog('compile')}
            disabled={compileBlocked !== null}
            title={compileBlocked ?? 'Compile the saved draft into a revision'}
          >
            Compile
          </Button>
        {/if}
        <Button
          variant="primary"
          onclick={() => openDialog(creating ? 'create' : 'save')}
          disabled={saveBlocked !== null}
          title={saveBlocked ?? undefined}
        >
          {creating ? 'Create' : 'Save'}
        </Button>
      {/if}
    </div>
    {#if archived}
      <p class="head-note" role="status">Archived: this connection accepts no change. Its revisions stay readable.</p>
    {:else if stale && row}
      <p class="head-note stale" role="status" data-testid="connection-stale">
        This connection changed since you opened it (draft version {row.version}; this form holds version {buffer.version}).
        <Button variant="ghost" size="sm" onclick={reload} loading={reloading}>Reload</Button>
      </p>
    {/if}
  </header>

  <div class="details-tabs">
    <Tabs label="Connection details" items={tabs} bind:value={tab} />
  </div>

  <div class="details-body" id={panelId} role="tabpanel" aria-label={tabs.find((item) => item.id === tab)?.label}>
    {#if tab === 'settings'}
      <ConnectionForm {schema} bind:buffer {placed} {readOnly} {identityErrors} {checkStatus} />
    {:else if tab === 'secrets'}
      <SecretBindingsEditor bind:bindings={buffer.bindings} problems={placed.byBinding} {readOnly} />
    {:else if tab === 'revisions' && row}
      <ConnectionRevisions
        {revisions}
        loading={revisionsLoading}
        error={revisionsError}
        {selectedRevisionId}
        onselect={(revisionId) => (selectedRevisionId = revisionId)}
        onretry={loadRevisions}
      />
    {:else if tab === 'usage' && row}
      <ConnectionUsage {row} {revisions} />
    {/if}
  </div>
</div>

<ConnectionReasonDialog
  open={dialogAction !== null}
  title={dialog.title}
  description={dialog.description}
  confirmText={dialog.confirm}
  variant={dialog.variant}
  loading={dialogBusy}
  submitError={dialogError}
  staleView={dialogStale && row !== null}
  onconfirm={confirm}
  oncancel={closeDialog}
  onreload={reload}
/>

<style>
  .details {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    height: 100%;
  }

  .details-head {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .head-line {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
    min-height: var(--size-control-sm);
  }

  .details-title {
    min-width: 0;
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-ui);
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .details-id {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
  }

  .spacer {
    flex: 1 1 auto;
  }

  .status {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    min-width: 0;
  }

  .head-note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .head-note.stale {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    color: var(--color-warning-text);
  }

  .details-tabs {
    display: flex;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .details-body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
  }
</style>
