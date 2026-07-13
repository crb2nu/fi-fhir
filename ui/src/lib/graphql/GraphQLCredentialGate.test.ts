import { beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';

const mocks = vi.hoisted(() => ({
  setProvider: vi.fn(),
  disposeClient: vi.fn().mockResolvedValue(undefined),
  graphqlFetch: vi.fn().mockResolvedValue({ health: { status: 'healthy' } })
}));

vi.mock('./credentials', () => ({
  setGraphQLCredentialProvider: mocks.setProvider
}));

vi.mock('./subscriptions', () => ({
  disposeClient: mocks.disposeClient
}));

vi.mock('./client', () => ({
  graphqlFetch: mocks.graphqlFetch
}));

import GraphQLCredentialGate from './GraphQLCredentialGate.svelte';

beforeEach(() => {
  mocks.setProvider.mockReset();
  mocks.disposeClient.mockReset();
  mocks.disposeClient.mockResolvedValue(undefined);
  mocks.graphqlFetch.mockReset();
  mocks.graphqlFetch.mockResolvedValue({ health: { status: 'healthy' } });
});

describe('GraphQLCredentialGate', () => {
  it('installs a memory-only provider and clears the password input', async () => {
    render(GraphQLCredentialGate);
    const input = screen.getByLabelText('Deployment bearer credential');
    const token = 'transitional-token-with-24-characters';

    await fireEvent.input(input, { target: { value: token } });
    await fireEvent.submit(screen.getByRole('form', { name: 'Install preview credential' }));

    await screen.findByText('Authenticated preview access active');
    expect(screen.queryByDisplayValue(token)).not.toBeInTheDocument();
    const provider = mocks.setProvider.mock.calls.find((call) => typeof call[0] === 'function')?.[0];
    expect(provider).toBeTypeOf('function');
    expect(provider()).toBe(token);
    expect(mocks.disposeClient).toHaveBeenCalledTimes(1);
    expect(mocks.graphqlFetch).toHaveBeenCalledTimes(1);
  });

  it('clears the provider and authenticated socket explicitly', async () => {
    render(GraphQLCredentialGate);
    await fireEvent.input(screen.getByLabelText('Deployment bearer credential'), {
      target: { value: 'transitional-token-with-24-characters' }
    });
    await fireEvent.submit(screen.getByRole('form', { name: 'Install preview credential' }));
    await fireEvent.click(await screen.findByRole('button', { name: 'Clear access' }));

    await waitFor(() => {
      expect(screen.getByLabelText('Deployment bearer credential')).toBeInTheDocument();
    });
    expect(mocks.setProvider).toHaveBeenLastCalledWith(null);
    expect(mocks.disposeClient).toHaveBeenCalledTimes(2);
  });

  it('rejects credentials shorter than the server minimum', async () => {
    render(GraphQLCredentialGate);
    await fireEvent.input(screen.getByLabelText('Deployment bearer credential'), {
      target: { value: 'too-short' }
    });
    await fireEvent.submit(screen.getByRole('form', { name: 'Install preview credential' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('at least 24 characters');
    expect(mocks.setProvider).not.toHaveBeenCalledWith(expect.any(Function));
    expect(mocks.disposeClient).not.toHaveBeenCalled();
  });

  it('does not unlock the IDE when the credential cannot be validated', async () => {
    mocks.graphqlFetch.mockRejectedValue(new Error('GraphQL HTTP 401'));
    render(GraphQLCredentialGate);
    const token = 'rejected-transitional-token-value';
    await fireEvent.input(screen.getByLabelText('Deployment bearer credential'), {
      target: { value: token }
    });
    await fireEvent.submit(screen.getByRole('form', { name: 'Install preview credential' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('Credential validation failed');
    expect(screen.queryByDisplayValue(token)).not.toBeInTheDocument();
    expect(screen.queryByText('Authenticated preview access active')).not.toBeInTheDocument();
    expect(mocks.setProvider).toHaveBeenLastCalledWith(null);
  });
});
