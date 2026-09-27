import { describe, expect, it } from 'vitest';
import type { ConnectionRow } from './connectionsApi';
import {
  bindingsOf,
  bufferFromConnection,
  canonicalJson,
  descriptionProblem,
  idProblem,
  isDirty,
  nameProblem,
  newBuffer,
  reasonProblem,
  reconcileBuffer,
  specOf,
  validationKey
} from './editBuffer';

function row(overrides: Partial<ConnectionRow> = {}): ConnectionRow {
  return {
    id: 'adt-east-mllp',
    direction: 'SOURCE',
    kind: 'MLLP',
    name: 'ADT east',
    description: '',
    spec: {
      source_id: 'adt-east',
      listen_address: '0.0.0.0:2575',
      encoding: 'utf-8',
      timeouts: { read_seconds: 5, write_seconds: 5, idle_seconds: 60, process_seconds: 30 },
      tls: { mode: 'disabled' },
      clients: { allowed_cidrs: ['10.0.0.0/8'] },
      acknowledgements: { mode: 'application' },
      max_message_bytes: 1048576,
      max_connections: 16
    },
    secretBindings: [],
    version: 3,
    archived: false,
    latestRevision: null,
    references: [],
    runtime: { mounted: false, role: null, detail: null },
    createdBy: { id: 'operator@example.test', kind: 'human' },
    createdAt: '2026-09-26T10:00:00Z',
    updatedBy: { id: 'operator@example.test', kind: 'human' },
    updatedReason: 'initial',
    updatedAt: '2026-09-26T10:00:00Z',
    ...overrides
  };
}

describe('editBuffer', () => {
  it('starts clean from the catalog and describes the same spec', () => {
    const buffer = bufferFromConnection(row());
    expect(isDirty(buffer)).toBe(false);
    expect(specOf(buffer)).toEqual(row().spec);
    expect(buffer.version).toBe(3);
  });

  it('is dirty after an edit and clean again when the edit is undone', () => {
    const buffer = bufferFromConnection(row());
    buffer.values['max_connections'] = 32;
    expect(isDirty(buffer)).toBe(true);
    buffer.values['max_connections'] = '16';
    expect(isDirty(buffer)).toBe(false);
    buffer.bindings.push({ name: 'x', provider: 'env', key: 'X', version: '' });
    expect(isDirty(buffer)).toBe(true);
  });

  it('sends bindings trimmed, with an empty version as null, and never a value', () => {
    const bindings = bindingsOf({
      bindings: [{ name: ' mllp-cert ', provider: 'file', key: ' mllp/cert.pem ', version: '' }]
    });
    expect(bindings).toEqual([{ name: 'mllp-cert', provider: 'file', key: 'mllp/cert.pem', version: null }]);
    expect(Object.keys(bindings[0] ?? {}).sort()).toEqual(['key', 'name', 'provider', 'version']);
  });

  it('keeps a dirty buffer across a reload and follows the catalog when clean', () => {
    const dirty = bufferFromConnection(row());
    dirty.name = 'Renamed';
    expect(reconcileBuffer(dirty, row({ version: 4 }))).toBe(dirty);
    const clean = bufferFromConnection(row());
    const next = reconcileBuffer(clean, row({ version: 4, name: 'Changed elsewhere' }));
    expect(next.name).toBe('Changed elsewhere');
    expect(next.version).toBe(4);
  });

  it('keys validation on kind, spec and bindings only', () => {
    const buffer = newBuffer('kafka');
    const before = validationKey(buffer);
    buffer.name = 'only a label';
    expect(validationKey(buffer)).toBe(before);
    buffer.values['kafka.topic'] = 'integration.delivery.v1';
    expect(validationKey(buffer)).not.toBe(before);
  });

  it('prefills only documented defaults on a new connection', () => {
    expect(specOf(newBuffer('mllp'))).toEqual({
      encoding: 'utf-8',
      framing: { start_byte: 11, end_byte: 28, trailer_byte: 13 },
      acknowledgements: { include_error_segment: false }
    });
    expect(specOf(newBuffer('kafka'))).toEqual({});
  });

  it('canonicalises key order', () => {
    expect(canonicalJson({ b: 1, a: { d: 2, c: 3 } })).toBe(canonicalJson({ a: { c: 3, d: 2 }, b: 1 }));
  });
});

describe('editBuffer — the write fields the catalog re-checks', () => {
  it('mirrors C-0 connectionIDPattern for IDs', () => {
    expect(idProblem('adt-east.mllp_1')).toBeNull();
    expect(idProblem('')).toMatch(/required/);
    expect(idProblem('-leading')).not.toBeNull();
    expect(idProblem('has space')).not.toBeNull();
    expect(idProblem('a/b')).not.toBeNull();
    expect(idProblem('a'.repeat(128))).toBeNull();
    expect(idProblem('a'.repeat(129))).not.toBeNull();
  });

  it('checks names, descriptions and reasons like the catalog does', () => {
    expect(nameProblem('ADT east')).toBeNull();
    expect(nameProblem(' padded')).not.toBeNull();
    expect(nameProblem('')).not.toBeNull();
    expect(descriptionProblem('line one\nline two')).toBeNull();
    expect(descriptionProblem('bell\u0007')).not.toBeNull();
    expect(reasonProblem('   ')).toMatch(/required/);
    expect(reasonProblem('Moving ADT east to the new listener')).toBeNull();
    expect(reasonProblem('x'.repeat(1025))).not.toBeNull();
  });
});
