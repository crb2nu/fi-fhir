/**
 * Returns an updated tab-memory source history. Source labels can be derived
 * from filenames and therefore must never be persisted by this helper.
 */
export function rememberRecentSource(
  current: readonly string[],
  source: string,
  maxEntries = 8
): string[] {
  const canonical = source.trim();
  if (!canonical || maxEntries <= 0) {
    return [...current];
  }
  return [canonical, ...current.filter((entry) => entry !== canonical)].slice(0, maxEntries);
}
