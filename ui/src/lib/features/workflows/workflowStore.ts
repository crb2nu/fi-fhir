import { browser } from '$app/environment';
import { writable, derived, get } from 'svelte/store';
import type { WorkflowDraft, RouteDraft, ActionDraft, TransformDraft } from './workflowTypes';
import {
  createEmptyWorkflow,
  createEmptyRoute,
  createEmptyAction,
  createEmptyTransform
} from './workflowTypes';

const STORAGE_KEY = 'fi-fhir:workflow:draft:v1';
const SAVED_DRAFTS_KEY = 'fi-fhir:workflow:drafts:v1';
const MAX_SAVED_DRAFTS = 25;

export type SavedWorkflowDraft = {
  id: string;
  name: string;
  savedAt: string;
  draft: WorkflowDraft;
};

function hasLocalStorage(): boolean {
  if (!browser) return false;
  const ls = globalThis.localStorage as Storage | undefined;
  return Boolean(
    ls &&
      typeof ls.getItem === 'function' &&
      typeof ls.setItem === 'function' &&
      typeof ls.removeItem === 'function'
  );
}

function load(): WorkflowDraft {
  if (!hasLocalStorage()) return createEmptyWorkflow();
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return createEmptyWorkflow();
    const parsed = JSON.parse(raw) as WorkflowDraft;
    if (!parsed || typeof parsed !== 'object' || !Array.isArray(parsed.routes)) {
      return createEmptyWorkflow();
    }
    return parsed;
  } catch {
    return createEmptyWorkflow();
  }
}

function save(draft: WorkflowDraft): void {
  if (!hasLocalStorage()) return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
}

function cloneDraft(draft: WorkflowDraft): WorkflowDraft {
  if (typeof globalThis.structuredClone === 'function') {
    return globalThis.structuredClone(draft);
  }
  return JSON.parse(JSON.stringify(draft)) as WorkflowDraft;
}

function loadSavedDrafts(): SavedWorkflowDraft[] {
  if (!hasLocalStorage()) return [];
  try {
    const raw = localStorage.getItem(SAVED_DRAFTS_KEY);
    if (!raw) return [];
    const parsed = JSON.parse(raw) as SavedWorkflowDraft[];
    if (!Array.isArray(parsed)) return [];
    return parsed.filter(
      (item) =>
        item &&
        typeof item.id === 'string' &&
        typeof item.name === 'string' &&
        typeof item.savedAt === 'string' &&
        item.draft &&
        typeof item.draft === 'object' &&
        Array.isArray(item.draft.routes)
    );
  } catch {
    return [];
  }
}

function saveSavedDrafts(items: SavedWorkflowDraft[]): void {
  if (!hasLocalStorage()) return;
  localStorage.setItem(SAVED_DRAFTS_KEY, JSON.stringify(items));
}

