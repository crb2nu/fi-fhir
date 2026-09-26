/**
 * StreamingUnavailable is the honest state every live surface shares (events,
 * HL7 intake sessions, debug, workflow monitor). Its contract — the test id,
 * `data-stream`, `data-reason`, a status role, and the sentence — is what the
 * browser smoke gate asserts, so the restyle must not move it.
 */
import { describe, expect, it } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import StreamingUnavailable from './StreamingUnavailable.svelte';

describe('StreamingUnavailable', () => {
  it('keeps the gate contract on its root', () => {
    render(StreamingUnavailable, {
      props: { root: 'eventStream', subject: 'the event stream', reason: 'not-allowlisted' }
    });

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('role', 'status');
    expect(state).toHaveAttribute('data-stream', 'eventStream');
    expect(state).toHaveAttribute('data-reason', 'not-allowlisted');
    expect(state).toHaveTextContent(
      'Live streaming for the event stream is not available on this deployment'
    );
    expect(state).toHaveTextContent('integrationSessionEvents');
    expect(state).toHaveTextContent('Integration Session runs stream in HL7 intake');
  });

  it('names the API switch when streaming is off', () => {
    render(StreamingUnavailable, {
      props: {
        root: 'integrationSessionEvents',
        subject: 'this session',
        reason: 'streaming-off',
        alternative: 'Preview still runs on the stateless path.'
      }
    });

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveAttribute('data-reason', 'streaming-off');
    expect(state).toHaveTextContent('FI_FHIR_INTEGRATION_SESSION_ENABLED');
    expect(state).toHaveTextContent('Preview still runs on the stateless path.');
    // A session root has no "streams in HL7 intake" pointer: it is HL7 intake.
    expect(state).not.toHaveTextContent('Integration Session runs stream in HL7 intake');
  });

  it('says the identity lacks the root when a session stream is refused', () => {
    render(StreamingUnavailable, {
      props: { root: 'sessionRunEvents', subject: 'run events', reason: 'not-allowlisted', compact: true }
    });

    const state = screen.getByTestId('streaming-unavailable');
    expect(state).toHaveTextContent("this identity's roles do not admit");
    expect(state).toHaveTextContent('sessionRunEvents');
    expect(state).toHaveClass('is-compact');
  });
});
