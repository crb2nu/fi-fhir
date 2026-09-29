import yaml from 'js-yaml';
import type {
  WorkflowDraft,
  RouteDraft,
  ActionDraft,
  FilterDraft,
  TransformDraft,
  TransformType,
  YamlOnlyValues
} from './workflowTypes';
import { ACTION_FIELDS, genKey } from './workflowTypes';

// The draft model is faithful: every key a YAML document carries either has a
// builder control or is kept verbatim in a `yamlOnly` (or `raw`) bag and
// written back. So yamlToDraft → draftToYaml never drops configuration, and a
// baseline computed that way can no longer hide a divergence (`.loom/42` E-5).

// ─── Draft → YAML ──────────────────────────────────────────────────────────

type YamlTransform = Record<string, unknown>;
type YamlRoute = Record<string, unknown>;

/**
 * Converts a WorkflowDraft to YAML string.
 */
export function draftToYaml(draft: WorkflowDraft): string {
  const wf: Record<string, unknown> = {
    name: draft.name || 'untitled',
    version: draft.version || '1.0',
    routes: draft.routes.map(routeToYaml),
    ...(draft.yamlOnly ?? {})
  };
  return yaml.dump(wf, { indent: 2, lineWidth: 120, noRefs: true });
}

function routeToYaml(route: RouteDraft): YamlRoute {
  const filter: Record<string, unknown> = {};

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
  Object.assign(filter, route.filter.yamlOnly ?? {});

  const result: YamlRoute = {
    name: route.name || 'unnamed',
    filter
  };

  if (route.transforms.length > 0) {
    result.transform = route.transforms.map(transformToYaml);
  }
  result.actions = route.actions.map(actionToYaml);

  return { ...result, ...(route.yamlOnly ?? {}) };
}

function transformToYaml(transform: TransformDraft): YamlTransform {
  if (transform.raw) return { ...transform.raw };
  const inner = transform.innerYamlOnly ?? {};
  let own: YamlTransform;
  switch (transform.type) {
    case 'set_field':
      own = { set_field: transform.config.expression ?? '' };
      break;
    case 'map_terminology':
      own = {
        map_terminology: {
          field: transform.config.field ?? '',
          from: transform.config.from ?? '',
          to: transform.config.to ?? '',
          ...inner
        }
      };
      break;
    case 'redact':
      own = {
        redact: {
          fields: (transform.config.fields ?? '').split(',').map((s) => s.trim()).filter(Boolean),
          ...inner
        }
      };
      break;
    case 'explain_warnings': {
      const ew: Record<string, unknown> = {};
      if (transform.config.model) ew.model = transform.config.model;
      if (transform.config.warnings_field) ew.warnings_field = transform.config.warnings_field;
      if (transform.config.include_fix) ew.include_fix = transform.config.include_fix === 'true';
      if (transform.config.enable_cache) ew.enable_cache = transform.config.enable_cache === 'true';
      if (transform.config.cache_ttl) ew.cache_ttl = transform.config.cache_ttl;
      own = { explain_warnings: { ...ew, ...inner } };
      break;
    }
    default:
      own = {};
  }
  return { ...own, ...(transform.yamlOnly ?? {}) };
}

function actionToYaml(action: ActionDraft): Record<string, unknown> {
  const result: Record<string, unknown> = { type: action.type };
  for (const [k, v] of Object.entries(action.config)) {
    if (v) result[k] = v;
  }
  return { ...result, ...(action.yamlOnly ?? {}) };
}

// ─── YAML → Draft ──────────────────────────────────────────────────────────

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** The entries of `raw` whose keys are not in `known`, or undefined if none. */
function rest(raw: Record<string, unknown>, known: readonly string[]): YamlOnlyValues | undefined {
  const out: YamlOnlyValues = {};
  for (const [k, v] of Object.entries(raw)) {
    if (!known.includes(k)) out[k] = v;
  }
  return Object.keys(out).length > 0 ? out : undefined;
}

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
  const wrapped = (parsed as { workflow?: unknown }).workflow;
  const wf = isRecord(wrapped) ? wrapped : parsed;

  const name = String(wf.name ?? '');
  const version = String(wf.version ?? '1.0');
  const rawRoutes = Array.isArray(wf.routes) ? (wf.routes as unknown[]) : [];

  const draft: WorkflowDraft = {
    name,
    version,
    routes: rawRoutes.filter(isRecord).map(parseRoute)
  };
  const yamlOnly = rest(wf, ['name', 'version', 'routes']);
  if (yamlOnly) draft.yamlOnly = yamlOnly;
  return draft;
}

