/**
 * Temporal Workflow API - GraphQL client functions for workflow management
 */

import {
  ListTemporalWorkflowsDocument,
  GetTemporalWorkflowDocument,
  CancelTemporalWorkflowDocument,
  SignalReviewDecisionDocument,
  type ListTemporalWorkflowsQuery,
  type GetTemporalWorkflowQuery,
  type TemporalWorkflowFilter,
  type SignalReviewDecisionInput
} from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

/**
 * List Temporal workflows with optional filters
 */
export async function listTemporalWorkflows(
  filter?: TemporalWorkflowFilter | null,
  first?: number | null,
  after?: string | null
): Promise<ListTemporalWorkflowsQuery['temporalWorkflows']> {
  const result = await graphqlFetch(ListTemporalWorkflowsDocument, {
    filter: filter ?? null,
    first: first ?? null,
    after: after ?? null
  });
  return result.temporalWorkflows;
}

/**
 * Get a single Temporal workflow by ID
 */
export async function getTemporalWorkflow(
  workflowId: string,
  runId?: string | null
): Promise<GetTemporalWorkflowQuery['temporalWorkflow']> {
  const result = await graphqlFetch(GetTemporalWorkflowDocument, {
    workflowId,
    runId: runId ?? null
  });
  return result.temporalWorkflow;
}

/**
 * Cancel a running Temporal workflow
 */
export async function cancelTemporalWorkflow(
  workflowId: string,
  reason?: string | null
): Promise<boolean> {
  const result = await graphqlFetch(CancelTemporalWorkflowDocument, {
    workflowId,
    reason: reason ?? null
  });
  return result.cancelTemporalWorkflow;
}

/**
 * Signal a review decision to a terminology review workflow
 */
export async function signalReviewDecision(
  input: SignalReviewDecisionInput
): Promise<boolean> {
  const result = await graphqlFetch(SignalReviewDecisionDocument, { input });
  return result.signalReviewDecision;
}
