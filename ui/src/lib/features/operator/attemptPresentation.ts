/**
 * Presentation rules for durable delivery attempts and dead letters.
 *
 * Pure so the tables, badges, and — critically — the *preconditions* for every
 * control action can be unit tested. Per the toast-budget policy (.loom/22,
 * B2), a control the operator cannot use is disabled with an explanatory
 * `title` rather than allowed to fire and then rejected by a toast.
 */

export type BadgeVariant = 'default' | 'primary' | 'success' | 'warning' | 'danger' | 'info';

/**
 * Maps a presentation variant onto the design system's Badge tone. The accent
 * is reserved for selection (`ui/docs/DESIGN.md`), so the old `primary`
 * variant — used for states such as "published" — reads as `info`.
 */
export function badgeTone(
  variant: BadgeVariant
): 'neutral' | 'success' | 'warning' | 'danger' | 'info' {
  switch (variant) {
    case 'success':
    case 'warning':
    case 'danger':
    case 'info':
      return variant;
    case 'primary':
      return 'info';
    default:
      return 'neutral';
  }
}

export interface AttemptLike {
  status: string;
  outboxStatus: string;
  deadLetter?: { active: boolean; resolution: string } | null;
}

/** Maps a durable attempt status onto the shared Badge variants. */
export function attemptStatusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'succeeded':
      return 'success';
    case 'failed':
      return 'danger';
    case 'queued':
      return 'info';
    default:
      return 'default';
  }
}

/** Maps a durable outbox status onto the shared Badge variants. */
export function outboxStatusVariant(status: string): BadgeVariant {
  switch (status) {
    case 'published':
      return 'success';
    case 'failed':
      return 'danger';
    case 'leased':
      return 'warning';
    case 'pending':
      return 'info';
    default:
      return 'default';
  }
}

/** Maps a circuit state onto the shared Badge variants. */
export function circuitStateVariant(state: string): BadgeVariant {
  return state === 'open' ? 'danger' : 'success';
}

/**
 * Describes what happened to a dead letter in operator language. An entry with
 * no recorded resolution is still open; the server guarantees a closed entry
 * always carries one.
 */
export function deadLetterStateLabel(entry: AttemptLike['deadLetter']): string {
  if (!entry) {
    return 'Never dead-lettered';
  }
  if (entry.active) {
    return 'Awaiting operator decision';
  }
  switch (entry.resolution) {
    case 'replayed':
      return 'Closed by replay';
    case 'resubmitted':
      return 'Closed by resubmit';
    case 'discarded':
      return 'Discarded by an operator';
    default:
      return 'Closed';
  }
}

export type DeliveryAction = 'replay' | 'resubmit' | 'discard';

/**
 * Returns null when the action is available, otherwise the reason it is not.
 *
 * The string is used as the disabled control's `title`, so a dead click never
 * happens and the operator still learns why. This mirrors the server's
 * precondition exactly: every recovery action requires an *active* dead letter.
 */
export function deliveryActionBlockedReason(
  attempt: AttemptLike | null,
  action: DeliveryAction
): string | null {
  if (!attempt) {
    return 'Select a delivery attempt first.';
  }
  if (!attempt.deadLetter) {
    return `Only a dead-lettered attempt can be ${pastTense(action)}. This attempt never entered the dead-letter queue.`;
  }
  if (!attempt.deadLetter.active) {
    return `This dead letter is already resolved (${deadLetterStateLabel(attempt.deadLetter).toLowerCase()}).`;
  }
  return null;
}

function pastTense(action: DeliveryAction): string {
  switch (action) {
    case 'replay':
      return 'replayed';
    case 'resubmit':
      return 'resubmitted';
    case 'discard':
      return 'discarded';
  }
}

export type DeploymentAction = 'pause' | 'resume' | 'retire' | 'deploy';

/**
 * Returns null when a lifecycle command is available for the current state,
 * otherwise why it is not. This encodes the closed Slice 2.1 state machine:
 * published -> deployed | retired; deployed -> paused | retired;
 * paused -> deployed | retired.
 */
export function deploymentActionBlockedReason(
  state: string | null,
  action: DeploymentAction
): string | null {
  if (!state) {
    return 'Select a deployment first.';
  }
  const allowed: Record<DeploymentAction, string[]> = {
    deploy: ['published'],
    pause: ['deployed'],
    resume: ['paused'],
    retire: ['published', 'deployed', 'paused']
  };
  if (allowed[action].includes(state)) {
    return null;
  }
  return `Cannot ${action} an integration in the "${state}" state. Allowed from: ${allowed[action].join(', ')}.`;
}

