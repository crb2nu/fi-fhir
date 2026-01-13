export type HL7PathLocation =
  | {
      kind: 'segment';
      segmentId: string;
    }
  | {
      kind: 'field';
      segmentId: string;
      field: number;
    }
  | {
      kind: 'component';
      segmentId: string;
      field: number;
      component: number;
    }
  | {
      kind: 'repetition';
      segmentId: string;
      field: number;
      repetition: number;
    }
  | {
      kind: 'repetition_component';
      segmentId: string;
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
 */
export function parseHL7Path(path: string | null | undefined): HL7PathLocation | null {
  const p = (path ?? '').trim();
  if (!p) return null;

  if (isSegmentId(p)) {
    return { kind: 'segment', segmentId: p };
  }

  // Dot notation: SEG.N, optionally with [rep], optionally with .component
  {
    const m = /^([A-Z0-9]{3})\.(\d+)(?:\[(\d+)\])?(?:\.(\d+))?$/.exec(p);
    if (m) {
      const segmentId = m[1]!;
      const field = Number(m[2]!);
      const repetition = m[3] !== undefined ? Number(m[3]) : null;
      const component = m[4] !== undefined ? Number(m[4]) : null;

      if (!Number.isFinite(field) || field <= 0) return null;

      if (repetition !== null && component !== null) {
        return { kind: 'repetition_component', segmentId, field, repetition, component };
      }
      if (repetition !== null) {
        return { kind: 'repetition', segmentId, field, repetition };
      }
      if (component !== null) {
        return { kind: 'component', segmentId, field, component };
      }
      return { kind: 'field', segmentId, field };
    }
  }

  // Dash notation: SEG-N, optionally .component
  {
    const m = /^([A-Z0-9]{3})-(\d+)(?:\.(\d+))?$/.exec(p);
    if (m) {
      const segmentId = m[1]!;
      const field = Number(m[2]!);
      const component = m[3] !== undefined ? Number(m[3]) : null;
      if (!Number.isFinite(field) || field <= 0) return null;
      if (component !== null) {
        return { kind: 'component', segmentId, field, component };
      }
      return { kind: 'field', segmentId, field };
    }
  }

  return null;
}

