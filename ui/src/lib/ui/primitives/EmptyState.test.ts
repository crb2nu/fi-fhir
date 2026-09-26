import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import Inbox from '@lucide/svelte/icons/inbox';
import EmptyState from './EmptyState.svelte';
import { text } from './test-utils';

describe('EmptyState', () => {
  it('renders one sentence with a decorative 16px icon', () => {
    render(EmptyState, {
      props: { icon: Inbox, message: 'No deliveries in the last 24 hours.', 'data-testid': 'empty' }
    });
    const root = screen.getByTestId('empty');
    expect(root).toHaveTextContent('No deliveries in the last 24 hours.');
    const svg = root.querySelector('svg');
    expect(svg).toHaveAttribute('aria-hidden', 'true');
    expect(svg).toHaveAttribute('width', '16');
    expect(screen.queryByRole('button')).toBeNull();
  });

  it('renders one primary action when actionLabel and onaction are given', async () => {
    const onaction = vi.fn();
    render(EmptyState, { props: { message: 'Nothing yet.', actionLabel: 'Send test message', onaction } });
    const button = screen.getByRole('button', { name: 'Send test message' });
    expect(button).toHaveClass('ui-button--primary');
    await fireEvent.click(button);
    expect(onaction).toHaveBeenCalledOnce();
  });

  it('accepts rich children, a custom action snippet and start alignment', () => {
    const { container } = render(EmptyState, {
      props: {
        align: 'start',
        children: text('Needs role integration.operator', 'code'),
        action: text('Request access', 'a')
      }
    });
    expect(container.querySelector('.ui-empty')).toHaveClass('ui-empty--start');
    expect(screen.getByText('Needs role integration.operator').tagName).toBe('CODE');
    expect(screen.getByText('Request access').tagName).toBe('A');
  });
});
