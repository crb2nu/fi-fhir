import type { HL7Field, HL7Message, HL7Segment } from '$lib/domain/hl7v2';
import type { HL7PathLocation } from '$lib/domain/hl7Path';

export function findFirstSegment(message: HL7Message, segmentId: string): HL7Segment | null {
  return message.segments.find((s) => s.id === segmentId) ?? null;
}

export function findField(segment: HL7Segment, fieldNumber: number): HL7Field | null {
  return segment.fields.find((f) => f.number === fieldNumber) ?? null;
}

export function getHL7Value(message: HL7Message, loc: HL7PathLocation | null): string | null {
  if (!loc) return null;
  const seg = findFirstSegment(message, loc.segmentId);
  if (!seg) return null;

  if (loc.kind === 'segment') return seg.raw;

  const field = findField(seg, loc.field);
  if (!field) return null;

  switch (loc.kind) {
    case 'field':
      return field.raw;
    case 'component':
      return field.components[loc.component - 1] ?? null;
    case 'repetition':
      return field.repetitions[loc.repetition]?.raw ?? null;
    case 'repetition_component':
      return field.repetitions[loc.repetition]?.components[loc.component - 1] ?? null;
  }
}

export function normalizeHL7Newlines(raw: string): string {
  return raw.replaceAll('\r\n', '\r').replaceAll('\n', '\r');
}

