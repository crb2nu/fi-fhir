/**
 * Verification reads (.loom/42 E-2) over the operator control plane:
 * canonical events joined to their receipts, admission statistics, and the
 * retention posture of the replica that answered.
 */
import { graphqlFetch } from '$lib/graphql/client';
import {
  VerificationAdmissionsDocument,
  VerificationRetentionPostureDocument,
  VerificationStatisticsDocument,
  type OperatorCanonicalEventFilter,
  type OperatorPageInput,
  type OperatorStatisticsBucket,
  type VerificationAdmissionsQuery,
  type VerificationRetentionPostureQuery,
  type VerificationStatisticsQuery
} from '$lib/gen/graphql';

/**
 * Every failure here has an inline home (the view's error state with Retry),
 * so these reads opt out of the global error toast, as the operator reads do
 * (toast-budget B4).
 */
const INLINE_ERRORS = { showErrorToast: false } as const;

export type AdmissionPage = VerificationAdmissionsQuery['operatorCanonicalEvents'];
export type Admission = AdmissionPage['nodes'][number];
export type AdmissionStatistics = VerificationStatisticsQuery['operatorAdmissionStatistics'];
export type RetentionPosture = VerificationRetentionPostureQuery['engineRuntime'];

export async function fetchAdmissions(
  filter: OperatorCanonicalEventFilter | null,
  page: OperatorPageInput | null
): Promise<AdmissionPage> {
  const result = await graphqlFetch(VerificationAdmissionsDocument, { filter, page }, INLINE_ERRORS);
  return result.operatorCanonicalEvents;
}

export async function fetchAdmissionStatistics(
  window: { from: string; to: string },
  bucket: OperatorStatisticsBucket
): Promise<AdmissionStatistics> {
  const result = await graphqlFetch(VerificationStatisticsDocument, { window, bucket }, INLINE_ERRORS);
  return result.operatorAdmissionStatistics;
}

export async function fetchRetentionPosture(): Promise<RetentionPosture> {
  const result = await graphqlFetch(VerificationRetentionPostureDocument, {}, INLINE_ERRORS);
  return result.engineRuntime;
}
