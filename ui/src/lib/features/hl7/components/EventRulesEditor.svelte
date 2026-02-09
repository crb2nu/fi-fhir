<script lang="ts">
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import Button from '$lib/ui/Button.svelte';
  import { afterUpdate, tick } from 'svelte';
  import { createDialogFocusController } from '$lib/domain/a11yDialog';

  $: hl7v2 = $selectedProfile?.hl7v2;
  $: eventClassifications = hl7v2?.eventClassifications || [];

  // Common message types
  const commonMessageTypes = [
    'ADT^A01',
    'ADT^A02',
    'ADT^A03',
    'ADT^A04',
    'ADT^A08',
    'ADT^A11',
    'ADT^A13',
    'ORM^O01',
    'ORU^R01',
    'SIU^S12',
    'DFT^P03'
  ];

  // Common event types
  const commonEventTypes = [
    'inpatient_admit',
    'outpatient_visit',
    'emergency_visit',
    'patient_discharge',
    'patient_transfer',
    'patient_update',
    'lab_order',
    'lab_result',
    'appointment_scheduled',
    'procedure_order'
  ];

  // Modal state
  let showModal = false;
  let editingIndex: number | null = null;
  let ruleMessageType = '';
  let ruleCondition = '';
  let ruleEventType = '';
  let rulePriority = 0;

  let modalEl: HTMLDivElement | null = null;
  let wasModalOpen = false;
  let focusCtl: ReturnType<typeof createDialogFocusController> | null = null;

  afterUpdate(() => {
    if (showModal && !wasModalOpen) {
      tick().then(() => {
        if (!modalEl) return;
        focusCtl = createDialogFocusController(modalEl);
        focusCtl.focusInitial();
      });
    }
    if (!showModal && wasModalOpen) {
      focusCtl?.restoreFocus();
      focusCtl = null;
    }
    wasModalOpen = showModal;
  });

  function handleWindowKeydown(e: KeyboardEvent) {
    if (!showModal) return;
    if (e.key === 'Escape') {
      showModal = false;
      return;
    }
    if (e.key === 'Tab') {
      focusCtl?.onKeydown(e);
    }
  }

  function openModal(index?: number) {
    if (index !== undefined && eventClassifications[index]) {
      const rule = eventClassifications[index];
      editingIndex = index;
      ruleMessageType = rule.messageType;
      ruleCondition = rule.condition || '';
      ruleEventType = rule.eventType;
      rulePriority = rule.priority;
    } else {
      editingIndex = null;
      ruleMessageType = '';
      ruleCondition = '';
      ruleEventType = '';
      rulePriority = eventClassifications.length;
    }
    showModal = true;
  }

  function saveRule() {
    if (!ruleMessageType.trim() || !ruleEventType.trim()) return;

    const newRule = {
      messageType: ruleMessageType.trim(),
      condition: ruleCondition.trim() || null,
      eventType: ruleEventType.trim(),
      priority: rulePriority
    };

    let newRules: typeof eventClassifications;
    if (editingIndex !== null) {
      newRules = eventClassifications.map((r, i) =>
        i === editingIndex ? newRule : r
      );
    } else {
      newRules = [...eventClassifications, newRule];
    }

    // Sort by priority
    newRules.sort((a, b) => a.priority - b.priority);

    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: hl7v2?.tolerance || {
          missingSegments: [],
          nteAnywhere: false,
          extraComponents: false,
          unknownSegments: false,
          nonStandardDelimiters: false
        },
        eventClassifications: newRules.map((r) => ({
          messageType: r.messageType,
          condition: r.condition,
          eventType: r.eventType,
          priority: r.priority
        }))
      }
    });

    showModal = false;
  }

  function deleteRule(index: number) {
    const newRules = eventClassifications.filter((_, i) => i !== index);

    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: hl7v2?.tolerance || {
          missingSegments: [],
          nteAnywhere: false,
          extraComponents: false,
          unknownSegments: false,
          nonStandardDelimiters: false
        },
        eventClassifications: newRules.map((r) => ({
          messageType: r.messageType,
          condition: r.condition,
          eventType: r.eventType,
          priority: r.priority
        }))
      }
    });
  }

  function moveRule(index: number, direction: 'up' | 'down') {
    const targetIndex = direction === 'up' ? index - 1 : index + 1;
    if (targetIndex < 0 || targetIndex >= eventClassifications.length) return;

    const newRules = [...eventClassifications];
    const temp = newRules[index]!;
    newRules[index] = newRules[targetIndex]!;
    newRules[targetIndex] = temp;

    // Update priorities to match new order
    newRules.forEach((r, i) => {
      r.priority = i;
    });

    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: hl7v2?.tolerance || {
          missingSegments: [],
          nteAnywhere: false,
          extraComponents: false,
          unknownSegments: false,
          nonStandardDelimiters: false
        },
        eventClassifications: newRules.map((r) => ({
          messageType: r.messageType,
          condition: r.condition,
          eventType: r.eventType,
          priority: r.priority
        }))
      }
    });
  }
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div class="editor">
  <div class="header">
    <div>
      <h4 class="title">Event Classification Rules</h4>
      <p class="desc">
        Map HL7 message types to semantic event types. Rules are evaluated in priority order;
        first matching rule wins.
      </p>
    </div>
    <Button variant="secondary" on:click={() => openModal()}>+ Add Rule</Button>
  </div>

  {#if eventClassifications.length === 0}
    <div class="empty">
      <p>No event classification rules configured.</p>
      <p class="empty-hint">
        Add rules to map HL7 message types (like ADT^A01) to semantic event types (like
        inpatient_admit).
      </p>
    </div>
  {:else}
    <div class="rules-list">
      {#each eventClassifications as rule, index (index)}
        <div class="rule-item">
          <div class="rule-order">
            <button
              class="order-btn"
              on:click={() => moveRule(index, 'up')}
              disabled={index === 0}
              title="Move up"
            >
              Up
            </button>
            <span class="priority">#{rule.priority}</span>
            <button
              class="order-btn"
              on:click={() => moveRule(index, 'down')}
              disabled={index === eventClassifications.length - 1}
              title="Move down"
            >
              Dn
            </button>
          </div>
          <div class="rule-content">
            <span class="rule-msg mono">{rule.messageType}</span>
            {#if rule.condition}
              <span class="rule-cond">+ {rule.condition}</span>
            {/if}
            <span class="rule-arrow">-></span>
            <span class="rule-event">{rule.eventType}</span>
          </div>
          <div class="rule-actions">
            <button class="action-btn" on:click={() => openModal(index)}>Edit</button>
            <button class="action-btn danger" on:click={() => deleteRule(index)}>Delete</button>
          </div>
        </div>
      {/each}
    </div>
  {/if}
</div>

<!-- Rule Editor Modal -->
{#if showModal}
  <div class="modal-overlay">
    <button
      type="button"
      class="modal-backdrop"
      tabindex="-1"
      aria-label="Close dialog"
      on:click={() => (showModal = false)}
    ></button>
    <div
      class="modal"
      bind:this={modalEl}
      role="dialog"
      aria-modal="true"
      aria-labelledby="rule-editor-modal-title"
      tabindex="-1"
    >
      <h3 id="rule-editor-modal-title" class="modal-title">
        {editingIndex !== null ? 'Edit Rule' : 'Add Rule'}
      </h3>
      <div class="modal-body">
        <label class="label">
          Message Type
          <div class="input-with-suggestions">
            <input
              class="input mono"
              type="text"
              bind:value={ruleMessageType}
              placeholder="e.g., ADT^A01"
            />
            <div class="suggestions">
              {#each commonMessageTypes.filter( (t) => t.toLowerCase().includes(ruleMessageType.toLowerCase()) && t !== ruleMessageType ) as suggestion (suggestion)}
                <button
                  class="suggestion"
                  on:click={() => (ruleMessageType = suggestion)}
                >
                  {suggestion}
                </button>
              {/each}
            </div>
          </div>
          <span class="hint">HL7 message type and trigger event (e.g., ADT^A01)</span>
        </label>

        <label class="label">
          Condition (optional)
          <input
            class="input mono"
            type="text"
            bind:value={ruleCondition}
            placeholder="e.g., PV1.2 == 'I'"
          />
          <span class="hint">Additional condition using HL7 path expressions</span>
        </label>

        <label class="label">
          Event Type
          <div class="input-with-suggestions">
            <input
              class="input"
              type="text"
              bind:value={ruleEventType}
              placeholder="e.g., inpatient_admit"
            />
            <div class="suggestions">
              {#each commonEventTypes.filter( (t) => t.toLowerCase().includes(ruleEventType.toLowerCase()) && t !== ruleEventType ) as suggestion (suggestion)}
                <button
                  class="suggestion"
                  on:click={() => (ruleEventType = suggestion)}
                >
                  {suggestion}
                </button>
              {/each}
            </div>
          </div>
          <span class="hint">Semantic event type to assign when this rule matches</span>
        </label>

        <label class="label">
          Priority
          <input
            class="input"
            type="number"
            bind:value={rulePriority}
            min="0"
          />
          <span class="hint">Lower numbers are evaluated first</span>
        </label>
      </div>
      <div class="modal-actions">
        <Button variant="secondary" on:click={() => (showModal = false)}>Cancel</Button>
        <Button on:click={saveRule} disabled={!ruleMessageType.trim() || !ruleEventType.trim()}>
          {editingIndex !== null ? 'Update' : 'Add'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .editor {
    display: grid;
    gap: 16px;
  }

  .header {
    display: flex;
    justify-content: space-between;
    align-items: flex-start;
    gap: 16px;
    flex-wrap: wrap;
  }

  .title {
    margin: 0 0 4px;
    font-size: 0.95rem;
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .desc {
    margin: 0;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.65);
    line-height: 1.4;
    max-width: 480px;
  }

  .empty {
    padding: 24px;
    text-align: center;
    border-radius: 12px;
    border: 1px dashed rgba(255, 255, 255, 0.15);
    color: rgba(229, 231, 235, 0.7);
  }

  .empty p {
    margin: 0;
  }

  .empty-hint {
    margin-top: 8px !important;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.5);
  }

  .rules-list {
    display: grid;
    gap: 8px;
  }

  .rule-item {
    display: grid;
    grid-template-columns: auto 1fr auto;
    gap: 12px;
    align-items: center;
    padding: 12px 14px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  @media (max-width: 600px) {
    .rule-item {
      grid-template-columns: 1fr;
      gap: 10px;
    }
  }

  .rule-order {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 2px;
  }

  .order-btn {
    padding: 2px 6px;
    border: none;
    background: transparent;
    color: rgba(229, 231, 235, 0.5);
    font-size: 0.75rem;
    cursor: pointer;
  }

  .order-btn:hover:not(:disabled) {
    color: rgba(229, 231, 235, 0.85);
  }

  .order-btn:disabled {
    opacity: 0.3;
    cursor: not-allowed;
  }

  .priority {
    font-size: 0.75rem;
    color: rgba(229, 231, 235, 0.4);
  }

  .rule-content {
    display: flex;
    align-items: center;
    gap: 8px;
    flex-wrap: wrap;
  }

  .rule-msg {
    font-weight: 700;
    color: rgba(59, 130, 246, 0.95);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .rule-cond {
    font-size: 0.85rem;
    color: rgba(245, 158, 11, 0.85);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
  }

  .rule-arrow {
    color: rgba(229, 231, 235, 0.4);
  }

  .rule-event {
    font-weight: 600;
    color: rgba(34, 197, 94, 0.9);
  }

  .rule-actions {
    display: flex;
    gap: 6px;
  }

  .action-btn {
    padding: 4px 10px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: transparent;
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .action-btn:hover {
    background: rgba(255, 255, 255, 0.06);
  }

  .action-btn.danger {
    color: rgba(239, 68, 68, 0.8);
  }

  .action-btn.danger:hover {
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
    min-width: 400px;
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

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .input-with-suggestions {
    position: relative;
  }

  .input {
    width: 100%;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    box-sizing: border-box;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .suggestions {
    display: flex;
    flex-wrap: wrap;
    gap: 4px;
    margin-top: 6px;
  }

  .suggestion {
    padding: 4px 8px;
    border-radius: 6px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.75);
    font-size: 0.8rem;
    cursor: pointer;
  }

  .suggestion:hover {
    background: rgba(255, 255, 255, 0.08);
    color: rgba(229, 231, 235, 0.9);
  }

  .hint {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.55);
  }
</style>