function parseRoute(raw: Record<string, unknown>): RouteDraft {
  const filter = isRecord(raw.filter) ? raw.filter : {};
  const actions = Array.isArray(raw.actions) ? (raw.actions as unknown[]).filter(isRecord) : [];
  const transforms = Array.isArray(raw.transform) ? (raw.transform as unknown[]).filter(isRecord) : [];

  const route: RouteDraft = {
    _key: genKey(),
    name: String(raw.name ?? ''),
    filter: parseFilter(filter),
    transforms: transforms.map(parseTransform),
    actions: actions.map(parseAction),
    expanded: false
  };
  const yamlOnly = rest(raw, ['name', 'filter', 'transform', 'actions']);
  if (yamlOnly) route.yamlOnly = yamlOnly;
  return route;
}

const TRANSFORM_KEYS = ['set_field', 'map_terminology', 'redact', 'explain_warnings'] as const;

function withBags(
  draft: TransformDraft,
  raw: Record<string, unknown>,
  ownKey: string,
  inner?: Record<string, unknown>,
  innerKnown: readonly string[] = []
): TransformDraft {
  const yamlOnly = rest(raw, [ownKey]);
  if (yamlOnly) draft.yamlOnly = yamlOnly;
  if (inner) {
    const innerYamlOnly = rest(inner, innerKnown);
    if (innerYamlOnly) draft.innerYamlOnly = innerYamlOnly;
  }
  return draft;
}

function keepRaw(raw: Record<string, unknown>): TransformDraft {
  return { _key: genKey(), type: 'set_field' as TransformType, config: {}, raw: { ...raw } };
}

function parseTransform(raw: Record<string, unknown>): TransformDraft {
  const own = TRANSFORM_KEYS.filter((key) => key in raw);
  // An unknown kind, or several kinds on one list item: keep it as written.
  if (own.length !== 1) return keepRaw(raw);

  if ('set_field' in raw) {
    const value = raw.set_field;
    if (isRecord(value) || Array.isArray(value)) return keepRaw(raw);
    return withBags(
      { _key: genKey(), type: 'set_field', config: { expression: String(value ?? '') } },
      raw,
      'set_field'
    );
  }
  if ('map_terminology' in raw) {
    if (!isRecord(raw.map_terminology)) return keepRaw(raw);
    const mt = raw.map_terminology;
    return withBags(
      {
        _key: genKey(),
        type: 'map_terminology',
        config: {
          field: String(mt.field ?? ''),
          from: String(mt.from ?? ''),
          to: String(mt.to ?? '')
        }
      },
      raw,
      'map_terminology',
      mt,
      ['field', 'from', 'to']
    );
  }
  if ('redact' in raw) {
    if (!isRecord(raw.redact)) return keepRaw(raw);
    const rd = raw.redact;
    const fields = Array.isArray(rd.fields)
      ? rd.fields.map(String).join(', ')
      : typeof rd.fields === 'string'
        ? rd.fields
        : '';
    return withBags({ _key: genKey(), type: 'redact', config: { fields } }, raw, 'redact', rd, ['fields']);
  }
  // explain_warnings
  const value = raw.explain_warnings;
  if (value !== null && value !== undefined && !isRecord(value)) return keepRaw(raw);
  const ew = isRecord(value) ? value : {};
  const config: Record<string, string> = {};
  if (ew.model) config.model = String(ew.model);
  if (ew.warnings_field) config.warnings_field = String(ew.warnings_field);
  if (ew.include_fix !== undefined) config.include_fix = String(ew.include_fix);
  if (ew.enable_cache !== undefined) config.enable_cache = String(ew.enable_cache);
  if (ew.cache_ttl) config.cache_ttl = String(ew.cache_ttl);
  return withBags({ _key: genKey(), type: 'explain_warnings', config }, raw, 'explain_warnings', ew, [
    'model',
    'warnings_field',
    'include_fix',
    'enable_cache',
    'cache_ttl'
  ]);
}

