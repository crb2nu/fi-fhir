import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import EventLineagePanel from './EventLineagePanel.svelte';
import { parseHL7Message } from '$lib/domain/hl7v2';
import type { ParsePreviewQuery } from '$lib/gen/graphql';
import type { IntegrationSessionLineage } from '$lib/features/integration-session';

type PreviewEvents = ParsePreviewQuery['parsePreview']['events'];

/** Synthetic ADT^A01: placeholder identifiers only. */
const ADT = parseHL7Message(
  [
    'MSH|^~\\&|TEST|TEST|FI_FHIR|TEST|20260101090000||ADT^A01|MSG-1|T|2.5.1',
    'EVN|A01|20260101090000',
    'PID|1||MRN-0001^^^TEST^MR||Testpatient^Alex||19800101|U',
    'PV1|1|I|WARD^101^A'
  ].join('\r')
);

const ADMIT_EVENT = {
  __typename: 'PatientAdmitEvent',
  id: 'evt-1',
  type: 'PATIENT_ADMIT',
  timestamp: '2026-01-01T09:00:00Z'
} as unknown as PreviewEvents[number];

const LINEAGE: IntegrationSessionLineage[] = [
  {
    sourcePath: 'PID-3[0].1',
    targetPath: 'event.patient.identifiers[0]',
    description: 'Medical record number'
  },
  { sourcePath: 'PID-5.1', targetPath: null, description: null }
];

let fetchSpy: ReturnType<typeof vi.fn>;

beforeEach(() => {
  fetchSpy = vi.fn(() => Promise.reject(new Error('EventLineagePanel must not fetch')));
  vi.stubGlobal('fetch', fetchSpy);
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
  expect(fetchSpy).not.toHaveBeenCalled();
});

describe('EventLineagePanel', () => {
  it('renders the empty state when no events were extracted', () => {
    render(EventLineagePanel, { props: { events: [], message: ADT, lineage: LINEAGE } });

    expect(screen.getByText('No semantic events extracted.')).toBeInTheDocument();
    expect(screen.queryByRole('table')).not.toBeInTheDocument();
  });

  it('renders server lineage rows from the session fixture', () => {
    render(EventLineagePanel, { props: { events: [ADMIT_EVENT], message: ADT, lineage: LINEAGE } });

    const table = screen.getByRole('table', { name: 'Server lineage' });
    expect(screen.getByText('2 links')).toBeInTheDocument();
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent('PID-3[0].1');
    expect(rows[0]).toHaveTextContent('event.patient.identifiers[0]');
    expect(rows[0]).toHaveTextContent('Medical record number');
    // A link with no target names the canonical event rather than leaving a blank cell.
    expect(rows[1]).toHaveTextContent('canonical event');
  });

  it('omits the server lineage table when the session sent none', () => {
    render(EventLineagePanel, { props: { events: [ADMIT_EVENT], message: ADT } });

    expect(screen.queryByRole('table', { name: 'Server lineage' })).not.toBeInTheDocument();
    const pointers = screen.getByRole('table', { name: 'Common HL7 pointers (ADT)' });
    expect(within(pointers).getByText('MRN-0001')).toBeInTheDocument();
    expect(within(pointers).getByText('Testpatient')).toBeInTheDocument();
  });

  it('asks the page to inspect a lineage source path', async () => {
    const onInspect = vi.fn();
    render(EventLineagePanel, {
      props: { events: [ADMIT_EVENT], message: ADT, lineage: LINEAGE },
      events: { inspectPath: onInspect }
    });

    const table = screen.getByRole('table', { name: 'Server lineage' });
    const firstRow = within(table).getAllByRole('row')[1]!;
    await fireEvent.click(within(firstRow).getByRole('button', { name: 'Inspect' }));

    expect(onInspect).toHaveBeenCalledTimes(1);
    expect(onInspect.mock.calls[0]![0].detail).toEqual({ path: 'PID-3[0].1' });
  });
});
