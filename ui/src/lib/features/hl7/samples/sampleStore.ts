import { derived, writable } from 'svelte/store';
import type { HL7Sample, NewHL7Sample } from './types';
import { parseHL7Message } from '$lib/domain/hl7v2';
import { demoSamples } from './demoSamples';

type State = {
  samples: HL7Sample[];
  activeId: string | null;
};

function nowIso(): string {
  return new Date().toISOString();
}

function makeId(): string {
  if (typeof globalThis.crypto?.randomUUID === 'function') {
    return globalThis.crypto.randomUUID();
  }
  throw new Error('secure random UUID generation is unavailable');
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

export function createHL7SampleStore() {
  // Raw clinical messages are intentionally scoped to this store instance.
  // Reloading the page or creating a new tab clears them; never persist PHI in
  // browser storage implicitly.
  const state = writable<State>({ samples: [], activeId: null });

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
        ...(input.feed?.trim() ? { feed: input.feed.trim() } : {}),
        ...(input.tags?.length ? { tags: input.tags } : {}),
        ...(input.redactionMode && input.redactionMode !== 'none'
          ? { redactionMode: input.redactionMode }
          : {}),
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

    addMany(inputs: NewHL7Sample[], activate: 'first' | 'last' = 'first'): HL7Sample[] {
      const normalized = inputs
        .map((input) => ({
          name: input.name?.trim() || 'Untitled sample',
          source: input.source.trim() || 'unknown',
          feed: input.feed?.trim() || undefined,
          tags: input.tags,
          redactionMode: input.redactionMode,
          raw: input.raw
        }))
        .filter((x) => x.raw.trim().length > 0);

      if (normalized.length === 0) return [];

      const samplesToAdd: HL7Sample[] = normalized.map((input) => ({
        id: makeId(),
        name: input.name,
        source: input.source,
        ...(input.feed ? { feed: input.feed } : {}),
        ...(input.tags?.length ? { tags: input.tags } : {}),
        ...(input.redactionMode && input.redactionMode !== 'none'
          ? { redactionMode: input.redactionMode }
          : {}),
        raw: input.raw,
        createdAt: nowIso(),
        ...summarize(input.raw)
      }));

      state.update((s) => {
        const activeId =
          activate === 'first' ? samplesToAdd[0]!.id : samplesToAdd[samplesToAdd.length - 1]!.id;
        return {
          ...s,
          activeId,
          samples: [...samplesToAdd, ...s.samples]
        };
      });

      return samplesToAdd;
    },

    remove(id: string): void {
      state.update((s) => {
        const samples = s.samples.filter((x) => x.id !== id);
        const activeId = s.activeId === id ? (samples[0]?.id ?? null) : s.activeId;
        return { ...s, samples, activeId };
      });
    },

    updateMeta(
      id: string,
      changes: Partial<Pick<HL7Sample, 'name' | 'source' | 'feed' | 'tags' | 'redactionMode'>>
    ): void {
      state.update((s) => {
        const samples = s.samples.map((x) => {
          if (x.id !== id) return x;

          const hasName = 'name' in changes;
          const hasSource = 'source' in changes;
          const hasFeed = 'feed' in changes;
          const hasTags = 'tags' in changes;
          const hasRedactionMode = 'redactionMode' in changes;

          const name = changes.name?.trim();
          const source = changes.source?.trim();
          const feed = changes.feed?.trim();
          const tags = changes.tags?.map((t) => t.trim()).filter((t) => t.length > 0).slice(0, 12);
          const redactionMode = changes.redactionMode;

          const next: HL7Sample = {
            ...x,
            ...(hasName && name ? { name } : {}),
            ...(hasSource && source ? { source } : {})
          };

          if (hasFeed) {
            if (feed) next.feed = feed;
            else delete next.feed;
          }
          if (hasTags) {
            if (tags?.length) next.tags = tags;
            else delete next.tags;
          }
          if (hasRedactionMode) {
            if (redactionMode && redactionMode !== 'none') next.redactionMode = redactionMode;
            else delete next.redactionMode;
          }

          return next;
        });

        return { ...s, samples };
      });
    },

    setActive(id: string): void {
      state.update((s) => ({ ...s, activeId: id }));
    },

    clear(): void {
      state.set({ samples: [], activeId: null });
    },

    loadDemoSamples(): void {
      state.update((s) => {
        // Add demo samples, avoiding duplicates by name
        const existingNames = new Set(s.samples.map((x) => x.name));
        const newSamples: HL7Sample[] = [];
        for (const demo of demoSamples) {
          if (!existingNames.has(demo.name ?? 'Untitled sample')) {
            newSamples.push({
              id: makeId(),
              name: demo.name?.trim() || 'Demo sample',
              source: demo.source.trim() || 'demo',
              raw: demo.raw,
              createdAt: nowIso(),
              ...summarize(demo.raw)
            });
          }
        }
        const samples = [...newSamples, ...s.samples];
        return {
          ...s,
          samples,
          activeId: newSamples[0]?.id ?? s.activeId
        };
      });
    },

    hasDemoSamples(): boolean {
      let has = false;
      state.subscribe((s) => {
        const demoNames = new Set(demoSamples.map((d) => d.name));
        has = s.samples.some((x) => demoNames.has(x.name));
      })();
      return has;
    }
  };
}
