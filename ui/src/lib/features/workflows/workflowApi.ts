import { graphqlFetch } from '$lib/graphql/client';
import {
  ListWorkflowsDocument,
  GetWorkflowDocument,
  TriggerWorkflowDocument,
  GenerateWorkflowDocument,
  ExplainWorkflowDocument,
  DryRunWorkflowDocument,
  type ListWorkflowsQuery,
  type GetWorkflowQuery,
  type GetWorkflowQueryVariables,
  type TriggerWorkflowMutation,
  type TriggerWorkflowMutationVariables,
  type GenerateWorkflowMutation,
  type GenerateWorkflowMutationVariables,
  type ExplainWorkflowQuery,
  type ExplainWorkflowQueryVariables,
  type DryRunWorkflowMutation,
  type DryRunWorkflowMutationVariables
} from '$lib/gen/graphql';

export function fetchWorkflows(): Promise<ListWorkflowsQuery> {
  return graphqlFetch(ListWorkflowsDocument);
}

export function fetchWorkflow(name: string): Promise<GetWorkflowQuery> {
  return graphqlFetch<GetWorkflowQuery, GetWorkflowQueryVariables>(GetWorkflowDocument, { name });
}

export function triggerWorkflow(
  name: string,
  event: unknown
): Promise<TriggerWorkflowMutation> {
  return graphqlFetch<TriggerWorkflowMutation, TriggerWorkflowMutationVariables>(
    TriggerWorkflowDocument,
    { name, event }
  );
}

export function generateWorkflow(
  description: string,
  eventTypes?: string[],
  actionTypes?: string[]
): Promise<GenerateWorkflowMutation> {
  return graphqlFetch<GenerateWorkflowMutation, GenerateWorkflowMutationVariables>(
    GenerateWorkflowDocument,
    { input: { description, eventTypes: eventTypes ?? null, actionTypes: actionTypes ?? null } }
  );
}

export function explainWorkflow(
  workflowYaml: string,
  audience?: string
): Promise<ExplainWorkflowQuery> {
  return graphqlFetch<ExplainWorkflowQuery, ExplainWorkflowQueryVariables>(
    ExplainWorkflowDocument,
    { input: { workflowYaml, audience: audience ?? null } }
  );
}

export function dryRunWorkflow(
  yaml: string,
  events: unknown[]
): Promise<DryRunWorkflowMutation> {
  return graphqlFetch<DryRunWorkflowMutation, DryRunWorkflowMutationVariables>(
    DryRunWorkflowDocument,
    { input: { yaml, events } }
  );
}
