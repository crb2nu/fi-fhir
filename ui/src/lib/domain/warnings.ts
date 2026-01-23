export type ParsePhase = 'byte' | 'syntactic' | 'semantic' | 'edi_companion' | string;

export type WarningSeverity = 'info' | 'warning' | 'error';

export type WarningLike = {
  phase: string;
  code: string;
  message: string;
  path?: string | null;
  // LLM-generated fields (optional, populated on-demand or via explainWarnings query)
  explanation?: string | null;
  fixSuggestion?: string | null;
  impact?: string | null;
  severity?: WarningSeverity | string | null;
  fromCache?: boolean | null;
};

/**
 * Checks if a warning has an LLM-generated explanation.
 */
export function hasExplanation(w: WarningLike): boolean {
  return !!w.explanation;
}

/**
 * Updates a warning in a list with explanation data.
 * Returns a new list with the updated warning.
 */
export function updateWarningWithExplanation(
  warnings: WarningLike[],
  code: string,
  explanation: {
    explanation: string;
    fixSuggestion?: string | null;
    impact?: string | null;
    fromCache: boolean;
  }
): WarningLike[] {
  return warnings.map((w) =>
    w.code === code
      ? {
          ...w,
          explanation: explanation.explanation,
          fixSuggestion: explanation.fixSuggestion ?? null,
          impact: explanation.impact ?? null,
          fromCache: explanation.fromCache
        }
      : w
  );
}

export type WarningGroup = {
  phase: ParsePhase;
  items: WarningLike[];
};

export function groupWarningsByPhase(warnings: readonly WarningLike[]): WarningGroup[] {
  const map = new Map<string, WarningLike[]>();
  for (const w of warnings) {
    const key = w.phase || 'unknown';
    const existing = map.get(key);
    if (existing) {
      existing.push(w);
    } else {
      map.set(key, [w]);
    }
  }

  const phaseOrder = ['byte', 'syntactic', 'semantic', 'edi_companion', 'unknown'] as const;
  const phaseRank = (p: string): number => {
    const idx = phaseOrder.indexOf(p as (typeof phaseOrder)[number]);
    return idx === -1 ? phaseOrder.length : idx;
  };

  const sortedEntries = Array.from(map.entries()).sort(([a], [b]) => {
    const ra = phaseRank(a);
    const rb = phaseRank(b);
    if (ra !== rb) return ra - rb;
    return a.localeCompare(b);
  });

  return sortedEntries.map(([phase, items]) => {
    const sortedItems = [...items].sort((x, y) => {
      const codeCmp = x.code.localeCompare(y.code);
      if (codeCmp !== 0) return codeCmp;
      return (x.path ?? '').localeCompare(y.path ?? '');
    });
    return { phase, items: sortedItems };
  });
}
