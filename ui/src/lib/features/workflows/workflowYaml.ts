import yaml from 'js-yaml';
import type {
  WorkflowDraft,
  RouteDraft,
  ActionDraft,
  FilterDraft,
  TransformDraft,
  TransformType,
  ScalarType,
  YamlOnlyValues
} from './workflowTypes';
import { ACTION_FIELDS, genKey } from './workflowTypes';

// The draft model is faithful: every key a YAML document carries either has a
// builder control or is kept verbatim in a `yamlOnly` (or `raw`) bag and
// written back. So yamlToDraft → draftToYaml never drops configuration, and a
// baseline computed that way can no longer hide a divergence (`.loom/42` E-5).

// ─── Scalars: text in the builder, their own type in YAML ──────────────────

/** Stores a scalar YAML value as builder text, remembering a non-string type. */
function recordScalar(
  config: Record<string, string>,
  types: Record<string, ScalarType>,
  key: string,
  value: unknown
): void {
  if (value === null) {
    config[key] = '';
    types[key] = 'null';
  } else if (value === '') {
    config[key] = '';
    types[key] = 'empty';
  } else if (typeof value === 'number') {
    config[key] = String(value);
    types[key] = 'number';
  } else if (typeof value === 'boolean') {
    config[key] = String(value);
    types[key] = 'boolean';
  } else {
    config[key] = String(value);
  }
}

/**
 * The YAML value for builder text `value` whose original type was `type`, or
 * `undefined` to omit the key (a field the operator left or made empty).
 */
function emitScalar(value: string, type: ScalarType | undefined): unknown {
  if (value === '') {
    if (type === 'null') return null;
    if (type === 'empty') return '';
    return undefined;
  }
  if (type === 'number' && Number.isFinite(Number(value)) && String(Number(value)) === value) {
    return Number(value);
  }
  if (type === 'boolean' && (value === 'true' || value === 'false')) return value === 'true';
  return value;
}

function withTypes<T extends { scalarTypes?: Record<string, ScalarType> }>(
  draft: T,
  types: Record<string, ScalarType>
): T {
  if (Object.keys(types).length > 0) draft.scalarTypes = types;
  return draft;
}

/** yamlOnly keys never override a setting the builder holds a value for. */
function mergeYamlOnly(target: Record<string, unknown>, bag: YamlOnlyValues | undefined): Record<string, unknown> {
  for (const [key, value] of Object.entries(bag ?? {})) {
    if (!(key in target)) target[key] = value;
  }
  return target;
}

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
    ...(draft.yamlOnly && 'routes' in draft.yamlOnly && draft.routes.length === 0
      ? {}
      : { routes: draft.routes.map(routeToYaml) })
  };
  mergeYamlOnly(wf, draft.yamlOnly);
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
  // A non-list `actions:` the builder could not read is kept in yamlOnly.
  if (route.actions.length > 0 || !(route.yamlOnly && 'actions' in route.yamlOnly)) {
    result.actions = route.actions.map(actionToYaml);
  }

  return mergeYamlOnly(result, route.yamlOnly);
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
      const types = transform.scalarTypes ?? {};
      for (const key of EXPLAIN_WARNINGS_KEYS) {
        const text = transform.config[key];
        if (text === undefined) continue;
        // The two flags are booleans in the engine whatever the operator typed.
        const type = key === 'include_fix' || key === 'enable_cache' ? (types[key] ?? 'boolean') : types[key];
        const value =
          type === 'boolean' && text !== '' && text !== 'true' && text !== 'false' ? text === 'true' : emitScalar(text, type);
        if (value !== undefined) ew[key] = value;
      }
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
  const types = action.scalarTypes ?? {};
  for (const [k, v] of Object.entries(action.config)) {
    const value = emitScalar(v, types[k]);
    if (value !== undefined) result[k] = value;
  }
  return mergeYamlOnly(result, action.yamlOnly);
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

  // List items that are not maps cannot be represented and are dropped; the
  // API parses every saved version into the engine's structs first, which
  // refuses such items, so a stored version never carries one.
  const draft: WorkflowDraft = {
    name,
    version,
    routes: rawRoutes.filter(isRecord).map(parseRoute)
  };
  // A `routes:` that is not a list is kept as written.
  const yamlOnly = rest(wf, Array.isArray(wf.routes) ? ['name', 'version', 'routes'] : ['name', 'version']);
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
  // A `transform:` or `actions:` that is not a list is kept as written.
  const known = ['name', 'filter'];
  if (raw.transform === undefined || Array.isArray(raw.transform)) known.push('transform');
  if (raw.actions === undefined || Array.isArray(raw.actions)) known.push('actions');
  const yamlOnly = rest(raw, known);
  if (yamlOnly) route.yamlOnly = yamlOnly;
  return route;
}

const TRANSFORM_KEYS = ['set_field', 'map_terminology', 'redact', 'explain_warnings'] as const;
const EXPLAIN_WARNINGS_KEYS = ['model', 'warnings_field', 'include_fix', 'enable_cache', 'cache_ttl'] as const;

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
  const types: Record<string, ScalarType> = {};
  for (const key of EXPLAIN_WARNINGS_KEYS) {
    if (ew[key] === undefined || isRecord(ew[key]) || Array.isArray(ew[key])) continue;
    recordScalar(config, types, key, ew[key]);
  }
  const known = EXPLAIN_WARNINGS_KEYS.filter((key) => key in config);
  return withBags(
    withTypes<TransformDraft>({ _key: genKey(), type: 'explain_warnings', config }, types),
    raw,
    'explain_warnings',
    ew,
    known
  );
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
  const types: Record<string, ScalarType> = {};
  const yamlOnly: YamlOnlyValues = {};
  for (const [k, v] of Object.entries(raw)) {
    if (k === 'type' || v === undefined) continue;
    if (typeof v === 'object' && v !== null) {
      // Nested maps and lists: never stringified ("[object Object]").
      yamlOnly[k] = v;
    } else {
      recordScalar(config, types, k, v);
    }
  }
  const action = withTypes<ActionDraft>({ _key: genKey(), type, config }, types);
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
