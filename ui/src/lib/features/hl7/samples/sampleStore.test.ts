import { beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { createHL7SampleStore } from './sampleStore';

const rawMessage =
  'MSH|^~\\&|SOURCE|FACILITY|FI|FI|20260713120000||ADT^A01|CONTROL-1|P|2.5.1\r' +
  'PID|1||MRN-123^^^FACILITY^MR||Doe^Jane||19800101|F\r';

describe('session samples from connection intake', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  const captured = {
    sessionId: 'session-1',
    sampleId: 'sample_capture_cap-1_1',
    name: 'capture cap-1 #1',
    provenance: 'capture:cap-1',
    sourceId: 'adt-east',
    raw: 'MSH|^~\\&|SYN|SYN|FI|FI|20260101090000||ADT^A01|SYN-1|T|2.5.1\rPID|1||REDACTED||REDACTED\r',
    payloadWithheld: false
  };

  it('adds each session sample once, with its id and provenance, in tab memory only', () => {
    const store = createHL7SampleStore();
    store.add({ name: 'pasted', source: 'ui', raw: rawMessage });
    const pastedId = get(store.activeId);

    const added = store.addSessionSamples([captured], false);
    expect(added).toHaveLength(1);
    expect(added[0]).toMatchObject({
      name: 'capture cap-1 #1',
      source: 'adt-east',
      messageType: 'ADT^A01',
      session: {
        sessionId: 'session-1',
        sampleId: 'sample_capture_cap-1_1',
        provenance: 'capture:cap-1',
        payloadWithheld: false
      }
    });
    // A capture arriving in the background never replaces the editor's sample.
    expect(get(store.activeId)).toBe(pastedId);

    expect(store.addSessionSamples([captured], true)).toEqual([]);
    expect(get(store.samples)).toHaveLength(2);
    expect(localStorage.length).toBe(0);
    expect(sessionStorage.length).toBe(0);
  });

  it('activates the first new sample when asked (a peek the user just read)', () => {
    const store = createHL7SampleStore();
    const [first] = store.addSessionSamples(
      [captured, { ...captured, sampleId: 'second', name: 'capture cap-1 #2' }],
      true
    );
    expect(get(store.activeId)).toBe(first!.id);
  });

  it('keeps a withheld sample empty and falls back to the provenance as its source', () => {
    const store = createHL7SampleStore();
    const [sample] = store.addSessionSamples([{ ...captured, raw: '', sourceId: null, payloadWithheld: true }], false);
    expect(sample).toMatchObject({ raw: '', source: 'capture:cap-1', session: { payloadWithheld: true } });
  });
});

describe('HL7 sample storage boundary', () => {
  beforeEach(() => {
    localStorage.clear();
    sessionStorage.clear();
  });

  it('keeps raw samples only in the current store instance', () => {
    localStorage.setItem('fi-fhir:theme', 'dark');
    const store = createHL7SampleStore();

    store.add({ name: 'ADT sample', source: 'test', raw: rawMessage });

    expect(get(store.samples)).toHaveLength(1);
    expect(get(store.activeSample)?.raw).toBe(rawMessage);
    expect(localStorage.getItem('fi-fhir:theme')).toBe('dark');
    expect(localStorage).toHaveLength(1);
    expect(sessionStorage).toHaveLength(0);

    const freshStore = createHL7SampleStore();
    expect(get(freshStore.samples)).toEqual([]);
    expect(get(freshStore.activeSample)).toBeNull();
  });
});
