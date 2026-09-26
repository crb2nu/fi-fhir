import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Badge from './Badge.svelte';
import { text } from './test-utils';

describe('Badge', () => {
  it('renders a neutral badge by default', () => {
    render(Badge, { props: { children: text('draft'), 'data-testid': 'badge' } });
    const badge = screen.getByTestId('badge');
    expect(badge).toHaveTextContent('draft');
    expect(badge).toHaveClass('ui-badge', 'ui-badge--neutral');
    expect(badge).toHaveAttribute('data-tone', 'neutral');
  });

  it.each(['accent', 'success', 'warning', 'danger', 'info'] as const)('applies the %s tone', (tone) => {
    render(Badge, { props: { tone, children: text(tone), 'data-testid': 'badge' } });
    expect(screen.getByTestId('badge')).toHaveClass(`ui-badge--${tone}`);
  });

  it('supports mono counts, a status dot and class passthrough', () => {
    render(Badge, {
      props: { mono: true, dot: true, class: 'count', children: text('1,284'), 'data-testid': 'badge' }
    });
    const badge = screen.getByTestId('badge');
    expect(badge).toHaveClass('ui-badge--mono', 'count');
    expect(badge.querySelector('.ui-badge-dot')).toHaveAttribute('aria-hidden', 'true');
  });
});
