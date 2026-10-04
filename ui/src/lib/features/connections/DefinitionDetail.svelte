<!--
  One definition revision: what it binds (with digests), its validation,
  approval and release evidence, its lifecycle history, and the authoring
  steps its state allows. Every write goes through the reason dialog and
  carries the snapshot version it started from; a conflict says "reload, then
  re-decide". Deploy is an operator act and lives on the Operator page.
-->
<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { resolve } from '$app/paths';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import ExternalLink from '@lucide/svelte/icons/external-link';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, Button, EmptyState, IconButton, KeyValue, Tabs, type TabItem } from '$lib/ui/primitives';
  import ConnectionReasonDialog from './ConnectionReasonDialog.svelte';
  import DefinitionHistory from './DefinitionHistory.svelte';
  import {
    approveDefinition,
    fetchDefinition,
    publishDefinition,
    validateDefinition,
    type DefinitionDetail,
    type IntegrationValidationMode
  } from './definitionsApi';
  import { describeDefinitionFailure } from './definitionsErrors';
  import {
    canValidate,
    definitionLink,
    definitionQuery,
    MIN_SKIP_REASON,
    MODE_TEXT,
    nextStep,
    validationLabel
  } from './definitionDraft';
  import { formatSecond, stateTone, validationTone, VALIDATION_TEXT } from './definitionPresentation';
  import { shortHash } from './presentation';

  interface Props {
    definitionId: string;
    revisionId: string;
    writeBlocked: string[] | null;
    onchanged: (detail: DefinitionDetail) => void;
  }

  let { definitionId, revisionId, writeBlocked, onchanged }: Props = $props();

  type DetailTab = 'overview' | 'validation' | 'release' | 'history';
  type Action = 'validate' | 'approve' | 'publish';
  const tabs: TabItem[] = [
    { id: 'overview', label: 'Overview', testid: 'definition-tab-overview' },
    { id: 'validation', label: 'Validation', testid: 'definition-tab-validation' },
    { id: 'release', label: 'Release', testid: 'definition-tab-release' },
    { id: 'history', label: 'History', testid: 'definition-tab-history' }
  ];

  let loadSeq = 0;
  let disposed = false;
  onDestroy(() => { disposed = true; loadSeq += 1; });

  let detail = $state<DefinitionDetail | null>(null);
  let loading = $state(true);
  let missing = $state(false);
  let error = $state<string | null>(null);
  let tab = $state<DetailTab>('overview');
  let mode = $state<IntegrationValidationMode>('STATIC');
  let action = $state<Action | null>(null);
  let busy = $state(false);
  let submitError = $state<string | null>(null);
  let staleView = $state(false);
  let now = $state(new Date());

  const writeReason = $derived(
    writeBlocked ? `Read only: this identity does not hold ${writeBlocked.join(', ')}.` : undefined
  );
  const definition = $derived(detail?.definition ?? null);
  const step = $derived(definition ? nextStep(definition, now) : { step: null, blockedReason: null });
  const validation = $derived(definition ? validationLabel(definition, now) : 'none');

  async function load(): Promise<void> {
    const seq = ++loadSeq;
    loading = true;
    error = null;
    try {
      const loaded = await fetchDefinition(definitionId, revisionId);
      if (seq !== loadSeq) return;
      detail = loaded;
      missing = loaded === null;
      now = new Date();
      if (loaded && !loaded.realValidationAvailable && mode === 'REAL') mode = 'STATIC';
    } catch (err) {
      if (seq === loadSeq) error = describeDefinitionFailure(err).message;
    } finally {
      if (seq === loadSeq) loading = false;
    }
  }

  onMount(() => {
    void load();
    const timer = setInterval(() => (now = new Date()), 15000);
    return () => clearInterval(timer);
  });

  function open(next: Action): void {
    submitError = null;
    staleView = false;
    action = next;
  }

  async function confirm(reason: string): Promise<void> {
    if (!definition || !action) return;
    if (action === 'validate' && mode === 'SKIP' && reason.length < MIN_SKIP_REASON) {
      submitError = `Skipping validation needs a reason of at least ${MIN_SKIP_REASON} characters.`;
      return;
    }
    busy = true;
    submitError = null;
    const command = {
      definitionId: definition.definitionId,
      revisionId: definition.revisionId,
      expectedVersion: definition.version,
      reason
    };
    try {
      let next: DefinitionDetail;
      if (action === 'validate') next = await validateDefinition({ ...command, mode });
      else if (action === 'approve') next = await approveDefinition(command);
      else next = await publishDefinition(command);
      detail = next;
      now = new Date();
      if (!disposed) onchanged(next);
      if (action === 'validate') tab = 'validation';
      action = null;
    } catch (err) {
      const failure = describeDefinitionFailure(err);
      submitError = failure.message;
      staleView = failure.staleView;
    } finally {
      busy = false;
    }
  }

  async function reload(): Promise<void> {
    action = null;
    await load();
    if (detail && !disposed) onchanged(detail);
  }

  const DIALOG = {
    validate: { title: 'Validate definition', confirm: 'Record validation' },
    approve: { title: 'Approve definition', confirm: 'Approve' },
    publish: { title: 'Publish definition', confirm: 'Publish release' }
  } as const;
  const dialogDescription = $derived.by(() => {
    if (!definition || !action) return undefined;
    const target = `${definition.definitionId}/${definition.revisionId} at version ${definition.version}`;
    if (action === 'validate') return `${mode}: ${MODE_TEXT[mode]} (${target})`;
    if (action === 'approve') return `Approves ${target}. Approval needs current validation evidence.`;
    return `Creates the immutable release of ${target}. It is then deployable from the Operator page until the evidence expires.`;
  });

  function modeDisabledReason(candidate: IntegrationValidationMode): string | undefined {
    if (candidate === 'REAL' && !detail?.realValidationAvailable) {
      return 'This replica does not mount this source, so it holds no credentials to contact it.';
    }
    return undefined;
  }
