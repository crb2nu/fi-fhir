import { describe, expect, it } from 'vitest';
import { SESSION_EXPORT_FORMAT, sessionExportFile, sessionExportFileName } from './sessionExport';
import { exportReasonProblem, rawPayloadBlockedSentence } from './sessionReason';
import type { SessionBundle } from './workspaceApi';

describe('sessionExportFileName', () => {
  it('names the file after the session and the export time, in UTC', () => {
    expect(sessionExportFileName('session_01HX', '2026-09-29T14:03:07.123Z')).toBe(
      'fi-fhir-session-session_01HX-20260929T140307Z.json'
    );
  });

  it('keeps only file-name-safe characters from the id', () => {
    expect(sessionExportFileName('../../etc/passwd', '2026-09-29T14:03:07Z')).toBe(
      'fi-fhir-session-etc-passwd-20260929T140307Z.json'
    );
    expect(sessionExportFileName('a b\\c:d*e?"<>|', '2026-09-29T14:03:07Z')).toBe(
      'fi-fhir-session-a-b-c-d-e-20260929T140307Z.json'
    );
    expect(sessionExportFileName('///', 'not a time')).toBe('fi-fhir-session-session-unknown-time.json');
    expect(sessionExportFileName('x'.repeat(200), '2026-09-29T14:03:07Z')).toHaveLength(
      'fi-fhir-session--20260929T140307Z.json'.length + 64
    );
  });
});

describe('sessionExportFile', () => {
  it('wraps the bundle as the server returned it, with what the request asked for', () => {
    const bundle = { sessionId: 's-1', exportedAt: '2026-09-29T14:03:07Z' } as SessionBundle;
    const file = sessionExportFile(bundle, 'audit: quarterly review', false);
    expect(file).toEqual({
      format: SESSION_EXPORT_FORMAT,
      export: { sessionId: 's-1', exportedAt: '2026-09-29T14:03:07Z', reason: 'audit: quarterly review', includeRawPayload: false },
      bundle
    });
    expect(file.bundle).toBe(bundle);
  });
});

describe('export reason and the raw-payload sentence', () => {
  it('refuses what the server would refuse', () => {
    expect(exportReasonProblem('   ')).toMatch(/required/);
    expect(exportReasonProblem('x'.repeat(1025))).toMatch(/1024 bytes/);
    expect(exportReasonProblem('bad\u0007bell')).toMatch(/control characters/);
    expect(exportReasonProblem('line one\nline two')).toBeNull();
  });

  it('names the missing role, falling back to integration.phi.export', () => {
    expect(rawPayloadBlockedSentence([])).toContain('needs integration.phi.export, which this identity does not hold');
    expect(rawPayloadBlockedSentence(['graphql:operator', 'integration.phi.export'])).toContain(
      'needs graphql:operator, integration.phi.export'
    );
  });
});
