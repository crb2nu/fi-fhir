/**
 * Profile Store - Svelte 5 state management for source profiles
 */

import { writable, derived, get } from 'svelte/store';
import type {
  SourceProfile,
  Hl7v2ConfigInput,
  IdentifierConfigInput,
  TerminologyConfigInput,
  UpdateProfileInput
} from '$lib/gen/graphql';
import {
  listProfiles,
  getProfile,
  createProfile,
  updateProfile as updateProfileApi,
  deleteProfile as deleteProfileApi,
  duplicateProfile as duplicateProfileApi
} from './profileApi';

// Types for the store
export type ProfileSummary = {
  id: string;
  name: string;
  version: string;
  isActive: boolean;
};

export type ProfileState = {
  profiles: ProfileSummary[];
  selectedProfileId: string | null;
  selectedProfile: SourceProfile | null;
  activeOnly: boolean;
  loading: boolean;
  saving: boolean;
  error: string | null;
  dirty: boolean;
};

// Initial state
const initialState: ProfileState = {
  profiles: [],
  selectedProfileId: null,
  selectedProfile: null,
  activeOnly: true,
  loading: false,
  saving: false,
  error: null,
  dirty: false
};

// Create the store
function createProfileStore() {
  const { subscribe, set, update } = writable<ProfileState>(initialState);

  return {
    subscribe,

    /**
     * Load the list of profiles
     */
    async loadProfiles(activeOnly = true): Promise<void> {
      update((s) => ({ ...s, loading: true, error: null }));
      try {
        const profiles = await listProfiles(activeOnly);
        update((s) => ({
          ...s,
          activeOnly,
          profiles: profiles.map((p) => ({
            id: p.id,
            name: p.name,
            version: p.version,
            isActive: p.isActive
          })),
          loading: false
        }));
      } catch (e) {
        update((s) => ({
          ...s,
          loading: false,
          error: e instanceof Error ? e.message : 'Failed to load profiles'
        }));
      }
    },

    /**
     * Select a profile by ID and load its full configuration
     */
    async selectProfile(id: string | null): Promise<void> {
      if (id === null) {
        update((s) => ({
          ...s,
          selectedProfileId: null,
          selectedProfile: null,
          dirty: false
        }));
        return;
      }

      update((s) => ({ ...s, loading: true, error: null }));
      try {
        const profile = await getProfile(id);
        update((s) => ({
          ...s,
          selectedProfileId: id,
          selectedProfile: profile as SourceProfile | null,
          loading: false,
          dirty: false
        }));
      } catch (e) {
        update((s) => ({
          ...s,
          loading: false,
          error: e instanceof Error ? e.message : 'Failed to load profile'
        }));
      }
    },

    /**
     * Create a new profile
     */
    async createNewProfile(id: string, name: string): Promise<string | null> {
      update((s) => ({ ...s, saving: true, error: null }));
      try {
        const created = await createProfile({ id, name });
        // Reload the list
        const state = get({ subscribe });
        const profiles = await listProfiles(state.activeOnly);
        update((s) => ({
          ...s,
          profiles: profiles.map((p) => ({
            id: p.id,
            name: p.name,
            version: p.version,
            isActive: p.isActive
          })),
          selectedProfileId: created.id,
          selectedProfile: created as SourceProfile,
          saving: false,
          dirty: false
        }));
        return created.id;
      } catch (e) {
        update((s) => ({
          ...s,
          saving: false,
          error: e instanceof Error ? e.message : 'Failed to create profile'
        }));
        return null;
      }
    },

    /**
     * Update the local state (marks as dirty)
     */
    updateLocal(changes: Partial<UpdateProfileInput>): void {
      update((s) => {
        if (!s.selectedProfile) return s;

        const profile = { ...s.selectedProfile };

        if (changes.name !== undefined) {
          profile.name = changes.name ?? profile.name;
        }

        if (changes.hl7v2) {
          profile.hl7v2 = mergeHL7v2Config(profile.hl7v2, changes.hl7v2);
        }

        if (changes.identifiers) {
          profile.identifiers = mergeIdentifierConfig(profile.identifiers, changes.identifiers);
        }

        if (changes.terminology) {
          profile.terminology = mergeTerminologyConfig(profile.terminology, changes.terminology);
        }

        return {
          ...s,
          selectedProfile: profile,
          dirty: true
        };
      });
    },

    /**
     * Save the current profile to the backend
     */
    async saveProfile(): Promise<boolean> {
      const state = get({ subscribe });
      if (!state.selectedProfile || !state.dirty) return true;

      update((s) => ({ ...s, saving: true, error: null }));
      try {
        const input: UpdateProfileInput = {
          name: state.selectedProfile.name,
          hl7v2: toHL7v2Input(state.selectedProfile.hl7v2),
          identifiers: toIdentifierInput(state.selectedProfile.identifiers),
          terminology: toTerminologyInput(state.selectedProfile.terminology)
        };

        const updated = await updateProfileApi(state.selectedProfile.id, input);

        // Update the profile list with new version
        const profiles = await listProfiles(state.activeOnly);

        update((s) => ({
          ...s,
          profiles: profiles.map((p) => ({
            id: p.id,
            name: p.name,
            version: p.version,
            isActive: p.isActive
          })),
          selectedProfile: updated as SourceProfile,
          saving: false,
          dirty: false
        }));
        return true;
      } catch (e) {
        update((s) => ({
          ...s,
          saving: false,
          error: e instanceof Error ? e.message : 'Failed to save profile'
        }));
        return false;
      }
    },

    /**
     * Delete the selected profile
     */
    async deleteSelectedProfile(): Promise<boolean> {
      const state = get({ subscribe });
      if (!state.selectedProfileId) return false;

      update((s) => ({ ...s, saving: true, error: null }));
      try {
        await deleteProfileApi(state.selectedProfileId);
        const profiles = await listProfiles(state.activeOnly);
        update((s) => ({
          ...s,
          profiles: profiles.map((p) => ({
            id: p.id,
            name: p.name,
            version: p.version,
            isActive: p.isActive
          })),
          selectedProfileId: null,
          selectedProfile: null,
          saving: false,
          dirty: false
        }));
        return true;
      } catch (e) {
        update((s) => ({
          ...s,
          saving: false,
          error: e instanceof Error ? e.message : 'Failed to delete profile'
        }));
        return false;
      }
    },

    /**
     * Duplicate the selected profile
     */
    async duplicateSelectedProfile(newId: string, newName: string): Promise<string | null> {
      const state = get({ subscribe });
      if (!state.selectedProfileId) return null;

      update((s) => ({ ...s, saving: true, error: null }));
      try {
        const duplicated = await duplicateProfileApi(state.selectedProfileId, newId, newName);
        const profiles = await listProfiles(state.activeOnly);
        update((s) => ({
          ...s,
          profiles: profiles.map((p) => ({
            id: p.id,
            name: p.name,
            version: p.version,
            isActive: p.isActive
          })),
          saving: false
        }));
        return duplicated.id;
      } catch (e) {
        update((s) => ({
          ...s,
          saving: false,
          error: e instanceof Error ? e.message : 'Failed to duplicate profile'
        }));
        return null;
      }
    },

    /**
     * Discard unsaved changes and reload the profile
     */
    async discardChanges(): Promise<void> {
      const state = get({ subscribe });
      if (state.selectedProfileId) {
        await this.selectProfile(state.selectedProfileId);
      }
    },

    /**
     * Clear the error
     */
    clearError(): void {
      update((s) => ({ ...s, error: null }));
    },

    /**
     * Reset the store
     */
    reset(): void {
      set(initialState);
    }
  };
}

