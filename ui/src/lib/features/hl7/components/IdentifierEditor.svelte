<script lang="ts">
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Button,
    EmptyState,
    Field,
    IconButton,
    Input,
    Panel,
    Select,
    Table,
    Td,
    Th,
    Tr,
    type SelectOption
  } from '$lib/ui/primitives';
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import { afterUpdate, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

  // Props
  export let showAdvanced = false;

  $: identifiers = $selectedProfile?.identifiers;
  $: validationRaw = identifiers?.validation;
  $: validation = {
    npi: validationRaw?.npi ?? { enabled: false, onInvalid: 'pass' as const },
    mbi: validationRaw?.mbi ?? { enabled: false, onInvalid: 'pass' as const },
    ssn: validationRaw?.ssn ?? { enabled: false, onInvalid: 'pass' as const }
  };
  $: normalization = identifiers?.normalization || {
    ssnStripDashes: false,
    ssnRejectPatterns: [],
    phoneNormalize: false,
    phoneFormat: null
  };
  $: assigningAuthorities = identifiers?.assigningAuthorities || [];

  // Modal state
  let showAAModal = false;
  let editingAA: { code: string; system: string; name: string | null } | null = null;
  let aaCode = '';
  let aaSystem = '';
  let aaName = '';

  let aaModalEl: HTMLDivElement | null = null;
  let wasAAModalOpen = false;
  let aaFocusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  afterUpdate(() => {
    if (showAAModal && !wasAAModalOpen) {
      tick().then(() => {
        if (!aaModalEl) return;
        aaFocusCtl = createDialogFocusController(aaModalEl);
        aaFocusCtl.focusInitial();
      });
    }
    if (!showAAModal && wasAAModalOpen) {
      aaFocusCtl?.restoreFocus();
      aaFocusCtl = null;
    }
    wasAAModalOpen = showAAModal;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!showAAModal) return;
    if (e.key === 'Escape') {
      showAAModal = false;
      return;
    }
    if (e.key === 'Tab') {
      aaFocusCtl?.onKeydown(e);
    }
  }

  // Update validation setting
  function updateValidation(
    type: 'npi' | 'mbi' | 'ssn',
    field: 'enabled' | 'onInvalid',
    value: boolean | string
  ) {
    const newValidation = {
      ...validation,
      [type]: {
        ...validation[type],
        [field]: value
      }
    };

    profileStore.updateLocal({
      identifiers: {
        assigningAuthorities: assigningAuthorities.map((aa) => ({
          code: aa.code,
          system: aa.system,
          name: aa.name
        })),
        primaryIdPreference:
          identifiers?.primaryIdPreference?.map((p) => ({
            type: p.type,
            assignerContains: p.assignerContains,
            priority: p.priority
          })) || [],
        validation: newValidation,
        normalization: normalization
      }
    });
  }

  // Update normalization setting
  function updateNormalization(field: keyof typeof normalization, value: boolean | string | string[]) {
    profileStore.updateLocal({
      identifiers: {
        assigningAuthorities: assigningAuthorities.map((aa) => ({
          code: aa.code,
          system: aa.system,
          name: aa.name
        })),
        primaryIdPreference:
          identifiers?.primaryIdPreference?.map((p) => ({
            type: p.type,
            assignerContains: p.assignerContains,
            priority: p.priority
          })) || [],
        validation: validation,
        normalization: {
          ...normalization,
          [field]: value
        }
      }
    });
  }

  // SSN reject patterns
  $: ssnRejectText = normalization.ssnRejectPatterns?.join(', ') || '';

  function updateSSNRejectPatterns(text: string) {
    const patterns = text
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    updateNormalization('ssnRejectPatterns', patterns);
  }

  // Assigning Authority management
  function openAAModal(aa?: (typeof assigningAuthorities)[0]) {
    if (aa) {
      editingAA = aa;
      aaCode = aa.code;
      aaSystem = aa.system;
      aaName = aa.name || '';
    } else {
      editingAA = null;
      aaCode = '';
      aaSystem = '';
      aaName = '';
    }
    showAAModal = true;
  }

  function saveAA() {
    if (!aaCode.trim() || !aaSystem.trim()) return;

    let newAAs: typeof assigningAuthorities;
    if (editingAA) {
      newAAs = assigningAuthorities.map((aa) =>
        aa.code === editingAA?.code
          ? { code: aaCode.trim(), system: aaSystem.trim(), name: aaName.trim() || null }
          : aa
      );
    } else {
      newAAs = [
        ...assigningAuthorities,
        { code: aaCode.trim(), system: aaSystem.trim(), name: aaName.trim() || null }
      ];
    }

    profileStore.updateLocal({
      identifiers: {
        assigningAuthorities: newAAs.map((aa) => ({
          code: aa.code,
          system: aa.system,
          name: aa.name
        })),
        primaryIdPreference:
          identifiers?.primaryIdPreference?.map((p) => ({
            type: p.type,
            assignerContains: p.assignerContains,
            priority: p.priority
          })) || [],
        validation: validation,
        normalization: normalization
      }
    });

    showAAModal = false;
  }

  function deleteAA(code: string) {
    const newAAs = assigningAuthorities.filter((aa) => aa.code !== code);
    profileStore.updateLocal({
      identifiers: {
        assigningAuthorities: newAAs.map((aa) => ({
          code: aa.code,
          system: aa.system,
          name: aa.name
        })),
        primaryIdPreference:
          identifiers?.primaryIdPreference?.map((p) => ({
            type: p.type,
            assignerContains: p.assignerContains,
            priority: p.priority
          })) || [],
        validation: validation,
        normalization: normalization
      }
    });
  }

  const validationTypes: { key: 'npi' | 'mbi' | 'ssn'; label: string }[] = [
    { key: 'npi', label: 'NPI' },
    { key: 'mbi', label: 'MBI' },
    { key: 'ssn', label: 'SSN' }
  ];

  const onInvalidOptions: SelectOption[] = [
    { value: 'pass', label: 'Pass' },
    { value: 'warn', label: 'Warn' },
    { value: 'error', label: 'Error' }
  ];
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div class="editor">
  <Panel title="Identifier validation" titleTag="h3" flush>
    <Table label="Identifier validation" layout="fixed">
      {#snippet head()}
        <tr>
          <Th width="96px">Identifier</Th>
          <Th width="120px">Validate</Th>
          <Th>On invalid</Th>
        </tr>
      {/snippet}
      {#each validationTypes as type (type.key)}
        <Tr>
          <Td mono value={type.label} />
          <Td>
            <label class="check">
              <input
                type="checkbox"
                checked={validation[type.key].enabled}
                on:change={(e) =>
                  updateValidation(type.key, 'enabled', (e.target as HTMLInputElement).checked)}
              />
              Enabled
            </label>
          </Td>
          <Td>
            <div class="on-invalid">
              <Select
                aria-label="{type.label} on invalid"
                value={validation[type.key].onInvalid}
                options={onInvalidOptions}
                onchange={(e) => updateValidation(type.key, 'onInvalid', e.currentTarget.value)}
              />
            </div>
          </Td>
        </Tr>
      {/each}
    </Table>
  </Panel>

  <Panel title="Normalization" titleTag="h3">
    <div class="norm-options">
      <label class="option">
        <input
          type="checkbox"
          checked={normalization.ssnStripDashes}
          on:change={(e) =>
            updateNormalization('ssnStripDashes', (e.target as HTMLInputElement).checked)}
        />
        <span class="option-text">
          <span class="option-label">Strip dashes from SSN</span>
          <span class="option-desc">Convert <span class="text-mono">123-45-6789</span> to <span class="text-mono">123456789</span>.</span>
        </span>
      </label>

      <label class="option">
        <input
          type="checkbox"
          checked={normalization.phoneNormalize}
          on:change={(e) =>
            updateNormalization('phoneNormalize', (e.target as HTMLInputElement).checked)}
        />
        <span class="option-text">
          <span class="option-label">Normalize phone numbers</span>
          <span class="option-desc">Strip formatting characters from phone numbers.</span>
        </span>
      </label>
    </div>

    <Field label="SSN reject patterns" hint="Comma-separated. SSNs matching these patterns are rejected.">
      <Input
        mono
        value={ssnRejectText}
        oninput={(e) => updateSSNRejectPatterns(e.currentTarget.value)}
        placeholder="e.g. 000*, *0000, 123456789"
      />
    </Field>
  </Panel>

  {#if showAdvanced}
    <Panel title="Assigning authorities" titleTag="h3" flush class="span-all">
      {#snippet actions()}
        <Button variant="ghost" icon={Plus} onclick={() => openAAModal()}>Add</Button>
      {/snippet}

      {#if assigningAuthorities.length === 0}
        <EmptyState
          align="start"
          message="No assigning authorities. Add one to map a local authority code to an OID or URI."
        />
      {:else}
        <Table label="Assigning authorities" layout="fixed">
          {#snippet head()}
            <tr>
              <Th width="120px">Code</Th>
              <Th>System</Th>
              <Th width="28%">Name</Th>
              <Th width="72px"><span class="sr-only">Actions</span></Th>
            </tr>
          {/snippet}
          {#each assigningAuthorities as aa (aa.code)}
            <Tr>
              <Td mono truncate value={aa.code} />
              <Td mono truncate value={aa.system} />
              <Td truncate muted={!aa.name} value={aa.name || '—'} />
              <Td class="row-actions">
                <IconButton icon={Pencil} label="Edit" onclick={() => openAAModal(aa)} />
                <IconButton icon={Trash2} label="Delete" onclick={() => deleteAA(aa.code)} />
              </Td>
            </Tr>
          {/each}
        </Table>
      {/if}
    </Panel>
  {/if}
</div>

<!-- Assigning Authority Modal -->
{#if showAAModal}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showAAModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={aaModalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="assigning-authority-modal-title"
      tabindex="-1"
    >
      <h3 id="assigning-authority-modal-title" class="modal-title">
        {editingAA ? 'Edit assigning authority' : 'Add assigning authority'}
      </h3>
      <div class="modal-body">
        <Field label="Code" hint="The local identifier for this authority.">
          <Input mono bind:value={aaCode} placeholder="e.g. EPIC" disabled={!!editingAA} />
        </Field>
        <Field label="System (OID or URI)" hint="The standard system identifier.">
          <Input mono bind:value={aaSystem} placeholder="e.g. urn:oid:1.2.840.114350.1.13" />
        </Field>
        <Field label="Display name (optional)">
          <Input bind:value={aaName} placeholder="e.g. Epic Systems" />
        </Field>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showAAModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={saveAA}
          disabled={!aaCode.trim() || !aaSystem.trim()}
        >
          {editingAA ? 'Update' : 'Add'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .editor {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: var(--space-3);
    align-items: start;
  }

  .editor :global(.span-all) {
    grid-column: 1 / -1;
  }

  .editor :global(.row-actions) {
    text-align: right;
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
    cursor: pointer;
    user-select: none;
  }

  .check input,
  .option input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  .on-invalid {
    max-width: 160px;
  }

  .norm-options {
    display: grid;
    gap: var(--space-2);
    margin-bottom: var(--space-3);
  }

  .option {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    cursor: pointer;
  }

  .option input {
    margin-top: 2px;
  }

  .option-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .option-label {
    font-size: var(--text-ui);
    color: var(--color-text-primary);
  }

  .option-desc {
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: var(--space-4);
    z-index: var(--z-modal);
  }

  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: var(--modal-backdrop);
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    width: 100%;
    max-width: var(--modal-width-md);
    background: var(--color-bg-overlay);
    border: 1px solid var(--color-border-default);
    border-radius: var(--modal-radius);
    box-shadow: var(--shadow-xl);
    outline: none;
  }

  .modal-title {
    margin: 0;
    padding: var(--space-4) var(--space-4) 0;
    font-size: var(--text-title);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .modal-body {
    display: grid;
    gap: var(--space-3);
    padding: var(--space-3) var(--space-4) var(--space-4);
  }

  .modal-actions {
    display: flex;
    gap: var(--space-2);
    justify-content: flex-end;
    padding: var(--space-3) var(--space-4);
    border-top: 1px solid var(--color-border-subtle);
  }
</style>
