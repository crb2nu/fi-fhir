import { graphqlFetch } from '$lib/graphql/client';
import { SubmitMessageDocument, type SubmitMessageMutation, type SubmitMessageMutationVariables } from '$lib/gen/graphql';

export type HL7SubmitInput = {
  source: string;
  data: string;
  correlationId?: string | null;
};

export async function submitHL7Message(input: HL7SubmitInput): Promise<SubmitMessageMutation['submitMessage']> {
  const vars: SubmitMessageMutationVariables = {
    input: {
      format: 'HL7V2',
      data: input.data,
      source: input.source || 'ui',
      correlationId: input.correlationId ?? null
    }
  };
  const result = await graphqlFetch<SubmitMessageMutation, SubmitMessageMutationVariables>(
    SubmitMessageDocument,
    vars,
    { showSuccessToast: true, successMessage: 'Message submitted' }
  );
  return result.submitMessage;
}

