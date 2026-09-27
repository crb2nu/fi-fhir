/**
 * The unsaved edits of one connection: its labels, the form's field strings,
 * its secret bindings, and the stored spec they overlay. One buffer per
 * connection lives in the page's memory, so switching rows keeps edits; a
 * buffer is rebuilt from the catalog whenever it has none.
 */
import type { ConnectionSecretBindingInput } from '$lib/gen/graphql';
import type { ConnectionRow } from './connectionsApi';
import {
  buildSpec,
  defaultValues,
  kindSchema,
  valuesFromSpec,
  type KindSchema,
  type SpecKind,
  type SpecProblem,
  type SpecRepeats,
  type SpecValues
} from './specSchema';

export interface BindingDraft {
  name: string;
  provider: string;
  key: string;
  version: string;
}

export interface EditBuffer {
  mode: 'create' | 'edit';
  kind: SpecKind;
  id: string;
  name: string;
  description: string;
  values: SpecValues;
  repeats: SpecRepeats;
  bindings: BindingDraft[];
  /** The stored spec the form overlays ({} for a new connection). */
  base: unknown;
  /** The draft version this buffer started from (0 for a new connection). */
  version: number;
  /** Canonical form of the buffer as it was made; a buffer is dirty when it differs. */
  originKey: string;
  /** Last problems the server reported, and the payload they were reported for. */
  problems: SpecProblem[];
  problemsKey: string | null;
}

/** JSON with object keys sorted, so equal documents compare equal. */
export function canonicalJson(value: unknown): string {
  return JSON.stringify(value, (_key, current: unknown) => {
    if (current && typeof current === 'object' && !Array.isArray(current)) {
      return Object.fromEntries(
        Object.entries(current as Record<string, unknown>).sort(([left], [right]) =>
          left < right ? -1 : left > right ? 1 : 0
        )
      );
    }
    return current;
  });
}

function schemaOf(kind: SpecKind): KindSchema {
  const schema = kindSchema(kind);
  if (!schema) throw new Error(`unknown connection kind ${kind}`);
  return schema;
}

/** The spec the buffer describes. */
export function specOf(buffer: EditBuffer): Record<string, unknown> {
  return buildSpec(schemaOf(buffer.kind), buffer.values, buffer.repeats, buffer.base);
}

/** The bindings as the API takes them: trimmed, with an empty version sent as null. */
export function bindingsOf(buffer: Pick<EditBuffer, 'bindings'>): ConnectionSecretBindingInput[] {
  return buffer.bindings.map((binding) => ({
    name: binding.name.trim(),
    provider: binding.provider.trim(),
    key: binding.key.trim(),
    version: binding.version.trim() === '' ? null : binding.version.trim()
  }));
}

/** What validateConnectionSpec is asked about, keyed canonically. */
export function validationKey(buffer: EditBuffer): string {
  return canonicalJson({ kind: buffer.kind, spec: specOf(buffer), secretBindings: bindingsOf(buffer) });
}

function contentKey(buffer: EditBuffer): string {
  return canonicalJson({
    id: buffer.id.trim(),
    name: buffer.name,
    description: buffer.description,
    spec: specOf(buffer),
    secretBindings: bindingsOf(buffer)
  });
}

export function isDirty(buffer: EditBuffer): boolean {
  return contentKey(buffer) !== buffer.originKey;
}

export function bufferFromConnection(row: ConnectionRow): EditBuffer {
  const kind = row.kind.toLowerCase() as SpecKind;
  const { values, repeats } = valuesFromSpec(schemaOf(kind), row.spec);
  const buffer: EditBuffer = {
    mode: 'edit',
    kind,
    id: row.id,
    name: row.name,
    description: row.description,
    values,
    repeats,
    bindings: row.secretBindings.map((binding) => ({
      name: binding.name,
      provider: binding.provider,
      key: binding.key,
      version: binding.version ?? ''
    })),
    base: row.spec ?? {},
    version: row.version,
    originKey: '',
    problems: [],
    problemsKey: null
  };
  buffer.originKey = contentKey(buffer);
  return buffer;
}

export function newBuffer(kind: SpecKind): EditBuffer {
  const { values, repeats } = defaultValues(schemaOf(kind));
  const buffer: EditBuffer = {
    mode: 'create',
    kind,
    id: '',
    name: '',
    description: '',
    values,
    repeats,
    bindings: [],
    base: {},
    version: 0,
    originKey: '',
    problems: [],
    problemsKey: null
  };
  buffer.originKey = contentKey(buffer);
  return buffer;
}

/**
 * The buffer to keep after the catalog answered with `row`: a clean buffer
 * follows the catalog (and is validated afresh); a dirty one is kept — its
 * save carries the version it started from, and the server refuses it if
 * that is stale.
 */
export function reconcileBuffer(buffer: EditBuffer | undefined, row: ConnectionRow): EditBuffer {
  if (buffer && isDirty(buffer)) return buffer;
  return bufferFromConnection(row);
}

// ── labels and the write's own fields (the server re-checks every one) ──

/** C-0's catalog ID rule (connectionIDPattern): it becomes an artifact ID and a file name stem. */
export const CONNECTION_ID_PATTERN = /^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$/;
export const MAX_NAME_BYTES = 256;
export const MAX_DESCRIPTION_BYTES = 4096;
export const MAX_REASON_BYTES = 1024;

function byteLength(value: string): number {
  return new TextEncoder().encode(value).length;
}

function hasControl(value: string, multiline: boolean): boolean {
  for (const character of value) {
    if (multiline && (character === '\n' || character === '\t')) continue;
    const code = character.codePointAt(0) ?? 0;
    if (code < 0x20 || (code >= 0x7f && code <= 0x9f)) return true;
  }
  return false;
}

export function idProblem(id: string): string | null {
  if (id === '') return 'An ID is required.';
  if (!CONNECTION_ID_PATTERN.test(id)) {
    return 'Use letters, digits, ".", "_" and "-", starting with a letter or digit; at most 128.';
  }
  return null;
}

export function nameProblem(name: string): string | null {
  if (name === '') return 'A name is required.';
  if (name.trim() !== name) return 'A name cannot start or end with whitespace.';
  if (byteLength(name) > MAX_NAME_BYTES) return `A name must be ${MAX_NAME_BYTES} bytes or fewer.`;
  if (hasControl(name, false)) return 'A name cannot contain control characters.';
  return null;
}

export function descriptionProblem(description: string): string | null {
  if (byteLength(description) > MAX_DESCRIPTION_BYTES) {
    return `A description must be ${MAX_DESCRIPTION_BYTES} bytes or fewer.`;
  }
  if (hasControl(description, true)) return 'A description cannot contain control characters.';
  return null;
}

export function reasonProblem(reason: string): string | null {
  const trimmed = reason.trim();
  if (trimmed === '') return 'A reason is required. It is recorded with your identity on the connection.';
  if (byteLength(trimmed) > MAX_REASON_BYTES) return `A reason must be ${MAX_REASON_BYTES} bytes or fewer.`;
  if (hasControl(trimmed, true)) return 'A reason cannot contain control characters.';
  return null;
}
