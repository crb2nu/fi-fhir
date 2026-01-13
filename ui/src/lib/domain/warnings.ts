export type ParsePhase = 'byte' | 'syntactic' | 'semantic' | 'edi_companion' | string;

export type WarningLike = {
  phase: string;
  code: string;
  message: string;
  path?: string | null;
};

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

  return Array.from(map.entries())
    .sort(([a], [b]) => a.localeCompare(b))
    .map(([phase, items]) => ({ phase, items }));
}

