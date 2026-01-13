<script lang="ts">
  import type { HL7Message, HL7Segment } from '$lib/domain/hl7v2';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';
  import { afterUpdate } from 'svelte';

  export let message: HL7Message;
  export let selected: HL7PathLocation | null = null;

  let root: HTMLElement | null = null;
  let lastKey = '';

  function selectedSegmentId(): string | null {
    return selected ? selected.segmentId : null;
  }

  function isSelected(seg: HL7Segment) {
    const segId = selectedSegmentId();
    return segId !== null && seg.id === segId;
  }

  function fieldSelected(segId: string, fieldNumber: number): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selected.kind === 'segment') return false;
    return selected.field === fieldNumber;
  }

  function componentSelected(segId: string, fieldNumber: number, componentNumber: number): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selected.kind === 'component') {
      return selected.field === fieldNumber && selected.component === componentNumber;
    }
    if (selected.kind === 'repetition_component') {
      return selected.field === fieldNumber && selected.component === componentNumber;
    }
    return false;
  }

  function selectionKey(): string {
    if (!selected) return '';
    switch (selected.kind) {
      case 'segment':
        return `${selected.segmentId}`;
      case 'field':
        return `${selected.segmentId}-${selected.field}`;
      case 'component':
        return `${selected.segmentId}-${selected.field}.${selected.component}`;
      case 'repetition':
        return `${selected.segmentId}-${selected.field}[${selected.repetition}]`;
      case 'repetition_component':
        return `${selected.segmentId}-${selected.field}[${selected.repetition}].${selected.component}`;
    }
  }

  afterUpdate(() => {
    const k = selectionKey();
    if (!k || k === lastKey) return;
    lastKey = k;
    const el = root?.querySelector<HTMLElement>(`[data-hl7-key="${CSS.escape(k)}"]`) ?? null;
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  });
</script>

<div class="wrap" bind:this={root}>
  <div class="meta">
    <div class="pill">segments: {message.segments.length}</div>
    <div class="pill mono">
      delims: field={message.delimiters.field} comp={message.delimiters.component} rep={message.delimiters.repetition}
    </div>
  </div>

  <div class="segments">
    {#each message.segments as seg, idx (idx)}
      <details
        class="seg"
        open={idx < 3 || isSelected(seg)}
        data-selected={isSelected(seg)}
        data-hl7-key={seg.id}
      >
        <summary class="summary">
          <span class="id mono">{seg.id}</span>
          <span class="hint">{seg.fields.length} fields</span>
        </summary>

        <div class="fields">
          {#each seg.fields as f (f.number)}
            <div
              class="field"
              class:selected={fieldSelected(seg.id, f.number)}
              data-hl7-key={seg.id + '-' + f.number}
            >
              <div class="field-head">
                <span class="mono label">{seg.id}-{f.number}</span>
                <span class="mono value">{f.raw || '∅'}</span>
              </div>

              {#if f.components.length > 1}
                <div class="components">
                  {#each f.components as c, i (i)}
                    <div
                      class="component"
                      class:selected={componentSelected(seg.id, f.number, i + 1)}
                      data-hl7-key={seg.id + '-' + f.number + '.' + (i + 1)}
                    >
                      <span class="mono comp-label">{seg.id}-{f.number}.{i + 1}</span>
                      <span class="mono comp-value">{c || '∅'}</span>
                    </div>
                  {/each}
                </div>
              {/if}
            </div>
          {/each}
        </div>
      </details>
    {/each}
  </div>
</div>

<style>
  .wrap {
    display: grid;
    gap: 12px;
  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }

  .pill {
    padding: 4px 10px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.86);
    font-weight: 650;
    font-size: 0.85rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .segments {
    display: grid;
    gap: 10px;
  }

  .seg {
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    overflow: hidden;
  }

  .seg[data-selected='true'] {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.12);
  }

  .summary {
    padding: 10px 12px;
    cursor: pointer;
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
    color: rgba(243, 244, 246, 0.95);
    font-weight: 800;
  }

  .hint {
    color: rgba(229, 231, 235, 0.7);
    font-weight: 650;
    font-size: 0.85rem;
  }

  .fields {
    padding: 10px 12px 12px;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
    display: grid;
    gap: 10px;
  }

  .field {
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
    padding: 10px;
  }

  .field.selected {
    border-color: rgba(59, 130, 246, 0.5);
    background: rgba(59, 130, 246, 0.12);
  }

  .field-head {
    display: grid;
    gap: 6px;
  }

  .label {
    color: rgba(229, 231, 235, 0.9);
    font-weight: 800;
  }

  .value {
    color: rgba(229, 231, 235, 0.82);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .components {
    margin-top: 10px;
    display: grid;
    gap: 6px;
    padding-top: 10px;
    border-top: 1px solid rgba(255, 255, 255, 0.08);
  }

  .component {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    gap: 10px;
  }

  .component.selected {
    border-radius: 8px;
    padding: 6px 8px;
    background: rgba(59, 130, 246, 0.12);
    border: 1px solid rgba(59, 130, 246, 0.22);
  }

  .comp-label {
    color: rgba(229, 231, 235, 0.75);
    font-weight: 700;
    font-size: 0.85rem;
  }

  .comp-value {
    color: rgba(229, 231, 235, 0.8);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    max-width: 70%;
  }
</style>
