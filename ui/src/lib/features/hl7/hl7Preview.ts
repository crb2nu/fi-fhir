import type {
  ParsePreviewQuery,
  ParsePreviewQueryVariables,
  ParsePreviewWithProfileQuery,
  ParsePreviewWithProfileQueryVariables
} from '$lib/gen/graphql';
import { ParsePreviewDocument, ParsePreviewWithProfileDocument } from '$lib/gen/graphql';
import { graphqlFetch } from '$lib/graphql/client';

export type HL7PreviewInput = {
  source: string;
  data: string;
  profileId?: string | null;
};

export type HL7PreviewResult = {
  parsePreview: ParsePreviewQuery['parsePreview'];
};

export async function parseHL7Preview(input: HL7PreviewInput): Promise<HL7PreviewResult> {
  // If a profileId is provided, use the profile-aware query
  if (input.profileId) {
    const vars: ParsePreviewWithProfileQueryVariables = {
      format: 'HL7V2',
      data: input.data,
      source: input.source || 'ui',
      profileId: input.profileId
    };
    const result = await graphqlFetch<
      ParsePreviewWithProfileQuery,
      ParsePreviewWithProfileQueryVariables
    >(ParsePreviewWithProfileDocument, vars);
    // Normalize the result to match the standard parsePreview shape
    return {
      parsePreview: result.parsePreviewWithProfile
    };
  }

  // Otherwise use the standard query
  const vars: ParsePreviewQueryVariables = {
    format: 'HL7V2',
    data: input.data,
    source: input.source || 'ui'
  };
  return graphqlFetch<ParsePreviewQuery, ParsePreviewQueryVariables>(ParsePreviewDocument, vars);
}

