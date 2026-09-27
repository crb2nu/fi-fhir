import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { print } from 'graphql';
import { toasts } from '$lib/ui/toastStore';
import { requireGraphQLAuthorization } from './credentials';

/** Tag errors that have already been shown as a toast so the catch block skips them. */
const TOASTED = Symbol('toasted');
type ToastedError = Error & { [TOASTED]: true };

function markToasted(err: Error): ToastedError {
  return Object.assign(err, { [TOASTED]: true as const });
}

/**
 * Reports whether an error was already surfaced as a toast by the global net
 * in `graphqlFetch` (i.e. carries the internal TOASTED flag).
 *
 * Component `catch` blocks use this to **defer to the global net** and avoid a
 * double-toast (toast-budget policy .loom/22, B4): a component that adds inline
 * field context keeps the inline message but only toasts when the global net did
 * not already — `if (!isErrorToasted(e)) toasts.error(...)`. A graphql failure is
 * therefore shown once (by the net); a local throw the net never saw (a
 * pre-fetch `JSON.parse`, etc.) still toasts from the component.
 */
export function isErrorToasted(err: unknown): err is ToastedError {
  return err instanceof Error && TOASTED in err;
}

/** One entry of a GraphQL response's `errors`, as the server presented it. */
export interface GraphQLResponseErrorEntry {
  message?: string;
  extensions?: Record<string, unknown>;
}

type GraphQLErrorResponse = {
  errors?: GraphQLResponseErrorEntry[];
};

/**
 * The error `graphqlFetch` throws when the response carries GraphQL `errors`.
 * Its message is the joined messages (unchanged for every existing caller);
 * `errors` keeps each entry's `extensions`, which a few catalog-safe errors use
 * to say which input field they refused (e.g. the connection catalog's
 * `extensions.problems`).
 */
export class GraphQLResponseError extends Error {
  readonly errors: readonly GraphQLResponseErrorEntry[];

  constructor(message: string, errors: readonly GraphQLResponseErrorEntry[]) {
    super(message);
    this.name = 'GraphQLResponseError';
    this.errors = errors;
  }
}

/** The GraphQL error entries behind a thrown value, or [] when it is not one. */
export function graphQLErrorEntries(err: unknown): readonly GraphQLResponseErrorEntry[] {
  return err instanceof GraphQLResponseError ? err.errors : [];
}

export interface GraphQLFetchOptions {
  /** Show error toast on failure. Default: true */
  showErrorToast?: boolean;
  /** Show success toast on completion. Default: false */
  showSuccessToast?: boolean;
  /** Custom success message. Default: 'Operation completed' */
  successMessage?: string;
}

/**
 * Fetches data from the GraphQL API with automatic error handling.
 *
 * By default, errors are shown as toast notifications. This behavior can be
 * disabled via the options parameter for cases where you want custom error handling.
 *
 * @param document - The typed GraphQL document to execute
 * @param variables - Variables to pass to the query/mutation
 * @param options - Options for toast notifications
 * @returns The data from the GraphQL response
 * @throws Error if the request fails or returns GraphQL errors
 */
export async function graphqlFetch<TData, TVars>(
  document: TypedDocumentNode<TData, TVars>,
  variables?: TVars,
  options?: GraphQLFetchOptions
): Promise<TData> {
  const { showErrorToast = true, showSuccessToast = false, successMessage } = options ?? {};

  try {
    const authorization = await requireGraphQLAuthorization();
    const headers: Record<string, string> = {
      'content-type': 'application/json'
    };
    if (authorization) {
      headers.Authorization = authorization;
    }
    const res = await fetch('/graphql', {
      method: 'POST',
      headers,
      body: JSON.stringify({ query: print(document), variables })
    });

    if (!res.ok) {
      const error = new Error(`GraphQL HTTP ${res.status}`);
      if (showErrorToast) {
        toasts.error(`Request failed: HTTP ${res.status}`);
        markToasted(error);
      }
      throw error;
    }

    const json = (await res.json()) as { data?: TData } & GraphQLErrorResponse;
    if (json.errors?.length) {
      const msg = json.errors.map((e) => e.message ?? 'Unknown error').join('; ');
      const error = new GraphQLResponseError(msg, json.errors);
      if (showErrorToast) {
        toasts.error(msg);
        markToasted(error);
      }
      throw error;
    }
    if (!json.data) {
      const error = new Error('GraphQL response missing data');
      if (showErrorToast) {
        toasts.error('Unexpected response from server');
        markToasted(error);
      }
      throw error;
    }

    if (showSuccessToast) {
      toasts.success(successMessage ?? 'Operation completed');
    }

    return json.data;
  } catch (err) {
    // Only toast network failures (fetch rejections) — skip errors already toasted above
    if (err instanceof Error && showErrorToast && !isErrorToasted(err)) {
      toasts.error(`Network error: ${err.message}`);
    }
    throw err;
  }
}
