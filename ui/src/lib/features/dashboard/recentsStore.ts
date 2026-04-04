/**
 * Tracks recently opened documents and significant actions.
 * Persists to localStorage and provides sorted/grouped derived stores.
 */
import { writable, derived } from 'svelte/store';

export interface RecentEntry {
  id: string;
  documentType: string; // 'route' | 'workflow-draft' | 'debug-session' | 'trace' | 'event' | 'profile'
  artifactId?: string | undefined;
  title: string;
  subtitle?: string | undefined;
  stage: string; // journey stage name
  timestamp: number;
  route?: string | undefined; // for route-type entries
}

const STORAGE_KEY = 'fi-fhir-recents';
const MAX_ENTRIES = 20;

function loadFromStorage(): RecentEntry[] {
  if (typeof window === 'undefined') return [];
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return [];
    const parsed: unknown = JSON.parse(raw);
    if (!Array.isArray(parsed)) return [];
    return parsed as RecentEntry[];
  } catch {
    return [];
  }
}

function saveToStorage(entries: RecentEntry[]): void {
  if (typeof window === 'undefined') return;
  try {
    localStorage.setItem(STORAGE_KEY, JSON.stringify(entries));
  } catch {
    // Ignore storage errors
  }
}

const entriesStore = writable<RecentEntry[]>(loadFromStorage());

// Persist on every change
entriesStore.subscribe((entries) => {
  saveToStorage(entries);
});

/** Sorted by timestamp descending. */
export const recents = derived(entriesStore, ($entries) =>
  [...$entries].sort((a, b) => b.timestamp - a.timestamp)
);

/** Grouped by stage name. */
export const recentsByStage = derived(recents, ($recents) => {
  const grouped: Record<string, RecentEntry[]> = {};
  for (const entry of $recents) {
    if (!grouped[entry.stage]) {
      grouped[entry.stage] = [];
    }
    grouped[entry.stage]!.push(entry);
  }
  return grouped;
});

/** Add a recent entry (deduplicates by id, enforces max 20). */
export function addRecent(entry: RecentEntry): void {
  entriesStore.update((entries) => {
    const filtered = entries.filter((e) => e.id !== entry.id);
    const next = [entry, ...filtered].slice(0, MAX_ENTRIES);
    return next;
  });
}

/** Clear all recent entries. */
export function clearRecents(): void {
  entriesStore.set([]);
}
