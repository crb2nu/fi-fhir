import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import KeyValue from './KeyValue.svelte';

describe('KeyValue', () => {
  it('renders a definition list of keys and values', () => {
    render(KeyValue, {
      props: {
        'data-testid': 'kv',
        items: [
          { key: 'Source', value: 'adt-http' },
          { key: 'Segments', value: 7, mono: true }
        ]
      }
    });
    const dl = screen.getByTestId('kv');
    expect(dl.tagName).toBe('DL');
    expect(Array.from(dl.querySelectorAll('dt')).map((dt) => dt.textContent)).toEqual(['Source', 'Segments']);
    expect(screen.getByText('7')).toHaveClass('is-mono');
  });

  it('shows an em dash for missing values', () => {
    render(KeyValue, {
      props: {
        items: [
          { key: 'Destination', value: null },
          { key: 'Profile', value: undefined },
          { key: 'Note', value: '' }
        ]
      }
    });
    const dashes = screen.getAllByText('—');
    expect(dashes).toHaveLength(3);
    dashes.forEach((dd) => expect(dd).toHaveClass('is-empty'));
  });

  it('truncates with the full value in title and supports two columns', () => {
    const id = 'evt_01J8Q3ZK4M7Y2R9C5T1B6N0PXA';
    const { container } = render(KeyValue, {
      props: { columns: 2, items: [{ key: 'Event id', value: id, mono: true, truncate: true }] }
    });
    expect(screen.getByText(id)).toHaveAttribute('title', id);
    expect(container.querySelector('dl')).toHaveClass('ui-kv--two');
  });
});
