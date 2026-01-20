<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import type { HL7RedactionMode } from '$lib/domain/hl7Redact';
  import { createEventDispatcher } from 'svelte';

  export let samples: readonly HL7Sample[];
  export let activeId: string | null;
  export let disabled = false;

  export let currentRaw: string;

  const dispatch = createEventDispatcher<{
    select: { id: string };
    remove: { id: string };
    saveCurrent: { name?: string; source?: string; feed?: string; tags?: string[]; redactionMode?: HL7RedactionMode };
    importFiles: { files: File[]; source?: string; feed?: string; tags?: string[]; redactionMode?: HL7RedactionMode };
    clear: Record<string, never>;
    loadExamples: Record<string, never>;
  }>();

  let name = '';
  let feed = '';
  let tags = '';
  let sourceOverride = '';
  let filter = '';
  let redactionMode: HL7RedactionMode = 'none';
  let fileInputEl: HTMLInputElement | null = null;
  let isDragging = false;

  function save() {
    const n = name.trim();
    const so = sourceOverride.trim();
    const f = feed.trim();
    const parsedTags = parseTags(tags);
    dispatch('saveCurrent', {
      ...(n ? { name: n } : {}),
      ...(so ? { source: so } : {}),
      ...(f ? { feed: f } : {}),
      ...(parsedTags.length ? { tags: parsedTags } : {}),
      ...(redactionMode !== 'none' ? { redactionMode } : {})
    });
    name = '';
  }

  function triggerImport() {
    fileInputEl?.click();
  }

  function onFileChange(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = input.files ? Array.from(input.files) : [];
    if (files.length) {
      const so = sourceOverride.trim();
      const f = feed.trim();
      const parsedTags = parseTags(tags);
      dispatch('importFiles', {
        files,
        ...(so ? { source: so } : {}),
        ...(f ? { feed: f } : {}),
        ...(parsedTags.length ? { tags: parsedTags } : {}),
        ...(redactionMode !== 'none' ? { redactionMode } : {})
      });
    }
    input.value = '';
  }

  function onDragOver(e: DragEvent) {
    if (disabled) return;
    if (!e.dataTransfer?.types?.includes('Files')) return;
    e.preventDefault();
    isDragging = true;
  }

  function onDragLeave() {
    isDragging = false;
  }

  function onDrop(e: DragEvent) {
    if (disabled) return;
    const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [];
    if (!files.length) return;
    e.preventDefault();
    isDragging = false;
    const so = sourceOverride.trim();
    const f = feed.trim();
    const parsedTags = parseTags(tags);
    dispatch('importFiles', {
      files,
      ...(so ? { source: so } : {}),
      ...(f ? { feed: f } : {}),
      ...(parsedTags.length ? { tags: parsedTags } : {}),
      ...(redactionMode !== 'none' ? { redactionMode } : {})
    });
  }

  function parseTags(raw: string): string[] {
    const parts = raw
      .split(',')
      .map((x) => x.trim())
      .filter((x) => x.length > 0);
    const uniq: string[] = [];
    for (const t of parts) {
      if (!uniq.includes(t)) uniq.push(t);
      if (uniq.length >= 12) break;
    }
    return uniq;
  }

  $: filtered = filter.trim()
    ? samples.filter((s) => {
        const q = filter.trim().toLowerCase();
        const hay = [
          s.name,
          s.source,
          s.feed ?? '',
          s.messageType ?? '',
          s.version ?? '',
          ...(s.tags ?? [])
        ]
          .join(' ')
          .toLowerCase();
        return hay.includes(q);
      })
    : samples;
</script>

