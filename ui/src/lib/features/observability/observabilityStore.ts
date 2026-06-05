/**
 * Observability store — surfaces Prometheus metrics, Loki logs, and Alertmanager
 * alerts via MCP tool calls. Falls back to realistic simulated data when the
 * platform is unreachable.
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

// ─── Constants ────────────────────────────────────────────────────────────────

const WORKFLOW_NAMES = ['ADT-to-FHIR', 'ORM-routing', 'Lab-result-pipeline', 'Pharmacy-feed'] as const;

const LOG_MESSAGES: Record<string, Array<{ level: LogEntry['level']; msg: string; event?: string | undefined }>> = {
  'ADT-to-FHIR': [
    { level: 'info', msg: 'PID segment parsed successfully', event: 'ADT^A01' },
    { level: 'info', msg: 'Patient resource created: Bundle/12849', event: 'ADT^A01' },
    { level: 'debug', msg: 'NK1 next-of-kin segment mapped to RelatedPerson', event: 'ADT^A01' },
    { level: 'warn', msg: 'PV1.3 ward code not in location registry, using fallback', event: 'ADT^A08' },
    { level: 'info', msg: 'Encounter status updated to finished', event: 'ADT^A03' },
    { level: 'error', msg: 'FHIR validation failed: Patient.identifier requires system', event: 'ADT^A04' },
    { level: 'debug', msg: 'MSH-9 trigger event resolved to ADT workflow branch', event: 'ADT^A01' },
    { level: 'info', msg: 'Insurance segment IN1 mapped to Coverage resource', event: 'ADT^A01' },
  ],
  'ORM-routing': [
    { level: 'info', msg: 'Order routed to Lab subsystem via OBR-4 code', event: 'ORM^O01' },
    { level: 'warn', msg: 'Route fallback: OBR-4 code not in routing table, using default', event: 'ORM^O01' },
    { level: 'info', msg: 'ServiceRequest created from ORC segment', event: 'ORM^O01' },
    { level: 'debug', msg: 'ORC-1 order control: NW (new order)', event: 'ORM^O01' },
    { level: 'error', msg: 'Duplicate order detected: correlationId already processed', event: 'ORM^O01' },
    { level: 'info', msg: 'Order priority escalated to STAT from OBR-27', event: 'ORM^O01' },
  ],
  'Lab-result-pipeline': [
    { level: 'info', msg: 'OBX result parsed: Glucose 95 mg/dL (normal)', event: 'ORU^R01' },
    { level: 'error', msg: 'FHIR validation: Observation.code requires LOINC system URI', event: 'ORU^R01' },
    { level: 'warn', msg: 'OBX-5 value type CE not fully mapped, using CodeableConcept', event: 'ORU^R01' },
    { level: 'info', msg: 'DiagnosticReport bundle created with 4 observations', event: 'ORU^R01' },
    { level: 'debug', msg: 'Terminology lookup: LOINC 2345-7 resolved to Glucose [Mass/Vol]', event: 'ORU^R01' },
    { level: 'info', msg: 'Critical result flagged: Potassium 6.2 mEq/L above range', event: 'ORU^R01' },
    { level: 'warn', msg: 'OBR-22 result status changed from P to F mid-stream', event: 'ORU^R01' },
  ],
  'Pharmacy-feed': [
    { level: 'info', msg: 'RXE dispense event mapped to MedicationDispense', event: 'RDE^O11' },
    { level: 'debug', msg: 'Terminology lookup: NDC 00069-3150-83 resolved to RxNorm 153165', event: 'RDE^O11' },
    { level: 'warn', msg: 'RXR route code not in SNOMED route valueset, using text fallback', event: 'RDE^O11' },
    { level: 'info', msg: 'MedicationRequest created from ORC+RXE segments', event: 'RDE^O11' },
    { level: 'error', msg: 'Pharmacy system timeout: retry 2/3 for dispense confirmation', event: 'RDE^O11' },
    { level: 'debug', msg: 'SIG parsing: "1 TAB PO BID" mapped to Dosage with timing', event: 'RDE^O11' },
  ],
};

// ─── Mock data generators ─────────────────────────────────────────────────────

function generateSparkline(count: number, min: number, max: number, volatility: number): Array<{ timestamp: number; value: number }> {
  const now = Date.now();
  const intervalMs = (60 * 60 * 1000) / count;
  const values: Array<{ timestamp: number; value: number }> = [];
  let current = min + (max - min) * 0.5;

  for (let i = 0; i < count; i++) {
    const drift = (Math.random() - 0.5) * 2 * volatility * (max - min);
    current = Math.max(min, Math.min(max, current + drift));
    values.push({
      timestamp: now - (count - 1 - i) * intervalMs,
      value: Math.round(current * 100) / 100,
    });
  }
  return values;
}

function generateMockMetrics(): MetricsSnapshot {
  const throughput: MetricSeries[] = WORKFLOW_NAMES.map((name) => ({
    name: 'workflow_events_per_minute',
    labels: { workflow: name },
    values: generateSparkline(20, 40, 220, 0.12),
  }));

  const latency: MetricSeries[] = [
    { name: 'workflow_latency_p50', labels: { quantile: '0.5' }, values: generateSparkline(20, 5, 18, 0.08) },
    { name: 'workflow_latency_p95', labels: { quantile: '0.95' }, values: generateSparkline(20, 20, 45, 0.1) },
    { name: 'workflow_latency_p99', labels: { quantile: '0.99' }, values: generateSparkline(20, 35, 55, 0.1) },
  ];

  const errorRate: MetricSeries[] = WORKFLOW_NAMES.map((name) => ({
    name: 'workflow_error_rate_pct',
    labels: { workflow: name },
    values: generateSparkline(20, 0.05, 2.5, 0.15),
  }));

  const dlqDepth: MetricSeries[] = [
    {
      name: 'dlq_depth',
      labels: { queue: 'default' },
      values: generateSparkline(20, 0, 8, 0.2),
    },
  ];

  return { throughput, latency, errorRate, dlqDepth, lastUpdated: Date.now() };
}

function generateMockLogs(count: number): LogEntry[] {
  const entries: LogEntry[] = [];
  const now = Date.now();

  for (let i = 0; i < count; i++) {
    const workflow = WORKFLOW_NAMES[Math.floor(Math.random() * WORKFLOW_NAMES.length)]!;
    const pool = LOG_MESSAGES[workflow]!;
    const entry = pool[Math.floor(Math.random() * pool.length)]!;
    entries.push({
      timestamp: now - i * (2000 + Math.random() * 3000),
      level: entry.level,
      message: entry.msg,
      labels: { workflow, source: 'fi-fhir-engine' },
      workflowName: workflow,
      eventType: entry.event,
    });
  }
  return entries.sort((a, b) => b.timestamp - a.timestamp);
}

function generateMockAlerts(): Alert[] {
  const now = Date.now();
  return [
    {
      id: 'alert-001',
      name: 'HighErrorRate',
      severity: 'critical',
      state: 'firing',
      summary: 'ORM-routing error rate above 2% for 5 minutes',
      description: 'The ORM-routing workflow error rate has exceeded the 2% threshold. Recent errors indicate duplicate order detection and routing table misses.',
      startsAt: now - 8 * 60 * 1000,
      labels: { workflow: 'ORM-routing', team: 'integration' },
    },
    {
      id: 'alert-002',
      name: 'DLQBacklog',
      severity: 'warning',
      state: 'firing',
      summary: 'Dead-letter queue depth at 4 messages',
      description: 'Messages accumulating in the dead-letter queue. Manual review recommended to prevent data loss.',
      startsAt: now - 22 * 60 * 1000,
      labels: { queue: 'default', team: 'platform' },
    },
    {
      id: 'alert-003',
      name: 'TerminologyServiceDegraded',
      severity: 'warning',
      state: 'pending',
      summary: 'LOINC terminology lookups averaging 340ms (threshold: 200ms)',
      description: 'Terminology service response times are elevated. Pharmacy-feed and Lab-result-pipeline latencies may be affected.',
      startsAt: now - 4 * 60 * 1000,
      labels: { service: 'terminology-svc', team: 'platform' },
    },
    {
      id: 'alert-004',
      name: 'PharmacyFeedRetries',
      severity: 'info',
      state: 'resolved',
      summary: 'Pharmacy downstream retries returned to normal',
      startsAt: now - 45 * 60 * 1000,
      labels: { workflow: 'Pharmacy-feed', team: 'integration' },
    },
  ];
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

// ─── Actions ──────────────────────────────────────────────────────────────────

export async function fetchMetrics(): Promise<void> {
  observabilityState.update((s) => ({ ...s, isLoadingMetrics: true, error: null }));

  try {
    const client = getPlatformClient();
    if (client?.isConnected()) {
      try {
        const result = await client.callTool('mcp-prometheus', 'query_range', {
          query: 'rate(workflow_events_total[5m])',
          start: new Date(Date.now() - 3600000).toISOString(),
          end: new Date().toISOString(),
          step: '180s',
        });
        if (result) {
          observabilityState.update((s) => ({
            ...s,
            metrics: result as MetricsSnapshot,
            isLoadingMetrics: false,
          }));
          return;
        }
      } catch {
        // Fall through to simulated data
      }
    }

    // Simulated data fallback
    const metrics = generateMockMetrics();
    observabilityState.update((s) => ({ ...s, metrics, isLoadingMetrics: false }));
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to fetch metrics';
    observabilityState.update((s) => ({ ...s, isLoadingMetrics: false, error: message }));
  }
}

export async function fetchLogs(filter?: LogFilter | undefined): Promise<void> {
  observabilityState.update((s) => ({ ...s, isLoadingLogs: true, error: null }));

  try {
    const client = getPlatformClient();
    if (client?.isConnected()) {
      try {
        const result = await client.callTool('mcp-loki', 'loki_query_range', {
          query: '{app="fi-fhir-engine"}',
          start: new Date(Date.now() - 3600000).toISOString(),
          end: new Date().toISOString(),
          limit: 50,
        });
        if (result) {
          observabilityState.update((s) => ({
            ...s,
            logs: result as LogEntry[],
            isLoadingLogs: false,
            logFilter: filter ?? s.logFilter,
          }));
          return;
        }
      } catch {
        // Fall through to simulated data
      }
    }

    // Simulated data fallback
    const logs = generateMockLogs(40);
    observabilityState.update((s) => ({
      ...s,
      logs,
      isLoadingLogs: false,
      logFilter: filter ?? s.logFilter,
    }));
  } catch (err) {
    const message = err instanceof Error ? err.message : 'Failed to fetch logs';
    observabilityState.update((s) => ({ ...s, isLoadingLogs: false, error: message }));
  }
}

export async function fetchAlerts(): Promise<void> {
  try {
    const client = getPlatformClient();
    if (client?.isConnected()) {
      try {
        const result = await client.callTool('mcp-alertmanager', 'am_list_alerts', {});
        if (result) {
          observabilityState.update((s) => ({
            ...s,
            alerts: result as Alert[],
          }));
          return;
        }
      } catch {
        // Fall through to simulated data
      }
    }

    // Simulated data fallback
    const alerts = generateMockAlerts();
    observabilityState.update((s) => ({ ...s, alerts }));
  } catch {
    // Alerts fetch is best-effort
  }
}

export function setLogFilter(filter: LogFilter): void {
  observabilityState.update((s) => ({ ...s, logFilter: filter }));
}
