/**
 * Type definitions for the Workflow Builder UI.
 * Mirrors the Go backend types in internal/workflow/types.go.
 */

// ─── Draft types (client-side builder state) ───────────────────────────────

/**
 * Keys a YAML document carried that the builder has no control for. They are
 * kept verbatim and written back by `draftToYaml`, so loading a version and
 * saving it again never drops them (`.loom/42` E-5). The builder lists them in
 * its "YAML-only fields" notice.
 */
export type YamlOnlyValues = Record<string, unknown>;

/**
 * The YAML type a scalar setting had before the builder turned it into text
 * (`retries: 3`, `enabled: true`, `key: null`, `key: ''`), so `draftToYaml`
 * writes the same type back while the value is unchanged.
 */
export type ScalarType = 'number' | 'boolean' | 'null' | 'empty';

export type WorkflowDraft = {
  name: string;
  version: string;
  routes: RouteDraft[];
  /** Top-level keys other than name, version and routes. */
  yamlOnly?: YamlOnlyValues;
};

export type RouteDraft = {
  /** Unique key for Svelte keyed each blocks */
  _key: string;
  name: string;
  filter: FilterDraft;
  transforms: TransformDraft[];
  actions: ActionDraft[];
  expanded: boolean;
  /** Route keys other than name, filter, transform and actions. */
  yamlOnly?: YamlOnlyValues;
};

export type FilterDraft = {
  eventTypes: string[];
  sources: string[];
  condition: string;
  /** Filter keys other than event_type, source and condition. */
  yamlOnly?: YamlOnlyValues;
};

export type TransformType = 'set_field' | 'map_terminology' | 'redact' | 'explain_warnings';

export type TransformDraft = {
  _key: string;
  type: TransformType;
  config: Record<string, string>;
  /** Keys beside the transform's own key on the same list item. */
  yamlOnly?: YamlOnlyValues;
  /** Keys inside the transform's block the builder has no field for. */
  innerYamlOnly?: YamlOnlyValues;
  /** Original YAML types of `config` values that were not strings. */
  scalarTypes?: Record<string, ScalarType>;
  /**
   * A transform the builder cannot represent at all (an unknown kind, or a
   * value of the wrong shape): the list item exactly as written. `type` and
   * `config` are placeholders and `draftToYaml` writes `raw` back.
   */
  raw?: YamlOnlyValues;
};

export type ActionDraft = {
  _key: string;
  type: string;
  /** Scalar settings (strings; numbers and booleans as their text). */
  config: Record<string, string>;
  /** Original YAML types of `config` values that were not strings. */
  scalarTypes?: Record<string, ScalarType>;
  /**
   * Nested maps and lists. The engine reads action settings as flat scalars
   * (`Action.UnmarshalYAML` in internal/workflow/types.go), so these have no
   * effect at runtime; they are kept so the document round-trips.
   */
  yamlOnly?: YamlOnlyValues;
};

// ─── Transform field registry ──────────────────────────────────────────────

export const TRANSFORM_FIELDS: Record<TransformType, ActionFieldDef[]> = {
  set_field: [
    { key: 'expression', label: 'Expression', placeholder: 'event.status = "processed"', required: true }
  ],
  map_terminology: [
    { key: 'field', label: 'Field', placeholder: 'code', required: true },
    { key: 'from', label: 'From System', placeholder: 'ICD-10', required: true },
    { key: 'to', label: 'To System', placeholder: 'SNOMED-CT', required: true }
  ],
  redact: [
    { key: 'fields', label: 'Fields (comma-separated)', placeholder: 'ssn, dob, name', required: true }
  ],
  explain_warnings: [
    { key: 'model', label: 'Model', placeholder: 'gpt-4' },
    { key: 'warnings_field', label: 'Warnings Field', placeholder: 'warnings' },
    { key: 'include_fix', label: 'Include Fix', placeholder: 'true' },
    { key: 'enable_cache', label: 'Enable Cache', placeholder: 'true' },
    { key: 'cache_ttl', label: 'Cache TTL', placeholder: '24h' }
  ]
};

/** All available transform types. */
export const TRANSFORM_TYPES = Object.keys(TRANSFORM_FIELDS) as TransformType[];

// ─── Action field registry ─────────────────────────────────────────────────

export type ActionFieldDef = {
  key: string;
  label: string;
  placeholder?: string;
  required?: boolean;
};

/**
 * Registry of config fields per action type.
 * Matches the backend action types from engine.go.
 */
