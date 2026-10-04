import { beforeEach, describe, expect, it, vi } from 'vitest';
import { print } from 'graphql';
import { IntegrationSessionSummariesDocument } from '$lib/gen/graphql';
import { fetchRecentSessions, fetchSessionSummaries } from './dashboardApi';

const { graphqlFetch } = vi.hoisted(() => ({ graphqlFetch: vi.fn() }));
vi.mock('$lib/graphql/client', () => ({ graphqlFetch }));

beforeEach(() => {
  graphqlFetch.mockReset().mockResolvedValue({ integrationSessionSummaries: { nodes: [], hasMore: false, nextOffset: null } });
});

describe('session summary reads', () => {
  it('requests one bounded metadata page with explicit defaults and inline errors', async () => {
    await expect(fetchSessionSummaries()).resolves.toEqual({ nodes: [], hasMore: false, nextOffset: null });
    expect(graphqlFetch).toHaveBeenCalledWith(IntegrationSessionSummariesDocument, {
      input: { search: null, includeArchived: false, hasRuns: null, limit: 25, offset: 0 }
    }, { showErrorToast: false });
    const query = print(IntegrationSessionSummariesDocument);
    expect(query).toContain('latestRun');
    expect(query).not.toMatch(/\b(integrationSessions|runs|samples|artifacts|events|diagnostics)\s*\{/);
  });

  it('passes literal search, paging and both run-filter values to the server', async () => {
    await fetchSessionSummaries({ search: '  ADT_%\\east  ', includeArchived: true, hasRuns: false, limit: 25, offset: 25 });
    expect(graphqlFetch).toHaveBeenLastCalledWith(IntegrationSessionSummariesDocument, {
      input: { search: 'ADT_%\\east', includeArchived: true, hasRuns: false, limit: 25, offset: 25 }
    }, expect.anything());
    await fetchSessionSummaries({ hasRuns: true, limit: 1 });
    expect(graphqlFetch).toHaveBeenLastCalledWith(IntegrationSessionSummariesDocument, {
      input: { search: null, includeArchived: false, hasRuns: true, limit: 1, offset: 0 }
    }, expect.anything());
  });

  it('keeps Recent at eight server-ordered summaries without fetching the rest', async () => {
    const nodes = [{ id: 'recent-b' }, { id: 'recent-a' }];
    graphqlFetch.mockResolvedValue({ integrationSessionSummaries: { nodes, hasMore: true, nextOffset: 8 } });
    await expect(fetchRecentSessions()).resolves.toBe(nodes);
    expect(graphqlFetch).toHaveBeenCalledTimes(1);
    expect(graphqlFetch).toHaveBeenCalledWith(IntegrationSessionSummariesDocument, {
      input: { search: null, includeArchived: false, hasRuns: null, limit: 8, offset: 0 }
    }, expect.anything());
  });
});
