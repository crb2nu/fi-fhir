<!--
  Engine: what this replica composed at startup (`engineRuntime`), read only.
  Process properties are immutable per process, so each one shows the
  environment variable that sets it — where the runtime lists that variable —
  and the change is made in GitOps. Four adapter panels always render, enabled
  or not; a secret property reads only "set" or "unset".
-->
<script lang="ts">
  import { onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import RefreshCw from '@lucide/svelte/icons/refresh-cw';
  import { Badge, EmptyState, IconButton, KeyValue, Panel, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import { fetchEngineRuntime, type EngineAdapterRow, type EngineRuntimeView } from './connectionsApi';
  import { describeConnectionFailure } from './connectionsErrors';
  import {
    DESTINATION_IDENTITY_KEYS,
    adapterPresentation,
    adapterValue,
    listedKey,
    listedKeys,
    onOff,
    yesNo
  } from './engineProperties';

  let runtime = $state<EngineRuntimeView | null>(null);
  let loading = $state(true);
  let error = $state<string | null>(null);
  let seq = 0;

  async function load(): Promise<void> {
    const current = ++seq;
    loading = true;
    error = null;
    try {
      const answer = await fetchEngineRuntime();
      if (current !== seq) return;
      runtime = answer;
    } catch (err) {
      if (current !== seq) return;
      error = describeConnectionFailure(err).message;
    } finally {
      if (current === seq) loading = false;
    }
  }

  onMount(() => {
    void load();
  });

  const listed = $derived(runtime ? listedKeys(runtime) : new Set<string>());

  function adapterItems(adapter: EngineAdapterRow) {
    return adapterPresentation(adapter.kind).fields.map((field) => ({
      label: field.label,
      value: adapterValue(adapter, field.field),
      mono: field.mono ?? false,
      envKey: listedKey(listed, field.envKey)
    }));
  }

  function enableKeys(kind: string): string[] {
    return adapterPresentation(kind).enableKeys.filter((key) => listed.has(key));
  }
</script>

<div class="engine" data-testid="engine-runtime">
  {#if loading && !runtime}
    <EmptyState message="Loading the engine runtime" aria-busy="true" aria-live="polite" />
  {:else if error && !runtime}
    <EmptyState icon={CircleAlert} role="alert" message={error} actionLabel="Retry" onaction={load} />
  {:else if runtime}
    <div class="engine-bar">
      <p class="engine-note">
        Read at startup by replica <code>{runtime.replicaId}</code>. Change a property in the deployment's
        environment (GitOps); a running process does not reload it.
      </p>
      <IconButton icon={RefreshCw} label="Refresh engine runtime" loading={loading} onclick={load} />
    </div>
    {#if error}
      <p class="engine-error" role="alert">{error}</p>
    {/if}

    <div class="row row-3">
      <Panel title="Identity and access" titleTag="h2" data-testid="engine-identity">
        <KeyValue
          items={[
            { key: 'Auth mode', value: runtime.authMode, mono: true },
            { key: 'Trusted network', value: yesNo(runtime.trustedNetwork) },
            { key: 'Access identity', value: yesNo(runtime.accessIdentity) },
            { key: 'Tenant', value: runtime.tenantId, mono: true },
            { key: 'Replica', value: runtime.replicaId, mono: true, truncate: true },
            { key: 'Version', value: runtime.version, mono: true }
          ]}
        />
      </Panel>
      <Panel title="Control plane" titleTag="h2" data-testid="engine-control-plane">
        <KeyValue
          items={[
            { key: 'Control plane', value: onOff(runtime.controlPlane) },
            { key: 'Integration sessions', value: onOff(runtime.integrationSessions) },
            { key: 'Streaming', value: onOff(runtime.streaming) },
            { key: 'Retention purge', value: onOff(runtime.retentionPurge) },
            { key: 'LLM', value: runtime.llmConfigured ? 'Configured' : 'Not configured' }
          ]}
        />
      </Panel>
      <Panel title="Registry" titleTag="h2" data-testid="engine-registry">
        <KeyValue items={[{ key: 'Integrations', value: runtime.registry.integrationCount, mono: true }]} />
        {#if runtime.registry.integrations.length > 0}
          <Table label="Registry integrations" layout="fixed" class="registry-table">
            {#snippet head()}
              <tr>
                <Th>Integration</Th>
                <Th>Definition</Th>
                <Th width="56px">Rev</Th>
                <Th>Source</Th>
                <Th width="64px">Format</Th>
              </tr>
            {/snippet}
            {#each runtime.registry.integrations as integration (integration.integrationId)}
              <Tr>
                <Td mono truncate value={integration.integrationId} />
                <Td mono truncate title={integration.digest} value={integration.definitionId} />
                <Td mono value={integration.revisionId} />
                <Td mono truncate value={integration.sourceId} />
                <Td mono value={integration.format} />
              </Tr>
            {/each}
          </Table>
        {/if}
      </Panel>
    </div>

    <div class="row adapters">
      {#each runtime.adapters as adapter (adapter.kind)}
        {@const presentation = adapterPresentation(adapter.kind)}
        <Panel
          title={presentation.title}
          titleTag="h2"
          data-testid="engine-adapter"
          data-kind={adapter.kind}
          data-enabled={adapter.enabled ? 'true' : 'false'}
        >
          {#snippet actions()}
            <Badge tone={adapter.enabled ? 'success' : 'neutral'} dot={adapter.enabled}>
              {adapter.enabled ? 'Enabled' : 'Disabled'}
            </Badge>
          {/snippet}
          {#if adapter.enabled}
            <KeyValue>
              {#each adapterItems(adapter) as item (item.label)}
                <dt>{item.label}</dt>
                <dd class="prop" class:mono={item.mono} class:empty={item.value === null}>
                  <span class="prop-value" title={item.value ?? undefined}>{item.value ?? '—'}</span>
                  {#if item.envKey}<code class="env-key">{item.envKey}</code>{/if}
                </dd>
              {/each}
            </KeyValue>
          {:else}
            {@const keys = enableKeys(adapter.kind)}
            <p class="disabled-note">
              Not enabled on this replica.{#if keys.length > 0}&nbsp;Set by
                {#each keys as key, index (key)}{#if index > 0}&nbsp;and&nbsp;{/if}<code>{key}</code>{/each}.{/if}
            </p>
          {/if}
          {#if presentation.note}
            <p class="adapter-note">{presentation.note}</p>
          {/if}
        </Panel>
      {/each}

      <Panel title="Destination identity" titleTag="h2" data-testid="engine-destination-identity">
        {#snippet actions()}
          <Badge tone={runtime?.destinationIdentity ? 'success' : 'neutral'} dot={!!runtime?.destinationIdentity}>
            {runtime?.destinationIdentity ? 'Loaded' : 'Not loaded'}
          </Badge>
        {/snippet}
        {#if runtime.destinationIdentity}
          {@const modeKey = listedKey(listed, DESTINATION_IDENTITY_KEYS[1])}
          <KeyValue>
            <dt>Mode</dt>
            <dd class="prop mono">
              <span class="prop-value">{runtime.destinationIdentity.mode}</span>
              {#if modeKey}<code class="env-key">{modeKey}</code>{/if}
            </dd>
          </KeyValue>
          {#if runtime.destinationIdentity.destinations.length > 0}
            <Table label="Loaded destinations" layout="fixed" class="destinations-table">
              {#snippet head()}
                <tr>
                  <Th>Destination</Th>
                  <Th width="48px">Rev</Th>
                  <Th width="64px">Transport</Th>
                  <Th width="84px">Class</Th>
                  <Th>Endpoint</Th>
                </tr>
              {/snippet}
              {#each runtime.destinationIdentity.destinations as destination (`${destination.artifactId}@${destination.revisionId}`)}
                <Tr>
                  <Td mono truncate title={destination.digest} value={destination.artifactId} />
                  <Td mono value={destination.revisionId} />
                  <Td mono value={destination.transport} />
                  <Td value={destination.class} />
                  <Td mono truncate muted value={destination.endpointAdvisory} />
                </Tr>
              {/each}
            </Table>
          {/if}
        {:else}
          <p class="disabled-note">No delivery identity registry is loaded on this replica.</p>
        {/if}
      </Panel>
    </div>

    <div class="row tables">
      <Panel title="Ledgers" titleTag="h2" flush data-testid="engine-ledgers">
        <Table label="Migration ledgers" layout="fixed">
          {#snippet head()}
            <tr>
              <Th>Ledger</Th>
              <Th width="72px" numeric>Version</Th>
            </tr>
          {/snippet}
          {#each runtime.ledgers as ledger (ledger.name)}
            <Tr>
              <Td mono value={ledger.name} />
              <Td numeric value={ledger.version} />
            </Tr>
          {/each}
        </Table>
      </Panel>
      <Panel title="Properties" titleTag="h2" flush data-testid="engine-properties">
        <Table label="Process properties" layout="fixed">
          {#snippet head()}
            <tr>
              <Th>Variable</Th>
              <Th>Value</Th>
              <Th width="72px">Source</Th>
            </tr>
          {/snippet}
          {#each runtime.properties as property (property.key)}
            <Tr data-secret={property.secret ? 'true' : 'false'}>
              <Td mono truncate value={property.key} />
              {#if property.secret}
                <Td><Badge tone="neutral" mono>{property.value}</Badge></Td>
              {:else}
                <Td mono truncate muted={property.value === ''} value={property.value === '' ? '—' : property.value} />
              {/if}
              <Td muted value={property.source} />
            </Tr>
          {/each}
        </Table>
      </Panel>
    </div>
  {/if}
</div>

<style>
  .engine {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
    padding: var(--space-3);
    min-height: 0;
    overflow: auto;
  }

  .engine-bar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-3);
  }

  .engine-note,
  .engine-error {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .engine-error {
    color: var(--color-danger-text);
  }

  .row {
    display: grid;
    gap: var(--space-3);
    align-items: start;
  }

  .row-3 {
    grid-template-columns: repeat(3, minmax(0, 1fr));
  }

  .adapters {
    grid-template-columns: repeat(auto-fill, minmax(340px, 1fr));
  }

  .tables {
    grid-template-columns: minmax(0, 1fr) minmax(0, 2fr);
  }

  .prop {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
  }

  .prop.mono .prop-value {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .prop.empty .prop-value {
    color: var(--color-text-muted);
  }

  .prop-value {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .env-key,
  .disabled-note code,
  .engine-note code {
    font-family: var(--font-mono);
    font-size: var(--text-label);
    color: var(--color-text-tertiary);
    overflow-wrap: anywhere;
  }

  .env-key {
    margin-left: auto;
    white-space: nowrap;
  }

  .disabled-note,
  .adapter-note {
    margin: 0;
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-secondary);
  }

  .adapter-note {
    margin-top: var(--space-2);
    color: var(--color-text-tertiary);
  }

  .engine :global(.registry-table),
  .engine :global(.destinations-table) {
    margin-top: var(--space-2);
  }

  @media (max-width: 1100px) {
    .row-3,
    .tables {
      grid-template-columns: minmax(0, 1fr);
    }
  }
</style>
