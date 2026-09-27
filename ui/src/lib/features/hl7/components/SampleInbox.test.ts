import { afterEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import SampleInbox from './SampleInbox.svelte';
import type { HL7Sample } from '../samples/types';
import { writable } from 'svelte/store';
import { EMPTY_INTAKE, type IntakeController, type IntakeControllerState } from '../intake/intakeController';
import type { ConnectionCaptureRow } from '../intake/intakeApi';

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

describe('SampleInbox — From connection…', () => {
  function fakeIntake(captures: ConnectionCaptureRow[] = []): IntakeController {
    const state = writable<IntakeControllerState>({ ...EMPTY_INTAKE, sessionId: 'session-1', captures });
    return {
      state: { subscribe: state.subscribe },
      refresh: vi.fn(async () => {}),
      startCapture: vi.fn(),
      cancelCapture: vi.fn(),
      peek: vi.fn(),
      dispose: vi.fn()
    } as unknown as IntakeController;
  }

  it('is absent without the session engine (no controller)', () => {
    renderInbox();
    expect(screen.queryByTestId('sample-from-connection')).not.toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Load examples' })).toBeInTheDocument();
  });

  it('is disabled with the reason in its title when the catalog is not configured', () => {
    render(SampleInbox, {
      props: {
        samples: SAMPLES,
        activeId: 's-1',
        currentRaw: SAMPLES[0]!.raw,
        intake: fakeIntake(),
        intakeDisabledReason: 'The connection catalog is not configured on this deployment.'
      }
    });
    const entry = screen.getByTestId('sample-from-connection');
    expect(entry).toBeDisabled();
    expect(entry).toHaveAttribute('title', 'The connection catalog is not configured on this deployment.');
  });

  it('shows an armed capture row with its count, countdown and Cancel', async () => {
    const expiresAt = new Date(Date.now() + 252_500).toISOString();
    render(SampleInbox, {
      props: {
        samples: SAMPLES,
        activeId: 's-1',
        currentRaw: SAMPLES[0]!.raw,
        intake: fakeIntake([
          {
            id: 'cap-1',
            sessionId: 'session-1',
            sourceId: 'adt-east',
            connectionId: null,
            mode: 'STREAM',
            status: 'ARMED',
            captured: 2,
            maxMessages: 5,
            expiresAt,
            requestedBy: { id: 'e2e-ide-operator', kind: 'service' },
            reason: 'synthetic capture',
            requestedAt: new Date().toISOString(),
            completedAt: null,
            problems: []
          }
        ])
      }
    });
    const row = await screen.findByTestId('connection-capture-row');
    expect(row).toHaveAttribute('data-status', 'ARMED');
    expect(row).toHaveTextContent(/2 \/ 5 captured · expires in 4:1[23]/);
    await fireEvent.click(within(row).getByRole('button', { name: 'Cancel' }));
    const dialog = await screen.findByTestId('connection-intake-dialog');
    expect(dialog).toHaveAttribute('data-step', 'cancel');
  });

  it('marks a captured sample as server-redacted and names its provenance', () => {
    const captured: HL7Sample = {
      ...sample('s-3', 'capture cap-1 #1', 'adt-east'),
      session: {
        sessionId: 'session-1',
        sampleId: 'sample_capture_cap-1_1',
        provenance: 'capture:cap-1',
        payloadWithheld: true
      }
    };
    render(SampleInbox, { props: { samples: [captured], activeId: 's-3', currentRaw: captured.raw } });
    const details = screen.getByRole('region', { name: 'Selected sample' });
    expect(details).toHaveTextContent('capture:cap-1');
    expect(details).toHaveTextContent('Capture redactor (server)');
    const withheld = within(details).getByTestId('sample-payload-withheld');
    expect(within(withheld).getByText('integration.operator').tagName).toBe('CODE');
  });
});
