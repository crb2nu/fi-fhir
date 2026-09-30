<!--
  New definition (.loom/42 E-1): bind a compiled source revision, a profile and
  workflow pair the runtime can resolve, and compiled destination revisions;
  bind each secret the chosen revisions name to a reference; choose retention
  and deployment policy; Check (every pre-flight the seed runs, no write);
  then Create draft through the reason dialog. Validation, approval and
  publication follow on the created definition.
-->
<script lang="ts">
  import { onDestroy, onMount } from 'svelte';
  import CircleAlert from '@lucide/svelte/icons/circle-alert';
  import ShieldCheck from '@lucide/svelte/icons/shield-check';
  import X from '@lucide/svelte/icons/x';
  import { Badge, Button, EmptyState, Field, IconButton, Input, Select } from '$lib/ui/primitives';
  import { toasts } from '$lib/ui/toastStore';
  import { clearDirty, markDirty } from '$lib/ui/ide/ideStore';
  import ConnectionReasonDialog from './ConnectionReasonDialog.svelte';
  import {
    checkDraft,
    createDraft,
    fetchConnectionChoices,
    fetchDefaultMaxAge,
    fetchRegistryArtifacts,
    type ConnectionChoice,
    type DefinitionDetail,
    type DefinitionProblem,
    type RegistryArtifact
  } from './definitionsApi';
  import { describeDefinitionFailure } from './definitionsErrors';
  import { blocking, compiled, deriveBindings, newDraftForm, toDraftInput, type DraftForm } from './definitionDraft';
  import { shortHash } from './presentation';

  interface Props {
    writeBlocked: string[] | null;
    oncreated: (detail: DefinitionDetail) => void;
    oncancel: () => void;
  }

  let { writeBlocked, oncreated, oncancel }: Props = $props();

  const PROVIDERS = ['env', 'file', 'vault', 'aws-ssm', 'k8s'].map((value) => ({ value, label: value }));

  let sources = $state<ConnectionChoice[]>([]);
  let destinations = $state<ConnectionChoice[]>([]);
  let artifacts = $state<RegistryArtifact[]>([]);
  let maxAge = $state<number | null>(null);
  let loading = $state(true);
  let loadError = $state<string | null>(null);
  let form = $state<DraftForm>(newDraftForm(null));
  let problems = $state<DefinitionProblem[] | null>(null);
  let checkedKey = $state<string | null>(null);
  let checking = $state(false);
  let checkError = $state<string | null>(null);
  let dialogOpen = $state(false);
  let creating = $state(false);
  let submitError = $state<string | null>(null);

  const compiledSources = $derived(compiled(sources));
  const compiledDestinations = $derived(compiled(destinations));
  const chosenSource = $derived(compiledSources.find((choice) => choice.id === form.sourceId) ?? null);
  const chosenDestinations = $derived(compiledDestinations.filter((choice) => form.destinationIds.includes(choice.id)));
  const artifact = $derived(artifacts.find((candidate) => candidate.integrationId === form.integrationId) ?? null);
  const input = $derived(toDraftInput(form, compiledSources, compiledDestinations, artifacts));
  const inputKey = $derived(JSON.stringify(input));
  const checkedClean = $derived(checkedKey === inputKey && problems !== null && !blocking(problems));
  const writeReason = $derived(
    writeBlocked ? `Read only: this identity does not hold ${writeBlocked.join(', ')}.` : undefined
  );

  onMount(async () => {
    try {
      const [sourceList, destinationList, artifactList, defaultMaxAge] = await Promise.all([
        fetchConnectionChoices('SOURCE'),
        fetchConnectionChoices('DESTINATION'),
        fetchRegistryArtifacts(),
        fetchDefaultMaxAge().catch(() => null)
      ]);
      sources = sourceList;
      destinations = destinationList;
      artifacts = artifactList;
      maxAge = defaultMaxAge;
      form.policy.validationMaxAgeSeconds = defaultMaxAge ?? 300;
    } catch (err) {
      loadError = describeDefinitionFailure(err).message;
    } finally {
      loading = false;
    }
  });

  // The /connections tab shows unsaved work while a draft is being composed
  // (E-4's dirty tabs); creating or leaving the form clears it.
  const DIRTY_TAB = '/connections';
  const dirty = $derived(
    form.definitionId.trim() !== '' ||
      form.sourceId !== '' ||
      form.destinationIds.length > 0 ||
      form.integrationId !== '' ||
      form.customPolicy ||
      form.retention.mode !== 'ephemeral'
  );
  $effect(() => {
    markDirty(DIRTY_TAB, dirty);
  });
  onDestroy(() => clearDirty(DIRTY_TAB));

  // Re-derive the binding rows whenever the chosen revisions change, keeping edits.
  $effect(() => {
    const derived = deriveBindings(chosenSource, chosenDestinations, form.bindings);
    if (JSON.stringify(derived) !== JSON.stringify(form.bindings)) form.bindings = derived;
  });

  function toggleDestination(id: string, on: boolean): void {
    form.destinationIds = on ? [...form.destinationIds, id] : form.destinationIds.filter((value) => value !== id);
  }

  function problemsAt(prefix: string): DefinitionProblem[] {
    return (problems ?? []).filter((problem) => problem.path === prefix || problem.path.startsWith(`${prefix}[`) || problem.path.startsWith(`${prefix}.`));
  }

  async function check(): Promise<void> {
    checking = true;
    checkError = null;
    const key = inputKey;
    try {
      problems = await checkDraft(input);
      checkedKey = key;
    } catch (err) {
      checkError = describeDefinitionFailure(err).message;
      problems = null;
    } finally {
      checking = false;
    }
  }

  async function create(reason: string): Promise<void> {
    creating = true;
    submitError = null;
    try {
      const result = await createDraft(input, reason);
      if (!result.definition) {
        problems = result.problems;
        checkedKey = inputKey;
        submitError = 'Nothing was written: the pre-flight found problems, listed on the form.';
        return;
      }
      toasts.success(`Created draft ${result.definition.definition.definitionId}/${result.definition.definition.revisionId}`);
      dialogOpen = false;
      clearDirty(DIRTY_TAB);
      oncreated(result.definition);
    } catch (err) {
      submitError = describeDefinitionFailure(err).message;
    } finally {
      creating = false;
    }
  }

  // `secretBindings[2].key` → `fhir-token key`: the row the problem is about.
  function bindingLabel(path: string): string {
    const match = /^secretBindings\[(\d+)\](?:\.(\w+))?/.exec(path);
    const row = match ? form.bindings[Number(match[1])] : undefined;
    return row ? `${row.name}${match?.[2] ? ` ${match[2]}` : ''}` : path;
  }

  const sourceOptions = $derived(
    compiledSources.map((choice) => ({
      value: choice.id,
      label: `${choice.name} — ${choice.id} r${choice.latestRevision?.revisionId} (${choice.kind.toLowerCase()}, source ${choice.latestRevision?.sourceId ?? '?'})`
    }))
  );
  const artifactOptions = $derived(
    artifacts.map((candidate) => ({ value: candidate.integrationId, label: `${candidate.integrationId} (source ${candidate.sourceId})` }))
  );