// Helper functions for merging nested config
function mergeHL7v2Config(
  existing: SourceProfile['hl7v2'] | null | undefined,
  changes: Hl7v2ConfigInput
): SourceProfile['hl7v2'] {
  const base = existing || {
    defaultVersion: '2.5.1',
    timezone: 'UTC',
    tolerance: {
      missingSegments: [],
      nteAnywhere: false,
      extraComponents: false,
      unknownSegments: false,
      nonStandardDelimiters: false
    },
    eventClassifications: []
  };

  const baseTolerance = base.tolerance || {
    missingSegments: [],
    nteAnywhere: false,
    extraComponents: false,
    unknownSegments: false,
    nonStandardDelimiters: false
  };

  return {
    defaultVersion: changes.defaultVersion ?? base.defaultVersion,
    timezone: changes.timezone ?? base.timezone,
    tolerance: {
      missingSegments: changes.tolerance?.missingSegments ?? baseTolerance.missingSegments,
      nteAnywhere: changes.tolerance?.nteAnywhere ?? baseTolerance.nteAnywhere,
      extraComponents: changes.tolerance?.extraComponents ?? baseTolerance.extraComponents,
      unknownSegments: changes.tolerance?.unknownSegments ?? baseTolerance.unknownSegments,
      nonStandardDelimiters:
        changes.tolerance?.nonStandardDelimiters ?? baseTolerance.nonStandardDelimiters
    },
    eventClassifications: changes.eventClassifications
      ? changes.eventClassifications.map((ec) => ({
          messageType: ec.messageType,
          condition: ec.condition ?? null,
          eventType: ec.eventType,
          priority: ec.priority
        }))
      : base.eventClassifications
  };
}

