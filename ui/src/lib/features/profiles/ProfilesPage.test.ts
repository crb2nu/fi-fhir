import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { cleanup, fireEvent, render, screen, waitFor, within } from '@testing-library/svelte';
import { tick } from 'svelte';
import { get } from 'svelte/store';
import { EditorView } from '@codemirror/view';
import ProfilesPage from './ProfilesPage.svelte';
import { isDirty as builderDirty, profileStore } from '$lib/features/hl7/profile/profileStore';
import {
  clearDraftState,
  createWorkspaceTab,
  isDirty,
  openTab,
  resetIDEState,
  setDraftState,
} from '$lib/ui/ide/ideStore';

const api = vi.hoisted(() => ({
  listProfiles: vi.fn(),
  getProfile: vi.fn(),
  getProfileRevisions: vi.fn(),
  fetchProfileYaml: vi.fn(),
  saveProfileYaml: vi.fn(),
}));

vi.mock('$lib/features/hl7/profile/profileApi', () => ({
  listProfiles: api.listProfiles,
  getProfile: api.getProfile,
  getProfileRevisions: api.getProfileRevisions,
  createProfile: vi.fn(),
  updateProfile: vi.fn(),
  deleteProfile: vi.fn(),
  duplicateProfile: vi.fn(),
}));
vi.mock('$lib/features/hl7/profile/profileYamlApi', () => ({
  fetchProfileYaml: api.fetchProfileYaml,
  saveProfileYaml: api.saveProfileYaml,
}));

const summaries = [
  { id: 'adt_east', name: 'ADT east', version: '1.0.0', isActive: true },
  { id: 'lab_core', name: 'Lab core', version: '1.0.0', isActive: true },
];
const originalYaml = 'id: adt_east\nname: ADT east\n';
const editedYaml = 'id: adt_east\nname: Edited intake\n';

function yamlEditor(): EditorView {
  const editor = EditorView.findFromDOM(screen.getByTestId('code-editor'));
  if (!editor) throw new Error('The profile YAML editor is not mounted');
  return editor;
}

async function editYaml(value = editedYaml): Promise<void> {
  const editor = yamlEditor();
  editor.dispatch({ changes: { from: 0, to: editor.state.doc.length, insert: value } });
  await tick();
}

async function openYaml() {
  const view = render(ProfilesPage);
  await fireEvent.click(await screen.findByRole('row', { name: /ADT east/ }));
  await waitFor(() => expect(api.fetchProfileYaml).toHaveBeenCalledWith('adt_east'));
  await fireEvent.click(screen.getByRole('tab', { name: 'YAML' }));
  await screen.findByTestId('code-editor');
  await waitFor(() => expect(yamlEditor().state.doc.toString()).toBe(originalYaml));
  return view;
}

beforeEach(() => {
  localStorage.clear();
  resetIDEState();
  openTab(createWorkspaceTab('/profiles'));
  profileStore.reset();
  vi.resetAllMocks();
  api.listProfiles.mockResolvedValue(summaries);
  api.getProfile.mockImplementation(async (id: string) => ({
    ...summaries.find((profile) => profile.id === id),
    createdAt: '2026-10-03T12:00:00Z',
    updatedAt: '2026-10-03T12:00:00Z',
    createdBy: null,
    hl7v2: null,
    identifiers: null,
    terminology: null,
  }));
  api.getProfileRevisions.mockResolvedValue([]);
  api.fetchProfileYaml.mockImplementation(async (id: string) => id === 'adt_east'
    ? originalYaml : 'id: lab_core\nname: Lab core\n');
  api.saveProfileYaml.mockResolvedValue(undefined);
});

afterEach(() => {
  cleanup();
  profileStore.reset();
  resetIDEState();
});

describe('Profiles YAML draft protection', { timeout: 30_000 }, () => {
  it('marks a YAML-only edit and clears it after reset or a successful save', async () => {
    await openYaml();
    expect(isDirty('/profiles')).toBe(false);
    expect(get(builderDirty)).toBe(false);

    await editYaml();
    expect(isDirty('/profiles')).toBe(true);
    expect(get(builderDirty)).toBe(false);
    await fireEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(isDirty('/profiles')).toBe(false);
    expect(yamlEditor().state.doc.toString()).toBe(originalYaml);

    await editYaml();
    let finishSave!: () => void;
    api.saveProfileYaml.mockReturnValueOnce(new Promise<void>((resolve) => { finishSave = resolve; }));
    api.fetchProfileYaml.mockResolvedValueOnce(editedYaml);
    await fireEvent.click(screen.getByRole('button', { name: 'Save YAML' }));
    expect(api.saveProfileYaml).toHaveBeenCalledWith('adt_east', editedYaml);
    expect(isDirty('/profiles')).toBe(true);
    finishSave();
    await waitFor(() => expect(isDirty('/profiles')).toBe(false));
    await waitFor(() => expect(yamlEditor().state.doc.toString()).toBe(editedYaml));
    expect(screen.getByRole('button', { name: 'Save YAML' })).toBeDisabled();
  });

  it('retains protection and edited text when saving fails', async () => {
    await openYaml();
    await editYaml();
    api.saveProfileYaml.mockRejectedValueOnce(new Error('YAML transport unavailable'));
    await fireEvent.click(screen.getByRole('button', { name: 'Save YAML' }));

    expect(await screen.findByRole('alert')).toHaveTextContent('YAML transport unavailable');
    expect(isDirty('/profiles')).toBe(true);
    expect(yamlEditor().state.doc.toString()).toBe(editedYaml);
  });

  it('clears only the YAML owner on reset and unmount', async () => {
    setDraftState('/profiles', 'profile-builder', true, false);
    const view = await openYaml();
    expect(isDirty('/profiles')).toBe(true);

    await editYaml();
    await fireEvent.click(screen.getByRole('button', { name: 'Reset' }));
    expect(isDirty('/profiles')).toBe(true);
    clearDraftState('/profiles', 'profile-builder');
    expect(isDirty('/profiles')).toBe(false);

    await editYaml();
    setDraftState('/profiles', 'profile-builder', true, false);
    view.unmount();
    expect(isDirty('/profiles')).toBe(true);
    clearDraftState('/profiles', 'profile-builder');
    expect(isDirty('/profiles')).toBe(false);
  });

  it('keeps the YAML draft when switching profiles is cancelled', async () => {
    await openYaml();
    await editYaml();
    await fireEvent.click(screen.getByRole('row', { name: /Lab core/ }));
    const dialog = await screen.findByRole('dialog', { name: 'Discard changes?' });
    await fireEvent.click(within(dialog).getByRole('button', { name: 'Cancel' }));

    expect(api.getProfile).toHaveBeenCalledTimes(1);
    expect(yamlEditor().state.doc.toString()).toBe(editedYaml);
    expect(isDirty('/profiles')).toBe(true);
    expect(screen.getByRole('row', { name: /ADT east/ })).toHaveAttribute('aria-selected', 'true');

    await fireEvent.click(screen.getByRole('row', { name: /Lab core/ }));
    await fireEvent.click(within(screen.getByRole('dialog', { name: 'Discard changes?' }))
      .getByRole('button', { name: 'Discard' }));
    await waitFor(() => expect(api.fetchProfileYaml).toHaveBeenLastCalledWith('lab_core'));
    await waitFor(() => expect(isDirty('/profiles')).toBe(false));
    await waitFor(() => expect(yamlEditor().state.doc.toString()).toContain('id: lab_core'));
  });
});
