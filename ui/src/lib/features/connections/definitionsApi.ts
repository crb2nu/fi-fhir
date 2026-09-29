/**
 * Integration definition authoring over GraphQL (.loom/42 E-1), one function
 * per operation. Like connectionsApi, every failure has an inline home (a
 * pane's EmptyState, the draft form's problem list, or the reason dialog), so
 * no call raises the global error toast; writes the user waits on keep the
 * shared success toast, and a draft write refused with problems is a result,
 * not an error.
 */
import { graphqlFetch } from '$lib/graphql/client';
import {
  ApproveIntegrationDefinitionDocument,
  CreateIntegrationDefinitionDraftDocument,
  DefinitionConnectionChoicesDocument,
  DefinitionPolicyDefaultsDocument,
  IntegrationDefinitionDocument,
  IntegrationDefinitionsDocument,
  IntegrationRegistryArtifactsDocument,
  PublishIntegrationDefinitionDocument,
  ValidateIntegrationDefinitionDocument,
  ValidateIntegrationDefinitionDraftDocument,
  type CreateIntegrationDefinitionDraftMutation,
  type DefinitionConnectionChoicesQuery,
  type DefinitionProblemFieldsFragment,
  type IntegrationDefinitionCommandInput,
  type IntegrationDefinitionDetailFieldsFragment,
  type IntegrationDefinitionDraftInput,
  type IntegrationDefinitionFieldsFragment,
  type IntegrationDefinitionValidateInput,
  type IntegrationRegistryArtifactsQuery,
  type IntegrationValidationMode
} from '$lib/gen/graphql';

const INLINE_ERRORS = { showErrorToast: false } as const;

export type DefinitionRow = IntegrationDefinitionFieldsFragment;
export type DefinitionDetail = IntegrationDefinitionDetailFieldsFragment;
export type DefinitionProblem = DefinitionProblemFieldsFragment;
export type RegistryArtifact = IntegrationRegistryArtifactsQuery['integrationRegistryArtifacts'][number];
export type ConnectionChoice = DefinitionConnectionChoicesQuery['connections'][number];
export type DraftResult = CreateIntegrationDefinitionDraftMutation['createIntegrationDefinitionDraft'];
export type {
  IntegrationDefinitionCommandInput,
  IntegrationDefinitionDraftInput,
  IntegrationDefinitionValidateInput,
  IntegrationValidationMode
};

/** The engine property that sets a new draft's default evidence max age. */
export const MAX_AGE_PROPERTY = 'FI_FHIR_LIFECYCLE_VALIDATION_MAX_AGE';

export async function fetchDefinitions(includeRetired: boolean): Promise<DefinitionRow[]> {
  const result = await graphqlFetch(IntegrationDefinitionsDocument, { includeRetired }, INLINE_ERRORS);
  return result.integrationDefinitions;
}

export async function fetchDefinition(definitionId: string, revisionId: string): Promise<DefinitionDetail | null> {
  const result = await graphqlFetch(IntegrationDefinitionDocument, { definitionId, revisionId }, INLINE_ERRORS);
  return result.integrationDefinition ?? null;
}

export async function fetchRegistryArtifacts(): Promise<RegistryArtifact[]> {
  const result = await graphqlFetch(IntegrationRegistryArtifactsDocument, {}, INLINE_ERRORS);
  return result.integrationRegistryArtifacts;
}

/** Non-archived connections of one direction with their latest compiled revision. */
export async function fetchConnectionChoices(direction: 'SOURCE' | 'DESTINATION'): Promise<ConnectionChoice[]> {
  const result = await graphqlFetch(DefinitionConnectionChoicesDocument, { direction }, INLINE_ERRORS);
  return result.connections;
}

/** The deployment's default validation max age in seconds, or null when not reported. */
export async function fetchDefaultMaxAge(): Promise<number | null> {
  const result = await graphqlFetch(DefinitionPolicyDefaultsDocument, {}, INLINE_ERRORS);
  const property = result.engineRuntime.properties.find((candidate) => candidate.key === MAX_AGE_PROPERTY);
  const value = property ? Number.parseInt(property.value, 10) : Number.NaN;
  return Number.isFinite(value) && value > 0 ? value : null;
}

/** Every pre-flight check, with no write. Never toasts: the form is its home. */
export async function checkDraft(input: IntegrationDefinitionDraftInput): Promise<DefinitionProblem[]> {
  const result = await graphqlFetch(ValidateIntegrationDefinitionDraftDocument, { input }, INLINE_ERRORS);
  return result.validateIntegrationDefinitionDraft;
}

/** Writes the draft unless a problem blocks; the caller toasts the case it got. */
export async function createDraft(input: IntegrationDefinitionDraftInput, reason: string): Promise<DraftResult> {
  const result = await graphqlFetch(CreateIntegrationDefinitionDraftDocument, { input, reason }, INLINE_ERRORS);
  return result.createIntegrationDefinitionDraft;
}

/** Records validation evidence; a failed check comes back as evidence, not an error. */
export async function validateDefinition(input: IntegrationDefinitionValidateInput): Promise<DefinitionDetail> {
  const result = await graphqlFetch(ValidateIntegrationDefinitionDocument, { input }, INLINE_ERRORS);
  return result.validateIntegrationDefinition;
}

export async function approveDefinition(input: IntegrationDefinitionCommandInput): Promise<DefinitionDetail> {
  const result = await graphqlFetch(
    ApproveIntegrationDefinitionDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: `Approved ${input.definitionId}/${input.revisionId}` }
  );
  return result.approveIntegrationDefinition;
}

export async function publishDefinition(input: IntegrationDefinitionCommandInput): Promise<DefinitionDetail> {
  const result = await graphqlFetch(
    PublishIntegrationDefinitionDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: `Published ${input.definitionId}/${input.revisionId}` }
  );
  return result.publishIntegrationDefinition;
}
