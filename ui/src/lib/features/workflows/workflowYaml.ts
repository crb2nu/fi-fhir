import yaml from 'js-yaml';
import type { WorkflowDraft, RouteDraft, ActionDraft, FilterDraft } from './workflowTypes';
import { genKey } from './workflowTypes';

// ─── Draft → YAML ──────────────────────────────────────────────────────────

type YamlAction = { type: string; [key: string]: string };
type YamlFilter = {
  event_type?: string | string[];
  source?: string | string[];
  condition?: string;
};
type YamlRoute = {
  name: string;
  filter: YamlFilter;
  actions: YamlAction[];
};
type YamlWorkflow = {
  name: string;
  version: string;
  routes: YamlRoute[];
};

/**
 * Converts a WorkflowDraft to YAML string.
 */
export function draftToYaml(draft: WorkflowDraft): string {
  const wf: YamlWorkflow = {
    name: draft.name || 'untitled',
    version: draft.version || '1.0',
    routes: draft.routes.map(routeToYaml)
  };
  return yaml.dump(wf, { indent: 2, lineWidth: 120, noRefs: true });
}

function routeToYaml(route: RouteDraft): YamlRoute {
  const filter: YamlFilter = {};

  if (route.filter.eventTypes.length === 1) {
    filter.event_type = route.filter.eventTypes[0]!;
  } else if (route.filter.eventTypes.length > 1) {
    filter.event_type = route.filter.eventTypes;
  }

  if (route.filter.sources.length === 1) {
    filter.source = route.filter.sources[0]!;
  } else if (route.filter.sources.length > 1) {
    filter.source = route.filter.sources;
  }

  if (route.filter.condition) {
    filter.condition = route.filter.condition;
  }

  return {
    name: route.name || 'unnamed',
    filter,
    actions: route.actions.map(actionToYaml)
  };
}

function actionToYaml(action: ActionDraft): YamlAction {
  const result: YamlAction = { type: action.type };
  for (const [k, v] of Object.entries(action.config)) {
    if (v) result[k] = v;
  }
  return result;
}

// ─── YAML → Draft ──────────────────────────────────────────────────────────

/**
 * Parses a YAML string into a WorkflowDraft.
 * Handles both top-level and `workflow:` wrapped formats.
 */
export function yamlToDraft(yamlStr: string): WorkflowDraft {
  const parsed = yaml.load(yamlStr) as Record<string, unknown>;
  if (!parsed || typeof parsed !== 'object') {
    throw new Error('Invalid YAML: expected an object');
  }

  // Handle `workflow:` wrapper
  const wf = (parsed as { workflow?: Record<string, unknown> }).workflow ?? parsed;

  const name = String(wf.name ?? '');
  const version = String(wf.version ?? '1.0');
  const rawRoutes = (wf.routes ?? []) as Record<string, unknown>[];

  return {
    name,
    version,
    routes: rawRoutes.map(parseRoute)
  };
}

function parseRoute(raw: Record<string, unknown>): RouteDraft {
  const filter = (raw.filter ?? {}) as Record<string, unknown>;
  const actions = (raw.actions ?? []) as Record<string, unknown>[];

  return {
    _key: genKey(),
    name: String(raw.name ?? ''),
    filter: parseFilter(filter),
    transforms: [],
    actions: actions.map(parseAction),
    expanded: false
  };
}

function parseFilter(raw: Record<string, unknown>): FilterDraft {
  return {
    eventTypes: toStringArray(raw.event_type),
    sources: toStringArray(raw.source),
    condition: String(raw.condition ?? '')
  };
}

function parseAction(raw: Record<string, unknown>): ActionDraft {
  const type = String(raw.type ?? 'log');
  const config: Record<string, string> = {};
  for (const [k, v] of Object.entries(raw)) {
    if (k !== 'type' && v !== undefined && v !== null) {
      config[k] = String(v);
    }
  }
  return { _key: genKey(), type, config };
}

function toStringArray(value: unknown): string[] {
  if (!value) return [];
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.map(String);
  return [];
}