/** Maps a lifecycle state onto the shared Badge variants. */
export function deploymentStateVariant(state: string): BadgeVariant {
  switch (state) {
    case 'deployed':
      return 'success';
    case 'paused':
      return 'warning';
    case 'retired':
      return 'danger';
    case 'published':
    case 'approved':
      return 'primary';
    default:
      return 'default';
  }
}

/** Maps reported deployment health onto the shared Badge variants. */
export function deploymentHealthVariant(health: string): BadgeVariant {
  switch (health) {
    case 'healthy':
      return 'success';
    case 'degraded':
      return 'warning';
    case 'unhealthy':
      return 'danger';
    case 'starting':
      return 'info';
    default:
      return 'default';
  }
}

/**
 * The destination provenance-ledger columns the operator UI renders (Slice
 * 4.2c). Deliberately narrow: the ledger is clinical-content-free by
 * construction, and the UI shows exactly these columns and nothing else — no
 * payload, no response body, no diagnostics text ever reaches this shape.
 */
export interface DestinationDeliveryLike {
  transport: string;
  outcome: string;
  endpointAdvisory: string;
  fhirResourceTypes: readonly string[];
  fhirEntryCount: number;
  fhirOutcomeCodesAdvisory: readonly string[];
}

export interface DestinationDeliveryDisplay {
  transportLabel: string;
  outcomeLabel: string;
  outcomeVariant: BadgeVariant;
  /** FHIR resource types in bundle order; empty for a non-FHIR delivery. */
  resourceTypes: string[];
  /** e.g. "2 bundle entries"; null when the transport sends no Bundle. */
  entryCountText: string | null;
  /** OperationOutcome issue codes only — never diagnostics. */
  outcomeCodes: string[];
  /** The endpoint the destination revision declares; advisory, never trusted. */
  endpointText: string;
}

/** Turns one provenance-ledger row into what the Delivery block displays. */
export function describeDestinationDelivery(
  delivery: DestinationDeliveryLike
): DestinationDeliveryDisplay {
  const isFHIR = delivery.transport === 'fhir';
  return {
    transportLabel: transportLabel(delivery.transport),
    outcomeLabel: deliveryOutcomeLabel(delivery.outcome),
    outcomeVariant: deliveryOutcomeVariant(delivery.outcome),
    resourceTypes: [...delivery.fhirResourceTypes],
    entryCountText: isFHIR ? entryCountText(delivery.fhirEntryCount) : null,
    outcomeCodes: [...delivery.fhirOutcomeCodesAdvisory],
    endpointText: delivery.endpointAdvisory || 'No endpoint declared'
  };
}

function transportLabel(transport: string): string {
  switch (transport) {
    case 'fhir':
      return 'FHIR';
    case 'https':
      return 'HTTPS';
    default:
      return transport.toUpperCase() || 'Unknown transport';
  }
}

/** Maps a ledger outcome onto the shared Badge variants. */
export function deliveryOutcomeVariant(outcome: string): BadgeVariant {
  switch (outcome) {
    case 'delivered':
      return 'success';
    case 'retryable':
      return 'warning';
    case 'refused':
      return 'danger';
    default:
      return 'default';
  }
}

function deliveryOutcomeLabel(outcome: string): string {
  switch (outcome) {
    case 'delivered':
      return 'Delivered';
    case 'retryable':
      return 'Retryable failure';
    case 'refused':
      return 'Refused';
    default:
      return outcome || 'Unknown outcome';
  }
}

function entryCountText(count: number): string {
  return `${count} bundle ${count === 1 ? 'entry' : 'entries'}`;
}

/** Shortens a `sha256:...` digest for dense tables without losing identity. */
export function shortDigest(digest: string): string {
  const value = digest.startsWith('sha256:') ? digest.slice('sha256:'.length) : digest;
  return value.length <= 12 ? value : `${value.slice(0, 12)}…`;
}

/** Formats a durable timestamp for display, tolerating absent values. */
export function formatTimestamp(value: string | null | undefined): string {
  if (!value) {
    return '—';
  }
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) {
    return value;
  }
  return parsed.toISOString().replace('T', ' ').replace(/\.\d+Z$/, 'Z');
}
