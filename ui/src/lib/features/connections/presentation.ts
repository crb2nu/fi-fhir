/** Small display helpers shared by the Connections views. */

/** A digest's hash without its `sha256:` prefix, cut to `length` characters. */
export function shortHash(digest: string | null | undefined, length = 12): string {
  if (!digest) return '';
  const value = digest.startsWith('sha256:') ? digest.slice('sha256:'.length) : digest;
  return value.length <= length ? value : `${value.slice(0, length)}…`;
}

/** The Download file name of a revision: `<connection id>-r<revision id>.json`. */
export function revisionFileName(artifactId: string, revisionId: string): string {
  return `${artifactId}-r${revisionId}.json`;
}

/** `2026-09-26 21:04` in UTC: minute precision for a table cell; the full instant goes in its title. */
export function formatMinute(value: string | null | undefined): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toISOString().slice(0, 16).replace('T', ' ');
}