</script>

<div class="details" data-testid="definition-details" data-definition-id={definitionId} data-revision-id={revisionId} data-state={definition?.state ?? ''}>
  {#if loading && !detail}
    <EmptyState message="Loading definition" aria-busy="true" />
  {:else if error && !detail}
    <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={load} />
  {:else if missing || !definition || !detail}
    <EmptyState
      icon={CircleAlert}
      data-testid="definition-missing"
      message={`Definition ${definitionId}/${revisionId} is not in this tenant's lifecycle catalog.`}
    />
  {:else}
    <header class="details-head">
      <div class="head-line">
        <h2 class="details-title">{definition.definitionId}</h2>
        <span class="details-id">rev {definition.revisionId} · v{definition.version}</span>
        <span class="spacer"></span>
        <IconButton icon={RefreshCw} label="Reload definition" {loading} onclick={reload} />
      </div>
      <div class="head-line">
        <span class="status">
          <Badge tone={stateTone(definition.state)} data-testid="definition-state">{definition.state}</Badge>
          <Badge tone={validationTone(validation)}>{VALIDATION_TEXT[validation]}</Badge>
          <Badge tone="neutral">health {definition.health}</Badge>
        </span>
        <span class="spacer"></span>
        {#if canValidate(definition.state)}
          <Button
            variant={step.step === 'validate' ? 'primary' : 'secondary'}
            disabled={writeReason !== undefined}
            title={writeReason ?? 'Record connection-validation evidence'}
            data-testid="definition-validate"
            onclick={() => open('validate')}
          >
            Validate
          </Button>
        {/if}
        {#if step.step === 'approve'}
          <Button
            variant="primary"
            disabled={writeReason !== undefined || step.blockedReason !== null}
            title={writeReason ?? step.blockedReason ?? 'Approve this revision'}
            data-testid="definition-approve"
            onclick={() => open('approve')}
          >
            Approve
          </Button>
        {:else if step.step === 'publish'}
          <Button
            variant="primary"
            disabled={writeReason !== undefined || step.blockedReason !== null}
            title={writeReason ?? step.blockedReason ?? 'Publish the release'}
            data-testid="definition-publish"
            onclick={() => open('publish')}
          >
            Publish
          </Button>
        {:else if step.step === 'deploy'}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- resolve() builds the path; the query is appended -->
          <a class="link-button" href={`${resolve('/operator')}${definitionQuery(definition)}`} data-testid="definition-deploy-link">
            Deploy on Operator <ExternalLink size={14} aria-hidden="true" />
          </a>
        {/if}
      </div>
      {#if step.blockedReason}
        <p class="head-note warn" role="status">{step.blockedReason}</p>
      {/if}
      {#if canValidate(definition.state) && !writeReason}
        <fieldset class="modes" data-testid="definition-validation-modes">
          <legend class="modes-legend">Validation mode</legend>
          {#each ['STATIC', 'REAL', 'SKIP'] as const as candidate (candidate)}
            {@const disabledReason = modeDisabledReason(candidate)}
            <label class="mode" class:disabled={disabledReason !== undefined} title={disabledReason}>
              <input
                type="radio"
                name={`mode-${definition.definitionId}-${definition.revisionId}`}
                value={candidate}
                checked={mode === candidate}
                disabled={disabledReason !== undefined}
                onchange={() => (mode = candidate)}
                data-testid={`definition-mode-${candidate.toLowerCase()}`}
              />
              <span class="mode-name">{candidate}</span>
              <span class="mode-text">{disabledReason ?? MODE_TEXT[candidate]}</span>
            </label>
          {/each}
        </fieldset>
      {/if}
    </header>

    <div class="details-tabs">
      <Tabs label="Definition views" items={tabs} value={tab} onchange={(id) => (tab = id as DetailTab)} />
    </div>

    <div class="details-body">
      {#if tab === 'overview'}
        <section class="section">
          <h3 class="section-title">Binds</h3>
          <KeyValue
            items={[
              { key: 'Source', value: `${definition.source.artifactId} r${definition.source.revisionId}`, mono: true },
              { key: 'Source ID', value: definition.source.sourceId, mono: true },
              { key: 'Source digest', value: definition.source.digest, mono: true, truncate: true },
              { key: 'Profile', value: `${definition.profile.artifactId} r${definition.profile.revisionId} · ${shortHash(definition.profile.digest)}`, mono: true },
              { key: 'Workflow', value: `${definition.workflow.artifactId} r${definition.workflow.revisionId} · ${shortHash(definition.workflow.digest)}`, mono: true }
            ]}
          />
          <p class="note">
            Profile and workflow refs are the ones the runtime resolves from the static integration registry at admission.
          </p>
          <h3 class="section-title">Destinations</h3>
          <ul class="plain" data-testid="definition-destinations">
            {#each definition.destinations as destination (destination.artifactId)}
              <li>
                <code>{destination.artifactId}</code> r{destination.revisionId}
                <Badge tone={destination.class === 'production' ? 'warning' : 'neutral'}>{destination.class}</Badge>
                <code class="muted">{shortHash(destination.digest)}</code>
              </li>
            {/each}
          </ul>
          <h3 class="section-title">Secret bindings</h3>
          {#if definition.secretBindings.length === 0}
            <p class="note">No secret bindings: nothing these revisions name needs one.</p>
          {:else}
            <ul class="plain" data-testid="definition-bindings">
              {#each definition.secretBindings as binding (binding.name)}
                <li>
                  <code>{binding.name}</code> → <code>{binding.provider}:{binding.key}</code>{#if binding.version}&nbsp;@{binding.version}{/if}
                </li>
              {/each}
            </ul>
            <p class="note">References only: where each secret lives, never its value.</p>
          {/if}
          <h3 class="section-title">Policy</h3>
          <KeyValue
            columns={2}
            items={[
              { key: 'Classification', value: definition.policy.classification },
              { key: 'Raw retention', value: definition.policy.rawRetention.mode },
              { key: 'Validation timeout', value: `${definition.deployment.validationTimeoutSeconds} s`, mono: true },
              { key: 'Validation max age', value: `${definition.deployment.validationMaxAgeSeconds} s`, mono: true },
              { key: 'Schedule', value: definition.deployment.scheduleMode },
              { key: 'In flight · queued', value: `${definition.deployment.maxInFlight} · ${definition.deployment.maxQueued}`, mono: true },
              { key: 'Messages / s', value: definition.deployment.maxMessagesPerSecond, mono: true },
              { key: 'Health interval', value: `${definition.deployment.healthCheckIntervalSeconds} s`, mono: true }
            ]}
          />
          <h3 class="section-title">Revision</h3>
          <KeyValue
            items={[
              { key: 'Digest', value: definition.digest, mono: true, truncate: true },
              { key: 'Created', value: `${formatSecond(definition.createdAt)} by ${definition.createdBy.id}` },
              { key: 'Reason', value: definition.createdReason },
              { key: 'Link', value: definitionLink(definition), mono: true, truncate: true }
            ]}
          />
        </section>
      {:else if tab === 'validation'}
        <section class="section" data-testid="definition-validation">
          {#if detail.validation}
            {@const record = detail.validation}
            <KeyValue
              items={[
                { key: 'Result', value: record.passed ? 'Passed' : 'Failed' },
                { key: 'Codes', value: record.codes.join(', '), mono: true },
                { key: 'Checked', value: formatSecond(record.checkedAt), mono: true },
                { key: 'Expires', value: formatSecond(record.expiresAt), mono: true },
                { key: 'Source revision', value: `${record.sourceRevision.artifactId} r${record.sourceRevision.revisionId} · ${shortHash(record.sourceRevision.digest)}`, mono: true },
                { key: 'Recorded by', value: `${record.actor.id} · ${record.reason}` }
              ]}
            />
            <p class="note">
              The codes say which check ran: VALIDATION_STATIC contacted nothing, VALIDATION_SKIPPED checked nothing,
              and INPUT_LISTED means the batch source was listed for real.
            </p>
          {:else}
            <EmptyState message="No validation has been recorded for this revision." />
          {/if}
        </section>
      {:else if tab === 'release'}
        <section class="section" data-testid="definition-release">
          {#if detail.approval}
            <h3 class="section-title">Approval</h3>
            <KeyValue
              items={[
                { key: 'Approved', value: `${formatSecond(detail.approval.occurredAt)} by ${detail.approval.actor.id}` },
                { key: 'Reason', value: detail.approval.reason },
                { key: 'Event', value: detail.approval.eventId, mono: true, truncate: true }
              ]}
            />
          {/if}
          {#if detail.release}
            <h3 class="section-title">Release</h3>
            <KeyValue
              items={[
                { key: 'Release', value: detail.release.releaseId, mono: true, truncate: true },
                { key: 'Release digest', value: detail.release.digest, mono: true, truncate: true },
                { key: 'Published', value: `${formatSecond(detail.release.publishedAt)} by ${detail.release.publishedBy.id}` },
                { key: 'Reason', value: detail.release.publishedReason },
                { key: 'Validation', value: detail.release.validationId, mono: true, truncate: true }
              ]}
            />
          {:else if !detail.approval}
            <EmptyState message="Not approved or published yet." />
          {:else}
            <p class="note">Approved; publish to create the release.</p>
          {/if}
        </section>
      {:else}
        <DefinitionHistory definitionId={definition.definitionId} revisionId={definition.revisionId} version={definition.version} />
      {/if}
    </div>

    <ConnectionReasonDialog
      open={action !== null}
      title={action ? DIALOG[action].title : ''}
      description={dialogDescription}
      confirmText={action ? DIALOG[action].confirm : ''}
      loading={busy}
      {submitError}
      {staleView}
      onconfirm={(reason) => void confirm(reason)}
      oncancel={() => (action = null)}
      onreload={() => void reload()}
    />
  {/if}
</div>

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
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .details-id {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .spacer {
    flex: 1 1 auto;
  }

  .status {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
  }

  .head-note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .head-note.warn {
    color: var(--color-warning-text);
  }

  .link-button {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    height: var(--size-control-sm);
    padding: 0 var(--space-3);
    border-radius: var(--radius-sm);
    background: var(--color-primary);
    color: var(--color-text-inverse);
    font-size: var(--text-ui);
    text-decoration: none;
  }

  .link-button:hover {
    background: var(--color-primary-hover);
  }

  .link-button:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 2px;
  }

  .modes {
    display: flex;
    flex-direction: column;
    gap: 2px;
    margin: var(--space-1) 0 0;
    padding: 0;
    border: 0;
  }

  .modes-legend {
    padding: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .mode {
    display: grid;
    grid-template-columns: auto 56px minmax(0, 1fr);
    align-items: baseline;
    gap: var(--space-2);
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    cursor: pointer;
  }

  .mode.disabled {
    cursor: not-allowed;
    color: var(--color-text-tertiary);
  }

  .mode-name {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
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

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-3);
  }

  .section-title {
    margin: var(--space-2) 0 0;
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-tertiary);
  }

  .section-title:first-child {
    margin-top: 0;
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .plain {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-ui);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }

  code.muted {
    color: var(--color-text-tertiary);
  }
</style>
