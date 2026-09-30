import { describe, expect, it } from 'vitest';
import { fleetSentence, summarizeFleet, type EngineObservation } from './fleet';

function row(replicaId: string, adapter: string, stale: boolean, extra: Partial<EngineObservation> = {}): EngineObservation {
  return {
    replicaId,
    adapter,
    artifactId: null,
    revisionId: null,
    digest: null,
    heartbeatAt: '2026-09-29T12:00:00Z',
    stale,
    ...extra
  };
}

describe('summarizeFleet', () => {
  it('groups by replica, puts the answering replica first, and counts fresh replicas', () => {
    const summary = summarizeFleet({
      replicaId: 'api-1',
      observations: [
        row('api-0', 'http', true, { heartbeatAt: '2026-09-29T11:00:00Z' }),
        row('api-1', 'mllp', false, { artifactId: 'east-mllp', revisionId: 'r2', digest: 'sha256:aa' }),
        row('api-1', 'http', false, { heartbeatAt: '2026-09-29T12:01:00Z' }),
        row('api-2', 'http', false)
      ]
    });
    expect(summary.replicas.map((replica) => replica.replicaId)).toEqual(['api-1', 'api-2', 'api-0']);
    expect(summary).toMatchObject({ fresh: 2, stale: 1 });
    const self = summary.replicas[0]!;
    expect(self.self).toBe(true);
    expect(self.heartbeatAt).toBe('2026-09-29T12:01:00Z');
    expect(self.mounted).toEqual([
      { adapter: 'http', document: null, digest: null, stale: false },
      { adapter: 'mllp', document: 'east-mllp@r2', digest: 'sha256:aa', stale: false }
    ]);
    expect(summary.replicas[2]!.stale).toBe(true);
    expect(fleetSentence(summary)).toBe('2 replicas with a fresh heartbeat; 1 stale, not counted');
  });

  it('says so when nothing has reported', () => {
    const summary = summarizeFleet({ replicaId: 'api-1', observations: [] });
    expect(fleetSentence(summary)).toBe('No replica has reported a heartbeat.');
    expect(fleetSentence(summarizeFleet({ replicaId: 'a', observations: [row('a', 'http', false)] }))).toBe(
      '1 replica with a fresh heartbeat'
    );
  });
});