</script>

<div class="new" data-testid="definition-new">
  <header class="head">
    <h2 class="title">New definition</h2>
    <span class="spacer"></span>
    <IconButton icon={X} label="Cancel new definition" onclick={oncancel} />
  </header>

  {#if loading}
    <EmptyState message="Loading compiled connections and registry artifacts" aria-busy="true" />
  {:else if loadError}
    <EmptyState icon={CircleAlert} role="alert" message={loadError} />
  {:else}
    <div class="body">
      <section class="section">
        <div class="row">
          <Field label="Definition ID" required error={problemsAt('definitionId')[0]?.message}>
            <Input mono bind:value={form.definitionId} placeholder="adt-east" data-testid="definition-new-id" />
          </Field>
          <Field label="Revision ID" required error={problemsAt('revisionId')[0]?.message}>
            <Input mono bind:value={form.revisionId} data-testid="definition-new-revision" />
          </Field>
        </div>
      </section>

      <section class="section">
        <h3 class="section-title">Source</h3>
        {#if compiledSources.length === 0}
          <p class="note">No source connection has a compiled revision. Compile one on the Sources tab first.</p>
        {:else}
          <Field label="Compiled source revision" required error={problemsAt('source')[0]?.message}>
            <Select
              options={sourceOptions}
              placeholder="Choose a source"
              bind:value={form.sourceId}
              data-testid="definition-new-source"
            />
          </Field>
          {#if chosenSource?.latestRevision}
            <p class="note mono">digest {shortHash(chosenSource.latestRevision.digest, 16)}</p>
          {/if}
          <p class="note">A connection's latest compiled revision is bound; compile again to bind a newer one.</p>
        {/if}
      </section>

      <section class="section">
        <h3 class="section-title">Profile and workflow</h3>
        <Field label="Registry integration" required error={problemsAt('profile')[0]?.message ?? problemsAt('workflow')[0]?.message}>
          <Select
            options={artifactOptions}
            placeholder="Choose a profile and workflow pair"
            bind:value={form.integrationId}
            data-testid="definition-new-artifacts"
          />
        </Field>
        {#if artifact}
          <p class="note mono">
            profile {artifact.profile.artifactId} r{artifact.profile.revisionId} · {shortHash(artifact.profile.digest)}<br />
            workflow {artifact.workflow.artifactId} r{artifact.workflow.revisionId} · {shortHash(artifact.workflow.digest)}
          </p>
        {/if}
        <p class="note">
          These refs come from the static integration registry, proven with the resolver the runtime admits messages
          with; a profile or workflow published only in the Studio's stores cannot be resolved at admission yet.
        </p>
      </section>

      <section class="section">
        <h3 class="section-title">Destinations</h3>
        {#if compiledDestinations.length === 0}
          <p class="note">No destination connection has a compiled revision. Compile one on the Destinations tab first.</p>
        {:else}
          <div class="checks" data-testid="definition-new-destinations">
            {#each compiledDestinations as choice (choice.id)}
              <label class="check">
                <input
                  type="checkbox"
                  checked={form.destinationIds.includes(choice.id)}
                  onchange={(event) => toggleDestination(choice.id, event.currentTarget.checked)}
                  data-destination={choice.id}
                />
                <code>{choice.id}</code> r{choice.latestRevision?.revisionId}
                <Badge tone={choice.latestRevision?.destinationClass === 'production' ? 'warning' : 'neutral'}>
                  {choice.latestRevision?.destinationClass ?? '?'}
                </Badge>
                <span class="muted">{choice.kind.toLowerCase()}</span>
              </label>
            {/each}
          </div>
        {/if}
        {#each problemsAt('destinations') as problem (problem.path + problem.code)}
          <p class="problem" role="alert">{problem.message}</p>
        {/each}
      </section>

      <section class="section">
        <h3 class="section-title">Secret bindings</h3>
        {#if form.bindings.length === 0}
          <p class="note">The chosen revisions name no secret bindings.</p>
        {:else}
          <p class="note">
            Pre-filled from each connection's current binding references (a compiled revision stores binding names only).
            A reference says where a secret lives: a key of at most 256 characters with no whitespace, never a value and
            never certificate or key material.
          </p>
          <div class="bindings" data-testid="definition-new-bindings">
            {#each form.bindings as binding (binding.name)}
              <div class="binding" data-binding={binding.name}>
                <span class="binding-name"><code>{binding.name}</code><span class="muted">{binding.requiredBy}</span></span>
                <Select size="sm" options={PROVIDERS} placeholder="provider" bind:value={binding.provider} aria-label={`${binding.name} provider`} />
                <Input mono bind:value={binding.key} placeholder="key" aria-label={`${binding.name} key`} />
              </div>
            {/each}
          </div>
        {/if}
        {#each problemsAt('secretBindings') as problem (problem.path + problem.code + problem.message)}
          <p class={problem.code === 'UNUSED_BINDING' ? 'warning' : 'problem'} role="alert">
            {#if problem.path !== 'secretBindings'}<code>{bindingLabel(problem.path)}</code>&nbsp;{/if}{problem.message}
          </p>
        {/each}
      </section>

      <section class="section">
        <h3 class="section-title">Data handling</h3>
        <div class="row">
          <Field label="Classification" hint="Every definition carries PHI.">
            <Input value="phi" readonly />
          </Field>
          <Field label="Raw retention">
            <Select
              options={[
                { value: 'ephemeral', label: 'ephemeral (source bytes are not kept)' },
                { value: 'encrypted', label: 'encrypted (kept for a TTL, access audited)' }
              ]}
              bind:value={form.retention.mode}
            />
          </Field>
        </div>
        {#if form.retention.mode === 'encrypted'}
          <div class="row">
            <Field label="TTL seconds"><Input type="number" bind:value={form.retention.ttlSeconds} /></Field>
            <Field label="Purpose"><Input bind:value={form.retention.purpose} /></Field>
          </div>
          <div class="row">
            <Field label="Storage artifact"><Input mono bind:value={form.retention.storageArtifactId} /></Field>
            <Field label="Storage revision"><Input mono bind:value={form.retention.storageRevisionId} /></Field>
          </div>
          <Field label="Storage digest"><Input mono bind:value={form.retention.storageDigest} placeholder="sha256:…" /></Field>
          <div class="row">
            <Field label="Key provider"><Select options={PROVIDERS} bind:value={form.retention.keyProvider} /></Field>
            <Field label="Key reference"><Input mono bind:value={form.retention.keyKey} /></Field>
          </div>
          <p class="note">You are recorded as the authorizer, and retained-data access is audited.</p>
        {/if}
        {#each [...problemsAt('rawRetention'), ...problemsAt('policy')] as problem (problem.path + problem.code)}
          <p class="problem" role="alert">{problem.path}: {problem.message}</p>
        {/each}
      </section>

      <section class="section">
        <h3 class="section-title">Deployment policy</h3>
        <label class="check">
          <input type="checkbox" bind:checked={form.customPolicy} />
          Customize (otherwise the default: validation timeout 5 s, max age {maxAge ?? 300} s from
          <code>FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE</code>, continuous, 2 in flight, 10 queued, 100 messages/s)
        </label>
        {#if form.customPolicy}
          <div class="row">
            <Field label="Validation timeout (s)"><Input type="number" bind:value={form.policy.validationTimeoutSeconds} /></Field>
            <Field label="Validation max age (s)"><Input type="number" bind:value={form.policy.validationMaxAgeSeconds} /></Field>
          </div>
          <div class="row">
            <Field label="Max in flight"><Input type="number" bind:value={form.policy.maxInFlight} /></Field>
            <Field label="Max queued"><Input type="number" bind:value={form.policy.maxQueued} /></Field>
            <Field label="Messages / s"><Input type="number" bind:value={form.policy.maxMessagesPerSecond} /></Field>
          </div>
        {/if}
        {#each problemsAt('deployment') as problem (problem.path + problem.code)}
          <p class="problem" role="alert">{problem.path}: {problem.message}</p>
        {/each}
      </section>

      {#if problems !== null && checkedKey === inputKey}
        <section class="section" data-testid="definition-problems" data-blocking={blocking(problems) ? 'true' : 'false'}>
          {#if problems.length === 0}
            <p class="ok"><ShieldCheck size={14} aria-hidden="true" /> Check passed: the runtime can resolve and plan this definition.</p>
          {:else}
            <ul class="problem-list">
              {#each problems as problem (problem.path + problem.code + problem.message)}
                <li class={problem.code === 'UNUSED_BINDING' ? 'warning' : 'problem'} data-code={problem.code}>
                  <code>{problem.code}</code>{#if problem.path}&nbsp;<code class="muted">{problem.path}</code>{/if} — {problem.message}
                </li>
              {/each}
            </ul>
          {/if}
        </section>
      {/if}
      {#if checkError}
        <p class="problem section" role="alert">{checkError}</p>
      {/if}
    </div>

    <footer class="footer">
      <Button variant="ghost" onclick={oncancel}>Cancel</Button>
      <span class="spacer"></span>
      <Button
        loading={checking}
        disabled={writeReason !== undefined}
        title={writeReason ?? 'Run every pre-flight check; nothing is written'}
        onclick={check}
        data-testid="definition-check"
      >
        Check
      </Button>
      <Button
        variant="primary"
        disabled={writeReason !== undefined || !checkedClean}
        title={writeReason ?? (checkedClean ? 'Create the draft' : 'Run Check on these inputs first; Create is enabled when nothing blocks.')}
        onclick={() => {
          submitError = null;
          dialogOpen = true;
        }}
        data-testid="definition-create"
      >
        Create draft
      </Button>
    </footer>

    <ConnectionReasonDialog
      open={dialogOpen}
      title="Create definition draft"
      description={`Writes ${input.definitionId}/${input.revisionId} as an immutable draft at version 1. It deploys nothing.`}
      confirmText="Create draft"
      loading={creating}
      {submitError}
      onconfirm={(reason) => void create(reason)}
      oncancel={() => (dialogOpen = false)}
    />
  {/if}
</div>

<style>
  .new {
    display: flex;
    flex-direction: column;
    min-height: 0;
    height: 100%;
  }

  .head,
  .footer {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .footer {
    border-top: 1px solid var(--color-border-subtle);
    border-bottom: 0;
  }

  .title {
    margin: 0;
    font-size: var(--text-lg);
    font-weight: var(--font-semibold);
  }

  .spacer {
    flex: 1 1 auto;
  }

  .body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
  }

  .section {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .section-title {
    margin: 0;
    font-size: var(--text-xs);
    font-weight: var(--font-semibold);
    text-transform: uppercase;
    letter-spacing: 0.04em;
    color: var(--color-text-tertiary);
  }

  .row {
    display: grid;
    grid-auto-flow: column;
    grid-auto-columns: minmax(0, 1fr);
    gap: var(--space-2);
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .note.mono {
    font-family: var(--font-mono);
  }

  .checks {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .check {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
  }

  .bindings {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
  }

  .binding {
    display: grid;
    grid-template-columns: minmax(0, 1.3fr) 110px minmax(0, 1.5fr);
    align-items: center;
    gap: var(--space-2);
  }

  .binding-name {
    display: flex;
    flex-direction: column;
    min-width: 0;
  }

  .muted {
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .problem-list {
    display: flex;
    flex-direction: column;
    gap: var(--space-1);
    margin: 0;
    padding: 0;
    list-style: none;
    font-size: var(--text-xs);
  }

  .problem {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .warning {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-warning-text);
  }

  .ok {
    display: flex;
    align-items: center;
    gap: var(--space-1);
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-success-text);
  }

  code {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-primary);
    overflow-wrap: anywhere;
  }

  code.muted {
    color: var(--color-text-tertiary);
  }
</style>
