<script lang="ts">
  import type { HL7Message, HL7Segment } from '$lib/domain/hl7v2';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';
  import { getHL7Value } from '$lib/domain/hl7Access';
  import { browser } from '$app/environment';
  import { afterUpdate, createEventDispatcher } from 'svelte';

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

  function selectedSegmentId(): string | null {
    return selected ? selected.segmentId : null;
  }

  function isSelected(seg: HL7Segment) {
    const segId = selectedSegmentId();
    return segId !== null && seg.id === segId && seg.occurrence === selectedSegmentOccurrence();
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

  function selectionKey(): string {
    if (!selected) return '';
    const segOcc = selected.segmentOccurrence ?? 0;
    switch (selected.kind) {
      case 'segment':
        return `${selected.segmentId}[${segOcc}]`;
      case 'field':
        return `${selected.segmentId}[${segOcc}]-${selected.field}`;
      case 'component':
        return `${selected.segmentId}[${segOcc}]-${selected.field}.${selected.component}`;
      case 'repetition':
        return `${selected.segmentId}[${segOcc}]-${selected.field}[${selected.repetition}]`;
      case 'repetition_component':
        return `${selected.segmentId}[${segOcc}]-${selected.field}[${selected.repetition}].${selected.component}`;
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

<div class="wrap" bind:this={root}>
  <div class="meta">
    <div class="pill">segments: {filteredSegments.length}/{message.segments.length}</div>
    <div class="pill mono">
      delims: field={message.delimiters.field} comp={message.delimiters.component} rep={message.delimiters.repetition}
    </div>
    <label class="sr-only" for="hl7-inspector-filter">
      Filter HL7 segments
    </label>
    <input
      id="hl7-inspector-filter"
      class="search"
      type="text"
      bind:value={filter}
      placeholder="Filter segments…"
    />
  </div>

  {#if selected}
    <div class="selected">
      <div class="pill mono">selected: {selectionKey()}</div>
      {#if selectedValue !== null}
        <div class="pill mono">value: {selectedValue || '∅'}</div>
      {/if}
      <button class="mini" type="button" on:click={() => copyText(selectionKey())}>Copy path</button>
      {#if selectedValue !== null}
        <button class="mini" type="button" on:click={() => copyText(selectedValue)}>Copy value</button>
      {/if}
    </div>
  {:else}
    <div class="note">Select a warning/path to highlight fields; or click a field to inspect its lineage.</div>
  {/if}

  <div class="segments">
    {#each filteredSegments as seg, idx (seg.index)}
      <details
        class="seg"
        open={idx < 3 || isSelected(seg)}
        data-selected={isSelected(seg)}
        data-hl7-key={seg.id + '[' + seg.occurrence + ']'}
      >
        <!-- svelte-ignore a11y_click_events_have_key_events -->
        <!-- svelte-ignore a11y_no_static_element_interactions -->
        <summary class="summary" on:click={(e) => handleSegmentClick(seg, e)}>
          <span class="id mono">{seg.id}</span>
          <span class="hint">#{seg.occurrence} • {seg.fields.length} fields</span>
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
              <div class="field-head">
                <span class="mono label">{seg.id}-{f.number}</span>
                <span class="mono value">{f.raw || '∅'}</span>
              </div>

              {#if f.repetitions.length > 1}
                <div class="reps">
                  {#each f.repetitions as r (r.index)}
                    <div
                      class="rep"
                      class:selected={repetitionSelected(seg.id, seg.occurrence, f.number, r.index)}
                      data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '[' + r.index + ']'}
                    >
                      <div class="rep-head">
                        <span class="mono rep-label">{seg.id}-{f.number}[{r.index}]</span>
                        <span class="mono rep-value">{r.raw || '∅'}</span>
                      </div>

                      {#if r.components.length > 1}
                        <div class="components">
                          {#each r.components as c, i (i)}
                            <!-- svelte-ignore a11y_click_events_have_key_events -->
                            <!-- svelte-ignore a11y_no_static_element_interactions -->
                            <div
                              class="component"
                              class:selected={repetitionComponentSelected(seg.id, seg.occurrence, f.number, r.index, i + 1)}
                              data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '[' + r.index + '].' + (i + 1)}
                              on:click={(e) => handleComponentClick(seg, f.number, i + 1, e)}
                            >
                              <span class="mono comp-label">{seg.id}-{f.number}[{r.index}].{i + 1}</span>
                              <span class="mono comp-value">{c || '∅'}</span>
                            </div>
                          {/each}
                        </div>
                      {/if}
                    </div>
                  {/each}
                </div>
              {:else if f.components.length > 1}
                <div class="components">
                  {#each f.components as c, i (i)}
                    <!-- svelte-ignore a11y_click_events_have_key_events -->
                    <!-- svelte-ignore a11y_no_static_element_interactions -->
                    <div
                      class="component"
                      class:selected={componentSelected(seg.id, seg.occurrence, f.number, i + 1)}
                      data-hl7-key={seg.id + '[' + seg.occurrence + ']-' + f.number + '.' + (i + 1)}
                      on:click={(e) => handleComponentClick(seg, f.number, i + 1, e)}
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
    align-items: center;
  }

	  .pill {
	    padding: 4px 10px;
	    border-radius: 999px;
	    border: 1px solid var(--color-border-strong);
	    background: var(--color-bg-surface);
	    color: var(--color-text-secondary);
	    font-weight: 650;
	    font-size: 0.85rem;
	  }

	  .mono {
	    font-family: var(--font-mono);
	  }

	  .search {
	    flex: 1;
	    min-width: 220px;
	    padding: 10px 12px;
	    border-radius: var(--radius-xl);
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-input);
	    color: var(--color-text-primary);
	    outline: none;
	  }

	  .search:focus {
	    border-color: var(--color-border-focus);
	    box-shadow: var(--shadow-focus);
	  }

  .selected {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    align-items: center;
  }

	  .note {
	    color: var(--color-text-tertiary);
	    font-size: 0.9rem;
	  }

	  .mini {
	    padding: 6px 10px;
	    border-radius: 10px;
	    border: 1px solid var(--color-border-strong);
	    background: var(--color-bg-surface);
	    color: var(--color-text-secondary);
	    font-weight: 750;
	    cursor: pointer;
	    white-space: nowrap;
	  }

	  .mini:hover {
	    background: var(--color-bg-hover);
	  }

  .segments {
    display: grid;
    gap: 10px;
  }

	  .seg {
	    border-radius: 12px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
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
	    color: var(--color-text-primary);
	    font-weight: 800;
	  }

	  .hint {
	    color: var(--color-text-tertiary);
	    font-weight: 650;
	    font-size: 0.85rem;
	  }

	  .fields {
	    padding: 10px 12px 12px;
	    border-top: 1px solid var(--color-border-default);
	    display: grid;
	    gap: 10px;
	  }

	  .field {
	    border-radius: 10px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
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
	    color: var(--color-text-primary);
	    font-weight: 800;
	  }

	  .value {
	    color: var(--color-text-secondary);
	    overflow: hidden;
	    text-overflow: ellipsis;
	    white-space: nowrap;
	  }

	  .components {
    margin-top: 10px;
    display: grid;
    gap: 6px;
    padding-top: 10px;
	    border-top: 1px solid var(--color-border-default);
	  }

	  .reps {
    margin-top: 10px;
    display: grid;
    gap: 8px;
    padding-top: 10px;
	    border-top: 1px solid var(--color-border-default);
	  }

	  .rep {
	    border-radius: 10px;
	    border: 1px solid var(--color-border-default);
	    background: var(--color-bg-elevated);
	    padding: 10px;
	  }

  .rep.selected {
    border-color: rgba(59, 130, 246, 0.5);
    background: rgba(59, 130, 246, 0.12);
  }

  .rep-head {
    display: grid;
    gap: 6px;
  }

	  .rep-label {
	    color: var(--color-text-secondary);
	    font-weight: 800;
	    font-size: 0.85rem;
	  }

	  .rep-value {
	    color: var(--color-text-secondary);
	    overflow: hidden;
	    text-overflow: ellipsis;
	    white-space: nowrap;
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
	    color: var(--color-text-tertiary);
	    font-weight: 700;
	    font-size: 0.85rem;
	  }

	  .comp-value {
	    color: var(--color-text-secondary);
	    overflow: hidden;
	    text-overflow: ellipsis;
	    white-space: nowrap;
	    max-width: 70%;
	  }
</style>
