<!--
  "From connection…" on /hl7 (.loom/38 C-3): pick a source, then either arm a
  stream capture or browse a batch source's objects and read one. Every call is
  a reason-required, audited intake row on the server; the samples land in the
  page's Integration Session, redacted by the capture redactor, and from there
  in the tab-memory inbox.

  Modelled on the operator's ControlReasonDialog: a 40 px header, Fields, the
  failure of the last attempt inside the dialog (never a toast), md buttons,
  focus kept inside while open. Honest states come from intakeState.ts; this
  component only renders them and runs the calls.
-->
<script lang="ts">
  import { tick, untrack } from 'svelte';
  import X from '@lucide/svelte/icons/x';
  import ArrowLeft from '@lucide/svelte/icons/arrow-left';
  import Cable from '@lucide/svelte/icons/cable';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import ServerOff from '@lucide/svelte/icons/server-off';
  import FolderSearch from '@lucide/svelte/icons/folder-search';
  import {
    Badge,
    Button,
    EmptyState,
    Field,
    IconButton,
    Input,
    KeyValue,
    Table,
    Td,
    Th,
    Textarea,
    Tr
  } from '$lib/ui/primitives';
  import { createDialogFocusController, type DialogFocusController } from '$lib/domain/a11yDialog';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { integrationSessionEngineEnabled } from '$lib/features/integration-session';
  import {
    fetchConnections,
    fetchEngineRuntime,
    type ConnectionRow,
    type EngineRuntimeView
  } from '$lib/features/connections/connectionsApi';
  import { describeConnectionFailure } from '$lib/features/connections/connectionsErrors';
  import { shortHash, formatMinute } from '$lib/features/connections/presentation';
  import type { BatchPeekObjectRow, ConnectionCaptureRow, IntakeProblem } from './intakeApi';
  import { EMPTY_INTAKE, type IntakeController, type IntakeControllerState } from './intakeController';
  import { describeIntakeFailure, problemGuidance } from './intakeErrors';
  import {
    CAPTURE_BOUNDS,
    KERNEL_LIMITATION_NOTE,
    MAX_INTAKE_REASON_BYTES,
    PEEK_BOUNDS,
    boundProblem,
    intakeDialogView,
    intakeReasonProblem,
    type IntakeSource,
    type Loadable
  } from './intakeState';

  interface Props {
    open: boolean;
    controller: IntakeController;
    /** Opens straight on cancelling this capture. */
    cancelTarget?: ConnectionCaptureRow | null | undefined;
    onclose: () => void;
  }

  let { open, controller, cancelTarget = null, onclose }: Props = $props();

  const uid = $props.id();
  const titleId = `${uid}-title`;
  // The controller's state, mirrored into runes state (the controller is a store-backed module).
  let intake = $state<IntakeControllerState>(EMPTY_INTAKE);
  $effect(() => controller.state.subscribe((value) => (intake = value)));

  type Step =
    | { kind: 'sources' }
    | { kind: 'capture'; source: IntakeSource }
    | { kind: 'browse'; source: IntakeSource }
    | { kind: 'cancel'; capture: ConnectionCaptureRow };

  let step = $state<Step>({ kind: 'sources' });
  let runtime = $state<Loadable<EngineRuntimeView>>({ status: 'idle' });
  let catalog = $state<Loadable<ConnectionRow[]>>({ status: 'idle' });

  // Form state, reset whenever a step is entered.
  let reason = $state('');
  // Number inputs bind numbers (null when empty); boundProblem reads either.
  let maxMessages = $state<number | string | null>(null);
  let ttlSeconds = $state<number | string | null>(null);
  let maxObjects = $state<number | string | null>(null);
  let attempted = $state(false);
  let busy = $state(false);
  let submitError = $state<string | null>(null);
  let objects = $state<BatchPeekObjectRow[] | null>(null);
  let selectedObject = $state<string | null>(null);
  let problems = $state<IntakeProblem[]>([]);
  let readCount = $state<number | null>(null);

  let dialogEl: HTMLDivElement | undefined = $state();
  let focus: DialogFocusController | null = null;

  const view = $derived(
    intakeDialogView({
      access: $accessCapabilities,
      sessionEngine: $integrationSessionEngineEnabled,
      runtime,
      catalog,
      captures: intake.captures
    })
  );

  const reasonIssue = $derived(intakeReasonProblem(reason));
  const messagesIssue = $derived(
    step.kind === 'capture'
      ? boundProblem(maxMessages, CAPTURE_BOUNDS.maxMessages.min, CAPTURE_BOUNDS.maxMessages.max)
      : boundProblem(maxMessages, PEEK_BOUNDS.maxMessages.min, PEEK_BOUNDS.maxMessages.max)
  );
  const ttlIssue = $derived(boundProblem(ttlSeconds, CAPTURE_BOUNDS.ttlSeconds.min, CAPTURE_BOUNDS.ttlSeconds.max));
  const objectsIssue = $derived(boundProblem(maxObjects, PEEK_BOUNDS.maxObjects.min, PEEK_BOUNDS.maxObjects.max));

  const captureBlocked = $derived(reasonIssue ?? messagesIssue ?? ttlIssue);
  const listBlocked = $derived(reasonIssue ?? objectsIssue);
  const readBlocked = $derived(
    selectedObject === null ? 'Choose an object from the list first.' : (reasonIssue ?? messagesIssue)
  );

  function enter(next: Step): void {
    step = next;
    attempted = false;
    busy = false;
    submitError = null;
    objects = null;
    selectedObject = null;
    problems = [];
    readCount = null;
    maxMessages = next.kind === 'capture' ? CAPTURE_BOUNDS.maxMessages.fallback : PEEK_BOUNDS.maxMessages.fallback;
    ttlSeconds = CAPTURE_BOUNDS.ttlSeconds.fallback;
    maxObjects = PEEK_BOUNDS.maxObjects.fallback;
    reason = '';
  }

  async function loadSources(): Promise<void> {
    if (view.kind === 'hidden' || view.kind === 'not-configured' || view.kind === 'missing-role') return;
    runtime = { status: 'loading' };
    catalog = { status: 'loading' };
    // The session's captures say which source is already armed; a page with
    // no session yet has none.
    const refreshing = controller.refresh().catch(() => undefined);
    const [runtimeResult, catalogResult] = await Promise.allSettled([
      fetchEngineRuntime(),
      fetchConnections('SOURCE', false)
    ]);
    runtime =
      runtimeResult.status === 'fulfilled'
        ? { status: 'ok', value: runtimeResult.value }
        : { status: 'error', message: `Engine runtime: ${describeConnectionFailure(runtimeResult.reason).message}` };
    catalog =
      catalogResult.status === 'fulfilled'
        ? { status: 'ok', value: catalogResult.value }
        : { status: 'error', message: `Connection catalog: ${describeConnectionFailure(catalogResult.reason).message}` };
    await refreshing;
  }

  $effect(() => {
    if (!open) return;
    untrack(() => {
      if (cancelTarget) {
        enter({ kind: 'cancel', capture: cancelTarget });
      } else {
        enter({ kind: 'sources' });
        void loadSources();
      }
    });
    void tick().then(() => {
      if (!dialogEl) return;
      focus = createDialogFocusController(dialogEl);
      focus.focusInitial();
    });
    return () => {
      focus?.restoreFocus();
      focus = null;
    };
  });

  function close(): void {
    if (busy) return;
    onclose();
  }

  function onKeydown(event: KeyboardEvent): void {
    if (event.key === 'Escape') {
      event.preventDefault();
      close();
      return;
    }
    focus?.onKeydown(event);
  }

  function choose(source: IntakeSource): void {
    enter(source.mode === 'stream' ? { kind: 'capture', source } : { kind: 'browse', source });
  }

  async function armCapture(): Promise<void> {
    attempted = true;
    if (step.kind !== 'capture' || captureBlocked || busy || !step.source.sourceId) return;
    busy = true;
    submitError = null;
    try {
      await controller.startCapture({
        sourceId: step.source.sourceId,
        maxMessages: Number(maxMessages),
        ttlSeconds: Number(ttlSeconds),
        reason: reason.trim()
      });
      busy = false;
      onclose();
    } catch (error) {
      submitError = describeIntakeFailure(error).message;
      busy = false;
      if (describeIntakeFailure(error).kind === 'already-armed') void controller.refresh().catch(() => undefined);
    }
  }

  async function listObjects(): Promise<void> {
    attempted = true;
    if (step.kind !== 'browse' || listBlocked || busy || !step.source.connectionId) return;
    busy = true;
    submitError = null;
    try {
      const result = await controller.peek({
        connectionId: step.source.connectionId,
        maxObjects: Number(maxObjects),
        reason: reason.trim()
      });
      objects = result.objects;
      problems = result.problems;
      selectedObject = null;
      readCount = null;
      attempted = false;
    } catch (error) {
      submitError = describeIntakeFailure(error).message;
    } finally {
      busy = false;
    }
  }

  async function readObject(): Promise<void> {
    attempted = true;
    if (step.kind !== 'browse' || readBlocked || busy || !step.source.connectionId || selectedObject === null) return;
    busy = true;
    submitError = null;
    try {
      const result = await controller.peek({
        connectionId: step.source.connectionId,
        objectPath: selectedObject,
        maxMessages: Number(maxMessages),
        reason: reason.trim()
      });
      problems = result.problems;
      readCount = result.samples.length;
      busy = false;
      if (result.problems.length === 0) onclose();
    } catch (error) {
      submitError = describeIntakeFailure(error).message;
      busy = false;
    }
  }

  async function cancelCapture(): Promise<void> {
    attempted = true;
    if (step.kind !== 'cancel' || reasonIssue || busy) return;
    busy = true;
    submitError = null;
    try {
      await controller.cancelCapture(step.capture.id, reason.trim());
      busy = false;
      onclose();
    } catch (error) {
      submitError = describeIntakeFailure(error).message;
      busy = false;
    }
  }

  function formatSize(bytes: number): string {
    if (bytes < 1024) return `${bytes} B`;
    if (bytes < 1024 * 1024) return `${(bytes / 1024).toFixed(1)} KB`;
    return `${(bytes / (1024 * 1024)).toFixed(1)} MB`;
  }

  function sourceItems(source: IntakeSource) {
    return [
      { key: 'Source ID', value: source.sourceId, mono: true },
      { key: 'Connection', value: source.connectionId, mono: true },
      { key: 'Kind', value: source.kind, mono: true },
      { key: 'State', value: source.state },
      { key: 'Digest', value: source.digest ? shortHash(source.digest, 16) : null, mono: true }
    ];
  }

  const title = $derived(
    step.kind === 'capture'
      ? 'Capture from a stream source'
      : step.kind === 'browse'
        ? 'Browse a batch source'
        : step.kind === 'cancel'
          ? 'Cancel capture'
          : 'Sample from a connection'
  );
