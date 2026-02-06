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

export type TransformDraft = {
  _key: string;
  setField: string;
  mapTerminology?: { field: string; from: string; to: string };
  redact?: { fields: string[] };
};

export type ActionDraft = {
  _key: string;
  type: string;
  config: Record<string, string>;
};

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

export function createEmptyAction(): ActionDraft {
  return { _key: genKey(), type: 'log', config: {} };
}

export function createEmptyWorkflow(): WorkflowDraft {
  return { name: '', version: '1.0', routes: [createEmptyRoute()] };
}
