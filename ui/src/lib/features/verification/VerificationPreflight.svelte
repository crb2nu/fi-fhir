<!--
  VerificationPreflight — why Verification queried nothing, in the Connections
  precedence (.loom/42 "Shared contracts"): the operator control plane is not
  configured on this deployment, or this identity cannot read it. The decision
  is E-0's operatorPreflight; this renders it for /events.
-->
<script lang="ts">
  import ServerOff from '@lucide/svelte/icons/server-off';
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import { EmptyState } from '$lib/ui/primitives';
  import {
    OPERATOR_ROLE_BUNDLE,
    ROLE_GRANT_LOCATIONS,
    TRANSPORT_OPERATOR_ROLE,
    type OperatorPreflight
  } from '$lib/features/operator/operatorAccess';

  interface Props {
    preflight: OperatorPreflight;
  }

  let { preflight }: Props = $props();

  // Separators between inline <code> items (a literal space at a block edge
  // would be trimmed by the compiler).
  const LIST_SEPARATOR = ', ';
  const CLAUSE_SEPARATOR = '; ';
</script>

<div class="preflight-wrap">
  {#if preflight.reason === 'not-configured'}
    <EmptyState
      icon={ServerOff}
      align="start"
      class="preflight"
      data-testid="verification-preflight"
      data-reason="not-configured"
      data-missing-roles={preflight.missingRoles.length > 0 ? preflight.missingRoles.join(',') : undefined}
    >
      <span class="line">
        Verification reads the durable admissions of the operator control plane, which is not configured on this
        deployment, so nothing was queried. It needs the PostgreSQL submission store (<code>{preflight.keys[0]}</code>)
        or <code>{preflight.keys[1]}</code>.
      </span>
      {#if preflight.missingRoles.length > 0}
        <span class="line muted">
          This identity{#if preflight.principal}&nbsp;(<code>{preflight.principal}</code>){/if} would also need
          {#each preflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each}.
        </span>
      {/if}
    </EmptyState>
  {:else}
    <EmptyState
      icon={ShieldAlert}
      align="start"
      class="preflight"
      data-testid="verification-preflight"
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
        so the durable admissions were not queried.
      </span>
      <span class="line muted">
        Roles are granted in the API's environment:
        {#each ROLE_GRANT_LOCATIONS as location, index (location.variable)}{#if index > 0}{CLAUSE_SEPARATOR}{/if}<code
            >{location.variable}</code
          >&nbsp;— {location.scope}{/each}.
      </span>
      <span class="line muted">
        The full operator bundle is <code>{OPERATOR_ROLE_BUNDLE.join(',')}</code>. Reload once the API has picked up
        the change.
      </span>
    </EmptyState>
  {/if}
</div>

<style>
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
</style>
