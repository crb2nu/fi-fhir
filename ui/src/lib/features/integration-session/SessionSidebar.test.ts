/**
 * The /hl7 session sidebar (.loom/42 E-3): honest states, runs and their
 * diagnostics, Accept fix, and the audited Export and Archive dialogs.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { get } from 'svelte/store';
import SessionSidebar from './SessionSidebar.svelte';
import { createSessionWorkspace } from './sessionWorkspace';
import type { SessionBundle, SessionWorkspace } from './workspaceApi';

const SESSION: SessionWorkspace = {
  id: 's-1',
  name: 'HL7 source profile workspace',
  description: null,
  archived: false,
  createdAt: '2026-01-01T09:00:00Z',
  updatedAt: '2026-01-01T09:05:00Z',
  samples: [{ id: 'sample-1', name: 'Mapping Studio preview', format: 'HL7V2', source: 'ui_preview', payloadChecksum: 'x', createdAt: '2026-01-01T09:00:00Z' }],
  currentProfileDraft: null,
  currentWorkflowDraft: null,
  workflowSimulations: [],
  publications: []
};

function fakeApi() {
  return {
    fetchSessionWorkspace: vi.fn(async (id: string) =>
      id === 's-1' ? ({ kind: 'found', session: SESSION } as const) : ({ kind: 'absent' } as const)
    ),
    fetchSessionRuns: vi.fn(async () => [
      {
        id: 'run-2',
        sessionId: 's-1',
        sampleId: 'sample-1',
        status: 'completed',
        profileRevisionId: null,
        profileRevisionDigest: null,
        createdAt: '2026-01-01T09:04:00Z',
        completedAt: '2026-01-01T09:04:01Z',
        stages: []
      }
    ]),
    fetchSessionRun: vi.fn(async () => null),
    fetchRunDiagnostics: vi.fn(async () => [
      {
        id: 'diag_001',
        sessionId: 's-1',
        runId: 'run-2',
        sampleId: 'sample-1',
        severity: 'warning',
        code: 'MISSING_PV1',
        message: 'PV1 is missing',
        path: 'PV1',
        fixSuggestion: 'Review the source profile or sample payload for this warning.',
        accepted: false,
        acceptedAt: null,
        lineage: []
      }
    ]),
    archiveSession: vi.fn(async () => ({ archived: true, updatedAt: '2026-01-01T10:00:00Z' })),
    acceptDiagnosticFix: vi.fn(async () => ({
      id: 'diag_001',
      sessionId: 's-1',
      runId: 'run-2',
      sampleId: 'sample-1',
      severity: 'warning',
      code: 'MISSING_PV1',
      message: 'PV1 is missing',
      path: 'PV1',
      fixSuggestion: 'Review the source profile or sample payload for this warning.',
      accepted: true,
      acceptedAt: '2026-01-01T10:00:00Z',
      lineage: []
    })),
    exportSessionBundle: vi.fn(async () => ({ sessionId: 's-1', exportedAt: '2026-01-01T10:00:00Z' }) as SessionBundle),
    subscribeRunEvents: vi.fn(() => () => {})
  };
}

/** jsdom's Blob has no text(). */
function blobText(blob: Blob): Promise<string> {
  return new Promise((resolve, reject) => {
    const reader = new FileReader();
    reader.onload = () => resolve(String(reader.result));
    reader.onerror = () => reject(reader.error);
    reader.readAsText(blob);
  });
}

let created: Blob[] = [];
beforeEach(() => {
  created = [];
  vi.stubGlobal('URL', Object.assign(URL, {
    createObjectURL: vi.fn((blob: Blob) => {
      created.push(blob);
      return 'blob:session-export';
    }),
    revokeObjectURL: vi.fn()
  }));
  vi.spyOn(HTMLAnchorElement.prototype, 'click').mockImplementation(() => {});
});

