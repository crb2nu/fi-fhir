import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import Sidebar from './Sidebar.svelte';

describe('Sidebar', () => {
  it('renders the stage context for the active view', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7/sample' } });

    expect(screen.getByRole('complementary', { name: 'Workbench sidebar' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Stage' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Source Intake' })).toBeInTheDocument();
    expect(screen.getByTitle('Stage 1 of 5')).toHaveTextContent('1/5');
    expect(screen.getByText(/Load inbound messages/)).toBeInTheDocument();
  });

  it('lists related views as links with their routes', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7' } });

    const related = screen.getByRole('region', { name: 'Related' });
    expect(within(related).getByRole('link', { name: /Profiles/ })).toHaveAttribute('href', '/profiles');
  });

  it('uses the plain view heading off the stage routes', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/operator' } });

    expect(screen.getByRole('heading', { name: 'View' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Operations' })).toBeInTheDocument();
    expect(screen.queryByTitle(/^Stage \d of 5$/)).not.toBeInTheDocument();
  });

  it('marks the active navigation link', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/terminology' } });

    const activeLink = screen.getByRole('link', { current: 'page' });
    expect(activeLink).toHaveAttribute('aria-current', 'page');
    expect(activeLink).toHaveAttribute('href', '/terminology');
  });

  it('does not render content when closed', () => {
    render(Sidebar, { props: { open: false, width: 320, pathname: '/' } });

    expect(screen.queryByText('Dashboard')).not.toBeInTheDocument();
  });
});
