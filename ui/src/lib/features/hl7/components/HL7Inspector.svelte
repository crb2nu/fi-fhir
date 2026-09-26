<script lang="ts">
  import type { HL7Message, HL7Segment } from '$lib/domain/hl7v2';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';
  import { getHL7Value } from '$lib/domain/hl7Access';
  import { browser } from '$app/environment';
  import { afterUpdate, createEventDispatcher } from 'svelte';
  import { Badge, Button, Icon, Input } from '$lib/ui/primitives';
  import ChevronRight from '@lucide/svelte/icons/chevron-right';
  import Copy from '@lucide/svelte/icons/copy';
  import Search from '@lucide/svelte/icons/search';

  export let message: HL7Message;
  export let selected: HL7PathLocation | null = null;

  const dispatch = createEventDispatcher<{
    selectPath: { path: string };
  }>();

  let root: HTMLElement | null = null;
  let lastKey = '';
  let filter = '';

  function selectedSegmentOccurrence(): number {
    return selected?.segmentOccurrence ?? 0;
  }

  // The markup passes `selected` explicitly: in legacy mode a call only
  // re-renders when a variable written in the markup expression changes.
  function isSelected(seg: HL7Segment, loc: HL7PathLocation | null = selected) {
    return loc !== null && seg.id === loc.segmentId && seg.occurrence === (loc.segmentOccurrence ?? 0);
  }

  function fieldSelected(segId: string, segOccurrence: number, fieldNumber: number): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selectedSegmentOccurrence() !== segOccurrence) return false;
    if (selected.kind === 'segment') return false;
    return selected.field === fieldNumber;
  }

  function componentSelected(
    segId: string,
    segOccurrence: number,
    fieldNumber: number,
    componentNumber: number
  ): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selectedSegmentOccurrence() !== segOccurrence) return false;
    if (selected.kind === 'component') {
      return selected.field === fieldNumber && selected.component === componentNumber;
    }
    if (selected.kind === 'repetition_component') {
      return selected.field === fieldNumber && selected.component === componentNumber;
    }
    return false;
  }

  function repetitionSelected(
    segId: string,
    segOccurrence: number,
    fieldNumber: number,
    repetitionIndex: number
  ): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selectedSegmentOccurrence() !== segOccurrence) return false;
    if (selected.kind !== 'repetition' && selected.kind !== 'repetition_component') return false;
    return selected.field === fieldNumber && selected.repetition === repetitionIndex;
  }

  function repetitionComponentSelected(
    segId: string,
    segOccurrence: number,
    fieldNumber: number,
    repetitionIndex: number,
    componentNumber: number
  ): boolean {
    if (!selected) return false;
    if (selected.segmentId !== segId) return false;
    if (selectedSegmentOccurrence() !== segOccurrence) return false;
    if (selected.kind !== 'repetition_component') return false;
    return (
      selected.field === fieldNumber &&
      selected.repetition === repetitionIndex &&
      selected.component === componentNumber
    );
  }

  function selectionKey(loc: HL7PathLocation | null = selected): string {
    if (!loc) return '';
    const segOcc = loc.segmentOccurrence ?? 0;
    switch (loc.kind) {
      case 'segment':
        return `${loc.segmentId}[${segOcc}]`;
      case 'field':
        return `${loc.segmentId}[${segOcc}]-${loc.field}`;
      case 'component':
        return `${loc.segmentId}[${segOcc}]-${loc.field}.${loc.component}`;
      case 'repetition':
        return `${loc.segmentId}[${segOcc}]-${loc.field}[${loc.repetition}]`;
      case 'repetition_component':
        return `${loc.segmentId}[${segOcc}]-${loc.field}[${loc.repetition}].${loc.component}`;
    }
  }

  afterUpdate(() => {
    const k = selectionKey();
    if (!k || k === lastKey) return;
    lastKey = k;
    const el = root?.querySelector<HTMLElement>(`[data-hl7-key="${CSS.escape(k)}"]`) ?? null;
    el?.scrollIntoView({ block: 'nearest', behavior: 'smooth' });
  });

  $: selectedValue = getHL7Value(message, selected);

  $: filteredSegments = filter.trim()
    ? message.segments.filter((s) => {
        const q = filter.trim().toLowerCase();
        return s.id.toLowerCase().includes(q) || s.raw.toLowerCase().includes(q);
      })
    : message.segments;

  async function copyText(text: string): Promise<void> {
    if (!browser) return;
    if (!text) return;
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  }

  function handleSegmentClick(seg: HL7Segment, event: MouseEvent) {
    if ((event.target as HTMLElement).tagName === 'SUMMARY') {
      const path = `${seg.id}[${seg.occurrence}]`;
      dispatch('selectPath', { path });
    }
  }

  function handleFieldClick(seg: HL7Segment, fieldNumber: number, event: MouseEvent) {
    event.stopPropagation();
    const path = `${seg.id}[${seg.occurrence}]-${fieldNumber}`;
    dispatch('selectPath', { path });
  }

  function handleComponentClick(seg: HL7Segment, fieldNumber: number, componentNumber: number, event: MouseEvent) {
    event.stopPropagation();
    const path = `${seg.id}[${seg.occurrence}]-${fieldNumber}.${componentNumber}`;
    dispatch('selectPath', { path });
  }
