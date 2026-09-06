<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import { HealthDocument } from '$lib/gen/graphql';
  import { graphqlFetch } from './client';
  import {
    setGraphQLCredentialProvider,
    setGraphQLTrustedNetworkAccess
  } from './credentials';
  import { disposeClient } from './subscriptions';

  const MIN_TOKEN_LENGTH = 24;

  export let authenticated = false;

  let accessToken = '';
  let memoryToken = '';
  let error: string | null = null;
  let busy = false;
  type HeaderlessVia = 'network' | 'cloudflare-access';
  let headerlessVia: HeaderlessVia | null = null;
  let principal = '';

  function isHeaderlessVia(value: unknown): value is HeaderlessVia {
    return value === 'network' || value === 'cloudflare-access';
  }

  // The server decides whether this browser is already authenticated without a
  // token: from the deployment's trusted network, or through the Cloudflare
  // Access session the edge verified. Either way the requests go out without an
  // Authorization header and the gate steps aside.
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
      headerlessVia = status.authVia;
      principal = status.principal ?? '';
      authenticated = true;
    } catch {
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
    try {
      await disposeClient();
      setGraphQLTrustedNetworkAccess(false);
      memoryToken = candidate;
      setGraphQLCredentialProvider(() => memoryToken || null);
      await graphqlFetch(HealthDocument, {}, { showErrorToast: false });
      accessToken = '';
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

  async function clearCredential(): Promise<void> {
    authenticated = false;
    accessToken = '';
    memoryToken = '';
    headerlessVia = null;
    principal = '';
    error = null;
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
    setGraphQLCredentialProvider(null);
    setGraphQLTrustedNetworkAccess(false);
    void disposeClient();
  });
</script>

{#if authenticated}
  <div class="access-strip" role="status" aria-live="polite">
    <span class="status-dot" aria-hidden="true"></span>
    <div class="access-copy">
      <strong>
        {headerlessVia === 'network'
          ? 'Trusted network access active'
          : headerlessVia === 'cloudflare-access'
            ? 'Signed in through Cloudflare Access'
            : 'Preview access active'}
      </strong>
      <span>
        {headerlessVia === 'network'
          ? 'Connected from the deployment trusted network.'
          : headerlessVia === 'cloudflare-access'
            ? `Signed in as ${principal}. Cloudflare Access supplies the credential; sign out there to end it.`
            : 'Held in memory only — cleared on reload.'}
      </span>
    </div>
    {#if !headerlessVia}
      <button class="clear-button" type="button" on:click={clearCredential}>Clear access</button>
    {/if}
  </div>
{:else}
  <main class="gate-shell" aria-labelledby="credential-gate-title">
    <section class="gate-card">
      <div class="eyebrow">Operator access</div>
      <h1 id="credential-gate-title">Enter access token</h1>
      <p class="intro">Paste the deployment bearer token to continue.</p>

      <div class="privacy-note" id="credential-storage-note">
        Held in this tab's memory only — never stored. Reloading clears access.
      </div>

      <form on:submit|preventDefault={installCredential} aria-label="Install preview credential">
        <label for="graphql-access-token">Deployment bearer credential</label>
        <input
          id="graphql-access-token"
          type="password"
          bind:value={accessToken}
          aria-describedby="credential-storage-note credential-error"
          aria-invalid={error ? 'true' : 'false'}
          autocomplete="off"
          autocapitalize="none"
          spellcheck="false"
          placeholder="Paste bearer credential"
          disabled={busy}
        />
        {#if error}
          <p class="error" id="credential-error" role="alert">{error}</p>
        {/if}
        <button class="install-button" type="submit" disabled={busy || !accessToken.trim()}>
          {busy ? 'Verifying…' : 'Continue'}
        </button>
      </form>
    </section>
  </main>
{/if}

<style>
  .gate-shell {
    min-height: 100vh;
    display: grid;
    place-items: center;
    padding: var(--space-6);
    color: var(--color-text-primary);
    background:
      radial-gradient(circle at 20% 10%, var(--color-primary-muted), transparent 34rem),
      var(--color-bg-base);
  }

  .gate-card {
    width: min(100%, 560px);
    padding: clamp(var(--space-6), 5vw, var(--space-10));
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-2xl);
    background: var(--color-bg-overlay);
    box-shadow: var(--shadow-xl);
  }

  .eyebrow {
    color: var(--color-primary);
    font-size: var(--text-xs);
    font-weight: var(--font-bold);
    letter-spacing: var(--tracking-wider);
    text-transform: uppercase;
  }

  h1 {
    margin: var(--space-3) 0 0;
    font-family: var(--font-heading);
    font-size: clamp(var(--text-2xl), 5vw, 2rem);
    line-height: var(--leading-tight);
  }

  .intro {
    margin: var(--space-4) 0 0;
    color: var(--color-text-secondary);
    line-height: var(--leading-relaxed);
  }

  .privacy-note {
    margin-top: var(--space-5);
    padding: var(--space-3) var(--space-4);
    border: 1px solid var(--color-info-border);
    border-radius: var(--radius-lg);
    color: var(--color-text-secondary);
    background: var(--color-info-bg);
    font-size: var(--text-sm);
    line-height: var(--leading-relaxed);
  }

  form {
    display: grid;
    gap: var(--space-3);
    margin-top: var(--space-6);
  }

  label {
    font-size: var(--text-sm);
    font-weight: var(--font-semibold);
  }

  input {
    width: 100%;
    box-sizing: border-box;
    height: var(--input-height-lg);
    padding: 0 var(--space-4);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    color: var(--color-text-primary);
    background: var(--color-bg-input);
    font: inherit;
    font-family: var(--font-mono);
  }

  input:focus-visible {
    outline: none;
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  input[aria-invalid='true'] {
    border-color: var(--color-danger-border);
  }

  .error {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-sm);
  }

  .install-button,
  .clear-button {
    border-radius: var(--radius-lg);
    cursor: pointer;
    font: inherit;
    font-weight: var(--font-semibold);
    transition: var(--transition-all);
  }

  .install-button {
    min-height: var(--btn-height-lg);
    margin-top: var(--space-1);
    border: 1px solid var(--color-primary-border);
    color: var(--color-text-inverse);
    background: linear-gradient(
      120deg,
      var(--color-brand-gradient-start),
      var(--color-brand-gradient-end)
    );
  }

  .install-button:hover:not(:disabled) {
    transform: translateY(-1px);
    box-shadow: var(--shadow-md);
  }

  .install-button:disabled {
    cursor: not-allowed;
    opacity: 0.55;
  }

  .install-button:focus-visible,
  .clear-button:focus-visible {
    outline: none;
    box-shadow: var(--shadow-focus);
  }

  .access-strip {
    min-height: 44px;
    display: flex;
    align-items: center;
    gap: var(--space-3);
    padding: var(--space-2) var(--space-4);
    border-bottom: 1px solid var(--color-success-border);
    color: var(--color-text-primary);
    background: var(--color-success-bg);
  }

  .status-dot {
    width: 9px;
    height: 9px;
    flex: 0 0 auto;
    border-radius: var(--radius-full);
    background: var(--color-success);
  }

  .access-copy {
    min-width: 0;
    display: flex;
    flex: 1 1 auto;
    align-items: baseline;
    gap: var(--space-2);
    font-size: var(--text-xs);
  }

  .access-copy span {
    color: var(--color-text-secondary);
  }

  .clear-button {
    min-height: var(--btn-height-sm);
    padding: 0 var(--space-3);
    border: 1px solid var(--color-border-default);
    color: var(--color-text-secondary);
    background: var(--color-bg-elevated);
  }

  .clear-button:hover {
    color: var(--color-text-primary);
    background: var(--color-bg-hover);
  }

  @media (max-width: 640px) {
    .gate-shell {
      padding: var(--space-3);
    }

    .gate-card {
      padding: var(--space-6);
    }

    .access-copy {
      display: grid;
      gap: var(--space-1);
    }
  }
</style>
