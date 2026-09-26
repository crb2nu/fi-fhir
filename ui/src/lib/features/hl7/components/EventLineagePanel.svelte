<script lang="ts">
  import type { HL7Message } from '$lib/domain/hl7v2';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import { getHL7Value } from '$lib/domain/hl7Access';
  import type { ParsePreviewQuery } from '$lib/gen/graphql';
  import { createEventDispatcher } from 'svelte';
  import { Badge, Button, EmptyState, Panel, Select, Table, Td, Th, Tr } from '$lib/ui/primitives';
  import type { IntegrationSessionLineage } from '$lib/features/integration-session';

  export let events: ParsePreviewQuery['parsePreview']['events'];
  export let message: HL7Message;
  export let lineage: IntegrationSessionLineage[] = [];

  const dispatch = createEventDispatcher<{ inspectPath: { path: string } }>();

  type PointerRow = { path: string; value: string; meaning: string };

  const adtPointers: ReadonlyArray<{ path: string; meaning: string }> = [
    { path: 'PID-3[0].1', meaning: 'MRN (first repetition)' },
    { path: 'PID-5.1', meaning: 'Family name' },
    { path: 'PID-5.2', meaning: 'Given name' },
    { path: 'PID-7', meaning: 'Date of birth' },
    { path: 'PID-8', meaning: 'Sex' },
    { path: 'PV1-2', meaning: 'Patient class' },
    { path: 'PV1-3', meaning: 'Assigned location' }
  ];

  // The page answers by opening the Inspector on that path.
  function inspect(path: string): void {
    dispatch('inspectPath', { path });
  }

  function inspectFromButton(event: MouseEvent, path: string): void {
    // The row inspects on click too; dispatch once.
    event.preventDefault();
    inspect(path);
  }

  function hl7(path: string): string | null {
    return getHL7Value(message, parseHL7Path(path));
  }

  function compact(s: string | null, max = 56): string {
    const v = (s ?? '').trim();
    if (!v) return '—';
    if (v.length <= max) return v;
    return v.slice(0, max - 1) + '…';
  }

  // Drop the Description column when the server sent none.
  $: hasDescriptions = lineage.some((link) => Boolean(link.description));

  $: obxSegments = message.segments.filter((s) => s.id === 'OBX');
  $: obrSegments = message.segments.filter((s) => s.id === 'OBR');
  let obxOccurrence = 0;
  let obrOccurrence = 0;

  $: if (obxSegments.length && obxOccurrence >= obxSegments.length) obxOccurrence = 0;
  $: if (obrSegments.length && obrOccurrence >= obrSegments.length) obrOccurrence = 0;

  function obxLabel(idx: number): string {
    const key = `OBX[${idx}]-3`;
    const id = compact(hl7(key), 44);
    const value = compact(hl7(`OBX[${idx}]-5`), 24);
    return `#${idx} ${id} → ${value}`;
  }

  function obrLabel(idx: number): string {
    const key = `OBR[${idx}]-4`;
    return `#${idx} ${compact(hl7(key), 56)}`;
  }

  function row(msg: HL7Message, path: string, meaning: string): PointerRow {
    return { path, value: (getHL7Value(msg, parseHL7Path(path)) ?? '').trim(), meaning };
  }

  function adtRows(msg: HL7Message): PointerRow[] {
    return adtPointers.map((p) => row(msg, p.path, p.meaning));
  }

  // Everything the rows depend on is an argument, so the markup re-runs this
  // when the message or either picker changes.
  function oruRows(msg: HL7Message, obx: number, obr: number, hasObr: boolean, hasObx: boolean): PointerRow[] {
    const rows: PointerRow[] = [row(msg, 'PID-3[0].1', 'MRN')];
    if (hasObr) rows.push(row(msg, `OBR[${obr}]-4`, 'Order / test'));
    if (hasObx) {
      rows.push(
        row(msg, `OBX[${obx}]`, 'Segment'),
        row(msg, `OBX[${obx}]-2`, 'Value type'),
        row(msg, `OBX[${obx}]-3`, 'Observation id'),
        row(msg, `OBX[${obx}]-5`, 'Value'),
        row(msg, `OBX[${obx}]-6`, 'Units'),
        row(msg, `OBX[${obx}]-11`, 'Status'),
        row(msg, `OBX[${obx}]-14`, 'Observation time')
      );
    }
    return rows;
  }
</script>

