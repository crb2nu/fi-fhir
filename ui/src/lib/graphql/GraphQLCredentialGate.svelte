<script module lang="ts">
  /** How this browser reached the API: headerless (network, Cloudflare Access) or a bearer held in memory. */
  export type AccessVia = 'network' | 'cloudflare-access' | 'bearer';

  export interface AccessSession {
    via: AccessVia;
    /** The identity the server named (Cloudflare Access email); empty otherwise. */
    principal: string;
  }

  export interface AccessDescription {
    /** Status-bar chip text. */
    chip: string;
    /** One-line state, announced on sign-in and shown in the chip's popover. */
    title: string;
    detail: string;
  }

  /** The words for an access session, shared by the announcement and the status-bar chip. */
  export function describeAccess(access: AccessSession): AccessDescription {
    if (access.via === 'network') {
      return {
        chip: 'Trusted network',
        title: 'Trusted network access active',
        detail: 'Connected from the deployment trusted network.'
      };
    }
    if (access.via === 'cloudflare-access') {
      return {
        chip: access.principal ? `Cloudflare Access · ${access.principal}` : 'Cloudflare Access',
        title: 'Signed in through Cloudflare Access',
        detail: `Signed in as ${access.principal}. Cloudflare Access supplies the credential; sign out there to end it.`
      };
    }
    return {
      chip: 'Bearer',
      title: 'Preview access active',
      detail: 'Held in memory only — cleared on reload.'
    };
  }
</script>

<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { HealthDocument } from '$lib/gen/graphql';
  import { Button, Field, Input } from '$lib/ui/primitives';
  import { resetAccessCapabilities, setAccessStatus } from './accessCapabilities';
  import { graphqlFetch } from './client';
  import {
    setGraphQLCredentialProvider,
    setGraphQLTrustedNetworkAccess
  } from './credentials';
  import { disposeClient } from './subscriptions';

  const MIN_TOKEN_LENGTH = 24;

  /**
   * When authenticated the gate renders no visible UI: the IDE shows the
   * session as a status-bar chip (`AccessChip`) fed by `access`, and "Clear
   * access" calls `clearCredential()`. Screen readers still hear the state
   * change through a visually hidden status line.
   */
  export let authenticated = false;
  export let access: AccessSession | null = null;

  let accessToken = '';
  let memoryToken = '';
  let error: string | null = null;
  let busy = false;
  type HeaderlessVia = Exclude<AccessVia, 'bearer'>;

  function isHeaderlessVia(value: unknown): value is HeaderlessVia {
    return value === 'network' || value === 'cloudflare-access';
  }

  // The server decides whether this browser is already authenticated without a
  // token: from the deployment's trusted network, or through the Cloudflare
  // Access session the edge verified. Either way the requests go out without an
  // Authorization header and the gate steps aside.
  //
  // The same response carries the identity's capabilities (roles held, operator
  // planes reachable, streaming on/off); it feeds the accessCapabilities store
  // before the IDE renders, so surfaces can pre-flight instead of failing. An
  // older server's two-key answer leaves the store "unknown".
  async function activateHeaderlessAccess(): Promise<void> {
    try {
      const response = await fetch('/api/auth/status', {
        headers: { Accept: 'application/json' },
        cache: 'no-store'
      });
      if (!response.ok) return;
      const status = (await response.json()) as {
        authenticated?: boolean;
        authVia?: string;
        principal?: string;
      };
      if (!status.authenticated || !isHeaderlessVia(status.authVia)) return;

      await disposeClient();
      setGraphQLCredentialProvider(null);
      setGraphQLTrustedNetworkAccess(true);
      await graphqlFetch(HealthDocument, {}, { showErrorToast: false });
      setAccessStatus(status);
      access = { via: status.authVia, principal: status.principal ?? '' };
      authenticated = true;
    } catch {
      resetAccessCapabilities();
      setGraphQLTrustedNetworkAccess(false);
    }
  }

  async function installCredential(): Promise<void> {
    const candidate = accessToken.trim();
    if (candidate.length < MIN_TOKEN_LENGTH) {
      error = `Access token must be at least ${MIN_TOKEN_LENGTH} characters.`;
      return;
    }

    busy = true;
    error = null;
    // The status probe ran before a token existed, so a bearer session's
    // capabilities are unknown and every surface keeps its try-then-explain path.
    resetAccessCapabilities();
    try {
      await disposeClient();
      setGraphQLTrustedNetworkAccess(false);
      memoryToken = candidate;
      setGraphQLCredentialProvider(() => memoryToken || null);
      await graphqlFetch(HealthDocument, {}, { showErrorToast: false });
      accessToken = '';
      access = { via: 'bearer', principal: '' };
      authenticated = true;
    } catch {
      accessToken = '';
      memoryToken = '';
      setGraphQLCredentialProvider(null);
      error = 'Credential validation failed. Confirm the deployment token and try again.';
    } finally {
      busy = false;
    }
  }

  /** Ends a bearer session: drops the in-memory token and closes the socket. */
  export async function clearCredential(): Promise<void> {
    authenticated = false;
    access = null;
    accessToken = '';
    memoryToken = '';
    error = null;
    resetAccessCapabilities();
    setGraphQLCredentialProvider(null);
    setGraphQLTrustedNetworkAccess(false);
    try {
      await disposeClient();
    } catch {
      error = 'Access was cleared, but the connection did not close cleanly. Reload this page.';
    }
  }

  onMount(() => {
    void activateHeaderlessAccess();
  });

  onDestroy(() => {
    memoryToken = '';
    resetAccessCapabilities();
    setGraphQLCredentialProvider(null);
    setGraphQLTrustedNetworkAccess(false);
    void disposeClient();
  });
