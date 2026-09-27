/**
 * The per-kind connection `spec`, as data the Settings form is generated from
 * (.loom/38 "Per-kind spec"). Field for field it mirrors C-0's Go documents in
 * internal/integration/connection/spec_*.go (MLLPSpec, HTTPSpec, BatchS3Spec,
 * BatchSFTPSpec, HTTPSSpec, FHIRSpec, KafkaSpec): the same snake_case paths,
 * the same bounds as `min`/`max`, the same closed sets as options, and the
 * documented defaults — and nothing else. The server's checker stays the
 * authority; these bounds only shape the controls.
 *
 * Secrets are bindings: every `*_binding` field names a binding declared on
 * the connection, never a value.
 */
import type { ConnectionKind } from '$lib/gen/graphql';

export type SpecKind = 'mllp' | 'http' | 'batch_s3' | 'batch_sftp' | 'https' | 'fhir' | 'kafka';
export type ConnectionDirectionId = 'source' | 'destination';

export type FieldControl = 'text' | 'number' | 'select' | 'boolean' | 'list' | 'binding';

/** A field is shown only while another field has (or has not) a value. */
export interface FieldCondition {
  path: string;
  equals?: string | undefined;
  notEquals?: string | undefined;
}

export interface SpecField {
  /** Dot path relative to the spec (or to the item, inside a repeated group). */
  path: string;
  label: string;
  control: FieldControl;
  required?: boolean | undefined;
  min?: number | undefined;
  max?: number | undefined;
  /** Closed set of a `select` field, in the document's own spelling. */
  options?: readonly string[] | undefined;
  /** The documented default, prefilled on a new connection. */
  defaultValue?: string | undefined;
  placeholder?: string | undefined;
  hint?: string | undefined;
  /** A hidden field is also removed from the spec, so a forbidden field never lingers. */
  when?: FieldCondition | undefined;
  /** Spans both columns of the form grid. */
  wide?: boolean | undefined;
  /** Display labels for a `select` field's options (the value is what is written). */
  optionLabels?: Readonly<Record<string, string>> | undefined;
  /** The only value the document allows (`defaultValue`): shown read only and always written. */
  fixed?: boolean | undefined;
  /**
   * A form-only choice that is not a spec key: it drives other fields' `when`
   * and is never written. Its value is read back from the first `deriveFrom`
   * path that holds a value.
   */
  virtual?: boolean | undefined;
  deriveFrom?: ReadonlyArray<{ path: string; value: string }> | undefined;
}

/** Shown on a `*_binding` field while the connection declares no binding. */
export const BINDING_DECLARE_FIRST = 'Declare a binding in Secrets first.';

/** A labelled set of fields; `path` is the JSON object they live under ('' = top level). */
export interface SpecGroup {
  type: 'group';
  id: string;
  label: string;
  path: string;
  fields: readonly SpecField[];
}

/** A list of objects (MLLP client identities): one sub-form per item. */
export interface SpecRepeat {
  type: 'repeat';
  id: string;
  label: string;
  itemLabel: string;
  path: string;
  fields: readonly SpecField[];
}

export type SpecSection = SpecGroup | SpecRepeat;

export interface KindSchema {
  kind: SpecKind;
  graphqlKind: ConnectionKind;
  direction: ConnectionDirectionId;
  label: string;
  /** One line for the New menu. */
  summary: string;
  sections: readonly SpecSection[];
}

/**
 * Form state for the scalar and list fields: the control's text. A bound
 * `type="number"` input writes a number (or null when empty), so readers go
 * through `fieldText`.
 */
export type SpecValues = Record<string, string | number | null | undefined>;
/** Form state for repeated groups: path → items → sub-field → text. */
export type SpecRepeats = Record<string, Array<SpecValues>>;

/** A form value as text: numbers as typed, null and undefined as empty. */
export function fieldText(value: string | number | null | undefined): string {
  return value === null || value === undefined ? '' : String(value);
}

export const SECRET_PROVIDERS = ['env', 'file', 'vault', 'aws-ssm', 'k8s'] as const;

const MAX_MESSAGE_BYTES = 1_048_576;

const idHint = 'At most 256 characters, no whitespace.';

