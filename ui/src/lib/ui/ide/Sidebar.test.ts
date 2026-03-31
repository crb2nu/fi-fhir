import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Sidebar from './Sidebar.svelte';

describe('Sidebar', () => {
  it('renders contextual sections for the active view', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7/sample' } });

    expect(screen.getByRole('complementary', { name: 'Workbench sidebar' })).toBeInTheDocument();
    expect(screen.getByText('Parser triage')).toBeInTheDocument();
    expect(screen.getByText('HL7 preview')).toBeInTheDocument();
    expect(screen.getByText('Sample inbox')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open profile builder' })).toHaveAttribute('href', '/profiles');
  });

  it('marks the active navigation link', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/terminology' } });

    const activeLink = screen.getByRole('link', { name: 'Terminology' });
    expect(activeLink).toHaveAttribute('aria-current', 'page');
  });

  it('does not render content when closed', () => {
    render(Sidebar, { props: { open: false, width: 320, pathname: '/' } });

    expect(screen.queryByText('Mapping studio')).not.toBeInTheDocument();
  });
});
