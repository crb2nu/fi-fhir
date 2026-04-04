import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Sidebar from './Sidebar.svelte';

describe('Sidebar', () => {
  it('renders contextual sections for the active view', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7/sample' } });

    expect(screen.getByRole('complementary', { name: 'Workbench sidebar' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Source Intake' })).toBeInTheDocument();
    expect(screen.getByText('Stage 1')).toBeInTheDocument();
    expect(screen.getByText('Raw payloads')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: /Continue to Normalization/ })[0]).toHaveAttribute('href', '/profiles');
  });

  it('marks the active navigation link', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/terminology' } });

    const activeLink = screen.getByRole('link', { current: 'page' });
    expect(activeLink).toHaveAttribute('aria-current', 'page');
  });

  it('does not render content when closed', () => {
    render(Sidebar, { props: { open: false, width: 320, pathname: '/' } });

    expect(screen.queryByText('Mission control')).not.toBeInTheDocument();
  });
});