{#if events.length === 0}
  <EmptyState align="start" message="No semantic events extracted." />
{:else}
  <div class="stack">
    {#if lineage.length > 0}
      <Panel title="Server lineage" titleTag="h3" flush>
        {#snippet actions()}
          <Badge mono>{lineage.length} links</Badge>
        {/snippet}
        <Table label="Server lineage" layout="fixed">
          {#snippet head()}
            <tr>
              <Th width="132px">Source path</Th>
              <Th>Target</Th>
              {#if hasDescriptions}
                <Th>Description</Th>
              {/if}
              <Th width="80px"><span class="sr-only">Action</span></Th>
            </tr>
          {/snippet}
          {#each lineage as link (`${link.sourcePath}:${link.targetPath ?? ''}`)}
            <Tr selectable onselect={() => inspect(link.sourcePath)}>
              <Td mono truncate value={link.sourcePath} />
              <Td mono truncate value={link.targetPath ?? 'canonical event'} />
              {#if hasDescriptions}
                <Td muted truncate value={link.description ?? ''} />
              {/if}
              <Td class="action-cell">
                <Button
                  variant="ghost"
                  tabindex={-1}
                  onclick={(e) => inspectFromButton(e, link.sourcePath)}
                >
                  Inspect
                </Button>
              </Td>
            </Tr>
          {/each}
        </Table>
      </Panel>
    {/if}

    {#each events as ev (ev.id)}
      <Panel flush aria-label={`${ev.__typename} HL7 pointers`}>
        {#snippet header()}
          <h3 class="event-title">{ev.__typename}</h3>
          <Badge mono>{String(ev.type)}</Badge>
          <span class="event-time">{new Date(String(ev.timestamp)).toLocaleString()}</span>
        {/snippet}

        {#if ev.__typename === 'PatientAdmitEvent' || ev.__typename === 'PatientDischargeEvent'}
          {@render pointers(adtRows(message), 'Common HL7 pointers (ADT)')}
        {:else if ev.__typename === 'LabResultEvent'}
          {#if obxSegments.length > 0}
            <div class="pickers">
              <label class="picker">
                <span class="picker-label">OBR</span>
                <Select bind:value={obrOccurrence} mono>
                  {#if obrSegments.length === 0}
                    <option value={0}>no OBR</option>
                  {:else}
                    {#each obrSegments as s (s.index)}
                      <option value={s.occurrence}>{obrLabel(s.occurrence)}</option>
                    {/each}
                  {/if}
                </Select>
              </label>
              <label class="picker">
                <span class="picker-label">OBX</span>
                <Select bind:value={obxOccurrence} mono>
                  {#each obxSegments as s (s.index)}
                    <option value={s.occurrence}>{obxLabel(s.occurrence)}</option>
                  {/each}
                </Select>
              </label>
            </div>
          {:else}
            <p class="note">No <span class="text-mono">OBX</span> segments in this message.</p>
          {/if}
          {@render pointers(
            oruRows(message, obxOccurrence, obrOccurrence, obrSegments.length > 0, obxSegments.length > 0),
            'Common HL7 pointers (ORU)'
          )}
        {:else}
          <p class="note">No lineage view for {ev.__typename} yet.</p>
        {/if}
      </Panel>
    {/each}
  </div>
{/if}

{#snippet pointers(rows: PointerRow[], label: string)}
  <Table {label} layout="fixed">
    {#snippet head()}
      <tr>
        <Th width="132px">Path</Th>
        <Th>Value</Th>
        <Th width="168px">Field</Th>
        <Th width="80px"><span class="sr-only">Action</span></Th>
      </tr>
    {/snippet}
    {#each rows as r (r.path)}
      <Tr selectable onselect={() => inspect(r.path)}>
        <Td mono truncate value={r.path} />
        <Td mono truncate muted={!r.value} value={r.value || '—'} />
        <Td muted truncate value={r.meaning} />
        <Td class="action-cell">
          <Button variant="ghost" tabindex={-1} onclick={(e) => inspectFromButton(e, r.path)}>
            Inspect
          </Button>
        </Td>
      </Tr>
    {/each}
  </Table>
{/snippet}

<style>
  .stack {
    display: flex;
    flex-direction: column;
    gap: var(--space-3);
  }

  .event-title {
    margin: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-ui);
    font-weight: var(--font-semibold);
    color: var(--color-text-primary);
  }

  .event-time {
    margin-left: auto;
    padding-right: var(--space-2);
    font-family: var(--font-mono);
    font-size: var(--text-mono);
    color: var(--color-text-tertiary);
    white-space: nowrap;
  }

  .pickers {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
    padding: var(--space-2) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .picker {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    min-width: 0;
  }

  .picker-label {
    flex: 0 0 auto;
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    color: var(--color-text-tertiary);
  }

  .note {
    margin: 0;
    padding: var(--space-2) var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .stack :global(.action-cell) {
    padding-right: var(--space-1);
    text-align: right;
  }
</style>
