import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import SessionStreamNotice from './SessionStreamNotice.svelte';
import { resetAccessCapabilities, setAccessStatus } from '$lib/graphql/accessCapabilities';
import { resetObservedStreams } from '$lib/graphql/streamAvailability';

const env = import.meta.env as Record<string, string | undefined>;

function status(streaming: boolean) {
  return {
    authenticated: true,
    authVia: 'network',
    capabilities: {
      operatorRead: false,
      integrationSessions: streaming,
      streaming,
      subscriptions: streaming ? ['integrationSessionEvents', 'sessionRunEvents'] : []
    }
  };
}

describe('SessionStreamNotice (HL7 intake)', () => {
  let previous: string | undefined;

  beforeEach(() => {
    previous = env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED;
    env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED = 'true';
  });

  afterEach(() => {
    env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED = previous;
    resetAccessCapabilities();
    resetObservedStreams();
  });

  it('shows the honest state when the build opted in but the API has streaming off', () => {
    setAccessStatus(status(false));
    render(SessionStreamNotice);

    const note = screen.getByTestId('streaming-unavailable');
    expect(note).toHaveAttribute('data-stream', 'integrationSessionEvents');
    expect(note).toHaveAttribute('data-reason', 'streaming-off');
    expect(note).toHaveTextContent('Live streaming for Integration Session runs is not available');
    expect(note).toHaveTextContent('stateless path');
  });

  it('blames the roles, not the allowlist, when streaming is on but the caller is not admitted', () => {
    setAccessStatus({
      ...status(true),
      capabilities: { ...status(true).capabilities, subscriptions: [] }
    });
    render(SessionStreamNotice);

    const note = screen.getByTestId('streaming-unavailable');
    expect(note).toHaveAttribute('data-reason', 'not-allowlisted');
    expect(note).toHaveTextContent("this identity's roles do not admit");
  });

  it('renders nothing when the session stream is available', () => {
    setAccessStatus(status(true));
    render(SessionStreamNotice);
    expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
  });

  it('renders nothing while capabilities are unknown', () => {
    render(SessionStreamNotice);
    expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
  });

  it('renders nothing when the UI build did not opt into sessions', () => {
    env.VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED = 'false';
    setAccessStatus(status(false));
    render(SessionStreamNotice);
    expect(screen.queryByTestId('streaming-unavailable')).not.toBeInTheDocument();
  });
});