</script>

{#snippet problemList(list: readonly IntakeProblem[])}
  <ul class="problems" data-testid="connection-intake-problems" role="alert">
    {#each list as problem, index (`${problem.code}:${problem.path}:${index}`)}
      <li data-code={problem.code} data-path={problem.path}>
        <span class="problem-head">
          <code class="problem-path">{problem.path || '—'}</code>
          <span class="problem-code">{problem.code}</span>
        </span>
        <span>{problem.message}</span>
        {#if problemGuidance(problem.code)}
          <span class="problem-hint">{problemGuidance(problem.code)}</span>
        {/if}
      </li>
    {/each}
  </ul>
{/snippet}

{#snippet reasonField()}
  <Field
    label="Reason"
    required
    hint="Recorded with your verified identity on the intake audit row."
    error={attempted ? reasonIssue : undefined}
  >
    <Textarea
      rows={2}
      maxlength={MAX_INTAKE_REASON_BYTES}
      bind:value={reason}
      placeholder="Why do you need these messages?"
    />
  </Field>
{/snippet}

{#if open}
  <div
    class="backdrop"
    role="presentation"
    onclick={(event) => {
      if (event.target === event.currentTarget) close();
    }}
    onkeydown={onKeydown}
  >
    <div
      class="dialog"
      bind:this={dialogEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby={titleId}
      data-testid="connection-intake-dialog"
      data-step={step.kind}
    >
      <header class="dialog-head">
        {#if step.kind === 'capture' || step.kind === 'browse'}
          <IconButton icon={ArrowLeft} label="Back to sources" onclick={() => { enter({ kind: 'sources' }); void loadSources(); }} disabled={busy} />
        {/if}
        <h2 id={titleId} class="title">{title}</h2>
        <IconButton icon={X} label="Close dialog" onclick={close} disabled={busy} tabindex={-1} />
      </header>

      <div class="dialog-body">
        {#if step.kind === 'sources'}
          <p class="description">
            Messages land in this page's Integration Session
            {#if intake.sessionId}(<code>{intake.sessionId}</code>){:else}(created on the first intake){/if},
            masked by the capture redactor: names, identifiers and dates read <code>REDACTED</code>.
          </p>

          {#if view.kind === 'not-configured'}
            <EmptyState icon={ServerOff} align="start" data-testid="connection-intake-preflight" data-reason="not-configured">
              {view.reason}
            </EmptyState>
          {:else if view.kind === 'missing-role'}
            <EmptyState
              icon={ShieldAlert}
              align="start"
              data-testid="connection-intake-preflight"
              data-reason="missing-role"
              data-missing-roles={view.roles.join(',')}
            >
              This identity{#if view.principal}&nbsp;(<code>{view.principal}</code>){/if} does not hold
              {#each view.roles as role, index (role)}{#if index > 0},&nbsp;{/if}<code>{role}</code>{/each}, which sampling
              from a connection requires, so no source was queried.
            </EmptyState>
          {:else if view.kind === 'loading'}
            <p class="muted">Reading the mounted and catalog sources…</p>
          {:else if view.kind === 'error'}
            <div class="load-error" role="alert">
              {#each view.messages as message (message)}<p>{message}</p>{/each}
              <Button onclick={() => void loadSources()}>Retry</Button>
            </div>
          {:else if view.kind === 'empty'}
            <EmptyState icon={Cable} align="start" data-testid="connection-intake-empty">
              <span class="line">No source connection is mounted on this deployment.</span>
              <span class="line muted">
                This replica runs no MLLP listener, HTTP ingress, or batch runner, and the catalog holds no source
                connection. Define one in Connections.
              </span>
            </EmptyState>
          {:else if view.kind === 'sources'}
            {#if view.errors.length > 0}
              <div class="load-error" role="alert">
                {#each view.errors as message (message)}<p>{message}</p>{/each}
                <Button onclick={() => void loadSources()}>Retry</Button>
              </div>
            {/if}
            <Table label="Source connections" layout="fixed" data-testid="connection-intake-sources">
              {#snippet head()}
                <tr>
                  <Th>Source</Th>
                  <Th width="92px">Kind</Th>
                  <Th width="210px">State</Th>
                  <Th width="136px"><span class="sr-only">Action</span></Th>
                </tr>
              {/snippet}
              {#each view.sources as source (source.key)}
                <Tr data-row={source.key} data-mode={source.mode} data-available={String(source.available)}>
                  <Td>
                    <span class="source-name" title={source.label}>{source.label}</span>
                    <span class="source-id mono" title={source.sourceId ?? source.connectionId ?? ''}>
                      {source.sourceId ?? source.connectionId ?? '—'}
                    </span>
                  </Td>
                  <Td mono value={source.kind} />
                  <Td truncate title={source.stateTitle}>
                    {source.state}
                    {#if source.armed}
                      <Badge tone="info" class="armed">Armed</Badge>
                    {/if}
                  </Td>
                  <Td class="action-cell">
                    <Button
                      size="sm"
                      disabled={!source.available || source.armed !== null}
                      title={source.armed
                        ? 'A capture is already armed on this source; its row in Samples offers Cancel.'
                        : (source.unavailableReason ?? undefined)}
                      onclick={() => choose(source)}
                    >
                      {source.mode === 'stream' ? 'Capture…' : 'Browse objects…'}
                    </Button>
                  </Td>
                </Tr>
              {/each}
            </Table>
          {/if}
        {:else if step.kind === 'capture'}
          <KeyValue columns={2} items={sourceItems(step.source)} />
          <p class="note">{KERNEL_LIMITATION_NOTE} Frames that a replica receives from now on are captured until the count or the expiry is reached.</p>
          <div class="form-grid">
            <Field
              label="Messages"
              required
              hint={`${CAPTURE_BOUNDS.maxMessages.min}–${CAPTURE_BOUNDS.maxMessages.max}`}
              error={attempted ? messagesIssue : undefined}
            >
              <Input type="number" mono min={CAPTURE_BOUNDS.maxMessages.min} max={CAPTURE_BOUNDS.maxMessages.max} bind:value={maxMessages} />
            </Field>
            <Field
              label="Expires after (seconds)"
              required
              hint={`${CAPTURE_BOUNDS.ttlSeconds.min}–${CAPTURE_BOUNDS.ttlSeconds.max}`}
              error={attempted ? ttlIssue : undefined}
            >
              <Input type="number" mono min={CAPTURE_BOUNDS.ttlSeconds.min} max={CAPTURE_BOUNDS.ttlSeconds.max} bind:value={ttlSeconds} />
            </Field>
          </div>
          {@render reasonField()}
        {:else if step.kind === 'browse'}
          <KeyValue columns={2} items={sourceItems(step.source)} />
          {#if objects === null}
            <p class="note">
              Listing is itself an audited peek. It takes no lease, writes no checkpoint, and moves nothing; the runner
              still ingests every object normally.
            </p>
            <div class="form-grid">
              <Field
                label="Objects to list"
                required
                hint={`${PEEK_BOUNDS.maxObjects.min}–${PEEK_BOUNDS.maxObjects.max}, in the provider's listing order`}
                error={attempted ? objectsIssue : undefined}
              >
                <Input type="number" mono min={PEEK_BOUNDS.maxObjects.min} max={PEEK_BOUNDS.maxObjects.max} bind:value={maxObjects} />
              </Field>
            </div>
          {:else if objects.length === 0}
            <EmptyState icon={FolderSearch} align="start" message="No object is listed under the source's input prefix." />
          {:else}
            <Table label="Objects" layout="fixed" data-testid="connection-intake-objects">
              {#snippet head()}
                <tr>
                  <Th>Path</Th>
                  <Th width="88px" numeric>Size</Th>
                  <Th width="136px">Modified (UTC)</Th>
                </tr>
              {/snippet}
              {#each objects as object (object.path)}
                <Tr selectable selected={selectedObject === object.path} onselect={() => (selectedObject = object.path)} data-path={object.path}>
                  <Td mono truncate value={object.path} />
                  <Td numeric mono value={formatSize(object.size)} />
                  <Td mono muted value={formatMinute(object.modifiedAt)} title="Advisory: the provider's modification time" />
                </Tr>
              {/each}
            </Table>
            <div class="form-grid">
              <Field
                label="Messages to read"
                required
                hint={`${PEEK_BOUNDS.maxMessages.min}–${PEEK_BOUNDS.maxMessages.max} from the chosen object`}
                error={attempted ? messagesIssue : undefined}
              >
                <Input type="number" mono min={PEEK_BOUNDS.maxMessages.min} max={PEEK_BOUNDS.maxMessages.max} bind:value={maxMessages} />
              </Field>
            </div>
          {/if}
          {#if problems.length > 0}
            {@render problemList(problems)}
          {/if}
          {#if readCount !== null}
            <p class="note" role="status">
              {readCount} sample{readCount === 1 ? '' : 's'} added to Samples before the peek stopped.
            </p>
          {/if}
          {@render reasonField()}
        {:else if step.kind === 'cancel'}
          <p class="description">
            Stop capturing <code>{step.capture.sourceId}</code> ({step.capture.captured} / {step.capture.maxMessages} captured).
            Samples already captured stay in the session.
          </p>
          {@render reasonField()}
        {/if}

        {#if submitError}
          <p class="submit-error" role="alert">{submitError}</p>
        {/if}
      </div>

      <footer class="actions">
        {#if step.kind === 'sources'}
          <Button variant="ghost" size="md" onclick={close}>Close</Button>
        {:else if step.kind === 'capture'}
          <Button variant="ghost" size="md" onclick={close} disabled={busy}>Cancel</Button>
          <Button
            variant="primary"
            size="md"
            onclick={() => void armCapture()}
            disabled={captureBlocked !== null || busy}
            loading={busy}
            title={captureBlocked ?? undefined}
          >
            Arm capture
          </Button>
        {:else if step.kind === 'browse'}
          <Button variant="ghost" size="md" onclick={close} disabled={busy}>{readCount !== null ? 'Close' : 'Cancel'}</Button>
          {#if objects === null}
            <Button
              variant="primary"
              size="md"
              onclick={() => void listObjects()}
              disabled={listBlocked !== null || busy}
              loading={busy}
              title={listBlocked ?? undefined}
            >
              List objects
            </Button>
          {:else}
            <Button size="md" onclick={() => (objects = null)} disabled={busy}>List again</Button>
            <Button
              variant="primary"
              size="md"
              onclick={() => void readObject()}
              disabled={readBlocked !== null || busy}
              loading={busy}
              title={readBlocked ?? undefined}
            >
              Read messages
            </Button>
          {/if}
        {:else if step.kind === 'cancel'}
          <Button variant="ghost" size="md" onclick={close} disabled={busy}>Keep capturing</Button>
          <Button
            variant="danger"
            size="md"
            onclick={() => void cancelCapture()}
            disabled={reasonIssue !== null || busy}
            loading={busy}
            title={reasonIssue ?? undefined}
          >
            Cancel capture
          </Button>
        {/if}
      </footer>
    </div>
  </div>
{/if}

<style>
  .backdrop {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    background: var(--modal-backdrop);
    z-index: var(--z-modal);
  }

  .dialog {
    display: flex;
    flex-direction: column;
    width: min(46rem, 100%);
    max-height: 90vh;
    overflow: hidden;
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    box-shadow: var(--shadow-xl);
  }

  .dialog-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: 40px;
    padding: 0 var(--space-2) 0 var(--space-4);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .dialog-head :global(.ui-icon-button:first-child) {
    margin-left: calc(var(--space-2) * -1);
  }

  .title {
    flex: 1 1 auto;
    margin: 0;
    font-family: var(--font-ui);
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .dialog-body {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    min-height: 0;
    padding: var(--space-4);
    overflow-y: auto;
  }

  .description {
    margin: 0;
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
    color: var(--color-text-secondary);
  }

  .note,
  .muted {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .line {
    display: block;
  }

  .line.muted {
    margin-top: var(--space-1);
  }

  code,
  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .source-name,
  .source-id {
    display: block;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .source-id {
    color: var(--color-text-tertiary);
  }

  .dialog-body :global(.armed) {
    margin-left: var(--space-1);
  }

  .dialog-body :global(td.action-cell) {
    text-align: right;
  }

  .load-error,
  .submit-error {
    display: grid;
    gap: var(--space-2);
    justify-items: start;
    margin: 0;
    padding: var(--space-2) var(--space-3);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  .load-error p {
    margin: 0;
  }

  .problems {
    display: grid;
    gap: var(--space-2);
    margin: 0;
    padding: var(--space-2) var(--space-3);
    list-style: none;
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .problems li {
    display: grid;
    gap: 2px;
  }

  .problem-head {
    display: flex;
    gap: var(--space-2);
    align-items: baseline;
  }

  .problem-path {
    color: var(--color-text-primary);
  }

  .problem-code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-warning-text);
  }

  .problem-hint {
    color: var(--color-text-tertiary);
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: var(--space-2);
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
