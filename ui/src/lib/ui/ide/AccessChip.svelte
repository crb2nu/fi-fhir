<!--
  AccessChip — the credential state as a status-bar chip. The popover holds
  the full statement ("Trusted network access active", the Cloudflare Access
  identity, or the memory-only bearer note) and, for bearer sessions only,
  the "Clear access" action. Headerless sessions end where they began (the
  network or Cloudflare Access), so they get no clear button.
-->
<script lang="ts">
  import KeyRound from '@lucide/svelte/icons/key-round';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import { Button, Icon, Popover } from '$lib/ui/primitives';
  import {
    describeAccess,
    type AccessSession
  } from '$lib/graphql/GraphQLCredentialGate.svelte';

  interface Props {
    access: AccessSession;
    onclear?: (() => void) | undefined;
  }

  let { access, onclear }: Props = $props();

  let open = $state(false);
  const words = $derived(describeAccess(access));

  function clear(): void {
    open = false;
    onclear?.();
  }
</script>

<Popover bind:open label="Access" placement="top-start" data-testid="access-popover">
  {#snippet trigger(props)}
    <button
      {...props}
      type="button"
      class="access-chip"
      data-testid="access-chip"
      data-via={access.via}
      title="Access details"
    >
      <Icon icon={access.via === 'bearer' ? KeyRound : ShieldCheck} size={12} />
      <span class="sr-only">Access:</span>
      <span class="access-chip-text">{words.chip}</span>
    </button>
  {/snippet}
  <div class="access-detail">
    <p class="access-title">{words.title}</p>
    <p class="access-body">{words.detail}</p>
    {#if access.via === 'bearer' && onclear}
      <Button variant="secondary" onclick={clear}>Clear access</Button>
    {/if}
  </div>
</Popover>

<style>
  .access-chip {
    display: inline-flex;
    align-items: center;
    gap: 5px;
    height: 20px;
    max-width: 280px;
    padding: 0 6px;
    border: none;
    border-radius: var(--radius-sm);
    background: transparent;
    color: inherit;
    font: inherit;
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .access-chip:hover,
  .access-chip[aria-expanded='true'] {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .access-chip:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .access-chip-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .access-detail {
    display: grid;
    gap: var(--space-2);
    justify-items: start;
  }

  .access-title {
    margin: 0;
    color: var(--color-text-primary);
    font-weight: var(--font-semibold);
  }

  .access-body {
    margin: 0;
    color: var(--color-text-secondary);
  }
</style>
