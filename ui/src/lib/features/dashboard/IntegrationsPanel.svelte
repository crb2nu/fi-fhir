<!--
  IntegrationsPanel — the home view of integration deployments, from the
  operator control plane's lifecycle query (`operatorDeployments`). Read-only:
  every lifecycle command lives on /operator behind the reason-required dialog.

  The query needs the operator control plane and `integration.operator`; when
  the status endpoint says the plane is not configured, or the identity lacks
  the role (in that order), the panel says which and issues nothing — the same
  pre-flight the operator page uses. Each row links to its lifecycle history on
  /operator.
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import Layers from '@lucide/svelte/icons/layers';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import ServerOff from '@lucide/svelte/icons/server-off';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import {
    Badge,
    Button,
    EmptyState,
    IconButton,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import { accessCapabilities } from '$lib/graphql/accessCapabilities';
  import { fetchDeployments, type OperatorDeployment } from '$lib/features/operator/operatorApi';
  import { describeOperatorFailure } from '$lib/features/operator/operatorErrors';
  import { operatorPreflight } from '$lib/features/operator/operatorAccess';
  import { operatorDeploymentHref } from '$lib/features/operator/operatorLinks';
  import {
    badgeTone,
    deploymentHealthVariant,
    deploymentStateVariant,
    describeValidation,
    formatTimestamp,
    shortDigest
  } from '$lib/features/operator/attemptPresentation';

  const preflight = $derived(operatorPreflight($accessCapabilities));
  const LIST_SEPARATOR = ', ';

  let deployments = $state<OperatorDeployment[]>([]);
  let loading = $state(false);
  let loaded = $state(false);
  let error = $state<string | null>(null);

  async function load(): Promise<void> {
    if (preflight) return;
    loading = true;
    error = null;
    try {
      deployments = await fetchDeployments();
    } catch (err) {
      error = describeOperatorFailure(err).message;
      deployments = [];
    } finally {
      loading = false;
      loaded = true;
    }
  }

  function openOperator(): void {
    void goto(resolve('/operator'));
  }

  onMount(() => {
    void load();
  });
</script>

<Panel title="Integrations" flush data-testid="integrations-panel">
  {#snippet actions()}
    {#if !preflight}
      <IconButton icon={RefreshCw} label="Refresh integrations" {loading} onclick={() => void load()} />
    {/if}
    <Button variant="ghost" onclick={openOperator}>Open Operator</Button>
  {/snippet}

  {#if preflight?.reason === 'not-configured'}
    <EmptyState
      icon={ServerOff}
      align="start"
      data-testid="integrations-preflight"
      data-reason="not-configured"
      data-missing-roles={preflight.missingRoles.length > 0 ? preflight.missingRoles.join(',') : undefined}
    >
      Integration deployments live in the operator control plane, which is not configured on this deployment
      (<code>{preflight.keys[0]}</code> or <code>{preflight.keys[1]}</code>); nothing was queried.
    </EmptyState>
  {:else if preflight}
    <EmptyState
      icon={ShieldAlert}
      align="start"
      data-testid="integrations-preflight"
      data-reason="missing-role"
      data-missing-roles={preflight.missingRoles.join(',')}
    >
      Integration deployments need
      {#each preflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each}; nothing was queried.
    </EmptyState>
  {:else if error}
    <EmptyState
      icon={CircleAlert}
      align="start"
      message={error}
      actionLabel="Retry"
      onaction={() => void load()}
    />
  {:else if loaded && deployments.length === 0}
    <EmptyState icon={Layers} align="start" message="No integration is deployed." />
  {:else if deployments.length > 0}
    <Table label="Integration deployments" layout="fixed">
      {#snippet head()}
        <tr>
          <Th>Integration</Th>
          <Th width="22%">Revision</Th>
          <Th width="56px" numeric>Ver</Th>
          <Th width="96px">State</Th>
          <Th width="96px">Health</Th>
          <Th width="104px">Validation</Th>
          <Th width="164px">Updated</Th>
        </tr>
      {/snippet}
      {#each deployments as deployment (deployment.definitionRevision.artifactId + deployment.definitionRevision.revisionId)}
        <Tr>
          <Td mono truncate title={`Lifecycle history of ${deployment.definitionRevision.artifactId}`}>
            <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- operatorLinks.ts builds this href from resolve() plus a query string -->
            <a href={operatorDeploymentHref( deployment.definitionRevision.artifactId, deployment.definitionRevision.revisionId )} class="history-link">
              {deployment.definitionRevision.artifactId}
            </a>
          </Td>
          <Td
            mono
            truncate
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
            <Badge tone={badgeTone(deploymentHealthVariant(deployment.health))}>{deployment.health}</Badge>
          </Td>
          <Td title={describeValidation(deployment).detail}>
            <Badge tone={describeValidation(deployment).tone}>{describeValidation(deployment).label}</Badge>
          </Td>
          <Td mono muted value={formatTimestamp(deployment.updatedAt)} />
        </Tr>
      {/each}
    </Table>
  {/if}
</Panel>

<style>
  .history-link {
    color: inherit;
    text-decoration: underline;
    text-decoration-color: var(--color-border-strong);
    text-underline-offset: 2px;
  }

  .history-link:hover {
    text-decoration-color: currentColor;
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
  }
</style>
