/**
 * Validation for the Dry Run panel's "Custom JSON" event source.
 *
 * The Run button is disabled whenever no events resolve, so a malformed custom
 * payload otherwise disables the button with no explanation. This pure helper
 * lets the panel surface the reason *inline* (and in the disabled-button
 * tooltip) instead of a post-click toast — per the toast-budget policy
 * (.loom/22, B1 persistent validation + B2 disabled-control preconditions).
 *
 * Custom events are a JSON array (a bare object is wrapped into one), so this
 * deliberately differs from `validateEventPayload`, which validates a single
 * object for a manual workflow run.
 */

/**
 * Returns an inline-ready message when the custom-events JSON is present but
 * unparseable, otherwise null. Only meaningful for the 'custom' event source;
 * other sources never carry a JSON parse error.
 */
export function customEventsJsonError(
  eventSource: string,
  customEventJson: string,
): string | null {
  if (eventSource !== 'custom') return null;
  if (!customEventJson.trim()) return null;

  try {
    JSON.parse(customEventJson);
    return null;
  } catch {
    return 'Invalid JSON for custom events';
  }
}