const bytes = (label: string, path: string): SpecField => ({
  path,
  label,
  control: 'number',
  required: true,
  min: 1,
  max: MAX_MESSAGE_BYTES
});

const batchSchedule: SpecGroup = {
  type: 'group',
  id: 'schedule',
  label: 'Schedule and limits',
  path: '',
  fields: [
    { path: 'poll_seconds', label: 'Poll seconds', control: 'number', required: true, min: 1, max: 3600 },
    {
      path: 'lease_seconds',
      label: 'Lease seconds',
      control: 'number',
      required: true,
      min: 1,
      max: 3600,
      hint: 'Longer than process seconds.'
    },
    { path: 'process_seconds', label: 'Process seconds', control: 'number', required: true, min: 1, max: 300 },
    {
      path: 'max_files_per_poll',
      label: 'Max files per poll',
      control: 'number',
      required: true,
      min: 1,
      max: 1000
    },
    bytes('Max message bytes', 'max_message_bytes')
  ]
};

const batchWorkload: SpecGroup = {
  type: 'group',
  id: 'workload',
  label: 'Workload identity',
  path: 'workload',
  fields: [
    { path: 'workload.subject', label: 'Subject', control: 'text', hint: 'Optional. ' + idHint },
    {
      path: 'workload.grants',
      label: 'Grants',
      control: 'list',
      placeholder: 'One grant per line',
      hint: 'Up to 16.'
    }
  ]
};

const destinationCommon: SpecGroup = {
  type: 'group',
  id: 'destination',
  label: 'Destination',
  path: '',
  fields: [
    { path: 'destination_id', label: 'Destination ID', control: 'text', required: true, hint: idHint },
    { path: 'class', label: 'Class', control: 'select', required: true, options: ['production', 'sandbox'] }
  ]
};

const destinationIdentity: SpecGroup = {
  type: 'group',
  id: 'identity',
  label: 'Client identity',
  path: 'identity',
  fields: [
    { path: 'identity.subject', label: 'Subject', control: 'text', hint: 'Optional. ' + idHint },
    {
      path: 'identity.grants',
      label: 'Grants',
      control: 'list',
      placeholder: 'One grant per line',
      hint: 'One to 16 when a subject is set.'
    }
  ]
};

/** The form-only SFTP credential choice (a `#` path is never a spec key). */
export const SFTP_CREDENTIAL = 'sftp.#credential';

const mutualTLS: FieldCondition = { path: 'tls.mode', equals: 'mutual' };
const notOAuth: FieldCondition = { path: 'auth_mode', notEquals: 'oauth2' };
const isOAuth: FieldCondition = { path: 'auth_mode', equals: 'oauth2' };

