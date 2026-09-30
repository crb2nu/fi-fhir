/**
 * Fleet observations (.loom/39 "Convergence" step 1, surfaced by E-0): every
 * replica writes a heartbeat per adapter naming the document it mounted
 * (`engineRuntime.observations`). A row is stale when its heartbeat is older
 * than three report intervals; the server decides that, not the browser.
 *
 * Read by Connections › Engine and Home › Health. The query needs
 * `integration.operator` and an observation ledger (the connection catalog);
 * without the catalog the list is empty, which the views say.
 */
import { graphqlFetch } from '$lib/graphql/client';
import { EngineObservationsDocument, type EngineObservationsQuery } from '$lib/gen/graphql';

export type EngineObservation = EngineObservationsQuery['engineRuntime']['observations'][number];

export interface FleetSnapshot {
  /** The replica that answered this read. */
  replicaId: string;
  observations: EngineObservation[];
}

export async function fetchFleet(): Promise<FleetSnapshot> {
  const result = await graphqlFetch(EngineObservationsDocument, {}, { showErrorToast: false });
  return { replicaId: result.engineRuntime.replicaId, observations: result.engineRuntime.observations };
}

export interface MountedDocument {
  adapter: string;
  /** `artifact@revision`, or null when the adapter mounts nothing. */
  document: string | null;
  digest: string | null;
  stale: boolean;
}

export interface ReplicaSummary {
  replicaId: string;
  /** True when every heartbeat of the replica is stale. */
  stale: boolean;
  /** The newest heartbeat across its adapters. */
  heartbeatAt: string;
  /** Whether this is the replica that answered. */
  self: boolean;
  mounted: MountedDocument[];
}

/**
 * Counts follow the server's own semantics (`ConnectionRuntimeState.totalReplicas`
 * is "replicas with any fresh heartbeat"): the fleet is the fresh replicas.
 * Stale replicas are listed for the record and counted apart, never in `fresh`.
 */
export interface FleetSummary {
  replicas: ReplicaSummary[];
  /** Replicas with at least one fresh heartbeat — the server's totalReplicas. */
  fresh: number;
  /** Replicas whose every heartbeat is stale: listed, not part of the fleet. */
  stale: number;
}

/** Groups heartbeat rows by replica: the answering replica first, then fresh, then stale. */
export function summarizeFleet(snapshot: FleetSnapshot): FleetSummary {
  const byReplica = new Map<string, EngineObservation[]>();
  for (const row of snapshot.observations) {
    byReplica.set(row.replicaId, [...(byReplica.get(row.replicaId) ?? []), row]);
  }
  const replicas: ReplicaSummary[] = [...byReplica.entries()].map(([replicaId, rows]) => ({
    replicaId,
    stale: rows.every((row) => row.stale),
    heartbeatAt: rows.map((row) => row.heartbeatAt).sort().at(-1) ?? '',
    self: replicaId === snapshot.replicaId,
    mounted: rows
      .map((row) => ({
        adapter: row.adapter,
        document: row.artifactId && row.revisionId ? `${row.artifactId}@${row.revisionId}` : null,
        digest: row.digest ?? null,
        stale: row.stale
      }))
      .sort((a, b) => a.adapter.localeCompare(b.adapter))
  }));
  replicas.sort(
    (a, b) =>
      Number(b.self) - Number(a.self) || Number(a.stale) - Number(b.stale) || a.replicaId.localeCompare(b.replicaId)
  );
  const fresh = replicas.filter((replica) => !replica.stale).length;
  return { replicas, fresh, stale: replicas.length - fresh };
}

/** "2 replicas with a fresh heartbeat; 1 stale, not counted" — the one line Home shows. */
export function fleetSentence(summary: FleetSummary): string {
  if (summary.fresh + summary.stale === 0) return 'No replica has reported a heartbeat.';
  const noun = summary.fresh === 1 ? 'replica' : 'replicas';
  const head = `${summary.fresh} ${noun} with a fresh heartbeat`;
  return summary.stale > 0 ? `${head}; ${summary.stale} stale, not counted` : head;
}
