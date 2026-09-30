<script lang="ts">
  /**
   * Operator control plane.
   *
   * Composes the durable message browser, its receipt-to-delivery trace, the
   * dead-letter/circuit console, and the deployment controls over the Slice
   * 4.2a GraphQL API. Every mutating action routes through one reason-required
   * dialog, and every failure has an inline home.
   *
   * When the status endpoint reports that the control plane is not configured
   * on this deployment, or that this identity cannot read it (in that order,
   * the Connections precedence), the page renders a pre-flight saying so and
   * mounts none of the views — so no query is issued that would be refused.
   *
   * Deep links (.loom/42): `?receipt=` opens a trace, `?attempt=` an attempt
   * inspector on Delivery, `?definition=&revision=` a deployment's history.
   */

  import { onMount } from 'svelte';
  import MousePointerClick from '@lucide/svelte/icons/mouse-pointer-click';
  import ServerOff from '@lucide/svelte/icons/server-off';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { EmptyState, Tabs, Toolbar, type TabItem } from '$lib/ui/primitives';
  import AttemptInspector from './AttemptInspector.svelte';
  import AttemptSearch from './AttemptSearch.svelte';
  import ControlReasonDialog from './ControlReasonDialog.svelte';
  import DeliveryConsole from './DeliveryConsole.svelte';
  import DeploymentControls from './DeploymentControls.svelte';
  import MessageBrowser from './MessageBrowser.svelte';
  import MessageTrace from './MessageTrace.svelte';
  import type { DeliveryAction, DeploymentAction } from './attemptPresentation';
  import {
    deployRelease,
    discardDeadLetter,
    fetchMessageTrace,
    pauseDeployment,
    replayDelivery,
    resubmitMessage,
    resumeDeployment,
    retireDeployment,
    type OperatorDeployment,
    type OperatorMessageTrace
  } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { parseOperatorDeepLink } from './operatorLinks';
  import {
    OPERATOR_ROLE_BUNDLE,
    ROLE_GRANT_LOCATIONS,
    TRANSPORT_OPERATOR_ROLE,
    operatorPreflight
  } from './operatorAccess';

  $: preflight = operatorPreflight($accessCapabilities);

  // Separators rendered between inline <code> items (a literal space at a
  // block edge would be trimmed by the compiler).
  const LIST_SEPARATOR = ', ';
  const CLAUSE_SEPARATOR = '; ';

  const views: TabItem[] = [
    { id: 'messages', label: 'Messages', controls: 'operator-view' },
    { id: 'delivery', label: 'Delivery', controls: 'operator-view' },
    { id: 'deployments', label: 'Deployments', controls: 'operator-view' }
  ];

  let activeTab = 'messages';

  let selectedReceiptId: string | null = null;
  let trace: OperatorMessageTrace | null = null;
  let traceLoading = false;
  let traceError: string | null = null;
  // Only the newest trace request may write the pane (rows can be opened in
  // quick succession; a slow earlier answer must not replace a later one).
  let traceRequest = 0;

  let deliveryConsole: DeliveryConsole;
  let deploymentControls: DeploymentControls;
  let attemptSearch: AttemptSearch | undefined;
  let attemptInspector: AttemptInspector | undefined;

  /** The attempt open in Delivery's inspector pane. */
  let inspectedAttemptId: string | null = null;
  /** A deep-linked deployment whose history opens on Deployments. */
  let deploymentFocus: { definitionId: string; revisionId: string } | null = null;

  function inspectAttempt(attemptId: string) {
    activeTab = 'delivery';
    inspectedAttemptId = attemptId;
  }

  function openTrace(receiptId: string) {
    activeTab = 'messages';
    void loadTrace(receiptId);
  }

  onMount(() => {
    if (preflight) return;
    const link = parseOperatorDeepLink(window.location.search);
    if (!link) return;
    if (link.tab === 'messages') {
      openTrace(link.receiptId);
    } else if (link.tab === 'delivery') {
      inspectAttempt(link.attemptId);
    } else {
      activeTab = 'deployments';
      deploymentFocus = { definitionId: link.definitionId, revisionId: link.revisionId };
    }
  });

  type PendingDelivery = { kind: 'delivery'; action: DeliveryAction; attemptId: string };
  type PendingDeployment = {
    kind: 'deployment';
    action: DeploymentAction;
    deployment: OperatorDeployment;
  };
  type Pending = PendingDelivery | PendingDeployment;

  let pending: Pending | null = null;
  let dialogOpen = false;
  let dialogBusy = false;
  let dialogError: string | null = null;

  async function loadTrace(receiptId: string) {
    const request = ++traceRequest;
    selectedReceiptId = receiptId;
    traceLoading = true;
    traceError = null;
    try {
      const result = await fetchMessageTrace(receiptId);
      if (request !== traceRequest) return;
      trace = result;
    } catch (err) {
      if (request !== traceRequest) return;
      // Operator reads opt out of the global toast; the panel is the only home
      // for the message (toast-budget B4).
      traceError = describeOperatorFailure(err).message;
      trace = null;
    } finally {
      if (request === traceRequest) traceLoading = false;
    }
  }

  function openDeliveryDialog(action: DeliveryAction, attemptId: string) {
    pending = { kind: 'delivery', action, attemptId };
    dialogError = null;
    dialogOpen = true;
  }

  function openDeploymentDialog(action: DeploymentAction, deployment: OperatorDeployment) {
    pending = { kind: 'deployment', action, deployment };
    dialogError = null;
    dialogOpen = true;
  }

  function closeDialog() {
    dialogOpen = false;
    pending = null;
    dialogError = null;
  }

  async function runDelivery(
    action: DeliveryAction,
    attemptId: string,
    reason: string,
    idempotencyKey: string
  ) {
    const input = { attemptId, reason, idempotencyKey };
    if (action === 'replay') return replayDelivery(input);
    if (action === 'resubmit') return resubmitMessage(input);
    return discardDeadLetter(input);
  }

  async function runDeployment(
    action: DeploymentAction,
    deployment: OperatorDeployment,
    reason: string
  ) {
    const input = {
      definitionId: deployment.definitionRevision.artifactId,
      revisionId: deployment.definitionRevision.revisionId,
      expectedVersion: deployment.version,
      reason
    };
    if (action === 'pause') return pauseDeployment(input);
    if (action === 'resume') return resumeDeployment(input);
    if (action === 'retire') return retireDeployment(input);
    return deployRelease(input);
  }

  async function handleConfirm(
    event: CustomEvent<{ reason: string; idempotencyKey: string }>
  ) {
    const intent = pending;
    if (!intent) return;
    dialogBusy = true;
    dialogError = null;
    try {
      if (intent.kind === 'delivery') {
        await runDelivery(
          intent.action,
          intent.attemptId,
          event.detail.reason,
          event.detail.idempotencyKey
        );
        await deliveryConsole?.reload();
        void attemptSearch?.reload();
        attemptInspector?.reload();
        if (selectedReceiptId) {
          await loadTrace(selectedReceiptId);
        }
      } else {
        await runDeployment(intent.action, intent.deployment, event.detail.reason);
        await deploymentControls?.reload();
      }
      dialogOpen = false;
      pending = null;
    } catch (err) {
      const failure = describeOperatorFailure(err);
      dialogError = failure.message;
      if (failure.staleView) {
        // The operator's view is behind the durable record. Refresh the source
        // of truth so their next decision uses the current version.
        if (intent.kind === 'delivery') {
          await deliveryConsole?.reload();
          if (selectedReceiptId) {
            await loadTrace(selectedReceiptId);
          }
        } else {
          await deploymentControls?.reload();
        }
      }
    } finally {
      dialogBusy = false;
    }
  }

  $: dialogTitle =
    pending === null
      ? 'Confirm operator action'
      : pending.kind === 'delivery'
        ? `${capitalize(pending.action)} delivery attempt`
        : `${capitalize(pending.action)} integration`;

  $: dialogDescription =
    pending === null
      ? ''
      : pending.kind === 'delivery'
        ? deliveryDescription(pending.action, pending.attemptId)
        : `Applies to ${pending.deployment.definitionRevision.artifactId}@${pending.deployment.definitionRevision.revisionId} at version ${pending.deployment.version}. A newer version is rejected rather than overwritten.`;

  function deliveryDescription(action: DeliveryAction, attemptId: string): string {
    switch (action) {
      case 'replay':
        return `Requeues attempt ${attemptId} exactly once. Repeating this request with the same key is a no-op.`;
      case 'resubmit':
        return `Creates one new child attempt from ${attemptId}. The original stays failed and closes as resubmitted.`;
      case 'discard':
        return `Abandons ${attemptId} without redelivering it. This closes the dead letter permanently and is recorded against your identity.`;
    }
  }

  function capitalize(value: string): string {
    return value.charAt(0).toUpperCase() + value.slice(1);
  }