export const KIND_SCHEMAS: readonly KindSchema[] = [
  {
    kind: 'mllp',
    graphqlKind: 'MLLP',
    direction: 'source',
    label: 'MLLP',
    summary: 'HL7v2 listener over MLLP',
    sections: [
      {
        type: 'group',
        id: 'listener',
        label: 'Listener',
        path: '',
        fields: [
          { path: 'source_id', label: 'Source ID', control: 'text', required: true, hint: idHint },
          {
            path: 'listen_address',
            label: 'Listen address',
            control: 'text',
            required: true,
            placeholder: 'host:port'
          },
          { path: 'encoding', label: 'Encoding', control: 'select', required: true, options: ['utf-8'], defaultValue: 'utf-8' }
        ]
      },
      {
        type: 'group',
        id: 'framing',
        label: 'Framing',
        path: 'framing',
        fields: [
          { path: 'framing.start_byte', label: 'Start byte', control: 'number', min: 1, max: 255, defaultValue: '11' },
          { path: 'framing.end_byte', label: 'End byte', control: 'number', min: 1, max: 255, defaultValue: '28' },
          { path: 'framing.trailer_byte', label: 'Trailer byte', control: 'number', min: 1, max: 255, defaultValue: '13' }
        ]
      },
      {
        type: 'group',
        id: 'timeouts',
        label: 'Timeouts',
        path: 'timeouts',
        fields: [
          { path: 'timeouts.read_seconds', label: 'Read seconds', control: 'number', required: true, min: 1, max: 300 },
          { path: 'timeouts.write_seconds', label: 'Write seconds', control: 'number', required: true, min: 1, max: 60 },
          {
            path: 'timeouts.idle_seconds',
            label: 'Idle seconds',
            control: 'number',
            required: true,
            min: 1,
            max: 3600,
            hint: 'At least read seconds.'
          },
          {
            path: 'timeouts.process_seconds',
            label: 'Process seconds',
            control: 'number',
            required: true,
            min: 1,
            max: 300
          }
        ]
      },
      {
        type: 'group',
        id: 'tls',
        label: 'TLS',
        path: 'tls',
        fields: [
          { path: 'tls.mode', label: 'Mode', control: 'select', required: true, options: ['disabled', 'mutual'] },
          {
            path: 'tls.server_certificate_binding',
            label: 'Server certificate binding',
            control: 'binding',
            required: true,
            when: mutualTLS
          },
          {
            path: 'tls.server_private_key_binding',
            label: 'Server private key binding',
            control: 'binding',
            required: true,
            when: mutualTLS
          },
          {
            path: 'tls.client_ca_binding',
            label: 'Client CA binding',
            control: 'binding',
            required: true,
            when: mutualTLS
          }
        ]
      },
      {
        type: 'group',
        id: 'clients',
        label: 'Clients',
        path: 'clients',
        fields: [
          {
            path: 'clients.allowed_cidrs',
            label: 'Allowed CIDRs',
            control: 'list',
            required: true,
            placeholder: '10.0.0.0/8',
            hint: 'One canonical prefix per line, up to 128.',
            wide: true
          }
        ]
      },
      {
        type: 'repeat',
        id: 'identities',
        label: 'Client identities',
        itemLabel: 'Client identity',
        path: 'clients.identities',
        fields: [
          { path: 'subject', label: 'Subject', control: 'text', required: true, hint: idHint },
          { path: 'uri_san', label: 'URI SAN', control: 'text', placeholder: 'spiffe://…' },
          { path: 'spki_sha256', label: 'SPKI SHA-256', control: 'text', placeholder: 'sha256:…' },
          { path: 'grants', label: 'Grants', control: 'list', placeholder: 'One grant per line', hint: 'Up to 16.' }
        ]
      },
      {
        type: 'group',
        id: 'acknowledgements',
        label: 'Acknowledgements',
        path: 'acknowledgements',
        fields: [
          {
            path: 'acknowledgements.mode',
            label: 'Mode',
            control: 'select',
            required: true,
            options: ['application', 'commit']
          },
          {
            path: 'acknowledgements.include_error_segment',
            label: 'Include error segment',
            control: 'boolean',
            defaultValue: 'false'
          }
        ]
      },
      {
        type: 'group',
        id: 'limits',
        label: 'Limits',
        path: '',
        fields: [
          bytes('Max message bytes', 'max_message_bytes'),
          { path: 'max_connections', label: 'Max connections', control: 'number', required: true, min: 1, max: 10000 }
        ]
      }
    ]
  },
  {
    kind: 'http',
    graphqlKind: 'HTTP',
    direction: 'source',
    label: 'HTTP',
    summary: 'HL7v2 over HTTP POST',
    sections: [
      {
        type: 'group',
        id: 'endpoint',
        label: 'Endpoint',
        path: '',
        fields: [
          { path: 'source_id', label: 'Source ID', control: 'text', required: true, hint: idHint },
          { path: 'path', label: 'Path', control: 'text', defaultValue: '/v1/hl7v2', placeholder: '/v1/hl7v2' },
          {
            path: 'max_body_bytes',
            label: 'Max body bytes',
            control: 'number',
            required: true,
            min: 1,
            max: MAX_MESSAGE_BYTES
          }
        ]
      },
      {
        type: 'group',
        id: 'authentication',
        label: 'Authentication',
        path: '',
        fields: [
          {
            path: 'auth_mode',
            label: 'Auth mode',
            control: 'select',
            required: true,
            options: ['bearer', 'hmac-sha256', 'oauth2']
          },
          {
            path: 'principal_id',
            label: 'Principal ID',
            control: 'text',
            required: true,
            hint: idHint,
            when: notOAuth
          },
          {
            path: 'credential_binding',
            label: 'Credential binding',
            control: 'binding',
            required: true,
            when: notOAuth
          }
        ]
      },
      {
        type: 'group',
        id: 'oauth',
        label: 'OAuth2',
        path: 'oauth',
        fields: [
          {
            path: 'oauth.issuer_url',
            label: 'Issuer URL',
            control: 'text',
            required: true,
            placeholder: 'https://issuer.example',
            when: isOAuth
          },
          { path: 'oauth.audience', label: 'Audience', control: 'text', required: true, when: isOAuth },
          {
            path: 'oauth.tenant_claim',
            label: 'Tenant claim',
            control: 'text',
            defaultValue: 'tenant_id',
            when: isOAuth
          },
          { path: 'oauth.roles_claim', label: 'Roles claim', control: 'text', defaultValue: 'roles', when: isOAuth },
          {
            path: 'oauth.client_id_claim',
            label: 'Client ID claim',
            control: 'text',
            defaultValue: 'client_id',
            when: isOAuth
          },
          {
            path: 'oauth.signing_algs',
            label: 'Signing algorithms',
            control: 'list',
            defaultValue: 'RS256',
            hint: 'RS256, RS384, RS512, PS256, PS384, PS512, ES256, ES384, ES512 or EdDSA; one per line.',
            when: isOAuth
          },
          {
            path: 'oauth.allowed_client_ids',
            label: 'Allowed client IDs',
            control: 'list',
            required: true,
            placeholder: 'One client ID per line',
            when: isOAuth
          }
        ]
      }
    ]
  },
  {
    kind: 'batch_s3',
    graphqlKind: 'BATCH_S3',
    direction: 'source',
    label: 'Batch S3',
    summary: 'Files from an S3-compatible bucket',
    sections: [
      {
        type: 'group',
        id: 'source',
        label: 'Source',
        path: '',
        fields: [{ path: 'source_id', label: 'Source ID', control: 'text', required: true, hint: idHint }]
      },
      {
        type: 'group',
        id: 's3',
        label: 'S3',
        path: 's3',
        fields: [
          { path: 's3.endpoint', label: 'Endpoint', control: 'text', required: true, placeholder: 'host:port' },
          { path: 's3.region', label: 'Region', control: 'text', hint: 'Optional.' },
          { path: 's3.bucket', label: 'Bucket', control: 'text', required: true },
          { path: 's3.input_prefix', label: 'Input prefix', control: 'text', required: true, placeholder: 'incoming' },
          {
            path: 's3.archive_prefix',
            label: 'Archive prefix',
            control: 'text',
            required: true,
            placeholder: 'archive'
          },
          {
            path: 's3.use_tls',
            label: 'Use TLS',
            control: 'boolean',
            required: true,
            hint: 'False only for a loopback endpoint.'
          },
          { path: 's3.access_key_binding', label: 'Access key binding', control: 'binding', required: true },
          {
            path: 's3.secret_access_key_binding',
            label: 'Secret access key binding',
            control: 'binding',
            required: true
          }
        ]
      },
      batchSchedule,
      batchWorkload
    ]
  },
  {
    kind: 'batch_sftp',
    graphqlKind: 'BATCH_SFTP',
    direction: 'source',
    label: 'Batch SFTP',
    summary: 'Files from an SFTP directory',
    sections: [
      {
        type: 'group',
        id: 'source',
        label: 'Source',
        path: '',
        fields: [{ path: 'source_id', label: 'Source ID', control: 'text', required: true, hint: idHint }]
      },
      {
        type: 'group',
        id: 'sftp',
        label: 'SFTP',
        path: 'sftp',
        fields: [
          { path: 'sftp.host', label: 'Host', control: 'text', required: true },
          { path: 'sftp.port', label: 'Port', control: 'number', required: true, min: 1, max: 65535 },
          { path: 'sftp.username', label: 'Username', control: 'text', required: true },
          {
            path: 'sftp.input_directory',
            label: 'Input directory',
            control: 'text',
            required: true,
            placeholder: '/inbound'
          },
          {
            path: 'sftp.archive_directory',
            label: 'Archive directory',
            control: 'text',
            required: true,
            placeholder: '/archive'
          },
          { path: 'sftp.known_hosts_binding', label: 'Known hosts binding', control: 'binding', required: true },
          {
            // Not a spec key: which one of the two credential bindings this
            // connection uses, so "exactly one" is the form's shape.
            path: SFTP_CREDENTIAL,
            label: 'Credential',
            control: 'select',
            required: true,
            virtual: true,
            options: ['password', 'private_key'],
            optionLabels: { password: 'Password', private_key: 'Private key' },
            deriveFrom: [
              { path: 'sftp.private_key_binding', value: 'private_key' },
              { path: 'sftp.password_binding', value: 'password' }
            ],
            hint: 'Exactly one of a password and a private key.'
          },
          {
            path: 'sftp.password_binding',
            label: 'Password binding',
            control: 'binding',
            required: true,
            when: { path: SFTP_CREDENTIAL, equals: 'password' }
          },
          {
            path: 'sftp.private_key_binding',
            label: 'Private key binding',
            control: 'binding',
            required: true,
            when: { path: SFTP_CREDENTIAL, equals: 'private_key' }
          },
          {
            path: 'sftp.private_key_passphrase_binding',
            label: 'Private key passphrase binding',
            control: 'binding',
            when: { path: SFTP_CREDENTIAL, equals: 'private_key' }
          }
        ]
      },
      batchSchedule,
      batchWorkload
    ]
  },
  {
    kind: 'https',
    graphqlKind: 'HTTPS',
    direction: 'destination',
    label: 'HTTPS',
    summary: 'POST or PUT to an HTTPS endpoint',
    sections: [
      destinationCommon,
      {
        type: 'group',
        id: 'https',
        label: 'HTTPS',
        path: 'https',
        fields: [
          {
            path: 'https.url',
            label: 'URL',
            control: 'text',
            required: true,
            placeholder: 'https://destination.example/inbound',
            wide: true
          },
          { path: 'https.method', label: 'Method', control: 'select', required: true, options: ['POST', 'PUT'] },
          { path: 'https.token_binding', label: 'Token binding', control: 'binding', required: true },
          { path: 'https.ca_bundle_binding', label: 'CA bundle binding', control: 'binding' }
        ]
      },
      destinationIdentity
    ]
  },
  {
    kind: 'fhir',
    graphqlKind: 'FHIR',
    direction: 'destination',
    label: 'FHIR',
    summary: 'Transaction bundles to a FHIR server',
    sections: [
      destinationCommon,
      {
        type: 'group',
        id: 'fhir',
        label: 'FHIR',
        path: 'fhir',
        fields: [
          {
            path: 'fhir.base_url',
            label: 'Base URL',
            control: 'text',
            required: true,
            placeholder: 'https://fhir.example/r4',
            wide: true
          },
          {
            path: 'fhir.interaction',
            label: 'Interaction',
            control: 'select',
            options: ['transaction'],
            defaultValue: 'transaction',
            fixed: true
          },
          { path: 'fhir.token_binding', label: 'Token binding', control: 'binding', required: true },
          { path: 'fhir.ca_bundle_binding', label: 'CA bundle binding', control: 'binding' }
        ]
      },
      destinationIdentity
    ]
  },
  {
    kind: 'kafka',
    graphqlKind: 'KAFKA',
    direction: 'destination',
    label: 'Kafka',
    summary: 'Messages to a Kafka topic',
    sections: [
      destinationCommon,
      {
        type: 'group',
        id: 'kafka',
        label: 'Kafka',
        path: 'kafka',
        fields: [
          {
            path: 'kafka.topic',
            label: 'Topic',
            control: 'text',
            required: true,
            hint: 'At most 249 characters, no whitespace.',
            wide: true
          }
        ]
      },
      destinationIdentity
    ]
  }
];

