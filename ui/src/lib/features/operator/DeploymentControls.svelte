<script lang="ts">
  /**
   * Deployment and channel controls: Operator › Deployments.
   *
   * Every command carries the snapshot version the operator was looking at.
   * When another operator moved first, the server rejects it with a version
   * conflict; that is surfaced inline with an explicit "reload, then re-decide"
   * instruction rather than retried silently.
   *
   * Each row opens its lifecycle history (`operatorDeploymentEvents`), and a
   * deep link (`/operator?definition=&revision=`) opens it on mount. The
   * Validation badge is the server's `validationCurrent` with the expiry beside
   * it; Deploy and Resume say why they are disabled when it is not current.
   */

  import { createEventDispatcher, onMount } from 'svelte';
  import Archive from '@lucide/svelte/icons/archive';
  import ChevronDown from '@lucide/svelte/icons/chevron-down';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
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
    describeValidation,
    formatTimestamp,
    shortDigest,
    type DeploymentAction
  } from './attemptPresentation';
  import { fetchDeployments, type OperatorDeployment } from './operatorApi';
  import { describeOperatorFailure } from './operatorErrors';
  import { deploymentControlBlock } from './operatorAccess';
  import DeploymentHistory from './DeploymentHistory.svelte';

  const dispatch = createEventDispatcher<{
    command: { action: DeploymentAction; deployment: OperatorDeployment };
  }>();

  /** A deep-linked revision whose history opens once the inventory loads. */
  export let focus: { definitionId: string; revisionId: string } | null = null;

  let deployments: OperatorDeployment[] = [];
  let loading = true;
  let error: string | null = null;
  let expanded: Record<string, boolean> = {};
  let focusMissing: string | null = null;
  let focusApplied = false;

  function rowKey(deployment: OperatorDeployment): string {
    return `${deployment.definitionRevision.artifactId}@${deployment.definitionRevision.revisionId}`;
  }

  function toggleHistory(deployment: OperatorDeployment) {
    const key = rowKey(deployment);
    expanded = { ...expanded, [key]: !expanded[key] };
  }

  function applyFocus() {
    if (!focus || focusApplied) return;
    focusApplied = true;
    const key = `${focus.definitionId}@${focus.revisionId}`;
    if (deployments.some((deployment) => rowKey(deployment) === key)) {
      expanded = { ...expanded, [key]: true };
      focusMissing = null;
    } else {
      focusMissing = `Revision ${key} is not in this tenant's lifecycle catalog, so it has no history to show.`;
    }
  }

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
      applyFocus();
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
  {:else if deployments.length === 0 && !focusMissing}
    <EmptyState
      icon={Layers}
      align="start"
      message="No integration deployments. Publish an integration revision from Workflows to manage it here."
    />
  {:else}
    {#if focusMissing}
      <p class="focus-missing" role="status" data-testid="deployment-focus-missing">{focusMissing}</p>
    {/if}
    {#if deployments.length > 0}
    <Table label="Integration deployments">
      {#snippet head()}
        <tr>
          <Th>Integration</Th>
          <Th>Revision</Th>
          <Th width="56px" numeric>Ver</Th>
          <Th width="104px">State</Th>
          <Th width="96px">Health</Th>
          <Th width="200px">Validation</Th>
          <Th>Release</Th>
          <Th>Last change</Th>
          <Th width="360px">Actions</Th>
        </tr>
      {/snippet}
      {#each deployments as deployment, index (rowKey(deployment))}
        {@const open = expanded[rowKey(deployment)] ?? false}
        {@const validation = describeValidation(deployment)}
        <Tr data-testid="deployment-row" data-definition-id={deployment.definitionRevision.artifactId}>
          <Td>
            <span class="integration">
              <IconButton
                icon={open ? ChevronDown : ChevronRight}
                label={open ? 'Hide lifecycle history' : 'Show lifecycle history'}
                aria-expanded={open ? 'true' : 'false'}
                aria-controls={`deployment-history-${index}`}
                onclick={() => toggleHistory(deployment)}
              />
              <span class="mono">{deployment.definitionRevision.artifactId}</span>
            </span>
          </Td>
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
          <Td title={validation.detail}>
            <span class="validation">
              <Badge tone={validation.tone} data-testid="validation-badge" data-validation={validation.label}>
                {validation.label}
              </Badge>
              {#if deployment.validationExpiresAt}
                <span class="expiry">{validation.label === 'current' ? 'until' : 'at'} {formatTimestamp(deployment.validationExpiresAt)}</span>
              {/if}
            </span>
          </Td>
          <Td mono truncate muted value={deployment.releaseId ?? 'No release'} />
          <Td truncate muted value={lastChange(deployment)} />
          <Td>
            <span class="row-actions">
              {#each commandActions as action (action)}
                {@const blocked =
                  $deploymentControlBlock ??
                  deploymentActionBlockedReason(deployment.state, action, deployment.validationCurrent)}
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
        {#if open}
          <tr class="detail-row" id={`deployment-history-${index}`}>
            <td colspan="9">
              <DeploymentHistory
                definitionId={deployment.definitionRevision.artifactId}
                revisionId={deployment.definitionRevision.revisionId}
              />
            </td>
          </tr>
        {/if}
      {/each}
    </Table>
    {/if}
  {/if}
</Panel>

<style>
  .integration,
  .validation {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    min-width: 0;
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .expiry {
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .focus-missing {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-warning-text);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .detail-row td {
    padding: var(--space-2) var(--space-3) var(--space-3) 44px;
    border-bottom: 1px solid var(--color-border-subtle);
    background: var(--color-bg-base);
  }

  .row-actions {
    display: inline-flex;
    gap: var(--space-1);
  }
</style>
