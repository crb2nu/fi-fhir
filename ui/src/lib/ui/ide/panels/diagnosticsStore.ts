/**
 * Unified cross-stage diagnostics store.
 *
 * Aggregates problems from all five journey stages into a single reactive
 * model that the Problems panel and badge indicator consume.
 */
import { writable, derived } from 'svelte/store';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export type DiagnosticSeverity = 'error' | 'warning' | 'info';

export type DiagnosticScope =
  | 'workflow'
  | 'route'
  | 'transform'
  | 'action'
  | 'runtime'
  | 'parser'
  | 'terminology';

export type JourneyStage =
  | 'intake'
  | 'normalization'
  | 'translation'
  | 'delivery'
  | 'verification';

export interface DiagnosticTarget {
  documentType?: string;
  artifactId?: string;
  route?: string;
  line?: number;
}

export interface Diagnostic {
  id: string;
  severity: DiagnosticSeverity;
  scope: DiagnosticScope;
  stage: JourneyStage;
  message: string;
  detail?: string;
  source: string;
  target?: DiagnosticTarget;
  timestamp: number;
}

export interface DiagnosticCounts {
  error: number;
  warning: number;
  info: number;
  total: number;
}

// ---------------------------------------------------------------------------
// Private writable
// ---------------------------------------------------------------------------

const _diagnostics = writable<Diagnostic[]>([]);

// ---------------------------------------------------------------------------
// Public mutation API
// ---------------------------------------------------------------------------

export function addDiagnostic(d: Diagnostic): void {
  _diagnostics.update((list) => [...list, d]);
}

export function addDiagnostics(ds: Diagnostic[]): void {
  if (ds.length === 0) return;
  _diagnostics.update((list) => [...list, ...ds]);
}

export function removeDiagnostic(id: string): void {
  _diagnostics.update((list) => list.filter((d) => d.id !== id));
}

export function clearBySource(source: string): void {
  _diagnostics.update((list) => list.filter((d) => d.source !== source));
}

export function clearByStage(stage: JourneyStage): void {
  _diagnostics.update((list) => list.filter((d) => d.stage !== stage));
}

export function clearAll(): void {
  _diagnostics.set([]);
}

// ---------------------------------------------------------------------------
// Derived stores
// ---------------------------------------------------------------------------

/** All diagnostics (read-only). */
export const diagnostics = derived(_diagnostics, ($d) => $d);

/** Diagnostics grouped by journey stage. */
export const diagnosticsByStage = derived(_diagnostics, ($d) => {
  const map = new Map<JourneyStage, Diagnostic[]>();
  for (const diag of $d) {
    const existing = map.get(diag.stage);
    if (existing) {
      existing.push(diag);
    } else {
      map.set(diag.stage, [diag]);
    }
  }
  return map;
});

/** Diagnostics grouped by severity. */
export const diagnosticsBySeverity = derived(_diagnostics, ($d) => {
  const errors: Diagnostic[] = [];
  const warnings: Diagnostic[] = [];
  const infos: Diagnostic[] = [];

  for (const diag of $d) {
    if (diag.severity === 'error') errors.push(diag);
    else if (diag.severity === 'warning') warnings.push(diag);
    else infos.push(diag);
  }

  return { errors, warnings, infos };
});

/** Counts per severity plus total. */
export const diagnosticCounts = derived(_diagnostics, ($d): DiagnosticCounts => {
  let error = 0;
  let warning = 0;
  let info = 0;

  for (const diag of $d) {
    if (diag.severity === 'error') error++;
    else if (diag.severity === 'warning') warning++;
    else info++;
  }

  return { error, warning, info, total: $d.length };
});
