import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import ProfileSelector from '$lib/features/hl7/components/ProfileSelector.svelte';
import { profileStore } from '$lib/features/hl7/profile/profileStore';

const { listProfilesMock, getProfileMock } = vi.hoisted(() => ({
  listProfilesMock: vi.fn(),
  getProfileMock: vi.fn()
}));

vi.mock('$lib/features/hl7/profile/profileApi', () => ({
  listProfiles: (...args: unknown[]) => listProfilesMock(...args),
  getProfile: (...args: unknown[]) => getProfileMock(...args),
  createProfile: vi.fn(),
  updateProfile: vi.fn(),
  deleteProfile: vi.fn(),
  duplicateProfile: vi.fn(),
  getProfileRevisions: vi.fn()
}));

const summaries = [
  { id: 'adt_east', name: 'ADT east', version: '1.0.0', isActive: true },
  { id: 'lab_core', name: 'Lab core', version: '2.1.0', isActive: false }
];

function fullProfile(id: string) {
  const summary = summaries.find((p) => p.id === id)!;
  return {
    ...summary,
    createdAt: '2026-09-26T10:00:00Z',
    updatedAt: '2026-09-26T10:00:00Z',
    createdBy: null,
    hl7v2: null,
    identifiers: null,
    terminology: null
  };
}

describe('ProfileSelector layout="table"', () => {
  beforeEach(() => {
    profileStore.reset();
    listProfilesMock.mockResolvedValue(summaries);
    getProfileMock.mockImplementation(async (id: string) => fullProfile(id));
  });

  afterEach(() => {
    listProfilesMock.mockReset();
    getProfileMock.mockReset();
  });

  it('lists profiles in a table and selects a row', async () => {
    const onProfileChange = vi.fn();
    render(ProfileSelector, { props: { layout: 'table', onProfileChange } });

    const row = await screen.findByRole('row', { name: /ADT east/ });
    expect(screen.getByRole('row', { name: /Lab core/ })).toHaveTextContent('Inactive');
    expect(screen.getByText('2 profiles')).toBeInTheDocument();

    await fireEvent.click(row);

    await waitFor(() => expect(onProfileChange).toHaveBeenCalledWith('adt_east'));
    expect(getProfileMock).toHaveBeenCalledWith('adt_east');
    await waitFor(() => expect(row).toHaveAttribute('aria-selected', 'true'));
  });

  it('asks before discarding unsaved changes when switching rows', async () => {
    const onProfileChange = vi.fn();
    const { rerender } = render(ProfileSelector, {
      props: { layout: 'table', onProfileChange }
    });

    await fireEvent.click(await screen.findByRole('row', { name: /ADT east/ }));
    await waitFor(() => expect(onProfileChange).toHaveBeenCalledWith('adt_east'));

    // Unsaved YAML edits on the page arrive as externalDirty.
    await rerender({ layout: 'table', onProfileChange, externalDirty: true });
    await fireEvent.click(screen.getByRole('row', { name: /Lab core/ }));

    const dialog = await screen.findByRole('dialog', { name: 'Discard changes?' });
    expect(dialog).toBeInTheDocument();
    expect(onProfileChange).toHaveBeenCalledTimes(1);

    await fireEvent.click(screen.getByRole('button', { name: 'Discard' }));

    await waitFor(() => expect(onProfileChange).toHaveBeenLastCalledWith('lab_core'));
    expect(getProfileMock).toHaveBeenLastCalledWith('lab_core');
  });
});