function mergeIdentifierConfig(
  existing: SourceProfile['identifiers'] | null | undefined,
  changes: IdentifierConfigInput
): SourceProfile['identifiers'] {
  const base = existing || {
    assigningAuthorities: [],
    primaryIdPreference: [],
    validation: {
      npi: { enabled: false, onInvalid: 'pass' },
      mbi: { enabled: false, onInvalid: 'pass' },
      ssn: { enabled: false, onInvalid: 'pass' }
    },
    normalization: {
      ssnStripDashes: false,
      ssnRejectPatterns: [],
      phoneNormalize: false,
      phoneFormat: null
    }
  };

  const baseValidation = base.validation || {
    npi: { enabled: false, onInvalid: 'pass' },
    mbi: { enabled: false, onInvalid: 'pass' },
    ssn: { enabled: false, onInvalid: 'pass' }
  };
  const baseNormalization = base.normalization || {
    ssnStripDashes: false,
    ssnRejectPatterns: [],
    phoneNormalize: false,
    phoneFormat: null
  };

  return {
    assigningAuthorities: changes.assigningAuthorities
      ? changes.assigningAuthorities.map((aa) => ({
          code: aa.code,
          system: aa.system,
          name: aa.name ?? null
        }))
      : base.assigningAuthorities,
    primaryIdPreference: changes.primaryIdPreference
      ? changes.primaryIdPreference.map((p) => ({
          type: p.type,
          assignerContains: p.assignerContains ?? null,
          priority: p.priority
        }))
      : base.primaryIdPreference,
    validation: {
      npi: changes.validation?.npi
        ? { enabled: changes.validation.npi.enabled, onInvalid: changes.validation.npi.onInvalid }
        : baseValidation.npi || { enabled: false, onInvalid: 'pass' },
      mbi: changes.validation?.mbi
        ? { enabled: changes.validation.mbi.enabled, onInvalid: changes.validation.mbi.onInvalid }
        : baseValidation.mbi || { enabled: false, onInvalid: 'pass' },
      ssn: changes.validation?.ssn
        ? { enabled: changes.validation.ssn.enabled, onInvalid: changes.validation.ssn.onInvalid }
        : baseValidation.ssn || { enabled: false, onInvalid: 'pass' }
    },
    normalization: {
      ssnStripDashes: changes.normalization?.ssnStripDashes ?? baseNormalization.ssnStripDashes,
      ssnRejectPatterns:
        changes.normalization?.ssnRejectPatterns ?? baseNormalization.ssnRejectPatterns,
      phoneNormalize: changes.normalization?.phoneNormalize ?? baseNormalization.phoneNormalize,
      phoneFormat: changes.normalization?.phoneFormat ?? baseNormalization.phoneFormat
    }
  };
}