const BY_KIND = new Map(KIND_SCHEMAS.map((schema) => [schema.kind, schema]));

/** The schema of a kind, from either spelling (`mllp` or GraphQL's `MLLP`). */
export function kindSchema(kind: SpecKind | ConnectionKind | string): KindSchema | undefined {
  return BY_KIND.get(kind.toLowerCase() as SpecKind);
}

export function kindsFor(direction: ConnectionDirectionId): KindSchema[] {
  return KIND_SCHEMAS.filter((schema) => schema.direction === direction);
}

/** Every scalar/list field of a kind, in form order (repeat sub-fields excluded). */
export function groupFields(schema: KindSchema): SpecField[] {
  return schema.sections.flatMap((section) => (section.type === 'group' ? section.fields : []));
}

export function repeatSections(schema: KindSchema): SpecRepeat[] {
  return schema.sections.filter((section): section is SpecRepeat => section.type === 'repeat');
}

/** Whether a field is shown for these values. */
export function fieldVisible(field: SpecField, values: SpecValues): boolean {
  const condition = field.when;
  if (!condition) return true;
  const current = fieldText(values[condition.path]);
  if (condition.equals !== undefined) return current === condition.equals;
  if (condition.notEquals !== undefined) return current !== condition.notEquals;
  return true;
}

