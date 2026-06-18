/**
 * Validation for the MappingUploader's file picker / drop zone.
 *
 * The uploader only accepts CSV files. A non-CSV selection is a *persistent*
 * validation state — true until the user picks a different file — so per the
 * toast-budget policy (.loom/22, B1) it belongs inline next to the drop zone,
 * not in a 4-second toast. This pure helper returns the inline-ready message so
 * the component can surface it without a toast.
 */

/**
 * Returns an inline-ready message when the file is not a CSV, otherwise null.
 *
 * The check mirrors the prior guard exactly (`name.endsWith('.csv')`) so this is
 * a behaviour-preserving redirect, not a semantics change: only a name ending in
 * the lowercase `.csv` extension is accepted, matching the `<input accept=".csv">`
 * filter on the picker.
 */
export function validateCsvFile(name: string): string | null {
  if (!name.endsWith('.csv')) {
    return 'Please select a CSV file';
  }
  return null;
}
