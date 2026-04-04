/**
 * Diagnostic adapters that convert domain-specific problem structures
 * into the unified Diagnostic format consumed by the diagnostics store.
 */
import type { Diagnostic, DiagnosticSeverity } from './diagnosticsStore';

// ---------------------------------------------------------------------------
// ID generation
// ---------------------------------------------------------------------------

let _sequence = 0;

function nextId(prefix: string): string {
  _sequence += 1;
  return `${prefix}-${Date.now()}-${_sequence}`;
}

// ---------------------------------------------------------------------------
// Workflow validation adapter
// ---------------------------------------------------------------------------

/**
 * Convert workflow schema validation errors into diagnostics.
 * Each error has a JSON-path location and a human message.
 */
export function adaptWorkflowValidation(
  errors: Array<{ path: string; message: string }>
): Diagnostic[] {
  const now = Date.now();
  return errors.map((err) => ({
    id: nextId('wf'),
    severity: 'error' as DiagnosticSeverity,
    scope: scopeFromPath(err.path),
    stage: 'delivery' as const,
    message: err.message,
    detail: `Path: ${err.path}`,
    source: 'validateWorkflowDraft',
    target: { route: err.path },
    timestamp: now,
  }));
}

function scopeFromPath(path: string): Diagnostic['scope'] {
  if (path.includes('transform')) return 'transform';
  if (path.includes('action')) return 'action';
  if (path.startsWith('Route ') || path.includes('route')) return 'route';
  return 'workflow';
}

// ---------------------------------------------------------------------------
// Parser warnings adapter
// ---------------------------------------------------------------------------

/**
 * Convert HL7 / EDI / CDA parse warnings into diagnostics.
 */
export function adaptParserWarnings(
  warnings: Array<{ segment?: string; line?: number; message: string; level: string }>
): Diagnostic[] {
  const now = Date.now();
  return warnings.map((w) => ({
    id: nextId('parser'),
    severity: levelToSeverity(w.level),
    scope: 'parser' as const,
    stage: 'intake' as const,
    message: w.message,
    detail: w.segment ? `Segment: ${w.segment}` : undefined,
    source: 'parser',
    target: w.line != null ? { line: w.line } : undefined,
    timestamp: now,
  }));
}

function levelToSeverity(level: string): DiagnosticSeverity {
  if (level === 'error') return 'error';
  if (level === 'warning') return 'warning';
  return 'info';
}

// ---------------------------------------------------------------------------
// Debug session adapter
// ---------------------------------------------------------------------------

/**
 * Convert debug step failures into diagnostics.
 */
export function adaptDebugFailures(
  failures: Array<{ stepId: string; error: string; breakpointId?: string }>
): Diagnostic[] {
  const now = Date.now();
  return failures.map((f) => ({
    id: nextId('dbg'),
    severity: 'error' as DiagnosticSeverity,
    scope: 'runtime' as const,
    stage: 'delivery' as const,
    message: f.error,
    detail: f.breakpointId ? `Breakpoint: ${f.breakpointId}` : undefined,
    source: 'debugSession',
    target: { artifactId: f.stepId },
    timestamp: now,
  }));
}

// ---------------------------------------------------------------------------
// Runtime error adapter
// ---------------------------------------------------------------------------

/**
 * Convert runtime output errors into diagnostics.
 */
export function adaptRuntimeErrors(
  errors: Array<{ workflowName?: string; message: string; timestamp: number }>
): Diagnostic[] {
  return errors.map((err) => ({
    id: nextId('rt'),
    severity: 'error' as DiagnosticSeverity,
    scope: 'runtime' as const,
    stage: 'delivery' as const,
    message: err.message,
    detail: err.workflowName ? `Workflow: ${err.workflowName}` : undefined,
    source: 'runtimeOutput',
    target: err.workflowName ? { artifactId: err.workflowName } : undefined,
    timestamp: err.timestamp,
  }));
}

// ---------------------------------------------------------------------------
// Terminology adapter
// ---------------------------------------------------------------------------

/**
 * Convert unresolved or low-confidence terminology mappings into diagnostics.
 */
export function adaptTerminologyIssues(
  issues: Array<{ code: string; display: string; confidence?: number; message: string }>
): Diagnostic[] {
  const now = Date.now();
  return issues.map((issue) => {
    const isLowConfidence = issue.confidence != null && issue.confidence < 0.7;
    return {
      id: nextId('term'),
      severity: (isLowConfidence ? 'warning' : 'info') as DiagnosticSeverity,
      scope: 'terminology' as const,
      stage: 'translation' as const,
      message: issue.message,
      detail: `Code: ${issue.code} — ${issue.display}${issue.confidence != null ? ` (${Math.round(issue.confidence * 100)}%)` : ''}`,
      source: 'terminology',
      target: { artifactId: issue.code },
      timestamp: now,
    };
  });
}
