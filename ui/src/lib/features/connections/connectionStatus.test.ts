import { describe, expect, it } from 'vitest';
import {
  connectionStatus,
  connectionStatusText,
  mountedRevisionId,
  runtimeRevisionId,
  type ConnectionStatusInput
} from './connectionStatus';

const notMounted = { mounted: false, detail: null };
const mountedR3 = { mounted: true, detail: 'revision 3: MLLP listener on 0.0.0.0:2575 for definition adt-east' };
const mountedR2 = { mounted: true, detail: 'revision 2: MLLP listener on 0.0.0.0:2575 for definition adt-east' };

function input(overrides: Partial<ConnectionStatusInput>): ConnectionStatusInput {
  return {
    version: 1,
    archived: false,
    latestRevision: null,
    references: [],
    runtime: notMounted,
    ...overrides
  };
}

const r3 = { revisionId: '3', compiledFromVersion: 4 };
const referencedDraft = { definitionId: 'adt-east-def', state: 'draft' };
const referencedDeployed = { definitionId: 'adt-east-def', state: 'deployed' };
const referencedPaused = { definitionId: 'adt-west-def', state: 'paused' };
const referencedRetired = { definitionId: 'adt-old-def', state: 'retired' };

describe('connectionStatus — the honest states, additive, from catalog and runtime facts only', () => {
  const cases: Array<[name: string, given: Partial<ConnectionStatusInput>, expected: string]> = [
    ['never compiled', { version: 1 }, 'Draft'],
    ['never compiled, edited', { version: 5 }, 'Draft'],
    ['compiled from the current version', { version: 4, latestRevision: r3 }, 'Compiled r3'],
    ['edited after the latest revision', { version: 5, latestRevision: r3 }, 'Draft'],
    ['compiled and referenced by a draft definition', { version: 4, latestRevision: r3, references: [referencedDraft] }, 'Compiled r3 · Referenced'],
    ['a retired reference is referenced, not deployed', { version: 4, latestRevision: r3, references: [referencedRetired] }, 'Compiled r3 · Referenced'],
    ['compiled, referenced and deployed', { version: 4, latestRevision: r3, references: [referencedDeployed] }, 'Compiled r3 · Referenced · Deployed'],
    ['a paused definition counts as deployed', { version: 4, latestRevision: r3, references: [referencedPaused] }, 'Compiled r3 · Referenced · Deployed'],
    ['compiled and mounted here', { version: 4, latestRevision: r3, runtime: mountedR3 }, 'Compiled r3 · Mounted here'],
    [
      'all five facts at once',
      { version: 4, latestRevision: r3, references: [referencedDraft, referencedDeployed], runtime: mountedR3 },
      'Compiled r3 · Referenced · Deployed · Mounted here'
    ],
    ['edited after the mounted latest revision', { version: 5, latestRevision: r3, runtime: mountedR3 }, 'Draft (r3 mounted)'],
    [
      'edited, referenced, deployed and mounted',
      { version: 5, latestRevision: r3, references: [referencedDeployed], runtime: mountedR3 },
      'Draft (r3 mounted) · Referenced · Deployed'
    ],
    ['an older revision than the latest is mounted', { version: 4, latestRevision: r3, runtime: mountedR2 }, 'Compiled r3 · r2 mounted here'],
    [
      'a mount whose detail names no revision is not guessed',
      { version: 5, latestRevision: r3, runtime: { mounted: true, detail: 'delivery identity registry' } },
      'Draft (older revision mounted)'
    ],
    ['archived after compiling (archive advances the version)', { version: 5, archived: true, latestRevision: r3 }, 'Archived · Draft'],
    ['archived and never compiled', { version: 2, archived: true }, 'Archived · Draft']
  ];

  it.each(cases)('%s', (_name, given, expected) => {
    expect(connectionStatusText(connectionStatus(input(given)))).toBe(expected);
  });

  it('never reads Referenced from anything but references, nor Mounted from anything but the runtime', () => {
    const tokens = connectionStatus(input({ version: 4, latestRevision: r3 }));
    expect(tokens.map((token) => token.key)).toEqual(['compiled']);
  });

  it('gives each token a title that says why it holds', () => {
    const tokens = connectionStatus(
      input({ version: 4, latestRevision: r3, references: [referencedDeployed], runtime: mountedR3 })
    );
    expect(tokens.find((token) => token.key === 'referenced')?.title).toContain('adt-east-def');
    expect(tokens.find((token) => token.key === 'mounted')?.title).toBe(mountedR3.detail);
    expect(tokens.find((token) => token.key === 'compiled')?.title).toContain('version 4');
  });

  it('reads the mounted revision only from the runtime detail C-0 writes', () => {
    expect(mountedRevisionId('revision 12: HTTP ingress /v1/hl7v2 bound to integration adt-east')).toBe('12');
    expect(mountedRevisionId('MLLP listener on 0.0.0.0:2575')).toBeNull();
    expect(mountedRevisionId(null)).toBeNull();
  });
});

describe('runtimeRevisionId — the structured field first, the detail parse only without it', () => {
  const detailR2 = 'revision 2: MLLP listener on 0.0.0.0:2575 for definition adt-east';
  const cases: Array<[string, ConnectionStatusInput['runtime'], string | null]> = [
    ['the field wins over the detail', { mounted: true, detail: detailR2, revisionId: '7' }, '7'],
    ['the field needs no detail', { mounted: true, detail: 'delivery identity registry', revisionId: '4' }, '4'],
    ['a null field falls back to the detail', { mounted: true, detail: detailR2, revisionId: null }, '2'],
    ['an older server omits the field', { mounted: true, detail: detailR2 }, '2'],
    ['neither names a revision', { mounted: true, detail: 'delivery identity registry', revisionId: null }, null]
  ];

  it.each(cases)('%s', (_name, runtime, expected) => {
    expect(runtimeRevisionId(runtime)).toBe(expected);
  });

  it.each([
    ['the field names an older revision', { mounted: true, detail: 'registry', revisionId: '2' }, 'Compiled r3 · r2 mounted here'],
    ['the field names the latest revision', { mounted: true, detail: detailR2, revisionId: '3' }, 'Compiled r3 · Mounted here'],
    ['the detail names an older revision', { mounted: true, detail: detailR2, revisionId: null }, 'Compiled r3 · r2 mounted here']
  ] as const)('status: %s', (_name, runtime, expected) => {
    expect(connectionStatusText(connectionStatus(input({ version: 4, latestRevision: r3, runtime })))).toBe(expected);
  });

  it('names the field revision on an edited draft', () => {
    const runtime = { mounted: true, detail: 'registry', revisionId: '3' };
    expect(connectionStatusText(connectionStatus(input({ version: 5, latestRevision: r3, runtime })))).toBe(
      'Draft (r3 mounted)'
    );
  });
});
