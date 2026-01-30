/**
 * Tests for the GraphQL client.
 */
import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { graphqlFetch } from './client';
import { toasts, toastList } from '$lib/ui/toastStore';
import { get } from 'svelte/store';

// Mock fetch
const mockFetch = vi.fn();
global.fetch = mockFetch;

// Create a minimal typed document for testing
// eslint-disable-next-line @typescript-eslint/no-explicit-any
const mockDocument = { kind: 'Document', definitions: [] } as any;

describe('graphqlFetch', () => {
  beforeEach(() => {
    vi.useFakeTimers();
    mockFetch.mockReset();
    toasts.dismissAll();
  });

  afterEach(() => {
    toasts.dismissAll();
    vi.useRealTimers();
  });

  it('should make a POST request to /graphql', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: { test: 'value' } })
    });

    await graphqlFetch(mockDocument, { foo: 'bar' });

    expect(mockFetch).toHaveBeenCalledWith('/graphql', {
      method: 'POST',
      headers: { 'content-type': 'application/json' },
      body: expect.stringContaining('"variables":{"foo":"bar"}')
    });
  });

  it('should return data from successful response', async () => {
    const expectedData = { users: [{ id: '1', name: 'Test' }] };
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: expectedData })
    });

    const result = await graphqlFetch(mockDocument);

    expect(result).toEqual(expectedData);
  });

  it('should show error toast on HTTP error', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500
    });

    await expect(graphqlFetch(mockDocument)).rejects.toThrow('GraphQL HTTP 500');

    const list = get(toastList);
    expect(list).toHaveLength(1);
    expect(list[0]!.variant).toBe('error');
    expect(list[0]!.message).toContain('HTTP 500');
  });

  it('should show error toast on GraphQL errors', async () => {
    const beforeCount = get(toastList).length;

    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        errors: [{ message: 'Invalid query' }]
      })
    });

    await expect(graphqlFetch(mockDocument)).rejects.toThrow('Invalid query');

    const list = get(toastList);
    expect(list.length).toBeGreaterThan(beforeCount);
    const errorToast = list.find((t) => t.message.includes('Invalid query'));
    expect(errorToast).toBeDefined();
    expect(errorToast!.variant).toBe('error');
  });

  it('should combine multiple GraphQL errors', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({
        errors: [{ message: 'Error 1' }, { message: 'Error 2' }]
      })
    });

    await expect(graphqlFetch(mockDocument)).rejects.toThrow('Error 1; Error 2');
  });

  it('should not show toast when showErrorToast is false', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: false,
      status: 500
    });

    await expect(
      graphqlFetch(mockDocument, undefined, { showErrorToast: false })
    ).rejects.toThrow();

    expect(get(toastList)).toHaveLength(0);
  });

  it('should show success toast when showSuccessToast is true', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({ data: { success: true } })
    });

    await graphqlFetch(mockDocument, undefined, {
      showSuccessToast: true,
      successMessage: 'Saved successfully!'
    });

    const list = get(toastList);
    expect(list).toHaveLength(1);
    expect(list[0]!.variant).toBe('success');
    expect(list[0]!.message).toBe('Saved successfully!');
  });

  it('should throw when data is missing', async () => {
    mockFetch.mockResolvedValueOnce({
      ok: true,
      json: async () => ({}) // No data field
    });

    await expect(graphqlFetch(mockDocument)).rejects.toThrow('missing data');
  });
});
