/**
 * Profile API - GraphQL client functions for source profile management
 */

import {
  ListProfilesDocument,
  GetProfileDocument,
  GetProfileRevisionsDocument,
  CreateProfileDocument,
  UpdateProfileDocument,
  DeleteProfileDocument,
  DuplicateProfileDocument,
  type ListProfilesQuery,
  type GetProfileQuery,
  type GetProfileRevisionsQuery,
  type CreateProfileMutation,
  type UpdateProfileMutation,
  type DuplicateProfileMutation,
  type CreateProfileInput,
  type UpdateProfileInput
} from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

/**
 * Fetch all profiles
 */
export async function listProfiles(activeOnly = true): Promise<ListProfilesQuery['profiles']> {
  const result = await graphqlFetch(ListProfilesDocument, { activeOnly });
  return result.profiles;
}

/**
 * Fetch a single profile by ID with full configuration
 */
export async function getProfile(id: string): Promise<GetProfileQuery['profile']> {
  const result = await graphqlFetch(GetProfileDocument, { id });
  return result.profile;
}

/**
 * Fetch revision history for a profile
 */
export async function getProfileRevisions(
  id: string
): Promise<GetProfileRevisionsQuery['profileRevisions']> {
  const result = await graphqlFetch(GetProfileRevisionsDocument, { id });
  return result.profileRevisions;
}

/**
 * Create a new profile
 */
export async function createProfile(
  input: CreateProfileInput
): Promise<CreateProfileMutation['createProfile']> {
  const result = await graphqlFetch(CreateProfileDocument, { input });
  return result.createProfile;
}

/**
 * Update an existing profile
 */
export async function updateProfile(
  id: string,
  input: UpdateProfileInput
): Promise<UpdateProfileMutation['updateProfile']> {
  const result = await graphqlFetch(UpdateProfileDocument, { id, input });
  return result.updateProfile;
}

/**
 * Delete (soft-delete) a profile
 */
export async function deleteProfile(id: string): Promise<boolean> {
  const result = await graphqlFetch(DeleteProfileDocument, { id });
  return result.deleteProfile;
}

/**
 * Duplicate a profile with a new ID and name
 */
export async function duplicateProfile(
  id: string,
  newId: string,
  newName: string
): Promise<DuplicateProfileMutation['duplicateProfile']> {
  const result = await graphqlFetch(DuplicateProfileDocument, { id, newId, newName });
  return result.duplicateProfile;
}
