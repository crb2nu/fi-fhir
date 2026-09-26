<script lang="ts">
  import ArrowDown from '@lucide/svelte/icons/arrow-down';
  import ArrowRight from '@lucide/svelte/icons/arrow-right';
  import ArrowUp from '@lucide/svelte/icons/arrow-up';
  import Pencil from '@lucide/svelte/icons/pencil';
  import Plus from '@lucide/svelte/icons/plus';
  import Trash2 from '@lucide/svelte/icons/trash-2';
  import {
    Button,
    EmptyState,
    Field,
    Icon,
    IconButton,
    Input,
    Panel,
    Table,
    Td,
    Th,
    Tr
  } from '$lib/ui/primitives';
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
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

  $: messageSuggestions = commonMessageTypes.filter(
    (t) => t.toLowerCase().includes(ruleMessageType.toLowerCase()) && t !== ruleMessageType
  );
  $: eventSuggestions = commonEventTypes.filter(
    (t) => t.toLowerCase().includes(ruleEventType.toLowerCase()) && t !== ruleEventType
  );
</script>

<svelte:window on:keydown={handleWindowKeydown} />

<div class="rules">
  <Panel title="Event classification rules" titleTag="h3" flush>
    {#snippet actions()}
      <Button variant="ghost" icon={Plus} onclick={() => openModal()}>Add rule</Button>
    {/snippet}

    {#if eventClassifications.length === 0}
      <EmptyState
        align="start"
        message="No event classification rules. Add one to map an HL7 message type such as ADT^A01 to a semantic event type."
      />
    {:else}
      <Table label="Event classification rules" layout="fixed">
        {#snippet head()}
          <tr>
            <Th width="76px" numeric>Priority</Th>
            <Th width="120px">Message type</Th>
            <Th>Condition</Th>
            <Th width="30%">Event type</Th>
            <Th width="128px"><span class="sr-only">Actions</span></Th>
          </tr>
        {/snippet}
        {#each eventClassifications as rule, index (index)}
          <Tr>
            <Td numeric value={rule.priority} />
            <Td mono truncate value={rule.messageType} />
            <Td mono truncate muted value={rule.condition || '—'} />
            <Td mono truncate>
              <span class="event-type">
                <Icon icon={ArrowRight} size={12} class="event-arrow" />
                {rule.eventType}
              </span>
            </Td>
            <Td class="row-actions">
              <IconButton
                icon={ArrowUp}
                label="Move up"
                onclick={() => moveRule(index, 'up')}
                disabled={index === 0}
              />
              <IconButton
                icon={ArrowDown}
                label="Move down"
                onclick={() => moveRule(index, 'down')}
                disabled={index === eventClassifications.length - 1}
              />
              <IconButton icon={Pencil} label="Edit rule" onclick={() => openModal(index)} />
              <IconButton icon={Trash2} label="Delete rule" onclick={() => deleteRule(index)} />
            </Td>
          </Tr>
        {/each}
      </Table>
      <p class="foot">Evaluated in priority order; the first matching rule wins.</p>
    {/if}
  </Panel>
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
        {editingIndex !== null ? 'Edit rule' : 'Add rule'}
      </h3>
      <div class="modal-body">
        <Field label="Message type" hint="HL7 message type and trigger event, e.g. ADT^A01.">
          <Input mono bind:value={ruleMessageType} placeholder="e.g. ADT^A01" />
        </Field>
        {#if messageSuggestions.length > 0}
          <div class="suggestions" role="group" aria-label="Message type suggestions">
            {#each messageSuggestions as suggestion (suggestion)}
              <button type="button" class="suggestion" on:click={() => (ruleMessageType = suggestion)}>
                {suggestion}
              </button>
            {/each}
          </div>
        {/if}

        <Field label="Condition (optional)" hint="Additional condition as an HL7 path expression.">
          <Input mono bind:value={ruleCondition} placeholder="e.g. PV1.2 == 'I'" />
        </Field>

        <Field label="Event type" hint="Semantic event type assigned when this rule matches.">
          <Input mono bind:value={ruleEventType} placeholder="e.g. inpatient_admit" />
        </Field>
        {#if eventSuggestions.length > 0}
          <div class="suggestions" role="group" aria-label="Event type suggestions">
            {#each eventSuggestions as suggestion (suggestion)}
              <button type="button" class="suggestion" on:click={() => (ruleEventType = suggestion)}>
                {suggestion}
              </button>
            {/each}
          </div>
        {/if}

        <div class="priority">
          <Field label="Priority" hint="Lower numbers are evaluated first.">
            <Input type="number" mono bind:value={rulePriority} min="0" />
          </Field>
        </div>
      </div>
      <div class="modal-actions">
        <Button size="md" onclick={() => (showModal = false)}>Cancel</Button>
        <Button
          variant="primary"
          size="md"
          onclick={saveRule}
          disabled={!ruleMessageType.trim() || !ruleEventType.trim()}
        >
          {editingIndex !== null ? 'Update' : 'Add'}
        </Button>
      </div>
    </div>
  </div>
{/if}

<style>
  .event-type {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
  }

  .event-type :global(.event-arrow) {
    color: var(--color-text-muted);
  }

  .rules :global(.row-actions) {
    text-align: right;
    white-space: nowrap;
  }

  .foot {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .suggestions {
    display: flex;
    flex-wrap: wrap;
    gap: var(--space-1);
    margin-top: calc(-1 * var(--space-2));
  }

  .suggestion {
    height: 22px;
    padding: 0 6px;
    border: 1px solid var(--color-border-default);
    border-radius: var(--radius-sm);
    background: transparent;
    color: var(--color-text-secondary);
    font-family: var(--font-mono);
    font-size: var(--text-xs);
    cursor: pointer;
    transition: var(--transition-colors);
  }

  .suggestion:hover {
    background: var(--color-bg-hover);
    color: var(--color-text-primary);
  }

  .suggestion:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 1px;
  }

  .priority {
    width: 50%;
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