// ── values ⇄ spec ──────────────────────────────────────────────────────────

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

export function getPath(root: unknown, path: string): unknown {
  let current: unknown = root;
  for (const key of path.split('.')) {
    if (!isRecord(current)) return undefined;
    current = current[key];
  }
  return current;
}

function setPath(root: Record<string, unknown>, path: string, value: unknown): void {
  const keys = path.split('.');
  let current = root;
  for (const key of keys.slice(0, -1)) {
    const next = current[key];
    if (!isRecord(next)) {
      current[key] = {};
    }
    current = current[key] as Record<string, unknown>;
  }
  current[keys[keys.length - 1] as string] = value;
}

function deletePath(root: Record<string, unknown>, path: string): void {
  const keys = path.split('.');
  let current: unknown = root;
  for (const key of keys.slice(0, -1)) {
    if (!isRecord(current)) return;
    current = current[key];
  }
  if (isRecord(current)) delete current[keys[keys.length - 1] as string];
}

/** The string a control shows for a stored value. */
export function controlText(field: SpecField, value: unknown): string {
  if (value === undefined || value === null) return '';
  if (field.control === 'list') {
    return Array.isArray(value) ? value.map((entry) => String(entry)).join('\n') : String(value);
  }
  if (typeof value === 'boolean') return value ? 'true' : 'false';
  if (typeof value === 'string' || typeof value === 'number') return String(value);
  // An object or a list where a scalar belongs: the checker reports it; show nothing.
  return '';
}

