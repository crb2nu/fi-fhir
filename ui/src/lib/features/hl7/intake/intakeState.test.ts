import { describe, expect, it } from 'vitest';
import type { AccessCapabilityState } from '$lib/graphql/accessCapabilities';
import {
  CATALOG_NOT_CONFIGURED_REASON,
  STALL_NOTE_AFTER_MS,
  anyArmed,
  boundProblem,
  captureRowView,
  captureSampleSignature,
  formatRemaining,
  intakeDialogView,
  intakeEntry,
  intakeReasonProblem,
  intakeSamplesFrom,
  intakeSources,
  streamCaptures,
  type IntakeDialogInput,
  type IntakeDialogView
} from './intakeState';
import {
  DIGEST,
  OTHER_DIGEST,
  REDACTED_TEXT,
  access,
  adapter,
  batchConnection,
  capture,
  connection,
  runtime,
  sessionSample
} from './intakeFixtures';

const ok = <T>(value: T) => ({ status: 'ok' as const, value });
const UNKNOWN: AccessCapabilityState = { state: 'unknown' };
const MLLP_MOUNTED = adapter('mllp', true, {
  sourceId: 'adt-east',
  sourceRevisionId: '1',
  sourceDigest: DIGEST,
  listenAddress: '0.0.0.0:22575'
});

function input(overrides: Partial<IntakeDialogInput> = {}): IntakeDialogInput {
  return {
    access: access(),
    sessionEngine: true,
    runtime: ok(runtime()),
    catalog: ok([]),
    captures: [],
    ...overrides
  };
}

/** The dialog's honest states, one row each (.loom/38 C-3 acceptance). */
const VIEW_CASES: Array<{
  name: string;
  input: IntakeDialogInput;
  kind: IntakeDialogView['kind'];
  check?: (view: IntakeDialogView) => void;
}> = [
  { name: 'engine off: the entry is absent', input: input({ sessionEngine: false }), kind: 'hidden' },
  {
    name: 'sessions reported off: absent even with the build flag on',
    input: input({ access: access({ integrationSessions: false }) }),
    kind: 'hidden'
  },
  {
    name: 'catalog off: not configured, naming the control-plane key',
    input: input({ access: access({ connectionCatalog: false, controlPlane: false }) }),
    kind: 'not-configured',
    check: (view) => {
      expect(view).toMatchObject({ reason: CATALOG_NOT_CONFIGURED_REASON });
      expect(CATALOG_NOT_CONFIGURED_REASON).toContain('FI_FHIR_OPERATOR_CONTROL_PLANE_ENABLED');
    }
  },
  {
    name: 'missing integration.operator: the role, in the precedence after not-configured',
    input: input({
      access: access({ connectionsRead: false }, { connectionsRead: ['integration.operator'] })
    }),
    kind: 'missing-role',
    check: (view) => expect(view).toMatchObject({ roles: ['integration.operator'], principal: 'e2e-ide-operator' })
  },
  {
    name: 'no enabled source and no catalog source: empty',
    input: input(),
    kind: 'empty'
  },
  {
    name: 'adapters only: one stream row per mounted listener',
    input: input({ runtime: ok(runtime([MLLP_MOUNTED])) }),
    kind: 'sources',
    check: (view) => {
      expect(view.kind === 'sources' && view.sources).toEqual([
        expect.objectContaining({
          key: 'stream:adt-east',
          mode: 'stream',
          mounted: true,
          available: true,
          digest: DIGEST,
          state: 'Mounted here r1',
          connectionId: null
        })
      ]);
    }
  },
  {
    name: 'catalog only: a compiled MLLP source is capturable and says it is not mounted here',
    input: input({ catalog: ok([connection()]) }),
    kind: 'sources',
    check: (view) => {
      expect(view.kind === 'sources' && view.sources).toEqual([
        expect.objectContaining({
          key: 'stream:adt-east',
          mounted: false,
          available: true,
          state: 'Compiled r1 · not mounted here',
          connectionId: 'adt-east-mllp'
        })
      ]);
    }
  },
  {
    name: 'both: the catalog connection joins the adapter it names; a batch connection is a peek row',
    input: input({
      runtime: ok(runtime([MLLP_MOUNTED])),
      catalog: ok([connection({ runtime: { mounted: true, role: 'mllp-listener', detail: 'revision 1: listener' } }), batchConnection()])
    }),
    kind: 'sources',
    check: (view) => {
      if (view.kind !== 'sources') throw new Error('expected sources');
      expect(view.sources.map((source) => [source.key, source.mode, source.connectionId, source.label])).toEqual([
        ['stream:adt-east', 'stream', 'adt-east-mllp', 'ADT east'],
        ['peek:adt-batch', 'peek', 'adt-batch', 'ADT batch']
      ]);
      expect(view.sources[0]!.state).toBe('Mounted here r1');
      expect(view.sources[1]).toMatchObject({ available: true, digest: OTHER_DIGEST });
    }
  },
  {
    name: 'armed capture: the source row carries it',
    input: input({ catalog: ok([connection()]), captures: [capture()] }),
    kind: 'sources',
    check: (view) => {
      expect(view.kind === 'sources' && view.sources[0]!.armed).toMatchObject({ id: 'cap-1', status: 'ARMED' });
    }
  },
  {
    name: 'expired capture: the source row is free again',
    input: input({ catalog: ok([connection()]), captures: [capture({ status: 'EXPIRED' })] }),
    kind: 'sources',
    check: (view) => {
      expect(view.kind === 'sources' && view.sources[0]!.armed).toBeNull();
    }
  },
  {
    name: 'unknown capabilities never block',
    input: input({ access: UNKNOWN, catalog: ok([connection()]) }),
    kind: 'sources'
  },
  {
    name: 'both reads failed: an error, never a claim that nothing is mounted',
    input: input({
      runtime: { status: 'error', message: 'runtime down' },
      catalog: { status: 'error', message: 'catalog down' }
    }),
    kind: 'error',
    check: (view) => expect(view).toMatchObject({ messages: ['runtime down', 'catalog down'] })
  },
  {
    name: 'one read failed and it alone would have listed nothing: an error',
    input: input({ runtime: ok(runtime()), catalog: { status: 'error', message: 'catalog down' } }),
    kind: 'error'
  },
  {
    name: 'one read failed but the other lists sources: the rows and the failure',
    input: input({ runtime: ok(runtime([MLLP_MOUNTED])), catalog: { status: 'error', message: 'catalog down' } }),
    kind: 'sources',
    check: (view) => expect(view).toMatchObject({ errors: ['catalog down'] })
  },
  {
    name: 'still reading: loading',
    input: input({ catalog: { status: 'loading' } }),
    kind: 'loading'
  }
];

