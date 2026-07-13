import { beforeEach, describe, expect, it } from 'vitest';
import { get } from 'svelte/store';
import { createHL7SampleStore } from './sampleStore';

const rawMessage =
  'MSH|^~\\&|SOURCE|FACILITY|FI|FI|20260713120000||ADT^A01|CONTROL-1|P|2.5.1\r' +
  'PID|1||MRN-123^^^FACILITY^MR||Doe^Jane||19800101|F\r';

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
