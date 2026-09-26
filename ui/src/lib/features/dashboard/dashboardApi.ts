/**
 * Home data that is not already owned by another feature's API.
 */
import { HomeRecentSessionsDocument, type HomeRecentSessionsQuery } from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

export type RecentSession = HomeRecentSessionsQuery['integrationSessions'][number];

/**
 * The tenant's integration sessions, most recently updated first, capped at
 * `limit`. Failures render inline on the home panel, so no global toast.
 */
export async function fetchRecentSessions(limit = 8): Promise<RecentSession[]> {
  const result = await graphqlFetch(HomeRecentSessionsDocument, {}, { showErrorToast: false });
  return [...result.integrationSessions]
    .sort((a, b) => Date.parse(b.updatedAt) - Date.parse(a.updatedAt))
    .slice(0, limit);
}