</script>

<svelte:head>
  <title>Operator | fi-fhir</title>
</svelte:head>

<div class="operator">
  {#if preflight?.reason === 'not-configured'}
    <Toolbar title="Operator" />
    <div class="preflight-wrap">
      <EmptyState
        icon={ServerOff}
        align="start"
        class="preflight"
        data-testid="operator-preflight"
        data-reason="not-configured"
        data-missing-roles={preflight.missingRoles.length > 0 ? preflight.missingRoles.join(',') : undefined}
      >
        <span class="line">
          The operator control plane is not configured on this deployment, so nothing was queried. It needs the
          PostgreSQL submission store (<code>{preflight.keys[0]}</code>) or <code>{preflight.keys[1]}</code>.
        </span>
        {#if preflight.missingRoles.length > 0}
          <span class="line muted">
            This identity{#if preflight.principal}&nbsp;(<code>{preflight.principal}</code>){/if} would also need
            {#each preflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each}.
          </span>
        {/if}
      </EmptyState>
    </div>
  {:else if preflight}
    <Toolbar title="Operator" />
    <div class="preflight-wrap">
      <EmptyState
        icon={ShieldAlert}
        align="start"
        class="preflight"
        data-testid="operator-preflight"
        data-reason="missing-role"
        data-missing-roles={preflight.missingRoles.join(',')}
      >
        <span class="line">
          This identity{#if preflight.principal}&nbsp;(<code>{preflight.principal}</code>){/if}
          {#if preflight.holdsTransportGrant}
            holds <code>{TRANSPORT_OPERATOR_ROLE}</code> but not
          {:else}
            does not hold
          {/if}
          {#each preflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
          so the operator plane was not queried.
        </span>
        <span class="line muted">
          Roles are granted in the API's environment:
          {#each ROLE_GRANT_LOCATIONS as location, index (location.variable)}{#if index > 0}{CLAUSE_SEPARATOR}{/if}<code
              >{location.variable}</code
            >&nbsp;— {location.scope}{/each}.
        </span>
        <span class="line muted">
          The full operator bundle is <code>{OPERATOR_ROLE_BUNDLE.join(',')}</code>. Reload once the
          API has picked up the change.
        </span>
      </EmptyState>
    </div>
  {:else}
    <Toolbar title="Operator">
      {#snippet tabs()}
        <Tabs label="Operator views" items={views} bind:value={activeTab} />
      {/snippet}
    </Toolbar>

    <div
      class="view"
      id="operator-view"
      role="tabpanel"
      aria-label={views.find((view) => view.id === activeTab)?.label}
    >
      {#if activeTab === 'messages'}
        <div class="split">
          <div class="split-main">
            <MessageBrowser
              {selectedReceiptId}
              on:select={(event) => loadTrace(event.detail.receiptId)}
            />
          </div>
          <div class="split-pane">
            <MessageTrace
              {trace}
              loading={traceLoading}
              error={traceError}
              receiptId={selectedReceiptId}
              on:retry={() => selectedReceiptId && loadTrace(selectedReceiptId)}
              on:control={(event) =>
                openDeliveryDialog(event.detail.action, event.detail.attemptId)}
              on:inspect={(event) => inspectAttempt(event.detail.attemptId)}
            />
          </div>
        </div>
      {:else if activeTab === 'delivery'}
        <div class="split">
          <div class="split-main stack">
            <AttemptSearch
              bind:this={attemptSearch}
              selectedAttemptId={inspectedAttemptId}
              oninspect={inspectAttempt}
              ontrace={openTrace}
            />
            <DeliveryConsole
              bind:this={deliveryConsole}
              on:control={(event) => openDeliveryDialog(event.detail.action, event.detail.attemptId)}
              on:inspect={(event) => inspectAttempt(event.detail.attemptId)}
            />
          </div>
          <div class="split-pane">
            {#if inspectedAttemptId}
              <AttemptInspector
                bind:this={attemptInspector}
                attemptId={inspectedAttemptId}
                ontrace={openTrace}
                oninspect={inspectAttempt}
                oncontrol={(action, attemptId) => openDeliveryDialog(action, attemptId)}
                onclose={() => (inspectedAttemptId = null)}
              />
            {:else}
              <EmptyState
                icon={MousePointerClick}
                align="start"
                message="No attempt selected. Pick one from Delivery attempts or a dead letter to see its ledger and audit trail."
              />
            {/if}
          </div>
        </div>
      {:else}
        <div class="stack">
          <DeploymentControls
            bind:this={deploymentControls}
            focus={deploymentFocus}
            on:command={(event) =>
              openDeploymentDialog(event.detail.action, event.detail.deployment)}
          />
        </div>
      {/if}
    </div>
  {/if}

  <ControlReasonDialog
    open={dialogOpen}
    title={dialogTitle}
    description={dialogDescription}
    confirmText={pending ? capitalize(pending.action) : 'Confirm'}
    variant={pending?.action === 'discard' || pending?.action === 'retire' ? 'danger' : 'primary'}
    requiresIdempotencyKey={pending?.kind === 'delivery'}
    action={pending?.action ?? ''}
    targetId={pending?.kind === 'delivery'
      ? pending.attemptId
      : (pending?.deployment.definitionRevision.revisionId ?? '')}
    loading={dialogBusy}
    submitError={dialogError}
    on:confirm={handleConfirm}
    on:cancel={closeDialog}
  />
</div>

<style>
  .operator {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .preflight-wrap {
    padding: var(--space-3);
  }

  .preflight-wrap :global(.preflight) {
    padding: var(--space-3);
    border: 1px solid var(--color-warning-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg-elevated);
  }

  .preflight-wrap :global(.preflight .ui-empty-icon) {
    color: var(--color-warning-text);
  }

  .line {
    display: block;
  }

  .line + .line {
    margin-top: var(--space-1);
  }

  .muted {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }

  .view {
    display: flex;
    flex-direction: column;
    flex: 1 1 auto;
    min-height: 0;
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr) minmax(380px, 42%);
    flex: 1 1 auto;
    min-height: 0;
  }

  .split-main,
  .split-pane {
    display: flex;
    flex-direction: column;
    min-width: 0;
    min-height: 0;
    overflow: auto;
  }

  .split-pane {
    border-left: 1px solid var(--color-border-subtle);
    background: var(--color-bg-elevated);
  }

  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
    min-height: 0;
    overflow: auto;
  }

  @media (max-width: 1100px) {
    .split {
      grid-template-columns: minmax(0, 1fr);
    }

    .split-pane {
      border-left: 0;
      border-top: 1px solid var(--color-border-subtle);
    }
  }
</style>