function createWorkflowDraftStore() {
  const store = writable<WorkflowDraft>(load());
  store.subscribe((d) => {
    if (browser) save(d);
  });

  return {
    subscribe: store.subscribe,
    set: store.set,
    update: store.update,

    /** Reset to empty workflow. */
    reset: () => store.set(createEmptyWorkflow()),

    /** Load a complete draft (e.g. from YAML parse or LLM generation). */
    loadDraft: (draft: WorkflowDraft) => store.set(draft),

    // ── Route operations ──────────────────────────────────────────────

    addRoute: () =>
      store.update((d) => ({
        ...d,
        routes: [...d.routes, createEmptyRoute()]
      })),

    removeRoute: (key: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.filter((r) => r._key !== key)
      })),

    updateRoute: (key: string, patch: Partial<RouteDraft>) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) => (r._key === key ? { ...r, ...patch } : r))
      })),

    toggleRouteExpanded: (key: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === key ? { ...r, expanded: !r.expanded } : r
        )
      })),

    moveRoute: (key: string, direction: 'up' | 'down') =>
      store.update((d) => {
        const idx = d.routes.findIndex((r) => r._key === key);
        if (idx < 0) return d;
        const newIdx = direction === 'up' ? idx - 1 : idx + 1;
        if (newIdx < 0 || newIdx >= d.routes.length) return d;
        const routes = [...d.routes];
        [routes[idx], routes[newIdx]] = [routes[newIdx]!, routes[idx]!];
        return { ...d, routes };
      }),

    // ── Action operations within a route ──────────────────────────────

    addAction: (routeKey: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, actions: [...r.actions, createEmptyAction()] }
            : r
        )
      })),

    removeAction: (routeKey: string, actionKey: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, actions: r.actions.filter((a) => a._key !== actionKey) }
            : r
        )
      })),

    updateAction: (routeKey: string, actionKey: string, patch: Partial<ActionDraft>) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? {
                ...r,
                actions: r.actions.map((a) =>
                  a._key === actionKey ? { ...a, ...patch } : a
                )
              }
            : r
        )
      })),

    moveAction: (routeKey: string, actionKey: string, direction: 'up' | 'down') =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) => {
          if (r._key !== routeKey) return r;
          const idx = r.actions.findIndex((a) => a._key === actionKey);
          if (idx < 0) return r;
          const newIdx = direction === 'up' ? idx - 1 : idx + 1;
          if (newIdx < 0 || newIdx >= r.actions.length) return r;
          const actions = [...r.actions];
          [actions[idx], actions[newIdx]] = [actions[newIdx]!, actions[idx]!];
          return { ...r, actions };
        })
      })),

    // ── Transform operations within a route ────────────────────────────

    addTransform: (routeKey: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, transforms: [...r.transforms, createEmptyTransform()] }
            : r
        )
      })),

    removeTransform: (routeKey: string, transformKey: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, transforms: r.transforms.filter((t) => t._key !== transformKey) }
            : r
        )
      })),

    updateTransform: (routeKey: string, transformKey: string, patch: Partial<TransformDraft>) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? {
                ...r,
                transforms: r.transforms.map((t) =>
                  t._key === transformKey ? { ...t, ...patch } : t
                )
              }
            : r
        )
      })),

    moveTransform: (routeKey: string, transformKey: string, direction: 'up' | 'down') =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) => {
          if (r._key !== routeKey) return r;
          const idx = r.transforms.findIndex((t) => t._key === transformKey);
          if (idx < 0) return r;
          const newIdx = direction === 'up' ? idx - 1 : idx + 1;
          if (newIdx < 0 || newIdx >= r.transforms.length) return r;
          const transforms = [...r.transforms];
          [transforms[idx], transforms[newIdx]] = [transforms[newIdx]!, transforms[idx]!];
          return { ...r, transforms };
        })
      })),

    // ── Filter operations ─────────────────────────────────────────────

    setEventTypes: (routeKey: string, types: string[]) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, filter: { ...r.filter, eventTypes: types } }
            : r
        )
      })),

    setSources: (routeKey: string, sources: string[]) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, filter: { ...r.filter, sources } }
            : r
        )
      })),

    setCondition: (routeKey: string, condition: string) =>
      store.update((d) => ({
        ...d,
        routes: d.routes.map((r) =>
          r._key === routeKey
            ? { ...r, filter: { ...r.filter, condition } }
            : r
        )
      }))
  };
}

export const workflowDraft = createWorkflowDraftStore();
const savedDraftsStore = writable<SavedWorkflowDraft[]>(loadSavedDrafts());

savedDraftsStore.subscribe((items) => {
  if (browser) saveSavedDrafts(items);
});

function normalizeDraftName(name?: string): string {
  const trimmed = (name ?? '').trim();
  if (trimmed) return trimmed;
  const current = get(workflowDraft);
  if (current.name.trim()) return current.name.trim();
  const stamp = new Date().toISOString().slice(0, 16).replace('T', ' ');
  return `Draft ${stamp}`;
}

export const workflowSavedDrafts = {
  subscribe: savedDraftsStore.subscribe,

  saveCurrent: (name?: string): SavedWorkflowDraft => {
    const entry: SavedWorkflowDraft = {
      id: `${Date.now()}-${Math.random().toString(36).slice(2, 10)}`,
      name: normalizeDraftName(name),
      savedAt: new Date().toISOString(),
      draft: cloneDraft(get(workflowDraft))
    };

    savedDraftsStore.update((items) => {
      const withoutSameName = items.filter((i) => i.name !== entry.name);
      return [entry, ...withoutSameName].slice(0, MAX_SAVED_DRAFTS);
    });

    return entry;
  },

  loadIntoBuilder: (id: string): SavedWorkflowDraft | null => {
    const entry = get(savedDraftsStore).find((i) => i.id === id) ?? null;
    if (!entry) return null;
    workflowDraft.loadDraft(cloneDraft(entry.draft));
    return entry;
  },

  deleteSnapshot: (id: string): void => {
    savedDraftsStore.update((items) => items.filter((i) => i.id !== id));
  },

  clear: (): void => {
    savedDraftsStore.set([]);
  }
};

/** Derived store: is the workflow valid enough to preview? */
export const isWorkflowValid = derived(workflowDraft, ($d) => {
  if (!$d.name.trim()) return false;
  if ($d.routes.length === 0) return false;
  return $d.routes.every(
    (r) => r.name.trim() && r.actions.length > 0 && r.actions.every((a) => a.type)
  );
});

/** Derived store: route count. */
export const routeCount = derived(workflowDraft, ($d) => $d.routes.length);