function mergeTerminologyConfig(
  existing: SourceProfile['terminology'] | null | undefined,
  changes: TerminologyConfigInput
): SourceProfile['terminology'] {
  if (changes.mappings) {
    return {
      mappings: changes.mappings.map((m) => ({
        id: m.id,
        sourceSystem: m.sourceSystem,
        targetSystem: m.targetSystem,
        entries: (m.entries || []).map((e) => ({
          sourceCode: e.sourceCode,
          targetCode: e.targetCode,
          display: e.display ?? null
        }))
      }))
    };
  }
  return existing || { mappings: [] };
}

// Helper functions for converting profile to input types
function toHL7v2Input(hl7v2: SourceProfile['hl7v2'] | null | undefined): Hl7v2ConfigInput | null {
  if (!hl7v2) return null;
  const tolerance = hl7v2.tolerance || {
    missingSegments: [],
    nteAnywhere: false,
    extraComponents: false,
    unknownSegments: false,
    nonStandardDelimiters: false
  };
  return {
    defaultVersion: hl7v2.defaultVersion,
    timezone: hl7v2.timezone,
    tolerance: {
      missingSegments: tolerance.missingSegments,
      nteAnywhere: tolerance.nteAnywhere,
      extraComponents: tolerance.extraComponents,
      unknownSegments: tolerance.unknownSegments,
      nonStandardDelimiters: tolerance.nonStandardDelimiters
    },
    eventClassifications: hl7v2.eventClassifications.map((ec) => ({
      messageType: ec.messageType,
      condition: ec.condition,
      eventType: ec.eventType,
      priority: ec.priority
    }))
  };
}

function toIdentifierInput(
  identifiers: SourceProfile['identifiers'] | null | undefined
): IdentifierConfigInput | null {
  if (!identifiers) return null;
  const validation = identifiers.validation || {
    npi: { enabled: false, onInvalid: 'pass' },
    mbi: { enabled: false, onInvalid: 'pass' },
    ssn: { enabled: false, onInvalid: 'pass' }
  };
  const normalization = identifiers.normalization || {
    ssnStripDashes: false,
    ssnRejectPatterns: [],
    phoneNormalize: false,
    phoneFormat: null
  };
  const npi = validation.npi || { enabled: false, onInvalid: 'pass' };
  const mbi = validation.mbi || { enabled: false, onInvalid: 'pass' };
  const ssn = validation.ssn || { enabled: false, onInvalid: 'pass' };

  return {
    assigningAuthorities: identifiers.assigningAuthorities.map((aa) => ({
      code: aa.code,
      system: aa.system,
      name: aa.name
    })),
    primaryIdPreference: identifiers.primaryIdPreference.map((p) => ({
      type: p.type,
      assignerContains: p.assignerContains,
      priority: p.priority
    })),
    validation: {
      npi: {
        enabled: npi.enabled,
        onInvalid: npi.onInvalid
      },
      mbi: {
        enabled: mbi.enabled,
        onInvalid: mbi.onInvalid
      },
      ssn: {
        enabled: ssn.enabled,
        onInvalid: ssn.onInvalid
      }
    },
    normalization: {
      ssnStripDashes: normalization.ssnStripDashes,
      ssnRejectPatterns: normalization.ssnRejectPatterns,
      phoneNormalize: normalization.phoneNormalize,
      phoneFormat: normalization.phoneFormat
    }
  };
}

function toTerminologyInput(
  terminology: SourceProfile['terminology'] | null | undefined
): TerminologyConfigInput | null {
  if (!terminology) return null;
  return {
    mappings: terminology.mappings.map((m) => ({
      id: m.id,
      sourceSystem: m.sourceSystem,
      targetSystem: m.targetSystem,
      entries: m.entries.map((e) => ({
        sourceCode: e.sourceCode,
        targetCode: e.targetCode,
        display: e.display
      }))
    }))
  };
}

// Export the singleton store
export const profileStore = createProfileStore();

// Derived stores for convenience
export const selectedProfile = derived(profileStore, ($s) => $s.selectedProfile);
export const profileList = derived(profileStore, ($s) => $s.profiles);
export const isLoading = derived(profileStore, ($s) => $s.loading);
export const isSaving = derived(profileStore, ($s) => $s.saving);
export const isDirty = derived(profileStore, ($s) => $s.dirty);
export const profileError = derived(profileStore, ($s) => $s.error);
