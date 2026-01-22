export type HL7Delimiters = {
  field: string;
  component: string;
  repetition: string;
  escape: string;
  subcomponent: string;
};

export type HL7Field = {
  number: number; // 1-based HL7 field number (MSH-1 is the field separator)
  raw: string;
  repetitions: HL7Repetition[];
  components: string[]; // Back-compat: first repetition components
};

export type HL7Repetition = {
  index: number; // 0-based repetition index
  raw: string;
  components: string[];
};

export type HL7Segment = {
  id: string;
  index: number; // 0-based segment index in message
  occurrence: number; // 0-based occurrence index for this segment ID
  fields: HL7Field[];
  raw: string;
};

export type HL7Message = {
  delimiters: HL7Delimiters;
  segments: HL7Segment[];
};

const defaultDelimiters: HL7Delimiters = {
  field: '|',
  component: '^',
  repetition: '~',
  escape: '\\',
  subcomponent: '&'
};

export function parseHL7Message(raw: string): HL7Message {
  const normalized = normalizeLines(raw);
  const lines = normalized.split('\r').filter((l) => l.trim().length > 0);

  const delimiters = parseDelimiters(lines[0] ?? '');
  const segments: HL7Segment[] = [];
  const occurrences = new Map<string, number>();

  for (const [index, line] of lines.entries()) {
    const seg = parseSegmentLine(line, delimiters);
    if (!seg) continue;
    const occ = occurrences.get(seg.id) ?? 0;
    occurrences.set(seg.id, occ + 1);
    segments.push({ ...seg, index, occurrence: occ });
  }

  return { delimiters, segments };
}

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

function parseSegmentLine(line: string, d: HL7Delimiters): Omit<HL7Segment, 'index' | 'occurrence'> | null {
  if (line.length < 3) return null;
  const id = line.slice(0, 3);
  const parts = line.split(d.field);

  if (parts.length === 0) return null;

  const fields: HL7Field[] = [];

  if (id === 'MSH') {
    // HL7 special case:
    // - MSH-1 is the field separator itself
    // - parts[1] is MSH-2 (encoding characters)
    fields.push({
      number: 1,
      raw: d.field,
      repetitions: [{ index: 0, raw: d.field, components: [d.field] }],
      components: [d.field]
    });

    for (let i = 1; i < parts.length; i++) {
      const num = i + 1; // parts[1] => MSH-2
      const value = parts[i] ?? '';
      const repetitions = splitRepetitions(value, d.repetition, d.component);
      fields.push({
        number: num,
        raw: value,
        repetitions,
        components: repetitions[0]?.components ?? ['']
      });
    }
  } else {
    // parts[0] is the segment ID; parts[1] corresponds to field 1, etc.
    for (let i = 1; i < parts.length; i++) {
      const num = i;
      const value = parts[i] ?? '';
      const repetitions = splitRepetitions(value, d.repetition, d.component);
      fields.push({
        number: num,
        raw: value,
        repetitions,
        components: repetitions[0]?.components ?? ['']
      });
    }
  }

  return { id, fields, raw: line };
}

function splitRepetitions(value: string, repetitionSep: string, componentSep: string): HL7Repetition[] {
  if (!value) {
    return [{ index: 0, raw: '', components: [''] }];
  }
  const reps = value.split(repetitionSep);
  return reps.map((raw, idx) => ({
    index: idx,
    raw,
    components: splitComponents(raw, componentSep)
  }));
}

function splitComponents(value: string, componentSep: string): string[] {
  if (!value) return [''];
  return value.split(componentSep);
}
