/**
 * Terminology Mapping API - GraphQL client functions for custom code mapping management
 */

import {
  ListMappingsDocument,
  GetMappingDocument,
  LookupMappingDocument,
  GetUploadBatchDocument,
  UploadMappingCsvDocument,
  CreateMappingDocument,
  DeleteMappingDocument,
  DeleteMappingBatchDocument,
  ResolveMappingDocument,
  SuggestMappingsDocument,
  type ListMappingsQuery,
  type GetMappingQuery,
  type LookupMappingQuery,
  type GetUploadBatchQuery,
  type UploadMappingCsvMutation,
  type CreateMappingMutation,
  type ResolveMappingQuery,
  type SuggestMappingsQuery,
  type ListMappingsInput,
  type UploadMappingCsvInput,
  type CreateMappingInput,
  type ResolveMappingInput,
  type SuggestMappingsInput
} from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

/**
 * List mappings with optional filters
 */
export async function listMappings(
  input?: ListMappingsInput | null
): Promise<ListMappingsQuery['listMappings']> {
  const result = await graphqlFetch(ListMappingsDocument, { input: input ?? null });
  return result.listMappings;
}

/**
 * Get a single mapping by ID
 */
export async function getMapping(id: string): Promise<GetMappingQuery['getMapping']> {
  const result = await graphqlFetch(GetMappingDocument, { id });
  return result.getMapping;
}

/**
 * Look up a mapping by source/target codes
 */
export async function lookupMapping(
  sourceSystem: string,
  sourceCode: string,
  targetSystem: string,
  profileId?: string | null
): Promise<LookupMappingQuery['lookupMapping']> {
  const result = await graphqlFetch(LookupMappingDocument, {
    sourceSystem,
    sourceCode,
    targetSystem,
    profileId: profileId ?? null
  });
  return result.lookupMapping;
}

/**
 * Get upload batch details
 */
export async function getUploadBatch(id: string): Promise<GetUploadBatchQuery['getUploadBatch']> {
  const result = await graphqlFetch(GetUploadBatchDocument, { id });
  return result.getUploadBatch;
}

/**
 * Upload mappings from CSV content
 */
export async function uploadMappingCSV(
  input: UploadMappingCsvInput
): Promise<UploadMappingCsvMutation['uploadMappingCSV']> {
  const result = await graphqlFetch(UploadMappingCsvDocument, { input });
  return result.uploadMappingCSV;
}

/**
 * Create a single mapping manually
 */
export async function createMapping(
  input: CreateMappingInput
): Promise<CreateMappingMutation['createMapping']> {
  const result = await graphqlFetch(CreateMappingDocument, { input });
  return result.createMapping;
}

/**
 * Delete a single mapping by ID
 */
export async function deleteMapping(id: string): Promise<boolean> {
  const result = await graphqlFetch(DeleteMappingDocument, { id });
  return result.deleteMapping;
}

/**
 * Delete all mappings from a batch
 */
export async function deleteMappingBatch(batchId: string): Promise<number> {
  const result = await graphqlFetch(DeleteMappingBatchDocument, { batchId });
  return result.deleteMappingBatch;
}

// =============================================================================
// Autoroute API - LLM-powered mapping suggestions
// =============================================================================

/**
 * Resolve a mapping using persistent lookup + autoroute fallback.
 * Returns a persistent mapping if found, otherwise uses LLM-powered semantic
 * search to suggest candidates.
 */
export async function resolveMapping(
  input: ResolveMappingInput
): Promise<ResolveMappingQuery['resolveMapping']> {
  const result = await graphqlFetch(ResolveMappingDocument, { input });
  return result.resolveMapping;
}

/**
 * Get LLM-powered mapping suggestions without checking persistent storage.
 * Useful for exploring possible mappings or generating candidates for review.
 */
export async function suggestMappings(
  input: SuggestMappingsInput
): Promise<SuggestMappingsQuery['suggestMappings']> {
  const result = await graphqlFetch(SuggestMappingsDocument, { input });
  return result.suggestMappings;
}