function parseFilter(raw: Record<string, unknown>): FilterDraft {
  const filter: FilterDraft = {
    eventTypes: toStringArray(raw.event_type),
    sources: toStringArray(raw.source),
    condition: String(raw.condition ?? '')
  };
  const yamlOnly = rest(raw, ['event_type', 'source', 'condition']);
  if (yamlOnly) filter.yamlOnly = yamlOnly;
  return filter;
}

function parseAction(raw: Record<string, unknown>): ActionDraft {
  const type = String(raw.type ?? 'log');
  const config: Record<string, string> = {};
  const yamlOnly: YamlOnlyValues = {};
  for (const [k, v] of Object.entries(raw)) {
    if (k === 'type' || v === undefined || v === null) continue;
    if (typeof v === 'object') {
      // Nested maps and lists: never stringified ("[object Object]").
      yamlOnly[k] = v;
    } else {
      config[k] = String(v);
    }
  }
  const action: ActionDraft = { _key: genKey(), type, config };
  if (Object.keys(yamlOnly).length > 0) action.yamlOnly = yamlOnly;
  return action;
}

function toStringArray(value: unknown): string[] {
  if (!value) return [];
  if (typeof value === 'string') return [value];
  if (Array.isArray(value)) return value.map(String);
  return [];
}

// ─── What the builder cannot edit ──────────────────────────────────────────

export type YamlOnlyField = {
  /** Where the key sits, e.g. `Route "admits", action 1 (log)`. */
  location: string;
  /** The key exactly as written in the YAML. */
  key: string;
  /**
   * `nested`: a map or list value (for an action, the engine does not read it).
   * `no-control`: a scalar the builder has no field for; kept as written.
   * `transform`: a whole transform the builder cannot represent.
   */
  reason: 'nested' | 'no-control' | 'transform';
};

/**
 * Every key in the draft that came from YAML and has no builder control, in
 * document order. The builder shows this list so an operator knows which
 * parts of a loaded version they can only change in YAML.
 */
export function listYamlOnlyFields(draft: WorkflowDraft): YamlOnlyField[] {
  const out: YamlOnlyField[] = [];
  const reasonFor = (value: unknown): YamlOnlyField['reason'] =>
    typeof value === 'object' && value !== null ? 'nested' : 'no-control';
  const addBag = (location: string, bag: YamlOnlyValues | undefined) => {
    for (const [key, value] of Object.entries(bag ?? {})) {
      out.push({ location, key, reason: reasonFor(value) });
    }
  };

  addBag('Workflow', draft.yamlOnly);
  draft.routes.forEach((route, i) => {
    const routeLabel = route.name.trim() ? `Route "${route.name.trim()}"` : `Route ${i + 1}`;
    addBag(routeLabel, route.yamlOnly);
    addBag(`${routeLabel}, filter`, route.filter.yamlOnly);
    route.transforms.forEach((transform, j) => {
      const label = `${routeLabel}, transform ${j + 1}`;
      if (transform.raw) {
        for (const key of Object.keys(transform.raw)) out.push({ location: label, key, reason: 'transform' });
        return;
      }
      addBag(label, transform.yamlOnly);
      addBag(`${label} (${transform.type})`, transform.innerYamlOnly);
    });
    route.actions.forEach((action, j) => {
      const label = `${routeLabel}, action ${j + 1} (${action.type})`;
      const known = new Set((ACTION_FIELDS[action.type] ?? []).map((field) => field.key));
      for (const key of Object.keys(action.config)) {
        if (!known.has(key)) out.push({ location: label, key, reason: 'no-control' });
      }
      addBag(label, action.yamlOnly);
    });
  });
  return out;
}
