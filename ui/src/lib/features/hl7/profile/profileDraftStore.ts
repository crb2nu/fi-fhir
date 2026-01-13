import { browser } from '$app/environment';
import { writable } from 'svelte/store';
import type { HL7ProfileDraft } from './types';

const STORAGE_KEY = 'fi-fhir:hl7:profileDraft:v1';

const defaultDraft: HL7ProfileDraft = {
  id: 'hl7_feed',
  name: 'HL7 Feed',
  version: '1.0.0',
  defaultVersion: '2.5.1',
  timezone: 'UTC',
  tolerate: {
    missingSegments: ['PV1', 'PD1', 'OBR'],
    nteAnywhere: true,
    extraComponents: true,
    unknownSegments: true,
    nonStandardDelimiters: true
  }
};

function load(): HL7ProfileDraft {
  if (!browser) return defaultDraft;
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return defaultDraft;
    const parsed = JSON.parse(raw) as HL7ProfileDraft;
    if (!parsed || typeof parsed !== 'object') return defaultDraft;
    return parsed;
  } catch {
    return defaultDraft;
  }
}

function save(draft: HL7ProfileDraft): void {
  if (!browser) return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
}

export function createHL7ProfileDraftStore() {
  const store = writable<HL7ProfileDraft>(load());
  if (browser) store.subscribe((d) => save(d));

  return {
    subscribe: store.subscribe,
    set: store.set,
    update: store.update,
    reset: () => store.set(defaultDraft)
  };
}

