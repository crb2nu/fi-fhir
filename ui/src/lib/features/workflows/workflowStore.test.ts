import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import { workflowDraft, isWorkflowValid, routeCount } from './workflowStore';

describe('workflowStore', () => {
  beforeEach(() => {
    workflowDraft.reset();
  });

  it('starts with a default empty workflow', () => {
    const draft = get(workflowDraft);
    expect(draft.name).toBe('');
    expect(draft.version).toBe('1.0');
    expect(draft.routes).toHaveLength(1);
  });

  it('adds a route', () => {
    workflowDraft.addRoute();
    const draft = get(workflowDraft);
    expect(draft.routes).toHaveLength(2);
  });

  it('removes a route', () => {
    const draft = get(workflowDraft);
    const key = draft.routes[0]!._key;
    workflowDraft.removeRoute(key);
    expect(get(workflowDraft).routes).toHaveLength(0);
  });

  it('updates route name', () => {
    const draft = get(workflowDraft);
    const key = draft.routes[0]!._key;
    workflowDraft.updateRoute(key, { name: 'test-route' });
    expect(get(workflowDraft).routes[0]!.name).toBe('test-route');
  });

  it('toggles route expanded', () => {
    const draft = get(workflowDraft);
    const key = draft.routes[0]!._key;
    const initialExpanded = draft.routes[0]!.expanded;
    workflowDraft.toggleRouteExpanded(key);
    expect(get(workflowDraft).routes[0]!.expanded).toBe(!initialExpanded);
  });

  it('adds an action to a route', () => {
    const key = get(workflowDraft).routes[0]!._key;
    workflowDraft.addAction(key);
    expect(get(workflowDraft).routes[0]!.actions).toHaveLength(1);
  });

  it('removes an action from a route', () => {
    const key = get(workflowDraft).routes[0]!._key;
    workflowDraft.addAction(key);
    const actionKey = get(workflowDraft).routes[0]!.actions[0]!._key;
    workflowDraft.removeAction(key, actionKey);
    expect(get(workflowDraft).routes[0]!.actions).toHaveLength(0);
  });

  it('updates an action type', () => {
    const key = get(workflowDraft).routes[0]!._key;
    workflowDraft.addAction(key);
    const actionKey = get(workflowDraft).routes[0]!.actions[0]!._key;
    workflowDraft.updateAction(key, actionKey, { type: 'webhook' });
    expect(get(workflowDraft).routes[0]!.actions[0]!.type).toBe('webhook');
  });

  it('moves routes up and down', () => {
    workflowDraft.addRoute();
    const draft = get(workflowDraft);
    const firstKey = draft.routes[0]!._key;
    const secondKey = draft.routes[1]!._key;

    workflowDraft.moveRoute(firstKey, 'down');
    const after = get(workflowDraft);
    expect(after.routes[0]!._key).toBe(secondKey);
    expect(after.routes[1]!._key).toBe(firstKey);
  });

  it('sets event types on a route filter', () => {
    const key = get(workflowDraft).routes[0]!._key;
    workflowDraft.setEventTypes(key, ['PATIENT_ADMIT', 'LAB_RESULT']);
    expect(get(workflowDraft).routes[0]!.filter.eventTypes).toEqual([
      'PATIENT_ADMIT',
      'LAB_RESULT'
    ]);
  });

  it('sets CEL condition', () => {
    const key = get(workflowDraft).routes[0]!._key;
    workflowDraft.setCondition(key, 'event.isCritical == true');
    expect(get(workflowDraft).routes[0]!.filter.condition).toBe('event.isCritical == true');
  });

  it('loads a full draft', () => {
    workflowDraft.loadDraft({
      name: 'loaded',
      version: '2.0',
      routes: []
    });
    const draft = get(workflowDraft);
    expect(draft.name).toBe('loaded');
    expect(draft.version).toBe('2.0');
  });

  describe('isWorkflowValid', () => {
    it('returns false for empty name', () => {
      expect(get(isWorkflowValid)).toBe(false);
    });

    it('returns false with no routes', () => {
      workflowDraft.loadDraft({ name: 'test', version: '1.0', routes: [] });
      expect(get(isWorkflowValid)).toBe(false);
    });

    it('returns true for valid workflow', () => {
      const key = get(workflowDraft).routes[0]!._key;
      workflowDraft.update((d) => ({ ...d, name: 'test' }));
      workflowDraft.updateRoute(key, { name: 'route1' });
      workflowDraft.addAction(key);
      expect(get(isWorkflowValid)).toBe(true);
    });
  });

  describe('routeCount', () => {
    it('reflects route count', () => {
      expect(get(routeCount)).toBe(1);
      workflowDraft.addRoute();
      expect(get(routeCount)).toBe(2);
    });
  });
});
