import { graphqlFetch } from '$lib/graphql/client';
import {
  DeployIntegrationReleaseDocument,
  DiscardDeadLetterDocument,
  OperatorAttemptAuditDocument,
  OperatorCircuitsDocument,
  OperatorDeadLettersDocument,
  OperatorDeliveryAttemptDocument,
  OperatorDeliveryAttemptsDocument,
  OperatorDeploymentEventsDocument,
  OperatorDeploymentsDocument,
  OperatorMessageTraceDocument,
  OperatorReceiptsDocument,
  PauseIntegrationDeploymentDocument,
  ReplayDeliveryDocument,
  ResubmitMessageDocument,
  ResumeIntegrationDeploymentDocument,
  RetireIntegrationDeploymentDocument,
  type OperatorAttemptAuditQuery,
  type OperatorAttemptFilter,
  type OperatorCircuitsQuery,
  type OperatorDeadLettersQuery,
  type OperatorDeliveryAttemptQuery,
  type OperatorDeliveryAttemptsQuery,
  type OperatorDeliveryControlInput,
  type OperatorDeploymentCommandInput,
  type OperatorDeploymentEventsQuery,
  type OperatorDeploymentsQuery,
  type OperatorMessageTraceQuery,
  type OperatorPageInput,
  type OperatorReceiptFilter,
  type OperatorReceiptsQuery
} from '$lib/gen/graphql';

/**
 * Every operator failure has an inline home — a panel error state with Retry,
 * a row's Delivery block, or the reason dialog's submit error — so these
 * operations opt out of the global GraphQL error toast. With the net on, a
 * refused call ("operator control-plane action forbidden") showed twice: the
 * toast and the inline guidance (toast-budget B4). The global default is
 * unchanged for every other caller; success toasts (R1) are kept.
 */
const INLINE_ERRORS = { showErrorToast: false } as const;

export type OperatorReceipt = OperatorReceiptsQuery['operatorReceipts']['nodes'][number];
export type OperatorMessageTrace = NonNullable<OperatorMessageTraceQuery['operatorMessageTrace']>;
export type OperatorAttempt =
  OperatorDeliveryAttemptsQuery['operatorDeliveryAttempts']['nodes'][number];
/** One destination provenance-ledger row, as the Delivery block renders it. */
export type OperatorDestinationDelivery = OperatorAttempt['deliveries'][number];
export type OperatorDeadLetter = OperatorDeadLettersQuery['operatorDeadLetters']['nodes'][number];
export type OperatorCircuit = OperatorCircuitsQuery['operatorCircuits'][number];
export type OperatorAuditRecord = OperatorAttemptAuditQuery['operatorAttemptAudit']['nodes'][number];
export type OperatorDeployment = OperatorDeploymentsQuery['operatorDeployments'][number];
export type OperatorDeploymentEvent =
  OperatorDeploymentEventsQuery['operatorDeploymentEvents'][number];

export async function fetchReceipts(
  filter: OperatorReceiptFilter | null,
  page: OperatorPageInput | null
): Promise<OperatorReceiptsQuery['operatorReceipts']> {
  const result = await graphqlFetch(OperatorReceiptsDocument, { filter, page }, INLINE_ERRORS);
  return result.operatorReceipts;
}

export async function fetchMessageTrace(receiptId: string): Promise<OperatorMessageTrace | null> {
  const result = await graphqlFetch(OperatorMessageTraceDocument, { receiptId }, INLINE_ERRORS);
  return result.operatorMessageTrace ?? null;
}

export async function fetchAttempts(
  filter: OperatorAttemptFilter | null,
  page: OperatorPageInput | null
): Promise<OperatorDeliveryAttemptsQuery['operatorDeliveryAttempts']> {
  const result = await graphqlFetch(
    OperatorDeliveryAttemptsDocument,
    { filter, page },
    INLINE_ERRORS
  );
  return result.operatorDeliveryAttempts;
}

export async function fetchAttempt(
  attemptId: string
): Promise<OperatorDeliveryAttemptQuery['operatorDeliveryAttempt']> {
  const result = await graphqlFetch(OperatorDeliveryAttemptDocument, { attemptId }, INLINE_ERRORS);
  return result.operatorDeliveryAttempt;
}

export async function fetchDeadLetters(
  activeOnly: boolean,
  page: OperatorPageInput | null
): Promise<OperatorDeadLettersQuery['operatorDeadLetters']> {
  const result = await graphqlFetch(
    OperatorDeadLettersDocument,
    { activeOnly, page },
    INLINE_ERRORS
  );
  return result.operatorDeadLetters;
}

export async function fetchCircuits(): Promise<OperatorCircuit[]> {
  const result = await graphqlFetch(OperatorCircuitsDocument, {}, INLINE_ERRORS);
  return result.operatorCircuits;
}

export async function fetchAttemptAudit(
  attemptId: string,
  page: OperatorPageInput | null
): Promise<OperatorAttemptAuditQuery['operatorAttemptAudit']> {
  const result = await graphqlFetch(
    OperatorAttemptAuditDocument,
    { attemptId, page },
    INLINE_ERRORS
  );
  return result.operatorAttemptAudit;
}

export async function fetchDeployments(): Promise<OperatorDeployment[]> {
  const result = await graphqlFetch(OperatorDeploymentsDocument, {}, INLINE_ERRORS);
  return result.operatorDeployments;
}

export async function fetchDeploymentEvents(
  definitionId: string,
  revisionId: string
): Promise<OperatorDeploymentEvent[]> {
  const result = await graphqlFetch(
    OperatorDeploymentEventsDocument,
    { definitionId, revisionId },
    INLINE_ERRORS
  );
  return result.operatorDeploymentEvents;
}

/**
 * Delivery recovery. Success is an async result of an explicit operator action
 * the user is waiting on, so the shared success toast (R1) is the right
 * surface; failures are rendered inline by the caller (the dialog), only.
 */
export async function replayDelivery(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    ReplayDeliveryDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Replayed the delivery attempt' }
  );
  return result.replayDelivery;
}

export async function resubmitMessage(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    ResubmitMessageDocument,
    { input },
    {
      ...INLINE_ERRORS,
      showSuccessToast: true,
      successMessage: 'Resubmitted the message as a new attempt'
    }
  );
  return result.resubmitMessage;
}

export async function discardDeadLetter(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    DiscardDeadLetterDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Discarded the dead letter' }
  );
  return result.discardDeadLetter;
}

export async function pauseDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    PauseIntegrationDeploymentDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Paused the integration' }
  );
  return result.pauseIntegrationDeployment;
}

export async function resumeDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    ResumeIntegrationDeploymentDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Resumed the integration' }
  );
  return result.resumeIntegrationDeployment;
}

export async function retireDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    RetireIntegrationDeploymentDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Retired the integration revision' }
  );
  return result.retireIntegrationDeployment;
}

export async function deployRelease(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    DeployIntegrationReleaseDocument,
    { input },
    { ...INLINE_ERRORS, showSuccessToast: true, successMessage: 'Deployed the published release' }
  );
  return result.deployIntegrationRelease;
}
