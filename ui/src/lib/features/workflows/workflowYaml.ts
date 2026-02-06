import yaml from 'js-yaml';
import type {
  WorkflowDraft,
  RouteDraft,
  ActionDraft,
  FilterDraft,
  TransformDraft,
  TransformType
} from './workflowTypes';
import { genKey } from './workflowTypes';

// ─── Draft → YAML ──────────────────────────────────────────────────────────

type YamlAction = { type: string; [key: string]: string };
type YamlTransform = Record<string, unknown>;
type YamlFilter = {
  event_type?: string | string[];
  source?: string | string[];
  condition?: string;
};
type YamlRoute = {
  name: string;
  filter: YamlFilter;
  transform?: YamlTransform[];
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

  const result: YamlRoute = {
    name: route.name || 'unnamed',
    filter,
    actions: route.actions.map(actionToYaml)
  };

  if (route.transforms.length > 0) {
    result.transform = route.transforms.map(transformToYaml);
  }

  return result;
}

function transformToYaml(transform: TransformDraft): YamlTransform {
  switch (transform.type) {
    case 'set_field':
      return { set_field: transform.config.expression ?? '' };
    case 'map_terminology':
      return {
        map_terminology: {
          field: transform.config.field ?? '',
          from: transform.config.from ?? '',
          to: transform.config.to ?? ''
        }
      };
    case 'redact':
      return {
        redact: {
          fields: (transform.config.fields ?? '').split(',').map((s) => s.trim()).filter(Boolean)
        }
      };
    case 'explain_warnings': {
      const ew: Record<string, unknown> = {};
      if (transform.config.model) ew.model = transform.config.model;
      if (transform.config.warnings_field) ew.warnings_field = transform.config.warnings_field;
      if (transform.config.include_fix) ew.include_fix = transform.config.include_fix === 'true';
      if (transform.config.enable_cache) ew.enable_cache = transform.config.enable_cache === 'true';
      if (transform.config.cache_ttl) ew.cache_ttl = transform.config.cache_ttl;
      return { explain_warnings: ew };
    }
    default:
      return {};
  }
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
  const transforms = (raw.transform ?? []) as Record<string, unknown>[];

  return {
    _key: genKey(),
    name: String(raw.name ?? ''),
    filter: parseFilter(filter),
    transforms: transforms.map(parseTransform),
    actions: actions.map(parseAction),
    expanded: false
  };
}

function parseTransform(raw: Record<string, unknown>): TransformDraft {
  if ('set_field' in raw) {
    return {
      _key: genKey(),
      type: 'set_field',
      config: { expression: String(raw.set_field ?? '') }
    };
  }
  if ('map_terminology' in raw) {
    const mt = (raw.map_terminology ?? {}) as Record<string, unknown>;
    return {
      _key: genKey(),
      type: 'map_terminology',
      config: {
        field: String(mt.field ?? ''),
        from: String(mt.from ?? ''),
        to: String(mt.to ?? '')
      }
    };
  }
  if ('redact' in raw) {
    const rd = (raw.redact ?? {}) as Record<string, unknown>;
    const fields = Array.isArray(rd.fields) ? rd.fields.map(String).join(', ') : '';
    return {
      _key: genKey(),
      type: 'redact',
      config: { fields }
    };
  }
  if ('explain_warnings' in raw) {
    const ew = (raw.explain_warnings ?? {}) as Record<string, unknown>;
    const config: Record<string, string> = {};
    if (ew.model) config.model = String(ew.model);
    if (ew.warnings_field) config.warnings_field = String(ew.warnings_field);
    if (ew.include_fix !== undefined) config.include_fix = String(ew.include_fix);
    if (ew.enable_cache !== undefined) config.enable_cache = String(ew.enable_cache);
    if (ew.cache_ttl) config.cache_ttl = String(ew.cache_ttl);
    return {
      _key: genKey(),
      type: 'explain_warnings',
      config
    };
  }
  // Fallback
  return { _key: genKey(), type: 'set_field' as TransformType, config: {} };
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