describe('intakeDialogView', () => {
  it.each(VIEW_CASES)('$name', ({ input: given, kind, check }) => {
    const view = intakeDialogView(given);
    expect(view.kind).toBe(kind);
    check?.(view);
  });
});

describe('intakeEntry', () => {
  it.each([
    { name: 'engine off', state: access(), engine: false, expected: { visible: false } },
    { name: 'on', state: access(), engine: true, expected: { visible: true, disabledReason: null } },
    { name: 'unknown', state: UNKNOWN, engine: true, expected: { visible: true, disabledReason: null } },
    {
      name: 'catalog reported off',
      state: access({ connectionCatalog: false }),
      engine: true,
      expected: { visible: true, disabledReason: CATALOG_NOT_CONFIGURED_REASON }
    },
    {
      name: 'catalog not reported (older API): unknown, not blocking',
      state: access({ connectionCatalog: undefined, controlPlane: undefined }),
      engine: true,
      expected: { visible: true, disabledReason: null }
    }
  ])('$name', ({ state, engine, expected }) => {
    expect(intakeEntry(state, engine)).toEqual(expected);
  });
});

describe('intakeSources edge states', () => {
  it('a never-compiled stream connection is listed but not capturable, with the reason', () => {
    const [row] = intakeSources(runtime(), [connection({ latestRevision: null })]);
    expect(row).toMatchObject({ available: false, state: 'Draft · not mounted here' });
    expect(row!.unavailableReason).toMatch(/Compile it in Connections/);
  });

  it('a mounted batch runner without a catalog connection cannot be browsed and says why', () => {
    const rows = intakeSources(runtime([adapter('batch', true, { sourceId: 'adt-batch-src', sourceDigest: DIGEST })]), []);
    expect(rows).toEqual([
      expect.objectContaining({ key: 'runtime-batch:adt-batch-src', available: false, mounted: true })
    ]);
    expect(rows[0]!.unavailableReason).toMatch(/Define this batch source in Connections/);
  });

  it('a batch runner whose source a catalog connection names is that connection’s row', () => {
    const rows = intakeSources(runtime([adapter('batch', true, { sourceId: 'adt-batch-src' })]), [batchConnection()]);
    expect(rows.map((row) => row.key)).toEqual(['peek:adt-batch']);
  });

  it('two catalog connections naming one source ID are one capture target', () => {
    const rows = intakeSources(runtime(), [
      connection({ id: 'a-draft', name: 'A', latestRevision: null }),
      connection({ id: 'b-compiled', name: 'B' })
    ]);
    expect(rows).toHaveLength(1);
    expect(rows[0]).toMatchObject({ key: 'stream:adt-east', connectionId: 'a-draft', available: true });
  });

  it('skips destinations, archived connections, disabled adapters and the delivery worker', () => {
    const rows = intakeSources(runtime([adapter('delivery', true), adapter('http', false, { sourceId: 'x' })]), [
      connection({ archived: true }),
      connection({ id: 'dest', direction: 'DESTINATION', kind: 'HTTPS' })
    ]);
    expect(rows).toEqual([]);
  });
});

