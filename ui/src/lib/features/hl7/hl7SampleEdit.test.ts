/**
 * HL7 intake: editing a saved sample's metadata must never touch the editor.
 * The samples store re-emits the active sample on every samples update, and
 * the page used to reload it on each emission, so renaming any sample
 * silently replaced unsaved editor text with the active sample's raw message.
 */
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { tick } from 'svelte';
import { cleanup, fireEvent, render, screen, within } from '@testing-library/svelte';
import { EditorView } from '@codemirror/view';
import HL7PreviewPage from './HL7PreviewPage.svelte';

function editorView(): EditorView {
  const view = EditorView.findFromDOM(screen.getByTestId('code-editor'));
  if (!view) throw new Error('HL7 intake editor is not mounted');
  return view;
}

beforeEach(() => {
  // Nothing on this path calls the API; answer anything with an empty result.
  vi.stubGlobal(
    'fetch',
    vi.fn(async () => new Response(JSON.stringify({ data: null }), { status: 200 }))
  );
});

afterEach(() => {
  cleanup();
  vi.unstubAllGlobals();
});

// The first HL7PreviewPage render compiles the whole page (CodeMirror included).
describe('HL7 intake sample metadata edits', { timeout: 30_000 }, () => {
  it('renaming another sample keeps unsaved editor text', async () => {
    render(HL7PreviewPage);
    await fireEvent.click(screen.getByRole('tab', { name: /^Samples/ }));
    await fireEvent.click(screen.getByRole('button', { name: 'Load examples' }));
    await tick();

    // Loading the examples activates (and loads) the first one; edit it.
    const edited = 'MSH|^~\\&|EDITED|TEST|FI_FHIR|TEST|20260101090000||ADT^A01|EDITED-001|T|2.5.1';
    const view = editorView();
    view.dispatch({ changes: { from: 0, to: view.state.doc.length, insert: edited } });
    await tick();

    const table = screen.getByRole('table', { name: 'Saved samples' });
    const rows = within(table).getAllByRole('row').slice(1);
    const other = rows[1]!;
    await fireEvent.click(within(other).getByRole('button', { name: 'Edit sample' }));

    const details = screen.getByRole('region', { name: 'Selected sample' });
    const nameInput = within(details).getByLabelText(/^Name/);
    await fireEvent.input(nameInput, { target: { value: 'Renamed synthetic sample' } });
    await fireEvent.click(within(details).getByRole('button', { name: 'Save changes' }));
    await tick();

    expect(within(table).getByRole('cell', { name: 'Renamed synthetic sample' })).toBeInTheDocument();
    expect(editorView().state.doc.toString()).toBe(edited);
  });
});
