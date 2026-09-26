import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Panel from './Panel.svelte';
import { text } from './test-utils';

describe('Panel', () => {
  it('renders a titled region labelled by its heading', () => {
    render(Panel, { props: { title: 'Deliveries', children: text('body'), 'data-testid': 'panel' } });
    const region = screen.getByRole('region', { name: 'Deliveries' });
    expect(region).toHaveAttribute('data-testid', 'panel');
    expect(screen.getByRole('heading', { level: 2, name: 'Deliveries' })).toHaveClass('ui-panel-title');
    expect(region).toHaveTextContent('body');
  });

  it('honours titleTag and renders header actions', () => {
    render(Panel, {
      props: { title: 'Alerts', titleTag: 'h3', actions: text('Refresh', 'button'), children: text('x') }
    });
    expect(screen.getByRole('heading', { level: 3, name: 'Alerts' })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: 'Refresh' }).closest('.ui-panel-actions')).not.toBeNull();
  });

  it('omits the header without title/header/actions and supports flush bodies', () => {
    const { container } = render(Panel, { props: { flush: true, class: 'extra', children: text('rows') } });
    expect(container.querySelector('.ui-panel-header')).toBeNull();
    expect(container.querySelector('.ui-panel')).toHaveClass('extra');
    expect(container.querySelector('.ui-panel-body')).toHaveClass('is-flush');
  });

  it('renders a custom header snippet in place of the title', () => {
    render(Panel, { props: { header: text('Custom header', 'strong'), children: text('x') } });
    expect(screen.getByText('Custom header').closest('.ui-panel-header')).not.toBeNull();
  });
});