</script>

<div class="inspector" bind:this={root}>
  <div class="filters">
    <div class="filter-search">
      <Icon icon={Search} size={14} class="filter-search-icon" />
      <label class="sr-only" for="hl7-inspector-filter">
        Filter HL7 segments
      </label>
      <Input id="hl7-inspector-filter" bind:value={filter} placeholder="Filter segments" />
    </div>
    <Badge mono>{filteredSegments.length}/{message.segments.length} segments</Badge>
    <Badge mono title="Field, component and repetition delimiters">
      {message.delimiters.field} {message.delimiters.component} {message.delimiters.repetition}
    </Badge>
  </div>

  {#if selected}
    <div class="selection">
      <span class="selection-key">Selected</span>
      <span class="selection-path">{selectionKey(selected)}</span>
      {#if selectedValue !== null}
        <span class="selection-value" class:is-empty={!selectedValue} title={selectedValue}>
          {selectedValue || '—'}
        </span>
      {/if}
      <span class="selection-actions">
        <Button variant="ghost" icon={Copy} onclick={() => copyText(selectionKey())}>Copy path</Button>
        {#if selectedValue !== null}
          <Button variant="ghost" icon={Copy} onclick={() => copyText(selectedValue ?? '')}>Copy value</Button>
        {/if}
      </span>
    </div>
  {:else}
    <p class="note">Select a warning or path to highlight its field, or click a field to inspect it.</p>
  {/if}

  <div class="segments">
    {#each filteredSegments as seg, idx (seg.index)}
      <details
        class="seg"
        open={idx < 3 || isSelected(seg, selected)}
        data-selected={isSelected(seg, selected)}
        data-hl7-key={seg.id + '[' + seg.occurrence + ']'}
      >
        <summary class="seg-head" on:click={(e) => handleSegmentClick(seg, e)}>
          <Icon icon={ChevronRight} size={14} class="seg-chevron" />
          <span class="seg-id">{seg.id}</span>
          <span class="seg-meta">#{seg.occurrence}</span>
          <span class="seg-meta seg-count">{seg.fields.length} fields</span>
        </summary>

        <div class="fields">
          {#each seg.fields as f (f.number)}
            <!-- svelte-ignore a11y_click_events_have_key_events -->
            <!-- svelte-ignore a11y_no_static_element_interactions -->
            <div
              class="field"
              class:selected={fieldSelected(seg.id, seg.occurrence, f.number)}
              data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number}
              on:click={(e) => handleFieldClick(seg, f.number, e)}
            >
              <div class="row field-row">
                <span class="path">{seg.id}-{f.number}</span>
                <span class="value" class:is-empty={!f.raw} title={f.raw}>{f.raw || '—'}</span>
              </div>

              {#if f.repetitions.length > 1}
                {#each f.repetitions as r (r.index)}
                  <div
                    class="rep"
                    class:selected={repetitionSelected(seg.id, seg.occurrence, f.number, r.index)}
                    data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '[' + r.index + ']'}
                  >
                    <div class="row depth-1">
                      <span class="path">{seg.id}-{f.number}[{r.index}]</span>
                      <span class="value" class:is-empty={!r.raw} title={r.raw}>{r.raw || '—'}</span>
                    </div>

                    {#if r.components.length > 1}
                      {#each r.components as c, i (i)}
                        <!-- svelte-ignore a11y_click_events_have_key_events -->
                        <!-- svelte-ignore a11y_no_static_element_interactions -->
                        <div
                          class="row depth-2 component"
                          class:selected={repetitionComponentSelected(seg.id, seg.occurrence, f.number, r.index, i + 1)}
                          data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '[' + r.index + '].' + (i + 1)}
                          on:click={(e) => handleComponentClick(seg, f.number, i + 1, e)}
                        >
                          <span class="path">{seg.id}-{f.number}[{r.index}].{i + 1}</span>
                          <span class="value" class:is-empty={!c} title={c}>{c || '—'}</span>
                        </div>
                      {/each}
                    {/if}
                  </div>
                {/each}
              {:else if f.components.length > 1}
                {#each f.components as c, i (i)}
                  <!-- svelte-ignore a11y_click_events_have_key_events -->
                  <!-- svelte-ignore a11y_no_static_element_interactions -->
                  <div
                    class="row depth-1 component"
                    class:selected={componentSelected(seg.id, seg.occurrence, f.number, i + 1)}
                    data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '.' + (i + 1)}
                    on:click={(e) => handleComponentClick(seg, f.number, i + 1, e)}
                  >
                    <span class="path">{seg.id}-{f.number}.{i + 1}</span>
                    <span class="value" class:is-empty={!c} title={c}>{c || '—'}</span>
                  </div>
                {/each}
              {/if}
            </div>
          {/each}
        </div>
      </details>
    {/each}
  </div>
</div>

<style>
  .inspector {
    display: flex;
    flex-direction: column;
    gap: var(--space-2);
    min-width: 0;
  }

  .filters {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .filter-search {
    position: relative;
    flex: 1 1 200px;
    max-width: 280px;
  }

  .filter-search :global(.filter-search-icon) {
    position: absolute;
    left: 8px;
    top: 50%;
    transform: translateY(-50%);
    color: var(--color-text-tertiary);
    pointer-events: none;
  }

  .filter-search :global(.ui-input) {
    padding-left: 26px;
  }

  .selection {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    min-width: 0;
    min-height: var(--size-control-sm);
  }

  .selection-key {
    flex: 0 0 auto;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .selection-path,
  .selection-value {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .selection-path {
    flex: 0 0 auto;
    color: var(--color-text-primary);
  }

  .selection-value {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-secondary);
  }

  .selection-actions {
    display: flex;
    flex: 0 0 auto;
    gap: 2px;
    margin-left: auto;
  }

  .note {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .segments {
    border: 1px solid var(--color-border-subtle);
    border-radius: var(--radius-sm);
    background: var(--color-bg-base);
    overflow: hidden;
  }

  .seg + .seg {
    border-top: 1px solid var(--color-border-subtle);
  }

  .seg-head {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    height: var(--size-control-sm);
    padding: 0 var(--space-3) 0 var(--space-2);
    background: var(--color-bg-surface);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    list-style: none;
    cursor: pointer;
    user-select: none;
  }

  .seg-head::-webkit-details-marker {
    display: none;
  }

  .seg-head:hover {
    background: var(--color-bg-hover);
  }

  .seg-head:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: -2px;
  }

  .seg[data-selected='true'] > .seg-head {
    box-shadow: inset 2px 0 0 var(--color-primary);
  }

  .seg-head :global(.seg-chevron) {
    color: var(--color-text-tertiary);
    transition: transform var(--duration-fast) var(--ease-out);
  }

  .seg[open] > .seg-head :global(.seg-chevron) {
    transform: rotate(90deg);
  }

  .seg-id {
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .seg-meta {
    color: var(--color-text-tertiary);
  }

  .seg-count {
    margin-left: auto;
  }

  .fields {
    padding: 2px 0;
    border-top: 1px solid var(--color-border-subtle);
  }

  /* Nesting indents the path only, so values stay in one column. */
  .row {
    display: grid;
    grid-template-columns: 136px minmax(0, 1fr);
    gap: var(--space-3);
    align-items: center;
    min-height: 22px;
    padding: 0 var(--space-3) 0 28px;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    line-height: 22px;
  }

  .field-row,
  .component {
    cursor: pointer;
  }

  .field-row:hover,
  .component:hover {
    background: var(--color-bg-hover);
  }

  .row.depth-1 .path {
    padding-left: var(--space-4);
  }

  .row.depth-2 .path {
    padding-left: var(--space-8);
  }

  .field.selected > .field-row,
  .rep.selected > .row,
  .component.selected {
    background: var(--color-primary-muted);
    box-shadow: inset 2px 0 0 var(--color-primary);
  }

  .path {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-tertiary);
  }

  .value {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    color: var(--color-text-primary);
  }

  .component .value,
  .rep .value {
    color: var(--color-text-secondary);
  }

  .value.is-empty,
  .selection-value.is-empty {
    color: var(--color-text-muted);
  }
</style>
