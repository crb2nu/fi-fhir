import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import DeploymentControls from './DeploymentControls.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';

const { fetchDeploymentsMock } = vi.hoisted(() => ({ fetchDeploymentsMock: vi.fn() }));

vi.mock('./operatorApi', () => ({
  fetchDeployments: (...args: unknown[]) => fetchDeploymentsMock(...args)
}));

function deployment(state: string) {
  return {
    definitionRevision: {
      artifactId: 'adt-to-fhir',
      revisionId: 'rev-1',
      digest: 'sha256:' + 'a'.repeat(64)
    },
    state,
    health: 'healthy',
    version: 3,
    validationPassed: true,
    updatedBy: { id: 'operator@example.test' },
    updatedAt: '2026-09-25T10:00:00Z',
    updatedReason: 'initial rollout'
  };
}

function statusWithDeployment(operatorDeployment: boolean) {
  return {
    authenticated: true,
    authVia: 'cloudflare-access',
    principal: 'operator@example.test',
    roles: ['graphql:operator', 'integration.operator'],
    capabilities: {
      operatorRead: true,
      operatorDelivery: true,
      operatorDeployment,
      clinicalRead: true,
      integrationSessions: false,
      streaming: false
    },
    missingRoles: operatorDeployment
      ? {}
      : { operatorDeployment: ['integration.deployment.operator'] }
  };
}

describe('DeploymentControls capability gating', () => {
  beforeEach(() => {
    vi.clearAllMocks();
    fetchDeploymentsMock.mockResolvedValue([deployment('active')]);
  });

  afterEach(() => {
    resetAccessCapabilities();
  });

  it('disables every command and names the missing deployment role', async () => {
    setAccessStatus(statusWithDeployment(false));
    render(DeploymentControls);

    for (const name of ['Deploy', 'Pause', 'Resume', 'Retire']) {
      const button = await screen.findByRole('button', { name });
      expect(button).toBeDisabled();
      expect(button).toHaveAttribute(
        'title',
        expect.stringMatching(/integration\.deployment\.operator/)
      );
    }
  });

  it('falls back to the state preconditions when the role is held', async () => {
    setAccessStatus(statusWithDeployment(true));
    render(DeploymentControls);

    const pause = await screen.findByRole('button', { name: 'Pause' });
    expect(pause.getAttribute('title') ?? '').not.toMatch(/integration\.deployment\.operator/);
  });
});
