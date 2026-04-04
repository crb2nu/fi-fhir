import { describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';

vi.mock('$app/paths', () => ({ resolve: (path: string) => path }));

const { default: HomePage } = await import('./+page.svelte');

describe('home mission control', () => {
  it('orients the operator around the five-stage journey', () => {
    render(HomePage);

    expect(screen.getByRole('heading', { name: 'Build the interface from source to destination' })).toBeInTheDocument();
    expect(screen.getByText('Recommended move')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Start Source Intake' })).toHaveAttribute('href', '/hl7');
    expect(screen.getByText('Operator surfaces')).toBeInTheDocument();
    expect(screen.getAllByRole('link', { name: /Continue to Normalization/ })[0]).toHaveAttribute('href', '/profiles');
    expect(screen.getByText('Operational telemetry')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Review Verification' })).toHaveAttribute('href', '/events');
  });
});
