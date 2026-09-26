import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import EventBrowser from './EventBrowser.svelte';
import type { EventsQuery } from '$lib/gen/graphql';

const { queryEventsMock } = vi.hoisted(() => ({
  queryEventsMock: vi.fn()
}));

vi.mock('./eventsApi', () => ({
  queryEvents: (...args: unknown[]) => queryEventsMock(...args)
}));

const ORDER = { field: 'TIMESTAMP', direction: 'DESC' };

function page(nodes: Array<Record<string, unknown>>, hasNextPage = false) {
  return {
    edges: nodes.map((node, index) => ({ cursor: `cursor-${index + 1}`, node })),
    totalCount: nodes.length,
    pageInfo: { endCursor: `cursor-${nodes.length}`, hasNextPage, hasPreviousPage: false }
  } as unknown as EventsQuery['events'];
}

const labResult = {
  id: 'event-1',
  type: 'LAB_RESULT',
  timestamp: '2026-03-31T10:30:00.000Z',
  source: 'lab-hub',
  sourceFormat: 'HL7V2',
  correlationId: 'corr-synthetic-1'
};
const admit = {
  id: 'event-2',
  type: 'PATIENT_ADMIT',
  timestamp: '2026-03-31T10:31:00.000Z',
  source: 'admission-feed',
  sourceFormat: 'HL7V2',
  correlationId: null
};

describe('EventBrowser', () => {
  beforeEach(() => {
    queryEventsMock.mockReset();
  });

  it('loads the default window into a table, with no explainer card', async () => {
    queryEventsMock.mockResolvedValue(page([labResult, admit], true));

    render(EventBrowser);

    const table = await screen.findByRole('table', { name: 'Events' });
    const rows = within(table).getAllByRole('row').slice(1);
    expect(rows).toHaveLength(2);
    expect(rows[0]).toHaveTextContent('LAB_RESULT');
    expect(rows[0]).toHaveTextContent('lab-hub');
    expect(rows[0]).toHaveTextContent('corr-synthetic-1');
    expect(screen.getByRole('button', { name: 'Load more' })).toBeInTheDocument();
    expect(queryEventsMock).toHaveBeenCalledWith(null, 50, null, ORDER);
    for (const copy of ['Event browser', 'Downstream verification', 'All downstream events']) {
      expect(screen.queryByText(copy)).toBeNull();
    }
  });

  it('shows the selected row in the details pane and follows the keyboard', async () => {
    queryEventsMock.mockResolvedValue(page([labResult, admit]));
    render(EventBrowser);

    const table = await screen.findByRole('table', { name: 'Events' });
    const [first, second] = within(table).getAllByRole('row').slice(1);
    await fireEvent.click(first!);

    const pane = screen.getByRole('complementary', { name: 'Selected event' });
    expect(within(pane).getByText('event-1')).toBeInTheDocument();
    expect(first).toHaveAttribute('aria-selected', 'true');

    await fireEvent.keyDown(first!, { key: 'ArrowDown' });
    await waitFor(() => expect(second).toHaveAttribute('aria-selected', 'true'));
    expect(within(pane).getByText('event-2')).toBeInTheDocument();
  });

  it('updates the event window size when the operator tightens the browse range', async () => {
    queryEventsMock.mockResolvedValue(page([]));

    render(EventBrowser);

    expect(await screen.findByText('No events match these filters.')).toBeInTheDocument();
    queryEventsMock.mockClear();

    await fireEvent.change(screen.getByLabelText('Window'), { target: { value: '25' } });

    await waitFor(() => {
      expect(queryEventsMock).toHaveBeenCalledWith(null, 25, null, ORDER);
    });
  });

  it('applies the source filter on submit, not per keystroke', async () => {
    queryEventsMock.mockResolvedValue(page([]));
    render(EventBrowser);
    await screen.findByText('No events match these filters.');
    queryEventsMock.mockClear();

    const source = screen.getByLabelText('Source');
    await fireEvent.input(source, { target: { value: 'lab-hub' } });
    expect(queryEventsMock).not.toHaveBeenCalled();

    await fireEvent.click(screen.getByRole('button', { name: 'Refresh' }));
    await waitFor(() =>
      expect(queryEventsMock).toHaveBeenCalledWith(
        expect.objectContaining({ sources: ['lab-hub'], types: null }),
        50,
        null,
        ORDER
      )
    );
  });
});
