<script lang="ts">
  import Button from '$lib/ui/Button.svelte';
  import Badge from '$lib/ui/Badge.svelte';
  import { isErrorToasted } from '$lib/graphql/client';
  import { toasts } from '$lib/ui/toastStore';
  import {
    approveSessionPublication,
    deploySessionPublication,
    publishIntegrationSession
  } from '../workflowApi';
  import type {
    PublishIntegrationSessionMutation,
    SessionDeploymentSnapshot
  } from '$lib/gen/graphql';

  export let sessionId: string;
  export let profileRevisionId: string;
  export let workflowSimulationId: string;

  type Publication = PublishIntegrationSessionMutation['publishIntegrationSession'];

  let definitionId = '';
  let definitionRevisionId = '';
  let reason = '';
  let expectedVersion = 1;
  let publication: Publication | null = null;
  let deployment: SessionDeploymentSnapshot | null = null;
  let working: 'publish' | 'approve' | 'deploy' | null = null;

  $: publishReady = Boolean(
    sessionId && profileRevisionId && workflowSimulationId &&
    definitionId.trim() && definitionRevisionId.trim() && reason.trim()
  );

  async function publish() {
    if (!publishReady) return;
    working = 'publish';
    try {
      const data = await publishIntegrationSession({
        sessionId,
        profileRevisionId,
        workflowSimulationId,
        definitionId: definitionId.trim(),
        definitionRevisionId: definitionRevisionId.trim(),
        reason: reason.trim()
      });
      publication = data.publishIntegrationSession;
      expectedVersion = publication.definitionVersion;
      deployment = null;
      toasts.success('Session evidence signed and published');
    } catch (error) {
      if (!isErrorToasted(error)) toasts.error('Session publication failed');
    } finally {
      working = null;
    }
  }

  async function approve() {
    if (!publication) return;
    working = 'approve';
    try {
      const data = await approveSessionPublication({
        sessionId,
        publicationId: publication.id,
        expectedVersion,
        reason: reason.trim()
      });
      deployment = data.approveSessionPublication;
      expectedVersion = deployment.version;
      toasts.success('Signed publication approved');
    } catch (error) {
      if (!isErrorToasted(error)) toasts.error('Publication approval failed');
    } finally {
      working = null;
    }
  }

  async function deploy() {
    if (!publication) return;
    working = 'deploy';
    try {
      const data = await deploySessionPublication({
        sessionId,
        publicationId: publication.id,
        expectedVersion,
        reason: reason.trim()
      });
      deployment = data.deploySessionPublication;
      expectedVersion = deployment.version;
      toasts.success('Signed publication deployed');
    } catch (error) {
      if (!isErrorToasted(error)) toasts.error('Publication deployment failed');
    } finally {
      working = null;
    }
  }
</script>

<section class="publication-panel" aria-labelledby="publication-heading">
  <header>
    <div>
      <h4 id="publication-heading">Publish tested integration</h4>
      <p>Bind this exact simulation and profile revision to an already validated production definition.</p>
    </div>
    {#if deployment}
      <Badge variant={deployment.state === 'deployed' ? 'success' : 'info'} size="sm">{deployment.state}</Badge>
    {/if}
  </header>

  {#if !profileRevisionId}
    <div class="warning" role="alert">Selected runs do not share one immutable profile revision.</div>
  {/if}

  <div class="fields">
    <label>
      <span>Definition ID</span>
      <input bind:value={definitionId} placeholder="adt-http" disabled={Boolean(publication)} />
    </label>
    <label>
      <span>Definition revision</span>
      <input bind:value={definitionRevisionId} placeholder="definition-7" disabled={Boolean(publication)} />
    </label>
    <label class="wide">
      <span>Promotion reason</span>
      <textarea bind:value={reason} maxlength="1024" placeholder="Explain why this tested revision is ready"></textarea>
    </label>
  </div>

  {#if publication}
    <div class="evidence">
      <div><span>Manifest</span><code>{publication.manifestDigest}</code></div>
      <div><span>Signature</span><code>{publication.signatureAlgorithm} / {publication.signingKeyId}</code></div>
      <div><span>Production profile</span><code>{publication.productionProfile.artifactId}/{publication.productionProfile.revisionId}</code></div>
      <div><span>Production workflow</span><code>{publication.productionWorkflow.artifactId}/{publication.productionWorkflow.revisionId}</code></div>
    </div>
  {/if}

  <div class="actions">
    <Button on:click={publish} loading={working === 'publish'} disabled={!publishReady || Boolean(publication)}>
      Sign &amp; Publish
    </Button>
    {#if publication}
      <label class="version-field">
        <span>Expected lifecycle version</span>
        <input type="number" min="1" bind:value={expectedVersion} />
      </label>
      <Button variant="secondary" on:click={approve} loading={working === 'approve'} disabled={working !== null || deployment !== null}>
        Approve
      </Button>
      <Button on:click={deploy} loading={working === 'deploy'} disabled={working !== null || (deployment?.state !== 'approved' && deployment?.state !== 'published')}>
        Deploy
      </Button>
    {/if}
  </div>
</section>

<style>
  .publication-panel {
    display: grid;
    gap: var(--space-4);
    padding: var(--space-4);
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-lg);
    background: var(--color-bg-surface);
  }

  header, .actions {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
    flex-wrap: wrap;
  }

  h4, p { margin: 0; }
  p { color: var(--color-text-secondary); font-size: var(--text-sm); }

  .fields {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  label { display: grid; gap: var(--space-1); font-size: var(--text-sm); }
  .wide { grid-column: 1 / -1; }
  textarea { min-height: 72px; resize: vertical; }

  .evidence {
    display: grid;
    gap: var(--space-2);
    font-size: var(--text-xs);
  }

  .evidence div { display: grid; grid-template-columns: 140px minmax(0, 1fr); gap: var(--space-2); }
  .evidence span { color: var(--color-text-secondary); }
  code { overflow-wrap: anywhere; }

  .version-field { margin-left: auto; }
  .version-field input { width: 110px; }
  .warning { color: var(--color-warning); font-size: var(--text-sm); }

  @media (max-width: 720px) {
    .fields { grid-template-columns: 1fr; }
    .wide { grid-column: auto; }
    .version-field { margin-left: 0; }
  }
</style>
