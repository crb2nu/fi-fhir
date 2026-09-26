/**
 * Compact local timestamp for dense tables and detail lines: `YYYY-MM-DD HH:mm`
 * (`HH:mm:ss` with `seconds`). Missing values render as an em dash;
 * unparseable values pass through as-is.
 */
export function formatProfileTimestamp(
  value: string | null | undefined,
  options: { seconds?: boolean } = {}
): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  const pad = (n: number) => String(n).padStart(2, '0');
  const time = `${pad(parsed.getHours())}:${pad(parsed.getMinutes())}`;
  return (
    `${parsed.getFullYear()}-${pad(parsed.getMonth() + 1)}-${pad(parsed.getDate())} ` +
    (options.seconds ? `${time}:${pad(parsed.getSeconds())}` : time)
  );
}
