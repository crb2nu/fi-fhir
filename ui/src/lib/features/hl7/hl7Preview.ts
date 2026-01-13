import type { ParsePreviewQuery, ParsePreviewQueryVariables } from '$lib/gen/graphql';
import { ParsePreviewDocument } from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

export type HL7PreviewInput = {
  source: string;
  data: string;
};

export async function parseHL7Preview(input: HL7PreviewInput): Promise<ParsePreviewQuery> {
  const vars: ParsePreviewQueryVariables = {
    format: 'HL7V2',
    data: input.data,
    source: input.source || 'ui'
  };
  return graphqlFetch(ParsePreviewDocument, vars);
}

