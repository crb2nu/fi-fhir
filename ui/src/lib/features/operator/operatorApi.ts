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

export type OperatorReceipt = OperatorReceiptsQuery['operatorReceipts']['nodes'][number];
export type OperatorMessageTrace = NonNullable<OperatorMessageTraceQuery['operatorMessageTrace']>;
export type OperatorAttempt =
  OperatorDeliveryAttemptsQuery['operatorDeliveryAttempts']['nodes'][number];
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
  const result = await graphqlFetch(OperatorReceiptsDocument, { filter, page });
  return result.operatorReceipts;
}

export async function fetchMessageTrace(receiptId: string): Promise<OperatorMessageTrace | null> {
  const result = await graphqlFetch(OperatorMessageTraceDocument, { receiptId });
  return result.operatorMessageTrace ?? null;
}

export async function fetchAttempts(
  filter: OperatorAttemptFilter | null,
  page: OperatorPageInput | null
): Promise<OperatorDeliveryAttemptsQuery['operatorDeliveryAttempts']> {
  const result = await graphqlFetch(OperatorDeliveryAttemptsDocument, { filter, page });
  return result.operatorDeliveryAttempts;
}

export async function fetchAttempt(
  attemptId: string
): Promise<OperatorDeliveryAttemptQuery['operatorDeliveryAttempt']> {
  const result = await graphqlFetch(OperatorDeliveryAttemptDocument, { attemptId });
  return result.operatorDeliveryAttempt;
}

export async function fetchDeadLetters(
  activeOnly: boolean,
  page: OperatorPageInput | null
): Promise<OperatorDeadLettersQuery['operatorDeadLetters']> {
  const result = await graphqlFetch(OperatorDeadLettersDocument, { activeOnly, page });
  return result.operatorDeadLetters;
}

export async function fetchCircuits(): Promise<OperatorCircuit[]> {
  const result = await graphqlFetch(OperatorCircuitsDocument, {});
  return result.operatorCircuits;
}

export async function fetchAttemptAudit(
  attemptId: string,
  page: OperatorPageInput | null
): Promise<OperatorAttemptAuditQuery['operatorAttemptAudit']> {
  const result = await graphqlFetch(OperatorAttemptAuditDocument, { attemptId, page });
  return result.operatorAttemptAudit;
}

export async function fetchDeployments(): Promise<OperatorDeployment[]> {
  const result = await graphqlFetch(OperatorDeploymentsDocument, {});
  return result.operatorDeployments;
}

export async function fetchDeploymentEvents(
  definitionId: string,
  revisionId: string
): Promise<OperatorDeploymentEvent[]> {
  const result = await graphqlFetch(OperatorDeploymentEventsDocument, { definitionId, revisionId });
  return result.operatorDeploymentEvents;
}

/**
 * Delivery recovery. Success is an async result of an explicit operator action
 * the user is waiting on, so the shared success toast (R1) is the right
 * surface; failures are rendered inline by the caller.
 */
export async function replayDelivery(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    ReplayDeliveryDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Replayed the delivery attempt' }
  );
  return result.replayDelivery;
}

export async function resubmitMessage(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    ResubmitMessageDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Resubmitted the message as a new attempt' }
  );
  return result.resubmitMessage;
}

export async function discardDeadLetter(input: OperatorDeliveryControlInput) {
  const result = await graphqlFetch(
    DiscardDeadLetterDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Discarded the dead letter' }
  );
  return result.discardDeadLetter;
}

export async function pauseDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    PauseIntegrationDeploymentDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Paused the integration' }
  );
  return result.pauseIntegrationDeployment;
}

export async function resumeDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    ResumeIntegrationDeploymentDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Resumed the integration' }
  );
  return result.resumeIntegrationDeployment;
}

export async function retireDeployment(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    RetireIntegrationDeploymentDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Retired the integration revision' }
  );
  return result.retireIntegrationDeployment;
}

export async function deployRelease(input: OperatorDeploymentCommandInput) {
  const result = await graphqlFetch(
    DeployIntegrationReleaseDocument,
    { input },
    { showSuccessToast: true, successMessage: 'Deployed the published release' }
  );
  return result.deployIntegrationRelease;
}
