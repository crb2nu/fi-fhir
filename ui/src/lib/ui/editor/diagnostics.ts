/**
 * Convert EditorDiagnostic[] to CodeMirror 6 Diagnostic[].
 */
import type { Diagnostic } from '@codemirror/lint';
import type { EditorDiagnostic } from './types';

export function toCM6Diagnostics(diagnostics: EditorDiagnostic[]): Diagnostic[] {
  return diagnostics.map((d) => ({
    from: d.from,
    to: d.to,
    severity: d.severity,
    message: d.message
  }));
}
