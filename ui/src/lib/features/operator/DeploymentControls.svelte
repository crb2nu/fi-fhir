<script lang="ts">
  /**
   * Deployment and channel controls: Operator › Deployments.
   *
   * Every command carries the snapshot version the operator was looking at.
   * When another operator moved first, the server rejects it with a version
   * conflict; that is surfaced inline with an explicit "reload, then re-decide"
   * instruction rather than retried silently.
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import Archive from '@lucide/svelte/icons/archive';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Layers from '@lucide/svelte/icons/layers';
  import Pause from '@lucide/svelte/icons/pause';
  import Play from '@lucide/svelte/icons/play';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import Rocket from '@lucide/svelte/icons/rocket';
  import {
    Badge,
    Button,
    EmptyState,
    IconButton,
    Panel,
    Table,
    Td,
    Th,
    Tr,
    type IconComponent
  } from '$lib/ui/primitives';
  import {
    badgeTone,
    deploymentActionBlockedReason,
    deploymentHealthVariant,
    deploymentStateVariant,
    formatTimestamp,
    shortDigest,
    type DeploymentAction
  } from './attemptPresentation';
  import { fetchDeployments, type OperatorDeployment } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { deploymentControlBlock } from './operatorAccess';

  const dispatch = createEventDispatcher<{
    command: { action: DeploymentAction; deployment: OperatorDeployment };
  }>();

  let deployments: OperatorDeployment[] = [];
  let loading = true;
  let error: string | null = null;

  const commandActions: DeploymentAction[] = ['deploy', 'pause', 'resume', 'retire'];
  const actionIcons: Record<DeploymentAction, IconComponent> = {
    deploy: Rocket,
    pause: Pause,
    resume: Play,
    retire: Archive
  };

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

  function lastChange(deployment: OperatorDeployment): string {
    const who = deployment.updatedBy.id || 'unknown';
    const why = deployment.updatedReason ? ` — “${deployment.updatedReason}”` : '';
    return `${who}, ${formatTimestamp(deployment.updatedAt)}${why}`;
  }

  onMount(() => {
    void reload();
  });
</script>

<Panel title="Deployments and channels" flush>
  {#snippet actions()}
    <IconButton icon={RefreshCw} label="Refresh deployments" {loading} onclick={reload} />
  {/snippet}

  {#if loading}
    <EmptyState align="start" message="Loading deployments" aria-busy="true" aria-live="polite" />
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      align="start"
      role="alert"
      message={error}
      actionLabel="Retry"
      onaction={reload}
    />
  {:else if deployments.length === 0}
    <EmptyState
      icon={Layers}
      align="start"
      message="No integration deployments. Publish an integration revision from Workflows to manage it here."
    />
  {:else}
    <Table label="Integration deployments">
      {#snippet head()}
        <tr>
          <Th>Integration</Th>
          <Th>Revision</Th>
          <Th width="56px" numeric>Ver</Th>
          <Th width="104px">State</Th>
          <Th width="96px">Health</Th>
          <Th width="120px">Validation</Th>
          <Th>Last change</Th>
          <Th width="360px">Actions</Th>
        </tr>
      {/snippet}
      {#each deployments as deployment (deployment.definitionRevision.artifactId + deployment.definitionRevision.revisionId)}
        <Tr>
          <Td mono value={deployment.definitionRevision.artifactId} />
          <Td
            mono
            muted
            title={deployment.definitionRevision.digest}
            value={`${deployment.definitionRevision.revisionId} · ${shortDigest(deployment.definitionRevision.digest)}`}
          />
          <Td numeric value={deployment.version} />
          <Td>
            <Badge tone={badgeTone(deploymentStateVariant(deployment.state))} dot>
              {deployment.state}
            </Badge>
          </Td>
          <Td>
            <Badge tone={badgeTone(deploymentHealthVariant(deployment.health))}>
              {deployment.health}
            </Badge>
          </Td>
          <Td>
            {#if deployment.validationPassed}
              <Badge tone="success">current</Badge>
            {:else}
              <Badge tone="warning">not current</Badge>
            {/if}
          </Td>
          <Td truncate muted value={lastChange(deployment)} />
          <Td>
            <span class="row-actions">
              {#each commandActions as action (action)}
                {@const blocked =
                  $deploymentControlBlock ?? deploymentActionBlockedReason(deployment.state, action)}
                <Button
                  variant={action === 'retire' ? 'danger' : 'secondary'}
                  icon={actionIcons[action]}
                  disabled={blocked !== null}
                  title={blocked ?? undefined}
                  onclick={() => dispatch('command', { action, deployment })}
                >
                  {actionLabel(action)}
                </Button>
              {/each}
            </span>
          </Td>
        </Tr>
      {/each}
    </Table>
  {/if}
</Panel>

<style>
  .row-actions {
    display: inline-flex;
    gap: var(--space-1);
  }
</style>