export const ACTION_FIELDS: Record<string, ActionFieldDef[]> = {
  log: [
    { key: 'level', label: 'Level', placeholder: 'info' },
    { key: 'message', label: 'Message', placeholder: 'Event processed' }
  ],
  webhook: [
    { key: 'url', label: 'URL', placeholder: 'https://...', required: true },
    { key: 'method', label: 'Method', placeholder: 'POST' },
    { key: 'headers', label: 'Headers', placeholder: 'Content-Type: application/json' }
  ],
  fhir: [
    { key: 'server', label: 'FHIR Server', placeholder: 'https://fhir.example.com', required: true },
    { key: 'resource_type', label: 'Resource Type', placeholder: 'Observation' },
    { key: 'method', label: 'Method', placeholder: 'POST' }
  ],
  email: [
    { key: 'to', label: 'To', placeholder: 'alerts@example.com', required: true },
    { key: 'subject', label: 'Subject', placeholder: 'Alert: {{.type}}' },
    { key: 'smtp_host', label: 'SMTP Host', placeholder: 'smtp.example.com' }
  ],
  exec: [
    { key: 'command', label: 'Command', placeholder: '/usr/bin/notify', required: true },
    { key: 'args', label: 'Arguments', placeholder: '--event {{.id}}' },
    { key: 'timeout', label: 'Timeout', placeholder: '30s' }
  ],
  file: [
    { key: 'path', label: 'File Path', placeholder: '/var/log/events.jsonl', required: true },
    { key: 'format', label: 'Format', placeholder: 'json' }
  ],
  database: [
    { key: 'dsn', label: 'DSN', placeholder: 'postgres://...', required: true },
    { key: 'table', label: 'Table', placeholder: 'events' },
    { key: 'query', label: 'Query', placeholder: 'INSERT INTO ...' }
  ],
  queue: [
    { key: 'broker', label: 'Broker', placeholder: 'nats://localhost:4222', required: true },
    { key: 'topic', label: 'Topic', placeholder: 'events.processed' }
  ],
  event_store: [
    { key: 'stream', label: 'Stream', placeholder: 'patient-events', required: true },
    { key: 'category', label: 'Category', placeholder: 'clinical' }
  ],
  llm_extract: [
    { key: 'model', label: 'Model', placeholder: 'gpt-4' },
    { key: 'field', label: 'Text Field', placeholder: 'notes', required: true },
    { key: 'min_confidence', label: 'Min Confidence', placeholder: '0.7' }
  ],
  llm_quality_check: [
    { key: 'model', label: 'Model', placeholder: 'gpt-4' },
    { key: 'min_score', label: 'Min Score', placeholder: '0.8' }
  ]
};

/** All available action types. */
export const ACTION_TYPES = Object.keys(ACTION_FIELDS);

/**
 * One structural problem in a workflow draft, located precisely enough for
 * the builder to put the message beside the field it is about.
 */
export type WorkflowDraftIssue = {
  /** Full sentence, "<location>: <message>" (the Problems panel's format). */
  text: string;
  /** The field the message belongs to. */
  field:
    | 'name'
    | 'routes'
    | 'route.name'
    | 'route.actions'
    | 'transform.field'
    | 'action.type'
    | 'action.field';
  message: string;
  routeKey?: string;
  transformKey?: string;
  actionKey?: string;
  /** For transform.field / action.field: the config key. */
  configKey?: string;
};

/**
 * Structural validation of a workflow draft, one issue per problem, each
 * pointing at a route, transform or action by its builder key.
 */
