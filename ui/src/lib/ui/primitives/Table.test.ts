import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, within } from '@testing-library/svelte';
import TableFixture from './__fixtures__/TableFixture.svelte';

const rows = [
  { id: 'evt_01J8Q3ZK4M7Y2R9C5T1B6N0PXA', type: 'ADT^A01', count: 7 },
  { id: 'evt_01J8Q3ZJ9W1D8H3K6F2Q5V7MZB', type: 'ORU^R01', count: 23 },
  { id: 'evt_01J8Q3ZH2P6T4X8N1C9R3L5GWC', type: 'ADT^A08', count: 6 }
];

function rowFor(id: string): HTMLElement {
  return screen.getByTestId(`row-${id}`);
}

describe('Table / Th / Td / Tr', () => {
  it('renders a labelled table with header cells and passthrough testid', () => {
    render(TableFixture, { props: { rows } });
    expect(screen.getByTestId('events-table')).toHaveClass('ui-table-wrap');
    const table = screen.getByRole('table', { name: 'Events' });
    const headers = within(table).getAllByRole('columnheader');
    expect(headers.map((th) => th.textContent?.trim())).toEqual(['Type', 'Id', 'Count', 'Status']);
    expect(headers[2]).toHaveClass('is-numeric');
    expect(headers[0]).toHaveAttribute('scope', 'col');
  });

  it('formats cells: mono + truncate with a title, numeric right-aligned', () => {
    render(TableFixture, { props: { rows } });
    const first = rows[0]!;
    const idCell = within(rowFor(first.id)).getByText(first.id).closest('td');
    expect(idCell).toHaveClass('is-mono', 'is-truncate');
    expect(idCell).toHaveAttribute('title', first.id);

    const countCell = within(rowFor(first.id)).getByText('7').closest('td');
    expect(countCell).toHaveClass('is-numeric', 'is-mono');
    expect(countCell).not.toHaveAttribute('title');
  });

  it('exposes aria-sort and calls onsort from the header button', async () => {
    const onsort = vi.fn();
    const { rerender } = render(TableFixture, { props: { rows, sort: 'none', onsort } });
    const typeHeader = screen.getByRole('columnheader', { name: /Type/ });
    expect(typeHeader).toHaveAttribute('aria-sort', 'none');
    expect(screen.getByRole('columnheader', { name: 'Id' })).not.toHaveAttribute('aria-sort');

    await fireEvent.click(within(typeHeader).getByRole('button'));
    expect(onsort).toHaveBeenCalledOnce();

    await rerender({ rows, sort: 'descending', onsort });
    expect(screen.getByRole('columnheader', { name: /Type/ })).toHaveAttribute('aria-sort', 'descending');
  });

  it('marks the selected row and selects on click, Enter and Space', async () => {
    const onselect = vi.fn();
    render(TableFixture, { props: { rows, selectedId: rows[1]!.id, onselect } });

    const selected = rowFor(rows[1]!.id);
    expect(selected).toHaveAttribute('aria-selected', 'true');
    expect(selected).toHaveClass('is-selected');
    expect(rowFor(rows[0]!.id)).toHaveAttribute('aria-selected', 'false');

    await fireEvent.click(within(rowFor(rows[0]!.id)).getByText('ADT^A01'));
    expect(onselect).toHaveBeenLastCalledWith(rows[0]!.id);

    await fireEvent.keyDown(rowFor(rows[2]!.id), { key: 'Enter' });
    expect(onselect).toHaveBeenLastCalledWith(rows[2]!.id);
    await fireEvent.keyDown(rowFor(rows[1]!.id), { key: ' ' });
    expect(onselect).toHaveBeenLastCalledWith(rows[1]!.id);
  });

  it('moves focus between selectable rows with the arrow keys, Home and End', async () => {
    render(TableFixture, { props: { rows } });
    const [a, b, c] = rows.map((row) => rowFor(row.id));
    a!.focus();
    await fireEvent.keyDown(a!, { key: 'ArrowDown' });
    expect(b).toHaveFocus();
    await fireEvent.keyDown(b!, { key: 'End' });
    expect(c).toHaveFocus();
    await fireEvent.keyDown(c!, { key: 'ArrowUp' });
    expect(b).toHaveFocus();
    await fireEvent.keyDown(b!, { key: 'Home' });
    expect(a).toHaveFocus();
    expect(a).toHaveAttribute('tabindex', '0');
  });
});