/**
 * The JSON value a control's string becomes, or undefined when it is empty
 * (the key is then left out, and a required field is reported as missing).
 * A number that does not parse is sent as typed, so the checker names the
 * field (INVALID_TYPE) rather than the form swallowing the input.
 */
export function controlValue(field: SpecField, value: string | number | null | undefined): unknown {
  const text = fieldText(value);
  const trimmed = text.trim();
  switch (field.control) {
    case 'list': {
      const entries = text
        .split('\n')
        .map((line) => line.trim())
        .filter((line) => line.length > 0);
      return entries.length > 0 ? entries : undefined;
    }
    case 'number': {
      if (trimmed === '') return undefined;
      const number = Number(trimmed);
      return Number.isFinite(number) ? number : trimmed;
    }
    case 'boolean':
      if (trimmed === 'true') return true;
      if (trimmed === 'false') return false;
      return undefined;
    default:
      return trimmed === '' ? undefined : trimmed;
  }
}

/** Form state from a stored spec. */
export function valuesFromSpec(schema: KindSchema, spec: unknown): { values: SpecValues; repeats: SpecRepeats } {
  const values: SpecValues = {};
  for (const field of groupFields(schema)) {
    values[field.path] = field.virtual
      ? derivedChoice(field, spec)
      : field.fixed
        ? (field.defaultValue ?? '')
        : controlText(field, getPath(spec, field.path));
  }
  const repeats: SpecRepeats = {};
  for (const section of repeatSections(schema)) {
    const stored = getPath(spec, section.path);
    repeats[section.path] = Array.isArray(stored)
      ? stored.map((item) => {
          const entry: SpecValues = {};
          for (const field of section.fields) entry[field.path] = controlText(field, getPath(item, field.path));
          return entry;
        })
      : [];
  }
  return { values, repeats };
}

