import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Toolbar from './Toolbar.svelte';
import { text } from './test-utils';

describe('Toolbar', () => {
  it('renders the page title as an h1 in the toolbar', () => {
    render(Toolbar, { props: { title: 'Events', 'data-testid': 'toolbar' } });
    const toolbar = screen.getByTestId('toolbar');
    expect(toolbar.tagName).toBe('HEADER');
    expect(screen.getByRole('heading', { level: 1, name: 'Events' })).toHaveClass('ui-toolbar-title');
  });

  it('lays out title, tabs and right-aligned actions in order', () => {
    const { container } = render(Toolbar, {
      props: { title: 'Operator', tabs: text('tabs-slot'), actions: text('Export', 'button') }
    });
    const regions = Array.from(container.querySelector('.ui-toolbar')?.children ?? []).map(
      (el) => el.className.split(' ')[0]
    );
    expect(regions).toEqual(['ui-toolbar-title', 'ui-toolbar-tabs', 'ui-toolbar-actions']);
    expect(screen.getByRole('button', { name: 'Export' })).toBeInTheDocument();
  });

  it('accepts a heading snippet in place of the title', () => {
    render(Toolbar, { props: { title: 'ignored', heading: text('Intake / HL7', 'nav') } });
    expect(screen.getByText('Intake / HL7').closest('.ui-toolbar-title')).not.toBeNull();
    expect(screen.queryByText('ignored')).toBeNull();
  });

  it('honours titleTag', () => {
    render(Toolbar, { props: { title: 'Nested', titleTag: 'h2' } });
    expect(screen.getByRole('heading', { level: 2, name: 'Nested' })).toBeInTheDocument();
  });
});
