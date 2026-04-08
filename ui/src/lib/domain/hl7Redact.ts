export type HL7RedactionMode = 'none' | 'mask_basic' | 'segment_sanitize' | 'pattern_replace';

type HL7Delimiters = {
  field: string;
  component: string;
  repetition: string;
  escape: string;
  subcomponent: string;
};

const defaultDelimiters: HL7Delimiters = {
  field: '|',
  component: '^',
  repetition: '~',
  escape: '\\',
  subcomponent: '&'
};

/**
 * Common patterns that likely contain PHI (best-effort).
 */
const PHI_PATTERNS = [
  // SSN: XXX-XX-XXXX or XXXXXXXXX
  /\b\d{3}-\d{2}-\d{4}\b/g,
  /\b\d{9}\b/g,
  // Phone: (XXX) XXX-XXXX or XXX-XXX-XXXX
  /\b\(\d{3}\) \d{3}-\d{4}\b/g,
  /\b\d{3}-\d{3}-\d{4}\b/g,
  // Email
  /\b[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Z|a-z]{2,}\b/g,
  // Date of Birth (basic YYYYMMDD or common separators)
  /\b(19|20)\d{2}[01]\d[0123]\d\b/g
];

function normalizeLines(raw: string): string {
  return raw.replaceAll('\r\n', '\r').replaceAll('\n', '\r');
}

function parseDelimiters(mshLine: string): HL7Delimiters {
  if (!mshLine.startsWith('MSH')) return defaultDelimiters;
  const field = mshLine.charAt(3) || defaultDelimiters.field;
  const enc = mshLine.length >= 8 ? mshLine.slice(4, 8) : '^~\\&';
  return {
    field,
    component: enc[0] ?? '^',
    repetition: enc[1] ?? '~',
    escape: enc[2] ?? '\\',
    subcomponent: enc[3] ?? '&'
  };
}

function redactFields(parts: string[], fieldsToMask: number[], replacement: string): string[] {
  // parts[0] is segment ID; for non-MSH segments, parts[i] corresponds to HL7 field i.
  const next = [...parts];
  for (const idx of fieldsToMask) {
    if (idx >= 1 && idx < next.length) {
      next[idx] = replacement;
    }
  }
  return next;
}

function sanitizeSegment(parts: string[], fieldSep: string): string {
  const id = parts[0] ?? '';
  return `${id}${fieldSep}REDACTED`;
}

function redactByPattern(text: string): string {
  let out = text;
  for (const pattern of PHI_PATTERNS) {
    out = out.replace(pattern, 'REDACTED');
  }
  return out;
}

/**
 * Best-effort PHI redaction for HL7v2 payloads.
 *
 * Notes:
 * - This is intentionally "basic": it prioritizes keeping the message parseable over full PHI removal.
 * - Free-text segments/fields (e.g., OBX-5 TX, NTE-3) may still contain PHI.
 */
export function redactHL7(raw: string, mode: HL7RedactionMode): string {
  if (mode === 'none') return raw;

  const normalized = normalizeLines(raw);
  const lines = normalized.split('\r').filter((l) => l.trim().length > 0);
  if (lines.length === 0) return raw;

  const d = parseDelimiters(lines[0] ?? '');
  const out: string[] = [];

  for (const line of lines) {
    const id = line.slice(0, 3);
    if (!id) continue;
    if (id === 'MSH') {
      out.push(line);
      continue;
    }

    if (mode === 'pattern_replace') {
      out.push(redactByPattern(line));
      continue;
    }

    const parts = line.split(d.field);

    if (mode === 'segment_sanitize') {
      // Keep segment presence but strip content for common PHI segments.
      if (['PID', 'NK1', 'GT1', 'IN1', 'IN2', 'IN3'].includes(id)) {
        out.push(sanitizeSegment(parts, d.field));
        continue;
      }
      out.push(line);
      continue;
    }

    // mode === 'mask_basic'
    if (id === 'PID') {
      // PID-2,3,5,6,7,11,13,14,19 commonly include PHI / identifiers.
      out.push(redactFields(parts, [2, 3, 5, 6, 7, 11, 13, 14, 19], 'REDACTED').join(d.field));
      continue;
    }
    if (id === 'NK1') {
      // NK1-2 (name), 4-6 (address/phone), etc.
      out.push(redactFields(parts, [2, 4, 5, 6], 'REDACTED').join(d.field));
      continue;
    }
    if (id === 'GT1') {
      out.push(redactFields(parts, [3, 4, 5, 6, 7, 8, 9], 'REDACTED').join(d.field));
      continue;
    }
    if (id === 'IN1' || id === 'IN2' || id === 'IN3') {
      out.push(sanitizeSegment(parts, d.field));
      continue;
    }
    if (id === 'PV1') {
      // PV1-19 (visit number) often correlates to patient encounter.
      out.push(redactFields(parts, [19], 'REDACTED').join(d.field));
      continue;
    }

    out.push(line);
  }

  return out.join('\r');
}
