<script lang="ts">
  import TriangleAlert from '@lucide/svelte/icons/triangle-alert';
  import { Badge, Button, Field, Icon, Input, KeyValue, Panel, Textarea } from '$lib/ui/primitives';
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

<Panel title="Publish tested integration" titleTag="h3">
  {#snippet actions()}
    {#if deployment}
      <Badge tone={deployment.state === 'deployed' ? 'success' : 'info'} dot>{deployment.state}</Badge>
    {/if}
  {/snippet}

  <div class="publication">
    <p class="note">
      Binds this simulation and profile revision to an already validated production definition.
    </p>

    {#if !profileRevisionId}
      <p class="note is-warning" role="alert">
        <Icon icon={TriangleAlert} />
        <span>Selected runs do not share one immutable profile revision.</span>
      </p>
    {/if}

    <div class="form-grid">
      <Field label="Definition id">
        <Input mono bind:value={definitionId} placeholder="adt-http" disabled={Boolean(publication)} />
      </Field>
      <Field label="Definition revision">
        <Input
          mono
          bind:value={definitionRevisionId}
          placeholder="definition-7"
          disabled={Boolean(publication)}
        />
      </Field>
      <Field label="Promotion reason" class="span-2">
        <Textarea
          bind:value={reason}
          maxlength={1024}
          rows={3}
          placeholder="Why this tested revision is ready"
        />
      </Field>
    </div>

    {#if publication}
      <KeyValue
        items={[
          { key: 'Manifest', value: publication.manifestDigest, mono: true, truncate: true },
          {
            key: 'Signature',
            value: `${publication.signatureAlgorithm} / ${publication.signingKeyId}`,
            mono: true,
            truncate: true
          },
          {
            key: 'Production profile',
            value: `${publication.productionProfile.artifactId}/${publication.productionProfile.revisionId}`,
            mono: true,
            truncate: true
          },
          {
            key: 'Production workflow',
            value: `${publication.productionWorkflow.artifactId}/${publication.productionWorkflow.revisionId}`,
            mono: true,
            truncate: true
          }
        ]}
      />
    {/if}

    <div class="button-row">
      <Button
        onclick={publish}
        loading={working === 'publish'}
        disabled={!publishReady || Boolean(publication)}
      >
        Sign and publish
      </Button>
      {#if publication}
        <Button
          onclick={approve}
          loading={working === 'approve'}
          disabled={working !== null || deployment !== null}
        >
          Approve
        </Button>
        <Button
          onclick={deploy}
          loading={working === 'deploy'}
          disabled={working !== null ||
            (deployment?.state !== 'approved' && deployment?.state !== 'published')}
        >
          Deploy
        </Button>
        <label class="version-field">
          <span class="version-label">Expected lifecycle version</span>
          <Input mono type="number" min="1" bind:value={expectedVersion} />
        </label>
      {/if}
    </div>
  </div>
</Panel>

<style>
  .publication {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .note {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .note.is-warning {
    color: var(--color-warning-text);
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .form-grid :global(.span-2) {
    grid-column: 1 / -1;
  }

  .button-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .version-field {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
  }

  .version-field :global(.ui-input) {
    width: 80px;
  }

  .version-label {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  @media (max-width: 720px) {
    .form-grid {
      grid-template-columns: 1fr;
    }

    .version-field {
      margin-left: 0;
    }
  }
</style>