<Panel title="Samples (local)">
  <div
    class="dropzone"
    class:dragging={isDragging}
    on:dragover={onDragOver}
    on:dragleave={onDragLeave}
    on:drop={onDrop}
    role="region"
    aria-label="Samples inbox. Drag and drop HL7 files to import."
  >
  <p class="note">
    Stored in <span class="mono">localStorage</span>. Don’t paste PHI unless you’re on an approved machine/profile.
  </p>

  <div class="controls">
    <label class="label">
      Feed (optional)
      <input class="input" type="text" bind:value={feed} placeholder="e.g., epic_adt_icu" disabled={disabled} />
    </label>
    <label class="label">
      Tags (comma-separated)
      <input class="input" type="text" bind:value={tags} placeholder="e.g., icu, admit, demo" disabled={disabled} />
    </label>
    <label class="label">
      Source override (optional)
      <input class="input" type="text" bind:value={sourceOverride} placeholder="defaults to current source / file name" disabled={disabled} />
    </label>
    <label class="label">
      Redaction
      <select class="select" bind:value={redactionMode} disabled={disabled}>
        <option value="none">None</option>
        <option value="mask_basic">Mask basic (PID/NK1/PV1)</option>
        <option value="segment_sanitize">Sanitize segments (PID/NK1/IN*)</option>
      </select>
      <span class="hint">Best-effort; free-text fields may still contain PHI.</span>
    </label>
  </div>

  <div class="save">
    <label class="label">
      Sample name (optional)
      <input class="input" type="text" bind:value={name} placeholder="ADT A01 - ICU admit" disabled={disabled} />
    </label>
    <div class="save-actions">
      <Button on:click={save} disabled={disabled || !currentRaw.trim()}>Save current</Button>
      <input
        class="file-input"
        type="file"
        multiple
        accept=".hl7,.txt,.msg,.dat,text/plain"
        bind:this={fileInputEl}
        on:change={onFileChange}
        disabled={disabled}
      />
      <Button variant="secondary" on:click={triggerImport} disabled={disabled}>
        Import files
      </Button>
      <Button variant="secondary" on:click={() => dispatch('loadExamples', {})} disabled={disabled}>
        Load Examples
      </Button>
      <Button variant="secondary" on:click={() => dispatch('clear', {})} disabled={disabled || samples.length === 0}>
        Clear
      </Button>
    </div>
  </div>

  {#if samples.length === 0}
    <div class="empty-state">
      <p class="empty">No saved samples yet.</p>
      <Button variant="secondary" on:click={() => dispatch('loadExamples', {})} disabled={disabled}>
        Load Example Messages
      </Button>
    </div>
  {:else}
    <div class="filter">
      <input
        class="input"
        type="text"
        bind:value={filter}
        placeholder="Filter by name, source, feed, message type, tag…"
        disabled={disabled}
      />
      {#if filter.trim()}
        <Button variant="secondary" on:click={() => (filter = '')} disabled={disabled}>Clear</Button>
      {/if}
      <span class="count mono">{filtered.length}/{samples.length}</span>
    </div>

    <ul class="list">
      {#each filtered as s (s.id)}
        <li class="li">
          <button
            type="button"
            class="item"
            class:active={activeId === s.id}
            on:click={() => dispatch('select', { id: s.id })}
            disabled={disabled}
          >
            <div class="top">
              <div class="title">{s.name}</div>
              <div class="meta">
                {#if s.messageType}<span class="pill mono">{s.messageType}</span>{/if}
                {#if s.version}<span class="pill mono">{s.version}</span>{/if}
                {#if s.feed}<span class="pill mono">feed:{s.feed}</span>{/if}
                {#if s.tags?.length}
                  {#each s.tags as t (t)}
                    <span class="pill tag">{t}</span>
                  {/each}
                {/if}
                {#if s.redactionMode && s.redactionMode !== 'none'}
                  <span class="pill warn">redacted</span>
                {/if}
              </div>
            </div>
            <div class="sub">
              <span class="mono">{s.source}</span>
              <span class="dot">•</span>
              <span class="mono">{new Date(s.createdAt).toLocaleString()}</span>
              {#if s.controlId}
                <span class="dot">•</span>
                <span class="mono">MSH-10={s.controlId}</span>
              {/if}
            </div>
          </button>
          <button
            type="button"
            class="trash"
            title="Remove"
            on:click={() => dispatch('remove', { id: s.id })}
            disabled={disabled}
          >
            Remove
          </button>
        </li>
      {/each}
    </ul>
  {/if}
  </div>
</Panel>

<style>
  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .file-input {
    display: none;
  }

  .controls {
    display: grid;
    gap: 10px;
    grid-template-columns: 1fr;
    margin-bottom: 14px;
  }

  @media (min-width: 980px) {
    .controls {
      grid-template-columns: 1fr 1fr;
    }
  }

  .hint {
    font-size: 0.8rem;
    color: rgba(229, 231, 235, 0.55);
  }

  .select {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .select:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .dropzone.dragging {
    outline: 2px dashed rgba(59, 130, 246, 0.7);
    outline-offset: 8px;
    border-radius: 12px;
  }

  .note {
    margin: 0 0 12px;
    color: rgba(229, 231, 235, 0.78);
    line-height: 1.45;
  }

  .save {
    display: grid;
    gap: 10px;
    margin-bottom: 14px;
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

  .save-actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }

  .empty-state {
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 12px;
    padding: 20px;
    border-radius: 12px;
    border: 1px dashed rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.02);
  }

  .empty {
    color: rgba(229, 231, 235, 0.7);
    margin: 0;
  }

  .filter {
    display: flex;
    align-items: center;
    gap: 10px;
    flex-wrap: wrap;
    margin-bottom: 12px;
  }

  .count {
    color: rgba(229, 231, 235, 0.6);
    font-size: 0.85rem;
    font-weight: 700;
  }

  .list {
    padding: 0;
    margin: 0;
    display: grid;
    gap: 10px;
  }

  .li {
    list-style: none;
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 10px;
    align-items: start;
  }

  .item {
    width: 100%;
    text-align: left;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    padding: 12px;
    cursor: pointer;
    color: rgba(229, 231, 235, 0.9);
  }

  .item:hover:enabled {
    background: rgba(255, 255, 255, 0.04);
  }

  .item.active {
    border-color: rgba(59, 130, 246, 0.45);
    background: rgba(59, 130, 246, 0.12);
  }

  .top {
    display: flex;
    align-items: flex-start;
    justify-content: space-between;
    gap: 10px;
  }

  .title {
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .sub {
    margin-top: 8px;
    color: rgba(229, 231, 235, 0.72);
    font-size: 0.9rem;
    display: flex;
    align-items: baseline;
    gap: 8px;
    flex-wrap: wrap;
  }

  .dot {
    color: rgba(229, 231, 235, 0.5);
  }

  .meta {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
    justify-content: flex-end;
  }

  .pill {
    padding: 2px 8px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.04);
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.85rem;
    font-weight: 650;
  }

  .pill.tag {
    border-color: rgba(59, 130, 246, 0.28);
    background: rgba(59, 130, 246, 0.10);
    color: rgba(219, 234, 254, 0.92);
  }

  .pill.warn {
    border-color: rgba(245, 158, 11, 0.35);
    background: rgba(245, 158, 11, 0.12);
    color: rgba(253, 230, 138, 0.95);
  }

  .trash {
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(239, 68, 68, 0.35);
    background: rgba(239, 68, 68, 0.08);
    color: rgba(254, 226, 226, 0.9);
    cursor: pointer;
    font-weight: 700;
  }

  .trash:hover:enabled {
    background: rgba(239, 68, 68, 0.14);
  }
</style>