</script>

{#if authenticated}
  {#if access}
    <p class="sr-only" role="status" aria-live="polite" data-testid="access-announcement">
      {describeAccess(access).title}
    </p>
  {/if}
{:else}
  <main class="gate-shell" aria-labelledby="credential-gate-title">
    <section class="gate-dialog">
      <div class="gate-wordmark" aria-hidden="true">fi-fhir</div>
      <h1 id="credential-gate-title">Enter access token</h1>
      <p class="intro">Paste the deployment bearer token to continue.</p>

      <form on:submit|preventDefault={installCredential} aria-label="Install preview credential">
        <Field
          label="Deployment bearer credential"
          id="graphql-access-token"
          hint="Held in this tab's memory only — never stored. Reloading clears access."
        >
          <Input
            type="password"
            size="md"
            mono
            bind:value={accessToken}
            invalid={Boolean(error)}
            aria-describedby={error ? 'credential-error' : undefined}
            autocomplete="off"
            autocapitalize="none"
            spellcheck="false"
            placeholder="Paste bearer credential"
            disabled={busy}
          />
        </Field>
        {#if error}
          <p class="error" id="credential-error" role="alert">{error}</p>
        {/if}
        <Button
          type="submit"
          variant="primary"
          size="md"
          loading={busy}
          disabled={!accessToken.trim()}
          class="install-button"
        >
          {busy ? 'Verifying…' : 'Continue'}
        </Button>
      </form>
    </section>
  </main>
{/if}

<style>
  .gate-shell {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: var(--space-6) var(--space-4);
    color: var(--color-text-primary);
    background: var(--color-bg-base);
  }

  .gate-dialog {
    width: min(100%, 400px);
    padding: var(--space-6);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-md);
    background: var(--color-bg-overlay);
    box-shadow: var(--shadow-xl);
  }

  .gate-wordmark {
    font-family: var(--font-heading);
    font-size: var(--text-lg);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-tight);
    color: var(--color-text-primary);
  }

  h1 {
    margin: var(--space-4) 0 0;
    font-family: var(--font-ui);
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    line-height: var(--leading-tight);
  }

  .intro {
    margin: var(--space-1) 0 0;
    color: var(--color-text-secondary);
    font-size: var(--text-ui);
    line-height: var(--leading-ui);
  }

  form {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-5);
  }

  .error {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
  }

  form :global(.install-button) {
    width: 100%;
    margin-top: var(--space-1);
  }
</style>
