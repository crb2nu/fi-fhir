<script lang="ts">
  /**
   * Operator control plane.
   *
   * Composes the durable message browser, its receipt-to-delivery trace, the
   * dead-letter/circuit console, and the deployment controls over the Slice
   * 4.2a GraphQL API. Every mutating action routes through one reason-required
   * dialog, and every failure has an inline home.
   */

  import PageHeader from '$lib/ui/PageHeader.svelte';
  import Tabs from '$lib/ui/Tabs.svelte';
  import type { TabItem } from '$lib/ui/types';
  import ControlReasonDialog from './ControlReasonDialog.svelte';
  import DeliveryConsole from './DeliveryConsole.svelte';
  import DeploymentControls from './DeploymentControls.svelte';
  import MessageBrowser from './MessageBrowser.svelte';
  import MessageTrace from './MessageTrace.svelte';
  import type { DeliveryAction, DeploymentAction } from './attemptPresentation';
  import {
    deployRelease,
    discardDeadLetter,
    fetchAttempt,
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

  const tabs: TabItem[] = [
    { key: 'messages', label: 'Messages' },
    { key: 'delivery', label: 'Delivery' },
    { key: 'deployments', label: 'Deployments' }
  ];

  let activeTab = 'messages';

  let selectedReceiptId: string | null = null;
  let trace: OperatorMessageTrace | null = null;
  let traceLoading = false;
  let traceError: string | null = null;

  let deliveryConsole: DeliveryConsole;
  let deploymentControls: DeploymentControls;

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
    selectedReceiptId = receiptId;
    traceLoading = true;
    traceError = null;
    try {
      trace = await fetchMessageTrace(receiptId);
    } catch (err) {
      // The global GraphQL net already toasted this; the panel is the durable
      // home for the message (toast-budget B4).
      traceError = describeOperatorFailure(err).message;
      trace = null;
    } finally {
      traceLoading = false;
    }
  }

  /**
   * Resolves an attempt to the receipt that produced it, then opens that
   * message's trace. The attempt lookup is tenant-scoped server-side, so an
   * unknown ID yields an honest "not found" rather than an empty view.
   */
  async function loadTraceForAttempt(attemptId: string) {
    traceLoading = true;
    traceError = null;
    trace = null;
    selectedReceiptId = null;
    try {
      const attempt = await fetchAttempt(attemptId);
      if (!attempt) {
        traceError = `Delivery attempt ${attemptId} is not available in your tenant.`;
        return;
      }
      await loadTrace(attempt.receiptId);
    } catch (err) {
      traceError = describeOperatorFailure(err).message;
    } finally {
      traceLoading = false;
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

<div class="operator">
  <PageHeader
    title="Operator control plane"
    subtitle="Browse durable messages and delivery, recover failures, and control deployments — every action is reason-required and audited."
  />

  <Tabs {tabs} active={activeTab} onChange={(key) => (activeTab = key)} />

  {#if activeTab === 'messages'}
    <div class="split">
      <MessageBrowser
        {selectedReceiptId}
        on:select={(event) => loadTrace(event.detail.receiptId)}
      />
      <MessageTrace
        {trace}
        loading={traceLoading}
        error={traceError}
        receiptId={selectedReceiptId}
        on:retry={() => selectedReceiptId && loadTrace(selectedReceiptId)}
        on:control={(event) => openDeliveryDialog(event.detail.action, event.detail.attemptId)}
      />
    </div>
  {:else if activeTab === 'delivery'}
    <DeliveryConsole
      bind:this={deliveryConsole}
      on:control={(event) => openDeliveryDialog(event.detail.action, event.detail.attemptId)}
      on:inspect={(event) => {
        activeTab = 'messages';
        void loadTraceForAttempt(event.detail.attemptId);
      }}
    />
  {:else}
    <DeploymentControls
      bind:this={deploymentControls}
      on:command={(event) => openDeploymentDialog(event.detail.action, event.detail.deployment)}
    />
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
    gap: var(--space-4);
    padding: var(--space-4);
  }

  .split {
    display: grid;
    grid-template-columns: minmax(0, 1fr);
    gap: var(--space-4);
  }

  @media (min-width: 1200px) {
    .split {
      grid-template-columns: minmax(0, 1fr) minmax(0, 1fr);
      align-items: start;
    }
  }
</style>
