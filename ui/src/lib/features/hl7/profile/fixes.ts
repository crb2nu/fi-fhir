import type { HL7ProfileDraft, ProfileFix } from './types';
import type { WarningLike } from '$lib/domain/warnings';

function uniqSorted(xs: string[]): string[] {
  return Array.from(new Set(xs)).sort((a, b) => a.localeCompare(b));
}

function addMissingSegment(draft: HL7ProfileDraft, seg: string): HL7ProfileDraft {
  const next = uniqSorted([...draft.tolerate.missingSegments, seg]);
  return { ...draft, tolerate: { ...draft.tolerate, missingSegments: next } };
}

export function suggestFixes(warnings: readonly WarningLike[]): ProfileFix[] {
  const fixes: ProfileFix[] = [];

  // Missing segments (semantic warnings): code like MISSING_PV1, path like PV1
  for (const w of warnings) {
    const m = /^MISSING_([A-Z0-9]{3})$/.exec(w.code);
    const seg = m?.[1];
    if (!seg) continue;

    fixes.push({
      id: `tolerate-missing-${seg}`,
      title: `Tolerate missing ${seg}`,
      description: `Allow parsing to continue when ${seg} is absent (records a warning instead of failing).`,
      apply: (draft) => addMissingSegment(draft, seg)
    });
  }

  // Dedupe by id
  const byId = new Map<string, ProfileFix>();
  for (const f of fixes) byId.set(f.id, f);
  return Array.from(byId.values()).sort((a, b) => a.id.localeCompare(b.id));
}

