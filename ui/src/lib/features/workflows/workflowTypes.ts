/**
 * Type definitions for the Workflow Builder UI.
 * Mirrors the Go backend types in internal/workflow/types.go.
 */

// ─── Draft types (client-side builder state) ───────────────────────────────

export type WorkflowDraft = {
  name: string;
  version: string;
  routes: RouteDraft[];
};

export type RouteDraft = {
  /** Unique key for Svelte keyed each blocks */
  _key: string;
  name: string;
  filter: FilterDraft;
  transforms: TransformDraft[];
  actions: ActionDraft[];
  expanded: boolean;
};

export type FilterDraft = {
  eventTypes: string[];
  sources: string[];
  condition: string;
};

export type TransformType = 'set_field' | 'map_terminology' | 'redact' | 'explain_warnings';

export type TransformDraft = {
  _key: string;
  type: TransformType;
  config: Record<string, string>;
};

export type ActionDraft = {
  _key: string;
  type: string;
  config: Record<string, string>;
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
 * Validate a workflow draft for import/edit execution readiness.
 * Returns a list of human-readable issues; empty means valid.
 */
export function validateWorkflowDraft(draft: WorkflowDraft): string[] {
  const issues: string[] = [];

  if (!draft.name.trim()) {
    issues.push('Workflow name is required');
  }

  if (draft.routes.length === 0) {
    issues.push('At least one route is required');
  }

  for (let i = 0; i < draft.routes.length; i += 1) {
    const route = draft.routes[i]!;
    const routeLabel = route.name.trim() || `Route ${i + 1}`;

    if (!route.name.trim()) {
      issues.push(`${routeLabel}: route name is required`);
    }

    if (route.actions.length === 0) {
      issues.push(`${routeLabel}: at least one action is required`);
    }

    for (let j = 0; j < route.transforms.length; j += 1) {
      const transform = route.transforms[j]!;
      const transformLabel = `${routeLabel}, transform ${j + 1}`;
      if (transform.type === 'set_field' && !(transform.config.expression ?? '').trim()) {
        issues.push(`${transformLabel}: expression is required`);
      }
      if (transform.type === 'map_terminology') {
        if (!(transform.config.field ?? '').trim()) issues.push(`${transformLabel}: field is required`);
        if (!(transform.config.from ?? '').trim()) issues.push(`${transformLabel}: from system is required`);
        if (!(transform.config.to ?? '').trim()) issues.push(`${transformLabel}: to system is required`);
      }
      if (transform.type === 'redact' && !(transform.config.fields ?? '').trim()) {
        issues.push(`${transformLabel}: fields are required`);
      }
    }

    for (let j = 0; j < route.actions.length; j += 1) {
      const action = route.actions[j]!;
      const actionLabel = `${routeLabel}, action ${j + 1}`;
      if (!action.type.trim()) {
        issues.push(`${actionLabel}: action type is required`);
        continue;
      }

      const defs = ACTION_FIELDS[action.type] ?? [];
      for (const def of defs) {
        if (!def.required) continue;
        const value = action.config[def.key] ?? '';
        if (!value.trim()) {
          issues.push(`${actionLabel}: ${def.label} is required`);
        }
      }
    }
  }

  return issues;
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
