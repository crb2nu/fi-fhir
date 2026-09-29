import { describe, expect, it } from 'vitest';
import { render, screen, within } from '@testing-library/svelte';
import Sidebar from './Sidebar.svelte';
import { evidenceOf } from './__fixtures__/journeyEvidence';

describe('Sidebar', () => {
  it('renders the stage context for the active view', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7/sample', evidence: null } });

    expect(screen.getByRole('complementary', { name: 'Workbench sidebar' })).toBeInTheDocument();
    expect(screen.getByRole('heading', { name: 'Stage' })).toBeInTheDocument();
    expect(screen.getByText('Source Intake', { selector: '.context-title' })).toBeInTheDocument();
    const badge = screen.getByTestId('sidebar-stage-badge');
    expect(badge).toHaveTextContent('1/5');
    expect(badge).toHaveAttribute('data-state', 'pending');
    expect(badge).toHaveAttribute('title', 'Stage 1 of 5: Source Intake — not checked yet.');
    expect(screen.getByText(/Load inbound messages/)).toBeInTheDocument();
  });

  it('badges the stage from its evidence and states the evidence', () => {
    const evidence = evidenceOf({ normalization: 'complete' });
    render(Sidebar, { props: { open: true, width: 320, pathname: '/profiles', evidence } });

    const badge = screen.getByTestId('sidebar-stage-badge');
    expect(badge).toHaveTextContent('2/5');
    expect(badge).toHaveAttribute('data-state', 'complete');
    expect(badge).toHaveAttribute('data-tone', 'success');
    expect(screen.getByTestId('sidebar-stage-evidence')).toHaveTextContent('normalization is complete.');
  });

  it('keeps an unknown stage neutral', () => {
    const evidence = evidenceOf({ translation: 'unknown' });
    render(Sidebar, { props: { open: true, width: 320, pathname: '/terminology', evidence } });

    const badge = screen.getByTestId('sidebar-stage-badge');
    expect(badge).toHaveAttribute('data-state', 'unknown');
    expect(badge).toHaveAttribute('data-tone', 'neutral');
  });

  it('lists related views as links with their routes', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/hl7' } });

    const related = screen.getByRole('region', { name: 'Related' });
    expect(within(related).getByRole('link', { name: /Profiles/ })).toHaveAttribute('href', '/profiles');
  });

  it('uses the plain view heading off the stage routes', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/operator' } });

    expect(screen.getByRole('heading', { name: 'View' })).toBeInTheDocument();
    expect(screen.getByText('Operator', { selector: '.context-title' })).toBeInTheDocument();
    // The route toolbar owns the "Operator" heading; the sidebar must not repeat it.
    // (A string name matches the whole accessible name in Testing Library.)
    expect(screen.queryByRole('heading', { name: 'Operator' })).not.toBeInTheDocument();
    expect(screen.queryByTestId('sidebar-stage-badge')).not.toBeInTheDocument();
  });

  it('marks the active navigation link', () => {
    render(Sidebar, { props: { open: true, width: 320, pathname: '/terminology' } });

    const activeLink = screen.getByRole('link', { current: 'page' });
    expect(activeLink).toHaveAttribute('aria-current', 'page');
    expect(activeLink).toHaveAttribute('href', '/terminology');
  });

  it('does not render content when closed', () => {
    render(Sidebar, { props: { open: false, width: 320, pathname: '/' } });

    expect(screen.queryByText('Home')).not.toBeInTheDocument();
  });
});