export function collectWorkflowDraftIssues(draft: WorkflowDraft): WorkflowDraftIssue[] {
  const issues: WorkflowDraftIssue[] = [];
  const push = (issue: Omit<WorkflowDraftIssue, 'text'>, location?: string) => {
    issues.push({ ...issue, text: location ? `${location}: ${issue.message}` : issue.message });
  };

  if (!draft.name.trim()) {
    push({ field: 'name', message: 'Workflow name is required' });
  }

  if (draft.routes.length === 0) {
    push({ field: 'routes', message: 'At least one route is required' });
  }

  for (let i = 0; i < draft.routes.length; i += 1) {
    const route = draft.routes[i]!;
    const routeLabel = route.name.trim() || `Route ${i + 1}`;
    const routeKey = route._key;

    if (!route.name.trim()) {
      push({ field: 'route.name', routeKey, message: 'route name is required' }, routeLabel);
    }

    if (route.actions.length === 0) {
      push({ field: 'route.actions', routeKey, message: 'at least one action is required' }, routeLabel);
    }

    for (let j = 0; j < route.transforms.length; j += 1) {
      const transform = route.transforms[j]!;
      // A transform kept verbatim from YAML is not the builder's to judge.
      if (transform.raw) continue;
      const transformLabel = `${routeLabel}, transform ${j + 1}`;
      const at = (configKey: string, message: string) =>
        push(
          { field: 'transform.field', routeKey, transformKey: transform._key, configKey, message },
          transformLabel
        );
      if (transform.type === 'set_field' && !(transform.config.expression ?? '').trim()) {
        at('expression', 'expression is required');
      }
      if (transform.type === 'map_terminology') {
        if (!(transform.config.field ?? '').trim()) at('field', 'field is required');
        if (!(transform.config.from ?? '').trim()) at('from', 'from system is required');
        if (!(transform.config.to ?? '').trim()) at('to', 'to system is required');
      }
      if (transform.type === 'redact' && !(transform.config.fields ?? '').trim()) {
        at('fields', 'fields are required');
      }
    }

    for (let j = 0; j < route.actions.length; j += 1) {
      const action = route.actions[j]!;
      const actionLabel = `${routeLabel}, action ${j + 1}`;
      if (!action.type.trim()) {
        push(
          { field: 'action.type', routeKey, actionKey: action._key, message: 'action type is required' },
          actionLabel
        );
        continue;
      }

      const defs = ACTION_FIELDS[action.type] ?? [];
      for (const def of defs) {
        if (!def.required) continue;
        const value = action.config[def.key] ?? '';
        if (!value.trim()) {
          push(
            {
              field: 'action.field',
              routeKey,
              actionKey: action._key,
              configKey: def.key,
              message: `${def.label} is required`
            },
            actionLabel
          );
        }
      }
    }
  }

  return issues;
}

/**
 * Validate a workflow draft for import/edit execution readiness.
 * Returns a list of human-readable issues; empty means valid.
 */
export function validateWorkflowDraft(draft: WorkflowDraft): string[] {
  return collectWorkflowDraftIssues(draft).map((issue) => issue.text);
}

// ─── Event type presets ────────────────────────────────────────────────────

export type EventTypePreset = {
  label: string;
  types: string[];
};

/**
 * Grouped event type presets matching the GraphQL EventType enum.
 */
export const EVENT_TYPE_CATEGORIES: Record<string, string[]> = {
  'ADT (Patient Flow)': [
    'PATIENT_ADMIT',
    'PATIENT_DISCHARGE',
    'PATIENT_TRANSFER',
    'PATIENT_UPDATE'
  ],
  'Lab / Results': ['LAB_RESULT', 'LAB_ORDERED'],
  Scheduling: ['APPOINTMENT_SCHEDULED', 'APPOINTMENT_CANCELLED', 'APPOINTMENT_NOSHOW'],
  'Claims / Financial': ['CLAIM_SUBMITTED', 'CLAIM_ADJUDICATED'],
  Clinical: ['VITAL_SIGN', 'CONDITION', 'PROCEDURE', 'IMMUNIZATION', 'DOCUMENT']
};

/** Flat list of all event types. */
export const ALL_EVENT_TYPES = Object.values(EVENT_TYPE_CATEGORIES).flat();

export const EVENT_TYPE_PRESETS: EventTypePreset[] = [
  { label: 'All ADT Events', types: EVENT_TYPE_CATEGORIES['ADT (Patient Flow)']! },
  { label: 'All Lab Events', types: EVENT_TYPE_CATEGORIES['Lab / Results']! },
  { label: 'All Clinical', types: EVENT_TYPE_CATEGORIES['Clinical']! },
  { label: 'All Events', types: ALL_EVENT_TYPES }
];

// ─── Helpers ───────────────────────────────────────────────────────────────

let nextKey = 0;
export function genKey(): string {
  return `_k${++nextKey}`;
}

export function createEmptyRoute(): RouteDraft {
  return {
    _key: genKey(),
    name: '',
    filter: { eventTypes: [], sources: [], condition: '' },
    transforms: [],
    actions: [],
    expanded: true
  };
}

export function createEmptyTransform(): TransformDraft {
  return { _key: genKey(), type: 'set_field', config: {} };
}

export function createEmptyAction(): ActionDraft {
  return { _key: genKey(), type: 'log', config: {} };
}

export function createEmptyWorkflow(): WorkflowDraft {
  return { name: '', version: '1.0', routes: [createEmptyRoute()] };
}
