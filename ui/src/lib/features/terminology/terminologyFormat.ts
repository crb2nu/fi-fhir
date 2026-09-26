/**
 * Display helpers shared by the terminology views: labels, badge tones and
 * the compact timestamp / percentage formats the tables use.
 */
import type {
  AutorouteDecision,
  MappingEquivalence,
  MappingOrigin,
  PendingAutorouteStatus,
  TemporalWorkflowStatus
} from '$lib/gen/graphql';
import type { BadgeTone } from '$lib/ui/primitives';

function pad(value: number): string {
  return String(value).padStart(2, '0');
}

/**
 * Local `YYYY-MM-DD HH:mm` (or `YYYY-MM-DD` with `withTime = false`). Missing
 * values return '' so KeyValue renders its em dash.
 */
export function formatTimestamp(
  value: string | null | undefined,
  withTime = true
): string {
  if (!value) return '';
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  const day = `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}`;
  return withTime ? `${day} ${pad(date.getHours())}:${pad(date.getMinutes())}` : day;
}

/** 0.953 → "95%" (digits = 0) or "95.3%" (digits = 1); missing → ''. */
export function formatPercent(value: number | null | undefined, digits = 0): string {
  if (value === null || value === undefined || Number.isNaN(value)) return '';
  return `${(value * 100).toFixed(digits)}%`;
}

export function formatDurationMs(ms: number | null | undefined): string {
  if (ms === null || ms === undefined) return '';
  if (ms < 1000) return `${ms} ms`;
  if (ms < 60000) return `${(ms / 1000).toFixed(1)} s`;
  const mins = Math.floor(ms / 60000);
  const secs = Math.floor((ms % 60000) / 1000);
  return `${mins}m ${secs}s`;
}

export function equivalenceLabel(eq: MappingEquivalence | null | undefined): string {
  switch (eq) {
    case 'EQUIVALENT':
      return 'Equivalent';
    case 'WIDER':
      return 'Wider';
    case 'NARROWER':
      return 'Narrower';
    case 'INEXACT':
      return 'Inexact';
    case null:
    case undefined:
      return '';
    default:
      return String(eq);
  }
}

/** Only an inexact match is a state worth colour; the rest stay neutral. */
export function equivalenceTone(eq: MappingEquivalence | null | undefined): BadgeTone {
  return eq === 'INEXACT' ? 'warning' : 'neutral';
}

export function originLabel(origin: MappingOrigin | string): string {
  switch (origin) {
    case 'CSV_UPLOAD':
      return 'CSV upload';
    case 'APPROVED_AUTOROUTE':
      return 'Approved autoroute';
    case 'MANUAL':
      return 'Manual';
    default:
      return String(origin);
  }
}

export function confidenceTone(value: number | null | undefined): BadgeTone {
  if (value === null || value === undefined) return 'neutral';
  if (value >= 0.9) return 'success';
  if (value >= 0.7) return 'neutral';
  if (value >= 0.5) return 'warning';
  return 'danger';
}

export function pendingStatusLabel(status: PendingAutorouteStatus): string {
  switch (status) {
    case 'PENDING':
      return 'Pending';
    case 'APPROVED':
      return 'Approved';
    case 'REJECTED':
      return 'Rejected';
    case 'EXPIRED':
      return 'Expired';
    default:
      return String(status);
  }
}

export function pendingStatusTone(status: PendingAutorouteStatus): BadgeTone {
  switch (status) {
    case 'PENDING':
      return 'warning';
    case 'APPROVED':
      return 'success';
    case 'REJECTED':
      return 'danger';
    default:
      return 'neutral';
  }
}

export function decisionLabel(decision: AutorouteDecision): string {
  switch (decision) {
    case 'PERSISTENT_HIT':
      return 'Persistent match';
    case 'AUTOROUTE_HIGH_CONF':
      return 'High confidence';
    case 'AUTOROUTE_MED_CONF':
      return 'Medium confidence';
    case 'AUTOROUTE_LOW_CONF':
      return 'Low confidence';
    case 'NO_MATCH':
      return 'No match';
    default:
      return String(decision);
  }
}

export function decisionTone(decision: AutorouteDecision): BadgeTone {
  switch (decision) {
    case 'PERSISTENT_HIT':
    case 'AUTOROUTE_HIGH_CONF':
      return 'success';
    case 'AUTOROUTE_MED_CONF':
      return 'warning';
    case 'AUTOROUTE_LOW_CONF':
    case 'NO_MATCH':
      return 'danger';
    default:
      return 'neutral';
  }
}

export function workflowStatusLabel(status: TemporalWorkflowStatus): string {
  switch (status) {
    case 'RUNNING':
      return 'Running';
    case 'COMPLETED':
      return 'Completed';
    case 'FAILED':
      return 'Failed';
    case 'CANCELED':
      return 'Canceled';
    case 'TERMINATED':
      return 'Terminated';
    case 'TIMED_OUT':
      return 'Timed out';
    case 'CONTINUED_AS_NEW':
      return 'Continued';
    default:
      return String(status);
  }
}

export function workflowStatusTone(status: TemporalWorkflowStatus): BadgeTone {
  switch (status) {
    case 'RUNNING':
      return 'info';
    case 'COMPLETED':
      return 'success';
    case 'FAILED':
    case 'TIMED_OUT':
      return 'danger';
    default:
      return 'neutral';
  }
}
