import { beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import {
  activateRuntimeOutputFeed,
  appendRuntimeOutputEntry,
  describeEventStreamOutput,
  describeWorkflowOutput,
  resetRuntimeOutputFeed,
  runtimeOutputState,
  formatRuntimeOutputTimestamp,
} from './runtimeOutputStore';

describe('runtimeOutputStore', () => {
  beforeEach(() => {
    resetRuntimeOutputFeed();
  });

  it('maps workflow notifications into warning output when no routes match', () => {
    const entry = describeWorkflowOutput({
      workflow: 'adt-routing',
      routesMatched: [],
      actionsExecuted: ['notify'],
      duration: 42,
      event: {
        id: 'event-1',
        type: 'PATIENT_ADMIT',
        timestamp: '2026-03-31T10:30:00.000Z',
        source: 'workflow-engine'
      }
    } as never);

    expect(entry.severity).toBe('warning');
    expect(entry.title).toBe('adt-routing · Patient Admit');
    expect(entry.message).toContain('No routes matched');
    expect(entry.message).toContain('Duration 42ms');
  });

  it('maps event stream notifications into structured output entries', () => {
    const entry = describeEventStreamOutput({
      id: 'event-2',
      type: 'LAB_RESULT',
      timestamp: '2026-03-31T11:15:00.000Z',
      source: 'event-bus',
      sourceFormat: 'HL7v2',
      correlationId: 'corr-123'
    } as never);

    expect(entry.severity).toBe('info');
    expect(entry.kind).toBe('event-stream');
    expect(entry.title).toBe('Lab Result');
    expect(entry.message).toContain('HL7v2 event from event-bus');
    expect(entry.details).toContain('Correlation ID: corr-123');
  });

  it('keeps only the most recent runtime output entries', () => {
    activateRuntimeOutputFeed('event-stream', 'Event stream', 'event-stream');

    for (let index = 0; index < 101; index += 1) {
      appendRuntimeOutputEntry({
        timestamp: `2026-03-31T12:${String(index % 60).padStart(2, '0')}:00.000Z`,
        severity: 'info',
        kind: 'event-stream',
        title: `Entry ${index}`,
        message: `Message ${index}`,
        source: 'event-bus',
        details: []
      });
    }

    const state = get(runtimeOutputState);
    expect(state.entries).toHaveLength(100);
    expect(state.entries[0]!.title).toBe('Entry 100');
    expect(state.entries[state.entries.length - 1]!.title).toBe('Entry 1');
  });

  it('formats timestamps for display', () => {
    expect(formatRuntimeOutputTimestamp('2026-03-31T12:34:56.000Z')).toContain(':');
  });
});
