/**
 * The connection catalog and engine runtime over GraphQL (.loom/38 C-0's
 * surface), one function per operation.
 *
 * Every failure here has an inline home — a pane's EmptyState with Retry, the
 * form's problem list, or the reason dialog's submit error — so every call opts
 * out of the global error toast (toast-budget B4). Mutations the user waits on
 * keep the shared success toast (R1); validation and reads never toast.
 */
import { graphqlFetch } from '$lib/graphql/client';
import {
  ArchiveConnectionDocument,
  CompileConnectionDocument,
  ConnectionDocument,
  ConnectionRevisionDocument,
  ConnectionRevisionsDocument,
  ConnectionsDocument,
  CreateConnectionDocument,
  EngineRuntimeDocument,
  UpdateConnectionDocument,
  ValidateConnectionSpecDocument,
  type CompileConnectionMutation,
  type ConnectionCommandInput,
  type ConnectionDirection,
  type ConnectionFieldsFragment,
  type ConnectionKind,
  type ConnectionProblemFieldsFragment,
  type ConnectionRevisionFieldsFragment,
  type ConnectionSecretBindingInput,
  type CreateConnectionInput,
  type EngineRuntimeFieldsFragment,
  type UpdateConnectionInput,
  type ValidateConnectionSpecInput
} from '$lib/gen/graphql';

const INLINE_ERRORS = { showErrorToast: false } as const;

/** One catalog connection: the draft, its latest revision, references and runtime state. */
export type ConnectionRow = ConnectionFieldsFragment;
/** One compiled revision, including the exact bytes `serve` mounts (`revisionJson`). */
export type ConnectionRevisionRow = ConnectionRevisionFieldsFragment;
/** The latest-revision summary a list row carries (no `revisionJson`). */
export type ConnectionRevisionSummary = NonNullable<ConnectionRow['latestRevision']>;
export type ConnectionReferenceRow = ConnectionRow['references'][number];
export type ConnectionSecretBindingRow = ConnectionRow['secretBindings'][number];
/** One field-level finding: `path` is JSON dot form relative to the spec. */
export type ConnectionProblemRow = ConnectionProblemFieldsFragment;
/** What this replica composed at startup. */
export type EngineRuntimeView = EngineRuntimeFieldsFragment;
/** One of the four adapter rows (http, mllp, batch, delivery), enabled or not. */
export type EngineAdapterRow = EngineRuntimeView['adapters'][number];
export type CompileConnectionResult = CompileConnectionMutation['compileConnection'];

export type {
  ConnectionCommandInput,
  ConnectionDirection,
  ConnectionKind,
  ConnectionSecretBindingInput,
  CreateConnectionInput,
  UpdateConnectionInput,
  ValidateConnectionSpecInput
};

export async function fetchConnections(
  direction: ConnectionDirection | null,
  includeArchived: boolean
): Promise<ConnectionRow[]> {
  const result = await graphqlFetch(ConnectionsDocument, { direction, includeArchived }, INLINE_ERRORS);
  return result.connections;
}

export async function fetchConnection(id: string): Promise<ConnectionRow | null> {
  const result = await graphqlFetch(ConnectionDocument, { id }, INLINE_ERRORS);
  return result.connection ?? null;
}

/** A connection's revisions, newest first. */
export async function fetchConnectionRevisions(id: string): Promise<ConnectionRevisionRow[]> {
  const result = await graphqlFetch(ConnectionRevisionsDocument, { id }, INLINE_ERRORS);
  return result.connectionRevisions;
}

export async function fetchConnectionRevision(
  artifactId: string,
  revisionId: string
): Promise<ConnectionRevisionRow | null> {
  const result = await graphqlFetch(
    ConnectionRevisionDocument,
    { artifactId, revisionId },
    INLINE_ERRORS
  );
  return result.connectionRevision ?? null;
}

export async function fetchEngineRuntime(): Promise<EngineRuntimeView> {
  const result = await graphqlFetch(EngineRuntimeDocument, {}, INLINE_ERRORS);
  return result.engineRuntime;
}

export async function createConnection(input: CreateConnectionInput): Promise<ConnectionRow> {
  const result = await graphqlFetch(
    CreateConnectionDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: `Created connection ${input.id}` }
  );
  return result.createConnection;
}

export async function updateConnection(input: UpdateConnectionInput): Promise<ConnectionRow> {
  const result = await graphqlFetch(
    UpdateConnectionDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: `Saved connection ${input.id}` }
  );
  return result.updateConnection;
}

export async function archiveConnection(input: ConnectionCommandInput): Promise<ConnectionRow> {
  const result = await graphqlFetch(
    ArchiveConnectionDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: `Archived connection ${input.id}` }
  );
  return result.archiveConnection;
}

/**
 * Compiles the saved draft at `expectedVersion`. A refused compile is a
 * result, not an error: `revision` is null and `problems` says why, so the
 * success toast is left to the caller, which knows which case it got.
 */
export async function compileConnection(input: ConnectionCommandInput): Promise<CompileConnectionResult> {
  const result = await graphqlFetch(CompileConnectionDocument, { input }, INLINE_ERRORS);
  return result.compileConnection;
}

/** Runs compile's checks with no write. Never toasts: the form is the only home. */
export async function validateConnectionSpec(
  input: ValidateConnectionSpecInput
): Promise<ConnectionProblemRow[]> {
  const result = await graphqlFetch(ValidateConnectionSpecDocument, { input }, INLINE_ERRORS);
  return result.validateConnectionSpec;
}
