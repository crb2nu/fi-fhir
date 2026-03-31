import { describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen } from '@testing-library/svelte';
import AuthoringFlowRail from './AuthoringFlowRail.svelte';

describe('AuthoringFlowRail', () => {
  it('renders the shared flow structure with link and button CTAs', async () => {
    const preview = vi.fn();

    render(AuthoringFlowRail, {
      props: {
        eyebrow: 'Source-to-mapping flow',
        title: 'From raw source to semantic handoff',
        summary: 'Keep source, profile, and downstream mapping in view.',
        steps: [
          {
            eyebrow: 'Raw source',
            title: 'Inspect the payload',
            description: 'Start from the exact source message.',
            metric: '12 segments',
            status: 'ready',
            actions: [
              { label: 'Preview', variant: 'primary', onClick: preview },
              { label: 'Open profiles', href: '/profiles' }
            ]
          },
          {
            eyebrow: 'Warnings',
            title: 'Review anomalies',
            description: 'Triage warnings before you move on.',
            metric: '3 warnings'
          }
        ]
      }
    });

    expect(screen.getByRole('heading', { name: 'From raw source to semantic handoff' })).toBeInTheDocument();
    expect(screen.getByText('Keep source, profile, and downstream mapping in view.')).toBeInTheDocument();
    expect(screen.getByText('12 segments')).toBeInTheDocument();
    expect(screen.getByRole('link', { name: 'Open profiles' })).toHaveAttribute('href', '/profiles');

    await fireEvent.click(screen.getByRole('button', { name: 'Preview' }));
    expect(preview).toHaveBeenCalledTimes(1);
  });
});
