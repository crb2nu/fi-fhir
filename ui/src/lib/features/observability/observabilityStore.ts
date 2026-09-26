/**
 * Observability store — surfaces Prometheus metrics, Loki logs, and Alertmanager
 * alerts via the loom platform's MCP tool calls.
 *
 * There is no simulated fallback. When the platform is not connected nothing is
 * fetched and the stores stay empty; `alertSource` says why, so a surface can
 * render the honest state ("No alert source configured") instead of invented
 * signals (`.loom/37` decision 3: no simulated data on a production surface).
 */
import { writable, derived } from 'svelte/store';
import { platformState, getPlatformClient } from '$lib/platform';

// ─── Types ────────────────────────────────────────────────────────────────────

export interface MetricSeries {
  name: string;
  labels: Record<string, string>;
  values: Array<{ timestamp: number; value: number }>;
}

export interface MetricsSnapshot {
  throughput: MetricSeries[];
  latency: MetricSeries[];
  errorRate: MetricSeries[];
  dlqDepth: MetricSeries[];
  lastUpdated: number;
}

export interface LogEntry {
  timestamp: number;
  level: 'debug' | 'info' | 'warn' | 'error';
  message: string;
  labels: Record<string, string>;
  workflowName?: string | undefined;
  eventType?: string | undefined;
}

export interface Alert {
  id: string;
  name: string;
  severity: 'critical' | 'warning' | 'info';
  state: 'firing' | 'pending' | 'resolved';
  summary: string;
  description?: string | undefined;
  startsAt: number;
  labels: Record<string, string>;
}

/**
 * Where the alert list came from.
 *
 * - `unconfigured`: no observability platform is connected, so there is no
 *   alert source at all. Surfaces say so; they never show placeholder alerts.
 * - `live`: the last fetch returned Alertmanager's list (possibly empty).
 * - `unavailable`: the platform is connected but the last alert query failed.
 */
export type AlertSource = 'unconfigured' | 'live' | 'unavailable';

/**
 * Human-readable severity label for an alert.
 *
 * Provides a non-color cue for alert severity (WCAG 1.4.1, Use of Color):
 * severity must not be conveyed by the colored marker alone.
 */
export function severityLabel(severity: Alert['severity']): string {
  switch (severity) {
    case 'critical':
      return 'Critical';
    case 'warning':
      return 'Warning';
    case 'info':
      return 'Info';
    default:
      return 'Info';
  }
}

export interface LogFilter {
  level?: string | undefined;
  workflowName?: string | undefined;
  search?: string | undefined;
}

export interface ObservabilityState {
  metrics: MetricsSnapshot | null;
  logs: LogEntry[];
  alerts: Alert[];
  isLoadingMetrics: boolean;
  isLoadingLogs: boolean;
  logFilter: LogFilter;
  error: string | null;
}

// ─── Store ────────────────────────────────────────────────────────────────────

const initialState: ObservabilityState = {
  metrics: null,
  logs: [],
  alerts: [],
  isLoadingMetrics: false,
  isLoadingLogs: false,
  logFilter: {},
  error: null,
};

export const observabilityState = writable<ObservabilityState>(initialState);

export const activeAlertCount = derived(observabilityState, ($s) =>
  $s.alerts.filter((a) => a.state === 'firing').length
);

export const isAvailable = derived(platformState, ($p) => $p.connected);

/** Source of the current alert list; see {@link AlertSource}. */
export const alertSource = writable<AlertSource>('unconfigured');

export const filteredLogs = derived(observabilityState, ($s) => {
  let logs = $s.logs;
  const { level, workflowName, search } = $s.logFilter;

  if (level) {
    logs = logs.filter((l) => l.level === level);
  }
  if (workflowName) {
    logs = logs.filter((l) => l.workflowName === workflowName);
  }
  if (search) {
    const q = search.toLowerCase();
    logs = logs.filter((l) => l.message.toLowerCase().includes(q));
  }
  return logs;
});

function connectedClient() {
  const client = getPlatformClient();
  return client?.isConnected() ? client : null;
}

function errorMessage(err: unknown, fallback: string): string {
  return err instanceof Error ? err.message : fallback;
}

// ─── Actions ──────────────────────────────────────────────────────────────────

export async function fetchMetrics(): Promise<void> {
  const client = connectedClient();
  if (!client) {
    observabilityState.update((s) => ({ ...s, metrics: null, isLoadingMetrics: false }));
    return;
  }

  observabilityState.update((s) => ({ ...s, isLoadingMetrics: true, error: null }));
  try {
    const result = await client.callTool('mcp-prometheus', 'query_range', {
      query: 'rate(workflow_events_total[5m])',
      start: new Date(Date.now() - 3600000).toISOString(),
      end: new Date().toISOString(),
      step: '180s',
    });
    observabilityState.update((s) => ({
      ...s,
      metrics: (result as MetricsSnapshot | null) ?? null,
      isLoadingMetrics: false,
    }));
  } catch (err) {
    const message = errorMessage(err, 'Failed to fetch metrics');
    observabilityState.update((s) => ({ ...s, metrics: null, isLoadingMetrics: false, error: message }));
  }
}

export async function fetchLogs(filter?: LogFilter | undefined): Promise<void> {
  const client = connectedClient();
  if (!client) {
    observabilityState.update((s) => ({
      ...s,
      logs: [],
      isLoadingLogs: false,
      logFilter: filter ?? s.logFilter,
    }));
    return;
  }

  observabilityState.update((s) => ({ ...s, isLoadingLogs: true, error: null }));
  try {
    const result = await client.callTool('mcp-loki', 'loki_query_range', {
      query: '{app="fi-fhir-engine"}',
      start: new Date(Date.now() - 3600000).toISOString(),
      end: new Date().toISOString(),
      limit: 50,
    });
    observabilityState.update((s) => ({
      ...s,
      logs: Array.isArray(result) ? (result as LogEntry[]) : [],
      isLoadingLogs: false,
      logFilter: filter ?? s.logFilter,
    }));
  } catch (err) {
    const message = errorMessage(err, 'Failed to fetch logs');
    observabilityState.update((s) => ({ ...s, logs: [], isLoadingLogs: false, error: message }));
  }
}

export async function fetchAlerts(): Promise<void> {
  const client = connectedClient();
  if (!client) {
    alertSource.set('unconfigured');
    observabilityState.update((s) => ({ ...s, alerts: [] }));
    return;
  }

  try {
    const result = await client.callTool('mcp-alertmanager', 'am_list_alerts', {});
    alertSource.set('live');
    observabilityState.update((s) => ({
      ...s,
      alerts: Array.isArray(result) ? (result as Alert[]) : [],
    }));
  } catch {
    // Best-effort: an unreachable Alertmanager is reported as such, never
    // papered over with placeholder alerts.
    alertSource.set('unavailable');
    observabilityState.update((s) => ({ ...s, alerts: [] }));
  }
}

export function setLogFilter(filter: LogFilter): void {
  observabilityState.update((s) => ({ ...s, logFilter: filter }));
}