/** A virtual field's value, read from the first `deriveFrom` path that holds one. */
function derivedChoice(field: SpecField, spec: unknown): string {
  for (const source of field.deriveFrom ?? []) {
    const stored = getPath(spec, source.path);
    if (typeof stored === 'string' && stored.trim() !== '') return source.value;
  }
  return '';
}

/** Form state for a new connection: the documented defaults and nothing else. */
export function defaultValues(schema: KindSchema): { values: SpecValues; repeats: SpecRepeats } {
  const values: SpecValues = {};
  for (const field of groupFields(schema)) values[field.path] = field.defaultValue ?? '';
  const repeats: SpecRepeats = {};
  for (const section of repeatSections(schema)) repeats[section.path] = [];
  return { values, repeats };
}

/** An empty item for a repeated group. */
export function emptyRepeatItem(section: SpecRepeat): SpecValues {
  const item: SpecValues = {};
  for (const field of section.fields) item[field.path] = field.defaultValue ?? '';
  return item;
}

function pruneEmptyObjects(value: Record<string, unknown>): void {
  for (const [key, child] of Object.entries(value)) {
    if (isRecord(child)) {
      pruneEmptyObjects(child);
      if (Object.keys(child).length === 0) delete value[key];
    }
  }
}

/**
 * The spec the form describes. It starts from the stored spec, so a key the
 * form does not know survives (the checker reports it as UNKNOWN_FIELD rather
 * than the form dropping it unseen); every field the form owns is then set
 * from its control, removed when empty or hidden, and an object left empty is
 * removed.
 */
export function buildSpec(
  schema: KindSchema,
  values: SpecValues,
  repeats: SpecRepeats,
  base: unknown = {}
): Record<string, unknown> {
  // A JSON round trip, not structuredClone: the base may be a Svelte state
  // proxy, and a spec is JSON by contract.
  const spec: Record<string, unknown> = isRecord(base) ? (JSON.parse(JSON.stringify(base)) as Record<string, unknown>) : {};
  for (const field of groupFields(schema)) {
    if (field.virtual) continue; // a form-only choice, never a spec key
    const text = field.fixed ? field.defaultValue : values[field.path];
    const value = fieldVisible(field, values) ? controlValue(field, text ?? '') : undefined;
    if (value === undefined) deletePath(spec, field.path);
    else setPath(spec, field.path, value);
  }
  for (const section of repeatSections(schema)) {
    const items = repeats[section.path] ?? [];
    if (items.length === 0) {
      deletePath(spec, section.path);
      continue;
    }
    setPath(
      spec,
      section.path,
      items.map((item) => {
        const built: Record<string, unknown> = {};
        for (const field of section.fields) {
          const value = controlValue(field, item[field.path] ?? '');
          if (value !== undefined) built[field.path] = value;
        }
        return built;
      })
    );
  }
  pruneEmptyObjects(spec);
  return spec;
}

// ── problems → fields ──────────────────────────────────────────────────────

export interface SpecProblem {
  code: string;
  path: string;
  message: string;
}

/** Only an unused binding is a warning; every other code blocks compile. */
export function isBlocking(problem: SpecProblem): boolean {
  return problem.code !== 'UNUSED_BINDING';
}

export interface PlacedProblems {
  /** Field path (or `<repeat path>[i].<sub-field>`) → messages. */
  byField: Record<string, string[]>;
  /** `secret_bindings[i]` → messages, for the Secrets tab. */
  byBinding: Record<number, SpecProblem[]>;
  /** Problems no Settings field shows, labelled for the problem list. */
  unplaced: Array<SpecProblem & { label: string }>;
}

const ELEMENT = /^(.*)\[(\d+)\]$/;
const BINDING = /^secret_bindings\[(\d+)\](?:\.(.+))?$/;
const REPEAT_ITEM = /^\[(\d+)\](?:\.(.+))?$/;

function addTo(target: Record<string, string[]>, key: string, message: string): void {
  (target[key] ??= []).push(message);
}

/**
 * Puts each problem on the field it names. A list element (`clients.allowed_cidrs[2]`)
 * lands on its list as "Entry 3: …"; a repeated item's field lands on that
 * item's control; a binding's problem is kept for the Secrets tab and listed;
 * anything else — the document, a whole group, a hidden or unknown key — is
 * listed with the best label the schema has for it.
 */
