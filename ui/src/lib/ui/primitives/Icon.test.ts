import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Play from '@lucide/svelte/icons/play';
import Icon from './Icon.svelte';

describe('Icon', () => {
  it('renders a decorative 16px icon with the system stroke', () => {
    const { container } = render(Icon, { props: { icon: Play, 'data-testid': 'icon' } });
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '16');
    expect(svg).toHaveAttribute('height', '16');
    expect(svg).toHaveAttribute('stroke-width', '1.75');
    expect(svg).toHaveAttribute('aria-hidden', 'true');
    expect(svg).toHaveClass('ui-icon');
    expect(screen.getByTestId('icon')).toBe(svg);
  });

  it('becomes a labelled image when given a label', () => {
    render(Icon, { props: { icon: Play, label: 'Running' } });
    const img = screen.getByRole('img', { name: 'Running' });
    expect(img).not.toHaveAttribute('aria-hidden');
  });

  it('accepts a size override and class passthrough', () => {
    const { container } = render(Icon, { props: { icon: Play, size: 12, class: 'muted' } });
    const svg = container.querySelector('svg');
    expect(svg).toHaveAttribute('width', '12');
    expect(svg).toHaveClass('ui-icon', 'muted');
  });
});
