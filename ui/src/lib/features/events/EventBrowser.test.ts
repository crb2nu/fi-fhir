import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import EventBrowser from './EventBrowser.svelte';
import type { EventsQuery } from '$lib/gen/graphql';

const { queryEventsMock } = vi.hoisted(() => ({
  queryEventsMock: vi.fn()
}));

vi.mock('./eventsApi', () => ({
  queryEvents: (...args: unknown[]) => queryEventsMock(...args)
}));

describe('EventBrowser', () => {
  beforeEach(() => {
    queryEventsMock.mockReset();
  });

  it('frames the browser for downstream verification and loads the default event window', async () => {
    const events = {
      edges: [
        {
          cursor: 'cursor-1',
          node: {
            id: 'event-1',
            type: 'LAB_RESULT',
            timestamp: '2026-03-31T10:30:00.000Z',
            source: 'lab-hub'
          }
        },
        {
          cursor: 'cursor-2',
          node: {
            id: 'event-2',
            type: 'PATIENT_ADMIT',
            timestamp: '2026-03-31T10:31:00.000Z',
            source: 'admission-feed'
          }
        }
      ],
      totalCount: 2,
      pageInfo: {
        endCursor: 'cursor-2',
        hasNextPage: true,
        hasPreviousPage: false
      }
    } as unknown as EventsQuery['events'];

    queryEventsMock.mockResolvedValue(events);

    render(EventBrowser);

    expect(await screen.findByText('Event browser')).toBeInTheDocument();
    expect(screen.getByText('Downstream verification')).toBeInTheDocument();
    expect(screen.getByText('2 events', { selector: '.count' })).toBeInTheDocument();
    expect(screen.getByText('All downstream events')).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /Lab Result from lab-hub at/i })).toBeInTheDocument();
    expect(queryEventsMock).toHaveBeenCalledWith(
      null,
      50,
      null,
      { field: 'TIMESTAMP', direction: 'DESC' }
    );
  });

  it('updates the event window size when the operator tightens the browse range', async () => {
    queryEventsMock.mockResolvedValue({
      edges: [],
      totalCount: 0,
      pageInfo: {
        endCursor: null,
        hasNextPage: false,
        hasPreviousPage: false
      }
    } as unknown as EventsQuery['events']);

    render(EventBrowser);

    await screen.findByText('Event browser');
    queryEventsMock.mockClear();

    await fireEvent.change(screen.getByLabelText('Window'), { target: { value: '25' } });

    await waitFor(() => {
      expect(queryEventsMock).toHaveBeenCalledWith(
        null,
        25,
        null,
        { field: 'TIMESTAMP', direction: 'DESC' }
      );
    });
  });
});