afterEach(() => {
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

async function renderSidebar(
  options: { phiExport?: boolean; reported?: boolean; missing?: string[]; open?: string } = {}
) {
  const api = fakeApi();
  const workspace = createSessionWorkspace({ api, canStream: () => false });
  await workspace.open(options.open ?? 's-1');
  const props = {
    workspace,
    view: get(workspace.state),
    phiExport: {
      allowed: options.phiExport ?? false,
      reported: options.reported ?? true,
      missing: options.missing ?? ['integration.phi.export']
    },
    onshowrun: vi.fn(),
    oninspectpath: vi.fn(),
    onaccepted: vi.fn(),
    onclose: vi.fn()
  };
  const rendered = render(SessionSidebar, props);
  // The page passes the store's value; follow it the same way.
  const unsubscribe = workspace.state.subscribe((view) => void rendered.rerender({ ...props, view }));
  return { api, workspace, props, unsubscribe };
}

describe('SessionSidebar', () => {
  it('says a deep-linked session is not in this store', async () => {
    const { unsubscribe } = await renderSidebar({ open: 'gone' });
    const absent = screen.getByTestId('hl7-session-absent');
    expect(absent).toHaveTextContent("Session gone is not in this deployment's session store");
    expect(absent).toHaveAttribute('data-reason', 'not-found');
    unsubscribe();
  });

  it('lists the runs, the selected run\'s diagnostics, and accepts a fix', async () => {
    const { api, props, unsubscribe } = await renderSidebar();
    expect(screen.getByTestId('hl7-session-sidebar')).toHaveAttribute('data-session-id', 's-1');
    const runRow = screen.getByTestId('hl7-session-run');
    expect(runRow).toHaveAttribute('data-run-id', 'run-2');
    expect(runRow).toHaveAttribute('data-status', 'completed');
    expect(screen.getByTestId('hl7-session-publications-empty')).toBeInTheDocument();
    expect(screen.getByTestId('hl7-session-simulations-empty')).toBeInTheDocument();

    const diagnostic = await screen.findByTestId('hl7-session-diagnostic');
    expect(diagnostic).toHaveAttribute('data-accepted', 'false');
    await fireEvent.click(within(diagnostic).getByRole('button', { name: 'PV1' }));
    expect(props.oninspectpath).toHaveBeenCalledWith('PV1');

    await fireEvent.click(within(diagnostic).getByRole('button', { name: 'Accept fix' }));
    await vi.waitFor(() => expect(screen.getByTestId('hl7-session-diagnostic')).toHaveAttribute('data-accepted', 'true'));
    expect(api.acceptDiagnosticFix).toHaveBeenCalledWith('s-1', 'diag_001');
    expect(props.onaccepted).toHaveBeenCalledWith(expect.objectContaining({ id: 'diag_001', accepted: true }));

    await fireEvent.click(screen.getByTestId('hl7-session-show-run'));
    expect(props.onshowrun).toHaveBeenCalledWith('run-2');
    unsubscribe();
  });

  it('exports without raw payloads, naming the missing role, and downloads the JSON', async () => {
    const { api, unsubscribe } = await renderSidebar({ phiExport: false });
    await fireEvent.click(screen.getByTestId('hl7-session-export'));
    const dialog = screen.getByTestId('hl7-session-export-dialog');
    const missing = within(dialog).getByTestId('session-export-phi-missing');
    expect(missing).toHaveTextContent('needs integration.phi.export, which this identity does not hold');
    expect(missing).toHaveAttribute('data-reason', 'missing-role');
    expect(dialog).toHaveTextContent('no raw sample text and no parsed patient fields');
    expect(within(dialog).queryByRole('checkbox')).not.toBeInTheDocument();
    const exportButton = within(dialog).getByRole('button', { name: 'Export' });
    expect(exportButton).toBeDisabled();

    await fireEvent.input(within(dialog).getByRole('textbox'), { target: { value: 'audit: quarterly review' } });
    await fireEvent.click(exportButton);
    await vi.waitFor(() => expect(screen.queryByTestId('hl7-session-export-dialog')).not.toBeInTheDocument());
    expect(api.exportSessionBundle).toHaveBeenCalledWith({ sessionId: 's-1', reason: 'audit: quarterly review', includeRawPayload: false });
    expect(created).toHaveLength(1);
    const file = JSON.parse(await blobText(created[0]!)) as { export: { reason: string; includeRawPayload: boolean } };
    expect(file.export).toMatchObject({ reason: 'audit: quarterly review', includeRawPayload: false });
    expect(screen.getByTestId('hl7-session-export-done')).toHaveTextContent('fi-fhir-session-s-1-20260101T100000Z.json');
    unsubscribe();
  });

  it('offers raw payloads only when phiExport is held', async () => {
    const { api, unsubscribe } = await renderSidebar({ phiExport: true, missing: [] });
    await fireEvent.click(screen.getByTestId('hl7-session-export'));
    const dialog = screen.getByTestId('hl7-session-export-dialog');
    expect(within(dialog).queryByTestId('session-export-phi-missing')).not.toBeInTheDocument();
    await fireEvent.click(within(dialog).getByRole('checkbox'));
    await fireEvent.input(within(dialog).getByRole('textbox'), { target: { value: 'regulator request 42' } });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Export' }));
    await vi.waitFor(() =>
      expect(api.exportSessionBundle).toHaveBeenCalledWith({ sessionId: 's-1', reason: 'regulator request 42', includeRawPayload: true })
    );
    unsubscribe();
  });

  it('does not offer raw payloads when the deployment did not report phiExport', async () => {
    const { unsubscribe } = await renderSidebar({ phiExport: false, reported: false, missing: [] });
    await fireEvent.click(screen.getByTestId('hl7-session-export'));
    const dialog = screen.getByTestId('hl7-session-export-dialog');
    expect(within(dialog).queryByRole('checkbox')).not.toBeInTheDocument();
    const sentence = within(dialog).getByTestId('session-export-phi-missing');
    expect(sentence).toHaveAttribute('data-reason', 'not-reported');
    expect(sentence).toHaveTextContent('did not report whether this identity holds integration.phi.export');
    unsubscribe();
  });

  it('archives without collecting a reason the API would not record', async () => {
    const { api, unsubscribe } = await renderSidebar();
    await fireEvent.click(screen.getByTestId('hl7-session-archive'));
    const dialog = screen.getByTestId('hl7-session-archive-dialog');
    expect(dialog).toHaveTextContent('The API records no reason for an archive.');
    expect(within(dialog).queryByRole('textbox')).not.toBeInTheDocument();
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Archive' }));
    await vi.waitFor(() => expect(screen.getByTestId('hl7-session-archived')).toBeInTheDocument());
    expect(api.archiveSession).toHaveBeenCalledWith('s-1');
    expect(screen.getByTestId('hl7-session-archive')).toBeDisabled();
    expect(screen.getByTestId('hl7-session-archived-note')).toHaveTextContent('Previews on this page still record runs in it');
    unsubscribe();
  });
});
