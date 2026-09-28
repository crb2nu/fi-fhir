<!--
  RolePreflightNotice — the honest state for a surface this identity's roles
  cannot reach (`rolePreflight.ts`): names the missing role in mono, the
  identity, and where roles are granted, and says nothing was queried. The
  same shape as the operator and connections pre-flights, so the three read
  alike. `data-testid`, `data-reason="missing-role"` and `data-missing-roles`
  sit on the EmptyState root for the e2e gate.
-->
<script lang="ts">
  import ShieldAlert from '@lucide/svelte/icons/shield-alert';
  import { EmptyState } from '$lib/ui/primitives';
  import { ROLE_GRANT_LOCATIONS } from '$lib/features/operator/operatorAccess';
  import type { RolePreflight } from './rolePreflight';

  interface Props {
    preflight: RolePreflight;
    /** The EmptyState's data-testid, e.g. `profiles-preflight`. */
    testid: string;
    /** The sentence's subject and verb, e.g. "Source profiles need". */
    lead: string;
  }

  let { preflight, testid, lead }: Props = $props();

  const LIST_SEPARATOR = ', ';
  const CLAUSE_SEPARATOR = '; ';
</script>

<div class="preflight-wrap">
  <EmptyState
    icon={ShieldAlert}
    align="start"
    class="preflight"
    data-testid={testid}
    data-reason="missing-role"
    data-missing-roles={preflight.missingRoles.join(',')}
  >
    <span class="line">
      {lead}
      {#each preflight.missingRoles as role, index (role)}{#if index > 0}{LIST_SEPARATOR}{/if}<code>{role}</code>{/each},
      which this identity{#if preflight.principal}&nbsp;(<code>{preflight.principal}</code>){/if} does not hold, so
      nothing was queried.
    </span>
    <span class="line muted">
      Roles are granted in the API's environment:
      {#each ROLE_GRANT_LOCATIONS as location, index (location.variable)}{#if index > 0}{CLAUSE_SEPARATOR}{/if}<code
          >{location.variable}</code
        >&nbsp;— {location.scope}{/each}.
    </span>
  </EmptyState>
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
