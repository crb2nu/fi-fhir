import { graphqlFetch } from '$lib/graphql/client';
import {
  ListWorkflowsDocument,
  ListWorkflowDefinitionsDocument,
  GetWorkflowVersionsDocument,
  GetWorkflowVersionByIdDocument,
  ListWorkflowRunsDocument,
  GetWorkflowRunDocument,
  GetWorkflowDocument,
  CreateWorkflowDefinitionDocument,
  SaveWorkflowVersionDocument,
  PublishWorkflowVersionDocument,
  RollbackWorkflowVersionDocument,
  RequestWorkflowApprovalDocument,
  TriggerWorkflowDocument,
  GenerateWorkflowDocument,
  ExplainWorkflowDocument,
  DryRunWorkflowDocument,
  type ListWorkflowsQuery,
  type ListWorkflowDefinitionsQuery,
  type ListWorkflowDefinitionsQueryVariables,
  type GetWorkflowVersionsQuery,
  type GetWorkflowVersionsQueryVariables,
  type GetWorkflowVersionByIdQuery,
  type GetWorkflowVersionByIdQueryVariables,
  type ListWorkflowRunsQuery,
  type ListWorkflowRunsQueryVariables,
  type GetWorkflowRunQuery,
  type GetWorkflowRunQueryVariables,
  type GetWorkflowQuery,
  type GetWorkflowQueryVariables,
  type CreateWorkflowDefinitionMutation,
  type CreateWorkflowDefinitionMutationVariables,
  type SaveWorkflowVersionMutation,
  type SaveWorkflowVersionMutationVariables,
  type PublishWorkflowVersionMutation,
  type PublishWorkflowVersionMutationVariables,
  type RollbackWorkflowVersionMutation,
  type RollbackWorkflowVersionMutationVariables,
  type RequestWorkflowApprovalMutation,
  type RequestWorkflowApprovalMutationVariables,
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

export function fetchWorkflowDefinitions(options?: {
  filter?: ListWorkflowDefinitionsQueryVariables['filter'];
  paging?: ListWorkflowDefinitionsQueryVariables['paging'];
}): Promise<ListWorkflowDefinitionsQuery> {
  return graphqlFetch<ListWorkflowDefinitionsQuery, ListWorkflowDefinitionsQueryVariables>(
    ListWorkflowDefinitionsDocument,
    {
      filter: options?.filter ?? null,
      paging: options?.paging ?? null
    }
  );
}

export function fetchWorkflowVersions(
  workflowId: string,
  paging?: GetWorkflowVersionsQueryVariables['paging']
): Promise<GetWorkflowVersionsQuery> {
  return graphqlFetch<GetWorkflowVersionsQuery, GetWorkflowVersionsQueryVariables>(
    GetWorkflowVersionsDocument,
    { workflowId, paging: paging ?? null }
  );
}

export function fetchWorkflowVersionById(id: string): Promise<GetWorkflowVersionByIdQuery> {
  return graphqlFetch<GetWorkflowVersionByIdQuery, GetWorkflowVersionByIdQueryVariables>(
    GetWorkflowVersionByIdDocument,
    { id }
  );
}

export function fetchWorkflowRuns(options?: {
  filter?: ListWorkflowRunsQueryVariables['filter'];
  paging?: ListWorkflowRunsQueryVariables['paging'];
}): Promise<ListWorkflowRunsQuery> {
  return graphqlFetch<ListWorkflowRunsQuery, ListWorkflowRunsQueryVariables>(
    ListWorkflowRunsDocument,
    {
      filter: options?.filter ?? null,
      paging: options?.paging ?? null
    }
  );
}

export function fetchWorkflowRun(id: string): Promise<GetWorkflowRunQuery> {
  return graphqlFetch<GetWorkflowRunQuery, GetWorkflowRunQueryVariables>(GetWorkflowRunDocument, {
    id
  });
}

export function fetchWorkflow(name: string): Promise<GetWorkflowQuery> {
  return graphqlFetch<GetWorkflowQuery, GetWorkflowQueryVariables>(GetWorkflowDocument, { name });
}

export function createWorkflowDefinition(input: {
  name: string;
  description?: string | null;
  createdBy?: string | null;
}): Promise<CreateWorkflowDefinitionMutation> {
  return graphqlFetch<CreateWorkflowDefinitionMutation, CreateWorkflowDefinitionMutationVariables>(
    CreateWorkflowDefinitionDocument,
    {
      input: {
        name: input.name,
        description: input.description ?? null,
        createdBy: input.createdBy ?? null
      }
    }
  );
}

export function saveWorkflowVersion(input: {
  workflowId: string;
  yaml: string;
  notes?: string | null;
  createdBy?: string | null;
}): Promise<SaveWorkflowVersionMutation> {
  return graphqlFetch<SaveWorkflowVersionMutation, SaveWorkflowVersionMutationVariables>(
    SaveWorkflowVersionDocument,
    {
      input: {
        workflowId: input.workflowId,
        yaml: input.yaml,
        notes: input.notes ?? null,
        createdBy: input.createdBy ?? null
      }
    }
  );
}

export function publishWorkflowVersion(input: {
  workflowId: string;
  versionId: string;
  environment: string;
  publishedBy?: string | null;
}): Promise<PublishWorkflowVersionMutation> {
  return graphqlFetch<PublishWorkflowVersionMutation, PublishWorkflowVersionMutationVariables>(
    PublishWorkflowVersionDocument,
    {
      input: {
        workflowId: input.workflowId,
        versionId: input.versionId,
        environment: input.environment,
        publishedBy: input.publishedBy ?? null
      }
    }
  );
}

export function rollbackWorkflowVersion(input: {
  workflowId: string;
  targetVersionId: string;
  environment: string;
  publishedBy?: string | null;
}): Promise<RollbackWorkflowVersionMutation> {
  return graphqlFetch<RollbackWorkflowVersionMutation, RollbackWorkflowVersionMutationVariables>(
    RollbackWorkflowVersionDocument,
    {
      input: {
        workflowId: input.workflowId,
        targetVersionId: input.targetVersionId,
        environment: input.environment,
        publishedBy: input.publishedBy ?? null
      }
    }
  );
}

export function requestWorkflowApproval(input: {
  workflowId: string;
  targetVersionId: string;
  environment: string;
  requestedBy?: string | null;
  comment?: string | null;
}): Promise<RequestWorkflowApprovalMutation> {
  return graphqlFetch<RequestWorkflowApprovalMutation, RequestWorkflowApprovalMutationVariables>(
    RequestWorkflowApprovalDocument,
    {
      input: {
        workflowId: input.workflowId,
        targetVersionId: input.targetVersionId,
        environment: input.environment,
        requestedBy: input.requestedBy ?? null,
        comment: input.comment ?? null
      }
    }
  );
}

export function triggerWorkflow(
  name: string,
  event: unknown,
  options?: {
    environment?: string;
    versionId?: string;
  }
): Promise<TriggerWorkflowMutation> {
  return graphqlFetch<TriggerWorkflowMutation, TriggerWorkflowMutationVariables>(
    TriggerWorkflowDocument,
    {
      name,
      event,
      environment: options?.environment ?? null,
      versionId: options?.versionId ?? null
    }
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
