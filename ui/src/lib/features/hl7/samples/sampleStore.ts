import { browser } from '$app/environment';
import { derived, writable } from 'svelte/store';
import type { HL7Sample, NewHL7Sample } from './types';
import { parseHL7Message } from '$lib/domain/hl7v2';

type State = {
  samples: HL7Sample[];
  activeId: string | null;
};

const STORAGE_KEY = 'fi-fhir:hl7:samples:v1';

function nowIso(): string {
  return new Date().toISOString();
}

function makeId(): string {
  if (typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
    return crypto.randomUUID();
  }
  return `${Date.now()}-${Math.random().toString(16).slice(2)}`;
}

function summarize(raw: string): Partial<Pick<HL7Sample, 'messageType' | 'controlId' | 'version'>> {
  const msg = parseHL7Message(raw);
  const msh = msg.segments.find((s) => s.id === 'MSH');
  if (!msh) return {};

  const f = (n: number) => msh.fields.find((x) => x.number === n)?.raw ?? '';
  const messageType = f(9).trim();
  const controlId = f(10).trim();
  const version = f(12).trim();
  return {
    ...(messageType ? { messageType } : {}),
    ...(controlId ? { controlId } : {}),
    ...(version ? { version } : {})
  };
}

function load(): State {
  if (!browser) return { samples: [], activeId: null };
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return { samples: [], activeId: null };
    const parsed = JSON.parse(raw) as State;
    if (!parsed || !Array.isArray(parsed.samples)) return { samples: [], activeId: null };
    return {
      samples: parsed.samples,
      activeId: typeof parsed.activeId === 'string' ? parsed.activeId : null
    };
  } catch {
    return { samples: [], activeId: null };
  }
}

function save(state: State): void {
  if (!browser) return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(state));
}

export function createHL7SampleStore() {
  const state = writable<State>(load());

  if (browser) {
    state.subscribe((s) => save(s));
  }

  const samples = derived(state, ($s) => $s.samples);
  const activeId = derived(state, ($s) => $s.activeId);
  const activeSample = derived(state, ($s) => $s.samples.find((x) => x.id === $s.activeId) ?? null);

  return {
    state,
    samples,
    activeId,
    activeSample,

    add(input: NewHL7Sample): HL7Sample {
      const sample: HL7Sample = {
        id: makeId(),
        name: input.name?.trim() || 'Untitled sample',
        source: input.source.trim() || 'unknown',
        raw: input.raw,
        createdAt: nowIso(),
        ...summarize(input.raw)
      };
      state.update((s) => ({
        ...s,
        activeId: sample.id,
        samples: [sample, ...s.samples]
      }));
      return sample;
    },

    remove(id: string): void {
      state.update((s) => {
        const samples = s.samples.filter((x) => x.id !== id);
        const activeId = s.activeId === id ? (samples[0]?.id ?? null) : s.activeId;
        return { ...s, samples, activeId };
      });
    },

    setActive(id: string): void {
      state.update((s) => ({ ...s, activeId: id }));
    },

    clear(): void {
      state.set({ samples: [], activeId: null });
    }
  };
}
