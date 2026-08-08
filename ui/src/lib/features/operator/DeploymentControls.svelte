<script lang="ts">
  /**
   * Deployment and channel controls.
   *
   * Every command carries the snapshot version the operator was looking at.
   * When another operator moved first, the server rejects it with a version
   * conflict; that is surfaced inline with an explicit "reload, then re-decide"
   * instruction rather than retried silently.
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import Button from '$lib/ui/Button.svelte';
  import EmptyState from '$lib/ui/EmptyState.svelte';
  import Panel from '$lib/ui/Panel.svelte';
  import Skeleton from '$lib/ui/Skeleton.svelte';
  import {
    deploymentActionBlockedReason,
    deploymentHealthVariant,
    deploymentStateVariant,
    formatTimestamp,
    shortDigest,
    type DeploymentAction
  } from './attemptPresentation';
  import { fetchDeployments, type OperatorDeployment } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';

  const dispatch = createEventDispatcher<{
    command: { action: DeploymentAction; deployment: OperatorDeployment };
  }>();

  let deployments: OperatorDeployment[] = [];
  let loading = true;
  let error: string | null = null;

  const actions: DeploymentAction[] = ['deploy', 'pause', 'resume', 'retire'];

  export async function reload() {
    loading = true;
    error = null;
    try {
      deployments = await fetchDeployments();
    } catch (err) {
      error = describeOperatorFailure(err).message;
      deployments = [];
    } finally {
      loading = false;
    }
  }

  function actionLabel(action: DeploymentAction): string {
    return action.charAt(0).toUpperCase() + action.slice(1);
  }

  onMount(() => {
    void reload();
  });
</script>

<Panel title="Deployments and channels" padding="md">
  <svelte:fragment slot="actions">
    <Button size="sm" variant="secondary" on:click={reload} disabled={loading}>Refresh</Button>
  </svelte:fragment>

  {#if loading}
    <div aria-busy="true" aria-live="polite">
      <Skeleton lines={3} />
      <span class="sr-only">Loading deployments</span>
    </div>
  {:else if error}
    <div class="error-state" role="alert">
      <p class="error-message">{error}</p>
      <Button size="sm" variant="secondary" on:click={reload}>Retry</Button>
    </div>
  {:else if deployments.length === 0}
    <EmptyState
      icon="folder"
      title="No integration deployments"
      description="Publish an integration revision from the workflow workspace to manage it here."
    />
  {:else}
    <ul class="deployments">
      {#each deployments as deployment (deployment.definitionRevision.artifactId + deployment.definitionRevision.revisionId)}
        <li class="deployment">
          <header class="deployment-header">
            <span class="identity mono" title={deployment.definitionRevision.digest}>
              {deployment.definitionRevision.artifactId}@{deployment.definitionRevision.revisionId}
              <span class="detail">{shortDigest(deployment.definitionRevision.digest)}</span>
            </span>
            <Badge variant={deploymentStateVariant(deployment.state)} size="sm">
              {deployment.state}
            </Badge>
            <Badge variant={deploymentHealthVariant(deployment.health)} size="sm">
              {deployment.health}
            </Badge>
            <span class="detail">version {deployment.version}</span>
            {#if !deployment.validationPassed}
              <Badge variant="warning" size="sm">validation not current</Badge>
            {/if}
          </header>

          <p class="detail last-change">
            Last change by {deployment.updatedBy.id || 'unknown'} at
            {formatTimestamp(deployment.updatedAt)}
            {#if deployment.updatedReason}— “{deployment.updatedReason}”{/if}
          </p>

          <div class="row-actions">
            {#each actions as action (action)}
              {@const blocked = deploymentActionBlockedReason(deployment.state, action)}
              <Button
                size="sm"
                variant={action === 'retire' ? 'danger' : 'secondary'}
                disabled={blocked !== null}
                title={blocked ?? undefined}
                on:click={() => dispatch('command', { action, deployment })}
              >
                {actionLabel(action)}
              </Button>
            {/each}
          </div>
        </li>
      {/each}
    </ul>
  {/if}
</Panel>

<style>
  .error-state {
    background: var(--color-danger-bg);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .error-message {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  .deployments {
    list-style: none;
    margin: 0;
    padding: 0;
  }

  .deployment {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-md);
    padding: var(--space-3);
    margin-bottom: var(--space-3);
  }

  .deployment-header {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
    margin-bottom: var(--space-2);
  }

  .identity {
    font-weight: 600;
    color: var(--color-text-primary);
  }

  .last-change {
    margin: 0 0 var(--space-3);
  }

  .row-actions {
    display: flex;
    gap: var(--space-2);
    flex-wrap: wrap;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-xs);
  }

  .detail {
    color: var(--color-text-tertiary);
    font-size: var(--text-xs);
  }

  .sr-only {
    position: absolute;
    width: 1px;
    height: 1px;
    padding: 0;
    margin: -1px;
    overflow: hidden;
    clip: rect(0, 0, 0, 0);
    white-space: nowrap;
    border: 0;
  }
</style>
