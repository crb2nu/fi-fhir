import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import PageHeader from './PageHeader.svelte';

describe('PageHeader', () => {
  it('renders title only', () => {
    render(PageHeader, { props: { title: 'Events' } });

    expect(screen.getByRole('heading', { name: 'Events' })).toBeInTheDocument();
    expect(screen.queryByText(/./,  { selector: '.subtitle' })).not.toBeInTheDocument();
  });

  it('renders title and subtitle', () => {
    render(PageHeader, {
      props: {
        title: 'Terminology Mapping',
        subtitle: 'Map codes after confirming the source profile and HL7 context.'
      }
    });

    expect(screen.getByRole('heading', { name: 'Terminology Mapping' })).toBeInTheDocument();
    expect(screen.getByText('Map codes after confirming the source profile and HL7 context.')).toBeInTheDocument();
  });
});
