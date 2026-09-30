/**
 * The downloaded form of an audited session export (.loom/42 E-3).
 *
 * The file is the server's bundle exactly as `exportIntegrationBundle`
 * returned it, under `bundle`, beside an `export` header that repeats what
 * the request asked for (the reason, whether raw payloads were requested).
 * The server's own record of the disclosure is the export row it wrote with
 * the verified caller; the header only lets a reader of the file see why it
 * exists without that database.
 */
import type { SessionBundle } from './workspaceApi';

export const SESSION_EXPORT_FORMAT = 'fi-fhir.integration-session-export/v1';

export interface SessionExportFile {
  format: typeof SESSION_EXPORT_FORMAT;
  export: {
    sessionId: string;
    exportedAt: string;
    reason: string;
    includeRawPayload: boolean;
  };
  bundle: SessionBundle;
}

export function sessionExportFile(bundle: SessionBundle, reason: string, includeRawPayload: boolean): SessionExportFile {
  return {
    format: SESSION_EXPORT_FORMAT,
    export: { sessionId: bundle.sessionId, exportedAt: bundle.exportedAt, reason, includeRawPayload },
    bundle
  };
}

/**
 * `fi-fhir-session-<id>-<UTC timestamp>.json`, from characters that are safe
 * in a file name on every platform: anything else in the id becomes `-`, runs
 * collapse, and the id is capped so a hostile id cannot make an unusable name.
 */
export function sessionExportFileName(sessionId: string, exportedAt: string): string {
  const id =
    sessionId
      .replace(/[^A-Za-z0-9._-]+/g, '-')
      .replace(/-{2,}/g, '-')
      .replace(/^[-.]+|[-.]+$/g, '')
      .slice(0, 64) || 'session';
  const parsed = Date.parse(exportedAt);
  const stamp = Number.isNaN(parsed)
    ? 'unknown-time'
    : new Date(parsed).toISOString().replace(/\.\d{3}Z$/, 'Z').replace(/[-:]/g, '');
  return `fi-fhir-session-${id}-${stamp}.json`;
}

/** Hands the browser the export as a JSON download. */
export function downloadSessionExport(file: SessionExportFile): string {
  const name = sessionExportFileName(file.export.sessionId, file.export.exportedAt);
  const blob = new Blob([JSON.stringify(file, null, 2)], { type: 'application/json' });
  const url = URL.createObjectURL(blob);
  const anchor = document.createElement('a');
  anchor.href = url;
  anchor.download = name;
  anchor.rel = 'noopener';
  document.body.append(anchor);
  anchor.click();
  anchor.remove();
  setTimeout(() => URL.revokeObjectURL(url), 1_000);
  return name;
}
