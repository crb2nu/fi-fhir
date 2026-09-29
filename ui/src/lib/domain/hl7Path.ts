export type HL7PathLocation =
  | {
      kind: 'segment';
      segmentId: string;
      segmentOccurrence?: number;
    }
  | {
      kind: 'field';
      segmentId: string;
      segmentOccurrence?: number;
      field: number;
    }
  | {
      kind: 'component';
      segmentId: string;
      segmentOccurrence?: number;
      field: number;
      component: number;
    }
  | {
      kind: 'repetition';
      segmentId: string;
      segmentOccurrence?: number;
      field: number;
      repetition: number;
    }
  | {
      kind: 'repetition_component';
      segmentId: string;
      segmentOccurrence?: number;
      field: number;
      repetition: number;
      component: number;
    };

function isSegmentId(s: string): boolean {
  return /^[A-Z0-9]{3}$/.test(s);
}

/**
 * Best-effort parser for warning paths originating from fi-fhir.
 *
 * Supports:
 * - `PV1` (segment only)
 * - `PV1.2` (segment.field)
 * - `PID.3[0]` (segment.field[repetitionIndex])
 * - `PID.3[0].1` (segment.field[repetitionIndex].component)
 * - `PV1-2` / `PV1-2.1` (segment-field(.component))
 * - `PID-3[0]` / `PID-3[0].1` (segment-field[repetitionIndex](.component)),
 *   the form the lineage panel and session diagnostics use
 */
export function parseHL7Path(path: string | null | undefined): HL7PathLocation | null {
  const p = (path ?? '').trim();
  if (!p) return null;

  if (isSegmentId(p)) {
    return { kind: 'segment', segmentId: p };
  }

  // Segment-only with occurrence: SEG[0]
  {
    const m = /^([A-Z0-9]{3})\[(\d+)\]$/.exec(p);
    if (m) {
      const segmentId = m[1]!;
      const segmentOccurrence = Number(m[2]!);
      if (!Number.isFinite(segmentOccurrence) || segmentOccurrence < 0) return null;
      return { kind: 'segment', segmentId, segmentOccurrence };
    }
  }

  // Dot notation: SEG.N, optionally with [rep], optionally with .component
  {
    const m = /^([A-Z0-9]{3})(?:\[(\d+)\])?\.(\d+)(?:\[(\d+)\])?(?:\.(\d+))?$/.exec(p);
    if (m) return fieldLocation(m);
  }

  // Dash notation: SEG-N, optionally with [rep], optionally with .component
  {
    const m = /^([A-Z0-9]{3})(?:\[(\d+)\])?-(\d+)(?:\[(\d+)\])?(?:\.(\d+))?$/.exec(p);
    if (m) return fieldLocation(m);
  }

  return null;
}

/**
 * Builds a field-level location from a dot- or dash-notation match: groups are
 * segment id, segment occurrence, field, field repetition and component.
 * Dot and dash notation address the same thing, so they share this.
 */
function fieldLocation(m: RegExpExecArray): HL7PathLocation | null {
  const segmentId = m[1]!;
  const segmentOccurrence = m[2] !== undefined ? Number(m[2]) : null;
  const field = Number(m[3]!);
  const repetition = m[4] !== undefined ? Number(m[4]) : null;
  const component = m[5] !== undefined ? Number(m[5]) : null;

  if (!Number.isFinite(field) || field <= 0) return null;
  if (segmentOccurrence !== null && (!Number.isFinite(segmentOccurrence) || segmentOccurrence < 0)) {
    return null;
  }
  const occurrence = segmentOccurrence !== null ? { segmentOccurrence } : {};

  if (repetition !== null && component !== null) {
    return { kind: 'repetition_component', segmentId, ...occurrence, field, repetition, component };
  }
  if (repetition !== null) {
    return { kind: 'repetition', segmentId, ...occurrence, field, repetition };
  }
  if (component !== null) {
    return { kind: 'component', segmentId, ...occurrence, field, component };
  }
  return { kind: 'field', segmentId, ...occurrence, field };
}
