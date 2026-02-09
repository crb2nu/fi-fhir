<script lang="ts">
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import Button from '$lib/ui/Button.svelte';
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
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div class="editor">
  <div class="section">
    <h4 class="section-title">Identifier Validation</h4>
    <p class="section-desc">
      Configure validation rules for common healthcare identifiers.
    </p>

    <div class="validation-grid">
      <div class="validation-row">
        <div class="id-type mono">NPI</div>
        <label class="toggle">
          <input
            type="checkbox"
            checked={validation.npi.enabled}
            on:change={(e) =>
              updateValidation('npi', 'enabled', (e.target as HTMLInputElement).checked)}
          />
          <span>Enabled</span>
        </label>
        <label class="select-label">
          On invalid:
          <select
            class="select-small"
            value={validation.npi.onInvalid}
            on:change={(e) =>
              updateValidation('npi', 'onInvalid', (e.target as HTMLSelectElement).value)}
          >
            <option value="pass">Pass</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
        </label>
      </div>

      <div class="validation-row">
        <div class="id-type mono">MBI</div>
        <label class="toggle">
          <input
            type="checkbox"
            checked={validation.mbi.enabled}
            on:change={(e) =>
              updateValidation('mbi', 'enabled', (e.target as HTMLInputElement).checked)}
          />
          <span>Enabled</span>
        </label>
        <label class="select-label">
          On invalid:
          <select
            class="select-small"
            value={validation.mbi.onInvalid}
            on:change={(e) =>
              updateValidation('mbi', 'onInvalid', (e.target as HTMLSelectElement).value)}
          >
            <option value="pass">Pass</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
        </label>
      </div>

      <div class="validation-row">
        <div class="id-type mono">SSN</div>
        <label class="toggle">
          <input
            type="checkbox"
            checked={validation.ssn.enabled}
            on:change={(e) =>
              updateValidation('ssn', 'enabled', (e.target as HTMLInputElement).checked)}
          />
          <span>Enabled</span>
        </label>
        <label class="select-label">
          On invalid:
          <select
            class="select-small"
            value={validation.ssn.onInvalid}
            on:change={(e) =>
              updateValidation('ssn', 'onInvalid', (e.target as HTMLSelectElement).value)}
          >
            <option value="pass">Pass</option>
            <option value="warn">Warn</option>
            <option value="error">Error</option>
          </select>
        </label>
      </div>
    </div>
  </div>

  <div class="section">
    <h4 class="section-title">Normalization</h4>
    <p class="section-desc">
      Configure how identifiers are cleaned and normalized.
    </p>

    <div class="norm-options">
      <label class="option-toggle">
        <input
          type="checkbox"
          checked={normalization.ssnStripDashes}
          on:change={(e) =>
            updateNormalization('ssnStripDashes', (e.target as HTMLInputElement).checked)}
        />
        <div class="option-content">
          <span class="option-label">Strip dashes from SSN</span>
          <span class="option-desc">Convert "123-45-6789" to "123456789"</span>
        </div>
      </label>

      <label class="option-toggle">
        <input
          type="checkbox"
          checked={normalization.phoneNormalize}
          on:change={(e) =>
            updateNormalization('phoneNormalize', (e.target as HTMLInputElement).checked)}
        />
        <div class="option-content">
          <span class="option-label">Normalize phone numbers</span>
          <span class="option-desc">Strip formatting characters from phone numbers</span>
        </div>
      </label>
    </div>

    <label class="label">
      SSN reject patterns (comma-separated)
      <input
        class="input mono"
        type="text"
        value={ssnRejectText}
        on:input={(e) => updateSSNRejectPatterns((e.target as HTMLInputElement).value)}
        placeholder="e.g., 000*, *0000, 123456789"
      />
      <span class="hint">SSNs matching these patterns will be rejected</span>
    </label>
  </div>

  {#if showAdvanced}
    <div class="section">
      <div class="section-header">
        <h4 class="section-title">Assigning Authorities</h4>
        <Button variant="secondary" on:click={() => openAAModal()}>+ Add</Button>
      </div>
      <p class="section-desc">
        Map local assigning authority codes to standard OID systems.
      </p>

      {#if assigningAuthorities.length === 0}
        <div class="empty">No assigning authorities configured.</div>
      {:else}
        <div class="aa-list">
          {#each assigningAuthorities as aa (aa.code)}
            <div class="aa-item">
              <div class="aa-code mono">{aa.code}</div>
              <div class="aa-arrow">-></div>
              <div class="aa-system">{aa.system}</div>
              {#if aa.name}
                <div class="aa-name">({aa.name})</div>
              {/if}
              <div class="aa-actions">
                <button class="icon-btn" on:click={() => openAAModal(aa)} title="Edit">
                  Edit
                </button>
                <button class="icon-btn danger" on:click={() => deleteAA(aa.code)} title="Delete">
                  Delete
                </button>
              </div>
            </div>
          {/each}
        </div>
      {/if}
    </div>
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
        {editingAA ? 'Edit Assigning Authority' : 'Add Assigning Authority'}
      </h3>
      <div class="modal-body">
        <label class="label">
          Code
          <input
            class="input mono"
            type="text"
            bind:value={aaCode}
            placeholder="e.g., EPIC"
            disabled={!!editingAA}
          />
          <span class="hint">The local identifier for this authority</span>
        </label>
        <label class="label">
          System (OID/URI)
          <input
            class="input mono"
            type="text"
            bind:value={aaSystem}
            placeholder="e.g., urn:oid:1.2.840.114350.1.13..."
          />
          <span class="hint">The standard system identifier</span>
        </label>
        <label class="label">
          Display Name (optional)
          <input class="input" type="text" bind:value={aaName} placeholder="e.g., Epic Systems" />
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showAAModal = false)}>Cancel</Button>
        <Button on:click={saveAA} disabled={!aaCode.trim() || !aaSystem.trim()}>
          {editingAA ? 'Update' : 'Add'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .editor {
    display: grid;
    gap: 20px;
  }

  .section {
    padding: 16px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .section-header {
    display: flex;
    justify-content: space-between;
    align-items: center;
    margin-bottom: 8px;
  }

  .section-title {
    margin: 0 0 8px;
    font-size: 0.95rem;
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .section-header .section-title {
    margin: 0;
  }

  .section-desc {
    margin: 0 0 14px;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.65);
    line-height: 1.4;
  }

  .validation-grid {
    display: grid;
    gap: 10px;
  }

  .validation-row {
    display: grid;
    grid-template-columns: 60px 1fr 1fr;
    gap: 14px;
    align-items: center;
    padding: 12px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.015);
  }

  @media (max-width: 500px) {
    .validation-row {
      grid-template-columns: 1fr;
      gap: 10px;
    }
  }

  .id-type {
    font-weight: 800;
    color: rgba(229, 231, 235, 0.85);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .toggle {
    display: flex;
    gap: 8px;
    align-items: center;
    color: rgba(229, 231, 235, 0.82);
    font-weight: 600;
    cursor: pointer;
  }

  .select-label {
    display: flex;
    gap: 8px;
    align-items: center;
    color: rgba(229, 231, 235, 0.75);
    font-size: 0.9rem;
  }

  .select-small {
    padding: 6px 10px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    font-size: 0.9rem;
  }

  .select-small:focus {
    border-color: rgba(59, 130, 246, 0.45);
  }

  .norm-options {
    display: grid;
    gap: 10px;
    margin-bottom: 14px;
  }

  .option-toggle {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 12px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.015);
    cursor: pointer;
  }

  .option-toggle:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .option-toggle input {
    margin-top: 2px;
  }

  .option-content {
    display: grid;
    gap: 2px;
  }

  .option-label {
    color: rgba(229, 231, 235, 0.9);
    font-weight: 650;
  }

  .option-desc {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.55);
    line-height: 1.35;
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .input {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .input:disabled {
    opacity: 0.6;
  }

  .hint {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.55);
  }

  .empty {
    padding: 16px;
    text-align: center;
    color: rgba(229, 231, 235, 0.5);
    font-style: italic;
  }

  .aa-list {
    display: grid;
    gap: 8px;
  }

  .aa-item {
    display: flex;
    gap: 10px;
    align-items: center;
    padding: 10px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.015);
    flex-wrap: wrap;
  }

  .aa-code {
    font-weight: 700;
    color: rgba(59, 130, 246, 0.95);
  }

  .aa-arrow {
    color: rgba(229, 231, 235, 0.5);
  }

  .aa-system {
    flex: 1;
    font-size: 0.9rem;
    color: rgba(229, 231, 235, 0.8);
    word-break: break-all;
  }

  .aa-name {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.55);
  }

  .aa-actions {
    display: flex;
    gap: 6px;
  }

  .icon-btn {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: transparent;
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .icon-btn:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .icon-btn.danger {
    color: rgba(239, 68, 68, 0.8);
  }

  .icon-btn.danger:hover {
    background: rgba(239, 68, 68, 0.1);
  }

  .modal-overlay {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    z-index: 1000;
  }

  .modal-backdrop {
    position: absolute;
    inset: 0;
    border: 0;
    padding: 0;
    background: rgba(0, 0, 0, 0.6);
    cursor: default;
  }

  .modal {
    position: relative;
    z-index: 1;
    background: #1f2937;
    border: 1px solid rgba(255, 255, 255, 0.1);
    border-radius: 16px;
    padding: 24px;
    min-width: 360px;
    max-width: 520px;
  }

  .modal-title {
    margin: 0 0 16px;
    font-size: 1.1rem;
    font-weight: 800;
    color: #f3f4f6;
  }

  .modal-body {
    display: grid;
    gap: 14px;
    margin-bottom: 20px;
  }

  .modal-actions {
    display: flex;
    gap: 10px;
    justify-content: flex-end;
  }
</style>