describe('capture rows', () => {
  const now = Date.parse('2026-09-27T12:00:48Z');

  it('an armed capture counts and counts down', () => {
    expect(captureRowView(capture({ captured: 2 }), now)).toMatchObject({
      armed: true,
      progress: '2 / 5 captured',
      detail: 'expires in 4:12',
      tone: 'info',
      stalled: false
    });
    // 48 s armed with nothing captured: the row says why that can be.
    expect(captureRowView(capture(), now).stalled).toBe(true);
  });

  it('past expiresAt it waits for the server instead of deciding', () => {
    const view = captureRowView(capture(), Date.parse('2026-09-27T12:05:01Z'));
    expect(view).toMatchObject({ armed: true, detail: 'expiring' });
  });

  it('says why nothing arrived only after a while', () => {
    const early = Date.parse('2026-09-27T12:00:00Z') + STALL_NOTE_AFTER_MS - 1;
    expect(captureRowView(capture(), early).stalled).toBe(false);
    expect(captureRowView(capture({ captured: 1 }), now).stalled).toBe(false);
  });

  it.each([
    ['COMPLETE', 'complete', 'success'],
    ['EXPIRED', 'expired', 'neutral'],
    ['CANCELLED', 'cancelled', 'neutral'],
    ['FAILED', 'failed', 'danger']
  ] as const)('%s reads %s', (status, detail, tone) => {
    expect(captureRowView(capture({ status }), now)).toMatchObject({ armed: false, detail, tone, stalled: false });
  });

  it('polls only while something is armed', () => {
    expect(anyArmed([capture({ status: 'COMPLETE' }), capture({ id: 'cap-2', status: 'ARMED' })])).toBe(true);
    expect(anyArmed([capture({ status: 'EXPIRED' })])).toBe(false);
    expect(anyArmed([])).toBe(false);
  });

  it('the sample signature changes with a count or a finish, not with a peek row', () => {
    const base = captureSampleSignature([capture()]);
    expect(captureSampleSignature([capture({ captured: 1 })])).not.toBe(base);
    expect(captureSampleSignature([capture({ status: 'CANCELLED' })])).not.toBe(base);
    expect(captureSampleSignature([capture(), capture({ id: 'peek-1', mode: 'PEEK', status: 'COMPLETE' })])).toBe(base);
  });

  it('lists stream captures newest first and leaves peek audit rows out', () => {
    const rows = streamCaptures([
      capture({ id: 'old', requestedAt: '2026-09-27T11:00:00Z' }),
      capture({ id: 'peek', mode: 'PEEK' }),
      capture({ id: 'new', requestedAt: '2026-09-27T12:30:00Z' })
    ]);
    expect(rows.map((row) => row.id)).toEqual(['new', 'old']);
  });

  it('formats a countdown', () => {
    expect(formatRemaining(252_000)).toBe('4:12');
    expect(formatRemaining(500)).toBe('0:01');
    expect(formatRemaining(-5)).toBe('0:00');
  });
});

describe('intakeSamplesFrom', () => {
  it('keeps captured and peeked samples with their id and provenance, not the ones Preview added', () => {
    const samples = intakeSamplesFrom(
      [
        sessionSample(),
        sessionSample({ id: 'pasted', source: null, name: 'Mapping Studio preview', redactedPayload: null }),
        sessionSample({ id: 'peeked', source: 'peek:peek-1', redactedPayload: null })
      ],
      [capture(), capture({ id: 'peek-1', mode: 'PEEK', sourceId: 'adt-batch-src' })]
    );
    expect(samples).toEqual([
      {
        sessionId: 'session-1',
        sampleId: 'sample_capture_cap-1_1',
        name: 'capture cap-1 #1',
        provenance: 'capture:cap-1',
        sourceId: 'adt-east',
        raw: REDACTED_TEXT,
        payloadWithheld: false
      },
      expect.objectContaining({ sampleId: 'peeked', sourceId: 'adt-batch-src', raw: '', payloadWithheld: true })
    ]);
  });
});

describe('form checks', () => {
  it('a reason is 1–1024 bytes after trimming', () => {
    expect(intakeReasonProblem('  ')).toMatch(/required/);
    expect(intakeReasonProblem('x'.repeat(1025))).toMatch(/1024 bytes/);
    expect(intakeReasonProblem('investigate feed')).toBeNull();
  });

  it('bounds are whole numbers in range; 0 is refused like the server refuses it', () => {
    expect(boundProblem('0', 1, 100)).not.toBeNull();
    expect(boundProblem('1.5', 1, 100)).not.toBeNull();
    expect(boundProblem('101', 1, 100)).not.toBeNull();
    expect(boundProblem('', 1, 100)).not.toBeNull();
    expect(boundProblem('5', 1, 100)).toBeNull();
  });
});
