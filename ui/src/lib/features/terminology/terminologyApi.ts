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
  UpdateMappingDocument,
  ExportMappingsCsvDocument,
  ResolveMappingDocument,
  SuggestMappingsDocument,
  ListPendingAutoroutesDocument,
  GetPendingAutorouteDocument,
  PendingAutorouteStatsDocument,
  ApprovePendingAutorouteDocument,
  RejectPendingAutorouteDocument,
  BulkApprovePendingAutoroutesDocument,
  StartTerminologyReviewDocument,
  type ListMappingsQuery,
  type GetMappingQuery,
  type LookupMappingQuery,
  type GetUploadBatchQuery,
  type UploadMappingCsvMutation,
  type CreateMappingMutation,
  type UpdateMappingMutation,
  type ResolveMappingQuery,
  type SuggestMappingsQuery,
  type ListPendingAutoroutesQuery,
  type GetPendingAutorouteQuery,
  type PendingAutorouteStatsQuery,
  type ApprovePendingAutorouteMutation,
  type BulkApprovePendingAutoroutesMutation,
  type StartTerminologyReviewMutation,
  type ListMappingsInput,
  type UploadMappingCsvInput,
  type CreateMappingInput,
  type UpdateMappingInput,
  type ResolveMappingInput,
  type SuggestMappingsInput,
  type ListPendingAutoroutesInput,
  type ApprovePendingAutorouteInput,
  type RejectPendingAutorouteInput,
  type BulkApproveInput,
  type StartTerminologyReviewInput
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

/**
 * Update an existing mapping
 */
export async function updateMapping(
  input: UpdateMappingInput
): Promise<UpdateMappingMutation['updateMapping']> {
  const result = await graphqlFetch(UpdateMappingDocument, { input });
  return result.updateMapping;
}

/**
 * Export mappings to CSV format
 */
export async function exportMappingsCSV(
  input?: ListMappingsInput | null
): Promise<string> {
  const result = await graphqlFetch(ExportMappingsCsvDocument, { input: input ?? null });
  return result.exportMappingsCSV;
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

// =============================================================================
// Pending Autoroute Review API
// =============================================================================

/**
 * List pending autoroutes awaiting human review.
 * Supports filtering by status, confidence, and system.
 */
export async function listPendingAutoroutes(
  input?: ListPendingAutoroutesInput | null
): Promise<ListPendingAutoroutesQuery['listPendingAutoroutes']> {
  const result = await graphqlFetch(ListPendingAutoroutesDocument, { input: input ?? null });
  return result.listPendingAutoroutes;
}

/**
 * Get a single pending autoroute by ID
 */
export async function getPendingAutoroute(
  id: string
): Promise<GetPendingAutorouteQuery['getPendingAutoroute']> {
  const result = await graphqlFetch(GetPendingAutorouteDocument, { id });
  return result.getPendingAutoroute;
}

/**
 * Get statistics about pending autoroutes (counts by status, avg confidence)
 */
export async function getPendingAutorouteStats(): Promise<
  PendingAutorouteStatsQuery['pendingAutorouteStats']
> {
  const result = await graphqlFetch(PendingAutorouteStatsDocument, {});
  return result.pendingAutorouteStats;
}

/**
 * Approve a pending autoroute, creating a persistent mapping.
 * Optionally override the equivalence or add a comment.
 */
export async function approvePendingAutoroute(
  input: ApprovePendingAutorouteInput
): Promise<ApprovePendingAutorouteMutation['approvePendingAutoroute']> {
  const result = await graphqlFetch(ApprovePendingAutorouteDocument, { input });
  return result.approvePendingAutoroute;
}

/**
 * Reject a pending autoroute with a reason.
 */
export async function rejectPendingAutoroute(
  input: RejectPendingAutorouteInput
): Promise<boolean> {
  const result = await graphqlFetch(RejectPendingAutorouteDocument, { input });
  return result.rejectPendingAutoroute;
}

/**
 * Bulk approve all pending autoroutes above a confidence threshold.
 * Returns the number approved and the created mappings.
 */
export async function bulkApprovePendingAutoroutes(
  input?: BulkApproveInput | null
): Promise<BulkApprovePendingAutoroutesMutation['bulkApprovePendingAutoroutes']> {
  const result = await graphqlFetch(BulkApprovePendingAutoroutesDocument, { input: input ?? null });
  return result.bulkApprovePendingAutoroutes;
}

// =============================================================================
// Terminology Review Workflow
// =============================================================================

/**
 * Start a terminology review workflow via Temporal.
 * Creates a new workflow that orchestrates LLM-powered mapping suggestion + human review.
 */
export async function startTerminologyReview(
  input: StartTerminologyReviewInput
): Promise<StartTerminologyReviewMutation['startTerminologyReview']> {
  const result = await graphqlFetch(StartTerminologyReviewDocument, { input }, {
    showSuccessToast: true,
    successMessage: 'Terminology review workflow started'
  });
  return result.startTerminologyReview;
}
