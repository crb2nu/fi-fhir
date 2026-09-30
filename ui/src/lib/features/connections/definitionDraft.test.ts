import { describe, expect, it } from 'vitest';
import {
  blocking,
  definitionFromQuery,
  definitionLink,
  deriveBindings,
  newDraftForm,
  nextStep,
  operatorDeployLink,
  toDraftInput,
  validationLabel
} from './definitionDraft';
import type { ConnectionChoice, DefinitionRow, RegistryArtifact } from './definitionsApi';

const DIGEST = 'sha256:' + 'ab'.repeat(32);

function choice(id: string, kind: string, names: string[], extra: Partial<NonNullable<ConnectionChoice['latestRevision']>> = {}): ConnectionChoice {
  return {
    id,
    kind: kind as ConnectionChoice['kind'],
    name: id,
    secretBindings: names.map((name) => ({ name, provider: 'file', key: `connections/${name}`, version: null })),
    latestRevision: {
      artifactId: id,
      revisionId: '1',
      digest: DIGEST,
      sourceId: null,
      destinationClass: null,
      secretBindingNames: names,
      ...extra
    }
  };
}

const artifact: RegistryArtifact = {
  integrationId: 'adt-east',
  profile: { artifactId: 'profile-adt', revisionId: '1', digest: DIGEST },
  workflow: { artifactId: 'workflow-adt', revisionId: 'workflow-version-1', digest: DIGEST },
  sourceId: 'adt-east',
  format: 'hl7v2'
};

function row(overrides: Partial<DefinitionRow> = {}): DefinitionRow {
  return {
    definitionId: 'adt-east-mllp',
    revisionId: 'v1',
    state: 'validated',
    validationPassed: true,
    validationCheckedAt: '2026-09-29T12:00:00Z',
    validationExpiresAt: '2026-09-29T12:05:00Z',
    ...overrides
  } as DefinitionRow;
}

describe('definition draft model', () => {
  it('derives one binding row per name the chosen revisions require, pre-filled from each connection', () => {
    const source = choice('adt-mllp', 'MLLP', ['mllp-cert'], { sourceId: 'adt-east' });
    const fhir = choice('fhir-primary', 'FHIR', ['fhir-token', 'mllp-cert'], { destinationClass: 'production' });
    const rows = deriveBindings(source, [fhir]);
    expect(rows.map((binding) => [binding.name, binding.key, binding.requiredBy])).toEqual([
      ['mllp-cert', 'connections/mllp-cert', 'source adt-mllp'],
      ['fhir-token', 'connections/fhir-token', 'destination fhir-primary']
    ]);
    const edited = rows.map((binding) => (binding.name === 'fhir-token' ? { ...binding, key: 'FHIR_TOKEN', provider: 'env' } : binding));
    expect(deriveBindings(source, [fhir], edited)[1]).toMatchObject({ provider: 'env', key: 'FHIR_TOKEN' });
  });

  it('builds the draft input from the chosen latest revisions and the registry pair', () => {
    const source = choice('adt-mllp', 'MLLP', [], { sourceId: 'adt-east', revisionId: '3' });
    const fhir = choice('fhir-primary', 'FHIR', ['fhir-token'], { destinationClass: 'sandbox' });
    const form = newDraftForm(600);
    form.definitionId = ' adt-east ';
    form.sourceId = 'adt-mllp';
    form.destinationIds = ['fhir-primary'];
    form.integrationId = 'adt-east';
    form.bindings = deriveBindings(source, [fhir]);
    const input = toDraftInput(form, [source], [fhir], [artifact]);
    expect(input).toMatchObject({
      definitionId: 'adt-east',
      revisionId: 'v1',
      parentRevisionId: null,
      source: { artifactId: 'adt-mllp', revisionId: '3' },
      destinations: [{ artifactId: 'fhir-primary', revisionId: '1' }],
      profile: artifact.profile,
      workflow: artifact.workflow,
      secretBindings: [{ name: 'fhir-token', provider: 'file', key: 'connections/fhir-token', version: null }],
      rawRetention: null,
      deployment: null
    });
    form.customPolicy = true;
    expect(toDraftInput(form, [source], [fhir], [artifact]).deployment).toMatchObject({
      validationMaxAgeSeconds: 600,
      scheduleMode: 'continuous',
      maxInFlight: 2
    });
  });

  it('treats only UNUSED_BINDING as a warning', () => {
    expect(blocking([{ code: 'UNUSED_BINDING' }])).toBe(false);
    expect(blocking([{ code: 'UNUSED_BINDING' }, { code: 'UNBOUND_SECRET' }])).toBe(true);
  });

  it('gates approve and publish on current evidence and sends deploy to Operator', () => {
    const before = new Date('2026-09-29T12:01:00Z');
    const after = new Date('2026-09-29T12:06:00Z');
    expect(nextStep(row({ state: 'draft' }), before)).toEqual({ step: 'validate', blockedReason: null });
    expect(nextStep(row(), before)).toEqual({ step: 'approve', blockedReason: null });
    expect(nextStep(row(), after).blockedReason).toMatch(/expired/);
    expect(nextStep(row({ state: 'approved' }), before).step).toBe('publish');
    expect(nextStep(row({ state: 'published' }), before).step).toBe('deploy');
    expect(validationLabel(row(), after)).toBe('expired');
    expect(validationLabel(row({ validationPassed: false }), before)).toBe('failed');
    expect(validationLabel(row({ validationCheckedAt: null }), before)).toBe('none');
  });

  it('writes and reads the deep links', () => {
    expect(operatorDeployLink(row())).toBe('/operator?definition=adt-east-mllp&revision=v1');
    expect(definitionLink(row())).toBe('/connections?definition=adt-east-mllp&revision=v1');
    expect(definitionFromQuery('?definition=adt-east-mllp&revision=v1')).toEqual({
      definitionId: 'adt-east-mllp',
      revisionId: 'v1'
    });
    expect(definitionFromQuery('?definition=adt-east-mllp')).toBeNull();
  });
});
