import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import SampleInbox from './SampleInbox.svelte';
import type { HL7Sample } from '../samples/types';

/** Synthetic samples: placeholder identifiers only. */
function sample(id: string, name: string, source: string): HL7Sample {
  return {
    id,
    name,
    source,
    raw: `MSH|^~\\&|TEST|TEST|FI_FHIR|TEST|20260101090000||ADT^A01|${id}|T|2.5.1\rEVN|A01|20260101090000`,
    createdAt: '2026-01-01T09:00:00Z'
  };
}

const SAMPLES = [sample('s-1', 'Admit sample', 'feed_a'), sample('s-2', 'Lab sample', 'feed_b')];

function renderInbox() {
  const onSelect = vi.fn();
  const onUpdateMeta = vi.fn();
  render(SampleInbox, {
    props: { samples: SAMPLES, activeId: 's-1', currentRaw: SAMPLES[0]!.raw },
    events: { select: onSelect, updateMeta: onUpdateMeta }
  });
  return { onSelect, onUpdateMeta };
}

function row(name: string): HTMLElement {
  return screen.getByRole('cell', { name }).closest('tr') as HTMLElement;
}

afterEach(() => cleanup());

describe('SampleInbox', () => {
  it("edits another sample's metadata without selecting (loading) it", async () => {
    const { onSelect, onUpdateMeta } = renderInbox();
    const details = screen.getByRole('region', { name: 'Selected sample' });
    expect(within(details).getByRole('heading', { name: 'Admit sample' })).toBeInTheDocument();

    await fireEvent.click(within(row('Lab sample')).getByRole('button', { name: 'Edit sample' }));

    expect(onSelect).not.toHaveBeenCalled();
    expect(within(details).getByRole('heading', { name: 'Lab sample' })).toBeInTheDocument();
    expect(within(details).getByText('Not loaded')).toBeInTheDocument();

    const nameInput = within(details).getByLabelText(/^Name/);
    await fireEvent.input(nameInput, { target: { value: 'Lab sample (renamed)' } });
    await fireEvent.click(within(details).getByRole('button', { name: 'Save changes' }));

    expect(onUpdateMeta).toHaveBeenCalledTimes(1);
    expect(onUpdateMeta.mock.calls[0]![0].detail).toMatchObject({
      id: 's-2',
      name: 'Lab sample (renamed)',
      source: 'feed_b'
    });
    expect(onSelect).not.toHaveBeenCalled();
  });

  it('returns the details pane to the active sample when editing stops or a row is opened', async () => {
    const { onSelect } = renderInbox();
    const details = screen.getByRole('region', { name: 'Selected sample' });

    await fireEvent.click(within(row('Lab sample')).getByRole('button', { name: 'Edit sample' }));
    await fireEvent.click(within(details).getByRole('button', { name: 'Stop editing' }));
    expect(within(details).getByRole('heading', { name: 'Admit sample' })).toBeInTheDocument();

    await fireEvent.click(within(row('Lab sample')).getByRole('button', { name: 'Edit sample' }));
    await fireEvent.click(row('Admit sample'));
    expect(onSelect).toHaveBeenCalledWith(expect.objectContaining({ detail: { id: 's-1' } }));
    expect(within(details).queryByText('Not loaded')).not.toBeInTheDocument();
  });

  it('clears the filter from its own Clear button', async () => {
    renderInbox();
    const filter = screen.getByRole('textbox', { name: 'Filter samples' });
    expect(screen.queryByRole('button', { name: 'Clear filter' })).not.toBeInTheDocument();

    await fireEvent.input(filter, { target: { value: 'lab' } });
    expect(screen.queryByRole('cell', { name: 'Admit sample' })).not.toBeInTheDocument();
    expect(screen.getByText('1/2')).toBeInTheDocument();

    await fireEvent.click(screen.getByRole('button', { name: 'Clear filter' }));
    expect(filter).toHaveValue('');
    expect(screen.getByRole('cell', { name: 'Admit sample' })).toBeInTheDocument();
    expect(screen.getByText('2/2')).toBeInTheDocument();
  });
});