export function placeProblems(
  schema: KindSchema,
  problems: readonly SpecProblem[],
  values: SpecValues,
  bindingNames: readonly string[] = []
): PlacedProblems {
  const placed: PlacedProblems = { byField: {}, byBinding: {}, unplaced: [] };
  const visible = new Map(
    groupFields(schema)
      .filter((field) => fieldVisible(field, values))
      .map((field) => [field.path, field])
  );
  const repeats = repeatSections(schema);
  const groups = schema.sections.filter((section): section is SpecGroup => section.type === 'group');

  for (const problem of problems) {
    const binding = BINDING.exec(problem.path);
    if (binding) {
      const index = Number(binding[1]);
      (placed.byBinding[index] ??= []).push(problem);
      const name = bindingNames[index];
      const which = name ? `Secret binding ${name}` : `Secret binding ${index + 1}`;
      placed.unplaced.push({ ...problem, label: binding[2] ? `${which} · ${binding[2]}` : which });
      continue;
    }

    if (visible.has(problem.path)) {
      addTo(placed.byField, problem.path, problem.message);
      continue;
    }
    const element = ELEMENT.exec(problem.path);
    if (element && visible.get(element[1] as string)?.control === 'list') {
      addTo(placed.byField, element[1] as string, `Entry ${Number(element[2]) + 1}: ${problem.message}`);
      continue;
    }

    const repeat = repeats.find(
      (section) => problem.path === section.path || problem.path.startsWith(`${section.path}[`)
    );
    if (repeat) {
      const rest = REPEAT_ITEM.exec(problem.path.slice(repeat.path.length));
      if (rest?.[2]) {
        const itemPath = `${repeat.path}[${rest[1]}]`;
        const sub = rest[2];
        const field = repeat.fields.find((candidate) => candidate.path === sub);
        if (field) {
          addTo(placed.byField, `${itemPath}.${sub}`, problem.message);
          continue;
        }
        const subElement = ELEMENT.exec(sub);
        if (subElement && repeat.fields.find((candidate) => candidate.path === subElement[1])?.control === 'list') {
          addTo(placed.byField, `${itemPath}.${subElement[1]}`, `Entry ${Number(subElement[2]) + 1}: ${problem.message}`);
          continue;
        }
      }
      const label = rest ? `${repeat.itemLabel} ${Number(rest[1]) + 1}` : repeat.label;
      placed.unplaced.push({ ...problem, label });
      continue;
    }

    const group = problem.path === '' ? undefined : groups.find((section) => section.path === problem.path);
    const hidden = groupFields(schema).find((field) => field.path === problem.path);
    placed.unplaced.push({
      ...problem,
      label: problem.path === '' ? 'Spec' : (group?.label ?? hidden?.label ?? problem.path)
    });
  }
  return placed;
}

// ── table helpers ──────────────────────────────────────────────────────────

function text(spec: unknown, path: string): string {
  const value = getPath(spec, path);
  return typeof value === 'string' || typeof value === 'number' ? String(value).trim() : '';
}

/** What the table's Endpoint column shows for a stored spec, or '' when it names none yet. */
export function endpointOf(kind: SpecKind | ConnectionKind | string, spec: unknown): string {
  switch (kind.toLowerCase()) {
    case 'mllp':
      return text(spec, 'listen_address');
    case 'http':
      return text(spec, 'path');
    case 'batch_s3':
      return [text(spec, 's3.endpoint'), text(spec, 's3.bucket'), text(spec, 's3.input_prefix')]
        .filter(Boolean)
        .join('/');
    case 'batch_sftp': {
      const host = text(spec, 'sftp.host');
      const port = text(spec, 'sftp.port');
      const directory = text(spec, 'sftp.input_directory');
      return host ? `${host}${port ? `:${port}` : ''}${directory}` : '';
    }
    case 'https':
      return text(spec, 'https.url');
    case 'fhir':
      return text(spec, 'fhir.base_url');
    case 'kafka':
      return text(spec, 'kafka.topic');
    default:
      return '';
  }
}
