/**
 * Home data that is not already owned by another feature's API.
 */
import { IntegrationSessionSummariesDocument, type IntegrationSessionSummariesQuery } from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

export type SessionSummaryPage = IntegrationSessionSummariesQuery['integrationSessionSummaries'];
export type RecentSession = SessionSummaryPage['nodes'][number];

export interface SessionSummaryOptions {
  search?: string;
  includeArchived?: boolean;
  hasRuns?: boolean;
  limit?: number;
  offset?: number;
}

export async function fetchSessionSummaries(options: SessionSummaryOptions = {}): Promise<SessionSummaryPage> {
  const result = await graphqlFetch(IntegrationSessionSummariesDocument, {
    input: {
      search: options.search?.trim() || null,
      includeArchived: options.includeArchived ?? false,
      hasRuns: options.hasRuns ?? null,
      limit: options.limit ?? 25,
      offset: options.offset ?? 0
    }
  }, { showErrorToast: false });
  return result.integrationSessionSummaries;
}

/**
 * The tenant's integration sessions, most recently updated first, capped at
 * `limit`. Failures render inline on the home panel, so no global toast.
 */
export async function fetchRecentSessions(limit = 8): Promise<RecentSession[]> {
  return (await fetchSessionSummaries({ limit })).nodes;
}
