import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { print } from 'graphql';

type GraphQLErrorResponse = {
  errors?: Array<{ message?: string }>;
};

export async function graphqlFetch<TData, TVars>(
  document: TypedDocumentNode<TData, TVars>,
  variables?: TVars
): Promise<TData> {
  const res = await fetch('/graphql', {
    method: 'POST',
    headers: { 'content-type': 'application/json' },
    body: JSON.stringify({ query: print(document), variables })
  });

  if (!res.ok) {
    throw new Error(`GraphQL HTTP ${res.status}`);
  }

  const json = (await res.json()) as { data?: TData } & GraphQLErrorResponse;
  if (json.errors?.length) {
    const msg = json.errors.map((e) => e.message ?? 'Unknown error').join('; ');
    throw new Error(msg);
  }
  if (!json.data) {
    throw new Error('GraphQL response missing data');
  }
  return json.data;
}
