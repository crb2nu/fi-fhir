import { browser } from '$app/environment';
import { writable, derived } from 'svelte/store';
import type { WorkflowDraft, RouteDraft, ActionDraft, TransformDraft } from './workflowTypes';
import {
  createEmptyWorkflow,
  createEmptyRoute,
  createEmptyAction,
  createEmptyTransform
} from './workflowTypes';

const STORAGE_KEY = 'fi-fhir:workflow:draft:v1';

function load(): WorkflowDraft {
  if (!browser) return createEmptyWorkflow();
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
  if (!browser) return;
  localStorage.setItem(STORAGE_KEY, JSON.stringify(draft));
}

function createWorkflowDraftStore() {
  const store = writable<WorkflowDraft>(load());
  if (browser) store.subscribe((d) => save(d));

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
