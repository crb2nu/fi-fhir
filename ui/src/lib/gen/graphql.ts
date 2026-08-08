import type { TypedDocumentNode as DocumentNode } from '@graphql-typed-document-node/core';
export type Maybe<T> = T | null;
export type InputMaybe<T> = Maybe<T>;
export type Exact<T extends { [key: string]: unknown }> = { [K in keyof T]: T[K] };
export type MakeOptional<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]?: Maybe<T[SubKey]> };
export type MakeMaybe<T, K extends keyof T> = Omit<T, K> & { [SubKey in K]: Maybe<T[SubKey]> };
export type MakeEmpty<T extends { [key: string]: unknown }, K extends keyof T> = { [_ in K]?: never };
export type Incremental<T> = T | { [P in keyof T]?: P extends ' $fragmentName' | '__typename' ? T[P] : never };
/** All built-in and custom scalars, mapped to their actual values */
export type Scalars = {
  ID: { input: string; output: string; }
  String: { input: string; output: string; }
  Boolean: { input: boolean; output: boolean; }
  Int: { input: number; output: number; }
  Float: { input: number; output: number; }
  DateTime: { input: string; output: string; }
  JSON: { input: unknown; output: unknown; }
};

export type AcceptDiagnosticFixInput = {
  acceptedBy: InputMaybe<Scalars['String']['input']>;
  diagnosticId: Scalars['ID']['input'];
  sessionId: Scalars['ID']['input'];
};

export type ActiveEncounter = {
  __typename?: 'ActiveEncounter';
  admitTime: Scalars['DateTime']['output'];
  bed: Maybe<Scalars['String']['output']>;
  class: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  lastUpdated: Scalars['DateTime']['output'];
  location: Maybe<Scalars['String']['output']>;
  patientMrn: Scalars['ID']['output'];
  patientName: Maybe<Scalars['String']['output']>;
  provider: Maybe<Scalars['String']['output']>;
  room: Maybe<Scalars['String']['output']>;
  unit: Maybe<Scalars['String']['output']>;
};

export type AddSessionSampleInput = {
  data: Scalars['String']['input'];
  format: SourceFormat;
  name: Scalars['String']['input'];
  payloadRef: InputMaybe<Scalars['String']['input']>;
  retainRawPayload: InputMaybe<Scalars['Boolean']['input']>;
  sessionId: Scalars['ID']['input'];
  source: InputMaybe<Scalars['String']['input']>;
};

export type Address = {
  __typename?: 'Address';
  city: Maybe<Scalars['String']['output']>;
  country: Maybe<Scalars['String']['output']>;
  line1: Maybe<Scalars['String']['output']>;
  line2: Maybe<Scalars['String']['output']>;
  postalCode: Maybe<Scalars['String']['output']>;
  state: Maybe<Scalars['String']['output']>;
};

export type AnalyzeQualityInput = {
  event: Scalars['JSON']['input'];
  eventType: EventType;
};

export type Appointment = {
  __typename?: 'Appointment';
  endTime: Maybe<Scalars['DateTime']['output']>;
  id: Scalars['ID']['output'];
  location: Maybe<Location>;
  provider: Maybe<Provider>;
  reason: Maybe<Scalars['String']['output']>;
  startTime: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
};

export type AppointmentEvent = Event & {
  __typename?: 'AppointmentEvent';
  appointment: Appointment;
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type ApprovePendingAutorouteInput = {
  comment: InputMaybe<Scalars['String']['input']>;
  equivalence: InputMaybe<MappingEquivalence>;
  id: Scalars['ID']['input'];
};

export type ApproveWorkflowVersionInput = {
  approvalRequestId: Scalars['ID']['input'];
  comment: InputMaybe<Scalars['String']['input']>;
  reviewedBy: InputMaybe<Scalars['String']['input']>;
};

export type ArchiveWorkflowDefinitionInput = {
  archivedBy: InputMaybe<Scalars['String']['input']>;
  workflowId: Scalars['ID']['input'];
};

export type AssigningAuthority = {
  __typename?: 'AssigningAuthority';
  code: Scalars['String']['output'];
  name: Maybe<Scalars['String']['output']>;
  system: Scalars['String']['output'];
};

export type AssigningAuthorityInput = {
  code: Scalars['String']['input'];
  name: InputMaybe<Scalars['String']['input']>;
  system: Scalars['String']['input'];
};

export type AutorouteDecision =
  | 'AUTOROUTE_HIGH_CONF'
  | 'AUTOROUTE_LOW_CONF'
  | 'AUTOROUTE_MED_CONF'
  | 'NO_MATCH'
  | 'PERSISTENT_HIT';

export type AutorouteStep = {
  __typename?: 'AutorouteStep';
  durationMs: Scalars['Int']['output'];
  metadata: Maybe<Scalars['JSON']['output']>;
  result: Scalars['String']['output'];
  step: Scalars['String']['output'];
};

export type AutorouteTrace = {
  __typename?: 'AutorouteTrace';
  steps: Array<AutorouteStep>;
  timestamp: Scalars['DateTime']['output'];
  totalDurationMs: Scalars['Int']['output'];
  traceId: Scalars['String']['output'];
};

export type BatchEventItem = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  data: Scalars['JSON']['input'];
  index: InputMaybe<Scalars['Int']['input']>;
  source: Scalars['String']['input'];
  type: EventType;
};

export type BatchItemResult = {
  __typename?: 'BatchItemResult';
  errors: Array<Scalars['String']['output']>;
  eventId: Maybe<Scalars['ID']['output']>;
  index: Scalars['Int']['output'];
  success: Scalars['Boolean']['output'];
  warnings: Array<ParseWarning>;
  workflowResults: Array<WorkflowResult>;
};

export type BatchMessageItem = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  data: Scalars['String']['input'];
  format: SourceFormat;
  index: InputMaybe<Scalars['Int']['input']>;
  source: Scalars['String']['input'];
};

export type BatchResult = {
  __typename?: 'BatchResult';
  durationMs: Scalars['Int']['output'];
  failureCount: Scalars['Int']['output'];
  results: Array<BatchItemResult>;
  successCount: Scalars['Int']['output'];
  totalItems: Scalars['Int']['output'];
};

export type Breakpoint = {
  __typename?: 'Breakpoint';
  enabled: Scalars['Boolean']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  type: Scalars['String']['output'];
};

export type BulkApproveInput = {
  maxCount: InputMaybe<Scalars['Int']['input']>;
  minConfidence: InputMaybe<Scalars['Float']['input']>;
};

export type BulkApproveResult = {
  __typename?: 'BulkApproveResult';
  approved: Scalars['Int']['output'];
  mappings: Array<CodeMapping>;
  skipped: Scalars['Int']['output'];
};

export type ClassifyMessageInput = {
  data: Scalars['String']['input'];
  format: SourceFormat;
};

export type CodeMapping = {
  __typename?: 'CodeMapping';
  approvedAt: Maybe<Scalars['DateTime']['output']>;
  approvedBy: Maybe<Scalars['String']['output']>;
  comment: Maybe<Scalars['String']['output']>;
  confidence: Maybe<Scalars['Float']['output']>;
  createdAt: Scalars['DateTime']['output'];
  createdBy: Maybe<Scalars['String']['output']>;
  equivalence: MappingEquivalence;
  id: Scalars['ID']['output'];
  origin: MappingOrigin;
  profileId: Maybe<Scalars['String']['output']>;
  sourceCode: Scalars['String']['output'];
  sourceDisplay: Maybe<Scalars['String']['output']>;
  sourceSystem: Scalars['String']['output'];
  targetCode: Scalars['String']['output'];
  targetDisplay: Maybe<Scalars['String']['output']>;
  targetSystem: Scalars['String']['output'];
  uploadBatchId: Maybe<Scalars['ID']['output']>;
};

export type CodeMappingConnection = {
  __typename?: 'CodeMappingConnection';
  nodes: Array<CodeMapping>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type ComponentHealth = {
  __typename?: 'ComponentHealth';
  message: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type Condition = {
  __typename?: 'Condition';
  category: Maybe<Scalars['String']['output']>;
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
};

export type ConditionEvent = Event & {
  __typename?: 'ConditionEvent';
  clinicalStatus: Maybe<Scalars['String']['output']>;
  condition: Condition;
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  onsetDate: Maybe<Scalars['String']['output']>;
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type CreateIntegrationSessionInput = {
  description: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
};

export type CreateMappingInput = {
  comment: InputMaybe<Scalars['String']['input']>;
  confidence: InputMaybe<Scalars['Float']['input']>;
  equivalence: InputMaybe<MappingEquivalence>;
  origin: InputMaybe<MappingOrigin>;
  profileId: InputMaybe<Scalars['String']['input']>;
  sourceCode: Scalars['String']['input'];
  sourceDisplay: InputMaybe<Scalars['String']['input']>;
  sourceSystem: Scalars['String']['input'];
  targetCode: Scalars['String']['input'];
  targetDisplay: InputMaybe<Scalars['String']['input']>;
  targetSystem: Scalars['String']['input'];
};

export type CreateProfileInput = {
  id: Scalars['ID']['input'];
  name: Scalars['String']['input'];
};

export type CreateSubscriptionInput = {
  criteria: Scalars['String']['input'];
  endpoint: Scalars['String']['input'];
  name: Scalars['String']['input'];
  server: Scalars['String']['input'];
};

export type CreateWorkflowDefinitionInput = {
  createdBy: InputMaybe<Scalars['String']['input']>;
  description: InputMaybe<Scalars['String']['input']>;
  name: Scalars['String']['input'];
};

export type DataQualityIssue = {
  __typename?: 'DataQualityIssue';
  actualValue: Maybe<Scalars['String']['output']>;
  description: Scalars['String']['output'];
  dimension: Scalars['String']['output'];
  expectedValue: Maybe<Scalars['String']['output']>;
  field: Maybe<Scalars['String']['output']>;
  severity: Scalars['String']['output'];
};

export type DataQualityScore = {
  __typename?: 'DataQualityScore';
  dimensions: QualityDimensions;
  issues: Array<DataQualityIssue>;
  model: Maybe<Scalars['String']['output']>;
  overallScore: Scalars['Float']['output'];
  processingTimeMs: Maybe<Scalars['Int']['output']>;
  recommendations: Array<QualityRecommendation>;
};

export type DebugSession = {
  __typename?: 'DebugSession';
  breakpoints: Array<Breakpoint>;
  createdAt: Scalars['DateTime']['output'];
  id: Scalars['ID']['output'];
  state: Scalars['String']['output'];
  steps: Array<WorkflowDebugStep>;
  workflowId: Scalars['String']['output'];
};

export type DocumentEvent = Event & {
  __typename?: 'DocumentEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  documentType: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  patient: Maybe<Patient>;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  title: Maybe<Scalars['String']['output']>;
  type: EventType;
};

export type DryRunResult = {
  __typename?: 'DryRunResult';
  routeResults: Array<DryRunRouteResult>;
  validationErrors: Array<Scalars['String']['output']>;
  warnings: Array<Scalars['String']['output']>;
};

export type DryRunRouteResult = {
  __typename?: 'DryRunRouteResult';
  actionsWouldRun: Scalars['Int']['output'];
  matched: Scalars['Boolean']['output'];
  routeName: Scalars['String']['output'];
  skipReason: Maybe<Scalars['String']['output']>;
};

export type DryRunWorkflowInput = {
  events: Array<Scalars['JSON']['input']>;
  yaml: Scalars['String']['input'];
};

export type Encounter = {
  __typename?: 'Encounter';
  admitDateTime: Maybe<Scalars['DateTime']['output']>;
  attendingProvider: Maybe<Provider>;
  class: Scalars['String']['output'];
  dischargeDateTime: Maybe<Scalars['DateTime']['output']>;
  id: Scalars['ID']['output'];
  location: Maybe<Location>;
  status: Maybe<Scalars['String']['output']>;
};

export type Event = {
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type EventClassificationRule = {
  __typename?: 'EventClassificationRule';
  condition: Maybe<Scalars['String']['output']>;
  eventType: Scalars['String']['output'];
  messageType: Scalars['String']['output'];
  priority: Scalars['Int']['output'];
};

export type EventClassificationRuleInput = {
  condition: InputMaybe<Scalars['String']['input']>;
  eventType: Scalars['String']['input'];
  messageType: Scalars['String']['input'];
  priority: Scalars['Int']['input'];
};

export type EventConnection = {
  __typename?: 'EventConnection';
  edges: Array<EventEdge>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type EventEdge = {
  __typename?: 'EventEdge';
  cursor: Scalars['String']['output'];
  node: Event;
};

export type EventFilter = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  fromTimestamp: InputMaybe<Scalars['DateTime']['input']>;
  patientMrn: InputMaybe<Scalars['String']['input']>;
  sources: InputMaybe<Array<Scalars['String']['input']>>;
  toTimestamp: InputMaybe<Scalars['DateTime']['input']>;
  types: InputMaybe<Array<EventType>>;
};

export type EventOrderBy = {
  direction: OrderDirection;
  field: EventOrderField;
};

export type EventOrderField =
  | 'SOURCE'
  | 'TIMESTAMP'
  | 'TYPE';

export type EventStatistics = {
  __typename?: 'EventStatistics';
  bySource: Array<SourceCount>;
  byType: Array<EventTypeCount>;
  totalEvents: Scalars['Int']['output'];
};

export type EventType =
  | 'ALLERGY_INTOLERANCE'
  | 'APPOINTMENT_CANCELLED'
  | 'APPOINTMENT_CHECKED_IN'
  | 'APPOINTMENT_MODIFIED'
  | 'APPOINTMENT_NOSHOW'
  | 'APPOINTMENT_RESCHEDULED'
  | 'APPOINTMENT_SCHEDULED'
  | 'CLAIM_ADJUDICATED'
  | 'CLAIM_STATUS_REQUEST'
  | 'CLAIM_STATUS_RESPONSE'
  | 'CLAIM_SUBMITTED'
  | 'CONDITION'
  | 'DOCUMENT'
  | 'DOCUMENT_ADDENDUM'
  | 'DOCUMENT_EDIT'
  | 'DOCUMENT_ORIGINAL'
  | 'DOCUMENT_REPLACEMENT'
  | 'DOCUMENT_STATUS_CHANGE'
  | 'ELIGIBILITY_INQUIRY'
  | 'ELIGIBILITY_RESPONSE'
  | 'FINANCIAL_TRANSACTION'
  | 'IMMUNIZATION'
  | 'LAB_CANCELLED'
  | 'LAB_ORDERED'
  | 'LAB_RESULT'
  | 'MEDICATION_REQUEST'
  | 'PATIENT_ADMIT'
  | 'PATIENT_DISCHARGE'
  | 'PATIENT_MERGE'
  | 'PATIENT_TRANSFER'
  | 'PATIENT_UPDATE'
  | 'PRIOR_AUTH_REQUEST'
  | 'PRIOR_AUTH_RESPONSE'
  | 'PROCEDURE'
  | 'SOCIAL_HISTORY'
  | 'VITAL_SIGN';

export type EventTypeCount = {
  __typename?: 'EventTypeCount';
  count: Scalars['Int']['output'];
  eventType: Scalars['String']['output'];
};

export type ExplainWorkflowInput = {
  audience: InputMaybe<Scalars['String']['input']>;
  workflowYaml: Scalars['String']['input'];
};

export type ExplainedWarning = {
  __typename?: 'ExplainedWarning';
  code: Scalars['String']['output'];
  explanation: Scalars['String']['output'];
  fixSuggestion: Maybe<Scalars['String']['output']>;
  fromCache: Scalars['Boolean']['output'];
  impact: Maybe<Scalars['String']['output']>;
};

export type ExportIntegrationBundleInput = {
  /**
   * Requesting raw sample payloads additionally requires the
   * `integration.phi.export` grant. Without it the export is refused.
   */
  includeRawPayload: InputMaybe<Scalars['Boolean']['input']>;
  /**
   * Why this PHI disclosure is being made. Recorded verbatim on the export record
   * alongside the verified caller identity. 1-1024 bytes; an empty reason is
   * refused before any bundle is assembled.
   */
  reason: Scalars['String']['input'];
  sessionId: Scalars['ID']['input'];
};

export type ExtractEntitiesInput = {
  documentType: InputMaybe<Scalars['String']['input']>;
  includeNegated: InputMaybe<Scalars['Boolean']['input']>;
  minConfidence: InputMaybe<Scalars['Float']['input']>;
  patientAge: InputMaybe<Scalars['Int']['input']>;
  patientGender: InputMaybe<Scalars['String']['input']>;
  text: Scalars['String']['input'];
};

export type ExtractedAllergy = {
  __typename?: 'ExtractedAllergy';
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  confidence: Scalars['Float']['output'];
  negated: Maybe<Scalars['Boolean']['output']>;
  reaction: Maybe<Scalars['String']['output']>;
  severity: Maybe<Scalars['String']['output']>;
  substance: Scalars['String']['output'];
  textSpan: Maybe<Scalars['String']['output']>;
};

export type ExtractedCondition = {
  __typename?: 'ExtractedCondition';
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  confidence: Scalars['Float']['output'];
  name: Scalars['String']['output'];
  negated: Maybe<Scalars['Boolean']['output']>;
  status: Maybe<Scalars['String']['output']>;
  textSpan: Maybe<Scalars['String']['output']>;
};

export type ExtractedMedication = {
  __typename?: 'ExtractedMedication';
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  confidence: Scalars['Float']['output'];
  dose: Maybe<Scalars['String']['output']>;
  frequency: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  negated: Maybe<Scalars['Boolean']['output']>;
  route: Maybe<Scalars['String']['output']>;
  textSpan: Maybe<Scalars['String']['output']>;
};

export type ExtractedProcedure = {
  __typename?: 'ExtractedProcedure';
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  confidence: Scalars['Float']['output'];
  name: Scalars['String']['output'];
  negated: Maybe<Scalars['Boolean']['output']>;
  status: Maybe<Scalars['String']['output']>;
  textSpan: Maybe<Scalars['String']['output']>;
};

export type ExtractedVitalSign = {
  __typename?: 'ExtractedVitalSign';
  confidence: Scalars['Float']['output'];
  interpretation: Maybe<Scalars['String']['output']>;
  loincCode: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  textSpan: Maybe<Scalars['String']['output']>;
  unit: Maybe<Scalars['String']['output']>;
  value: Scalars['String']['output'];
};

export type ExtractionResult = {
  __typename?: 'ExtractionResult';
  allergies: Array<ExtractedAllergy>;
  conditions: Array<ExtractedCondition>;
  medications: Array<ExtractedMedication>;
  model: Maybe<Scalars['String']['output']>;
  overallConfidence: Scalars['Float']['output'];
  procedures: Array<ExtractedProcedure>;
  processingTimeMs: Scalars['Int']['output'];
  vitalSigns: Array<ExtractedVitalSign>;
};

export type FhirSubscription = {
  __typename?: 'FhirSubscription';
  createdAt: Scalars['DateTime']['output'];
  criteria: Scalars['String']['output'];
  endpoint: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  server: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type GenerateWorkflowInput = {
  actionTypes: InputMaybe<Array<Scalars['String']['input']>>;
  description: Scalars['String']['input'];
  eventTypes: InputMaybe<Array<Scalars['String']['input']>>;
};

export type GeneratedWorkflow = {
  __typename?: 'GeneratedWorkflow';
  explanation: Scalars['String']['output'];
  warnings: Array<Scalars['String']['output']>;
  yaml: Scalars['String']['output'];
};

export type Hl7v2Config = {
  __typename?: 'HL7v2Config';
  defaultVersion: Scalars['String']['output'];
  eventClassifications: Array<EventClassificationRule>;
  timezone: Scalars['String']['output'];
  tolerance: Maybe<ToleranceConfig>;
};

export type Hl7v2ConfigInput = {
  defaultVersion: InputMaybe<Scalars['String']['input']>;
  eventClassifications: InputMaybe<Array<EventClassificationRuleInput>>;
  timezone: InputMaybe<Scalars['String']['input']>;
  tolerance: InputMaybe<ToleranceConfigInput>;
};

export type HealthStatus = {
  __typename?: 'HealthStatus';
  components: Array<ComponentHealth>;
  status: Scalars['String']['output'];
  uptime: Scalars['Int']['output'];
  version: Scalars['String']['output'];
};

export type IdPreferenceRule = {
  __typename?: 'IDPreferenceRule';
  assignerContains: Maybe<Scalars['String']['output']>;
  priority: Scalars['Int']['output'];
  type: Scalars['String']['output'];
};

export type IdPreferenceRuleInput = {
  assignerContains: InputMaybe<Scalars['String']['input']>;
  priority: Scalars['Int']['input'];
  type: Scalars['String']['input'];
};

export type Identifier = {
  __typename?: 'Identifier';
  assigner: Maybe<Scalars['String']['output']>;
  system: Maybe<Scalars['String']['output']>;
  type: Scalars['String']['output'];
  value: Scalars['String']['output'];
};

export type IdentifierConfig = {
  __typename?: 'IdentifierConfig';
  assigningAuthorities: Array<AssigningAuthority>;
  normalization: Maybe<NormalizationSettingsConfig>;
  primaryIdPreference: Array<IdPreferenceRule>;
  validation: Maybe<ValidationSettingsConfig>;
};

export type IdentifierConfigInput = {
  assigningAuthorities: InputMaybe<Array<AssigningAuthorityInput>>;
  normalization: InputMaybe<NormalizationSettingsInput>;
  primaryIdPreference: InputMaybe<Array<IdPreferenceRuleInput>>;
  validation: InputMaybe<ValidationSettingsInput>;
};

export type Immunization = {
  __typename?: 'Immunization';
  status: Maybe<Scalars['String']['output']>;
  vaccineCode: Maybe<Scalars['String']['output']>;
  vaccineName: Scalars['String']['output'];
};

export type ImmunizationEvent = Event & {
  __typename?: 'ImmunizationEvent';
  administeredDate: Maybe<Scalars['String']['output']>;
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  immunization: Immunization;
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type IntegrationArtifactRevision = {
  __typename?: 'IntegrationArtifactRevision';
  artifactId: Scalars['ID']['output'];
  digest: Scalars['String']['output'];
  revisionId: Scalars['ID']['output'];
};

export type IntegrationBundle = {
  __typename?: 'IntegrationBundle';
  artifacts: Array<SessionArtifact>;
  diagnostics: Array<SessionDiagnostic>;
  exportedAt: Scalars['DateTime']['output'];
  publications: Array<SessionPublication>;
  runs: Array<SessionRun>;
  samples: Array<SessionSample>;
  session: IntegrationSession;
  sessionId: Scalars['ID']['output'];
  workflowSimulations: Array<SessionWorkflowSimulation>;
};

export type IntegrationExecutionArtifactRevisions = {
  __typename?: 'IntegrationExecutionArtifactRevisions';
  profile: IntegrationArtifactRevision;
  source: IntegrationArtifactRevision;
  workflow: IntegrationArtifactRevision;
};

export type IntegrationPreviewCorrelations = {
  __typename?: 'IntegrationPreviewCorrelations';
  correlationId: Scalars['ID']['output'];
  eventIds: Array<Scalars['ID']['output']>;
  sourceMessageId: Maybe<Scalars['String']['output']>;
  tenantId: Scalars['ID']['output'];
  traceId: Maybe<Scalars['ID']['output']>;
  workflowRunId: Maybe<Scalars['ID']['output']>;
};

export type IntegrationPreviewDelivery = {
  __typename?: 'IntegrationPreviewDelivery';
  action: Scalars['String']['output'];
  destination: IntegrationPreviewDestination;
  diagnosticCodes: Array<Scalars['String']['output']>;
  eventId: Scalars['ID']['output'];
  route: Scalars['String']['output'];
  status: Scalars['String']['output'];
  tenantId: Scalars['ID']['output'];
};

export type IntegrationPreviewDestination = {
  __typename?: 'IntegrationPreviewDestination';
  artifactId: Scalars['ID']['output'];
  class: Scalars['String']['output'];
  digest: Scalars['String']['output'];
  revisionId: Scalars['ID']['output'];
};

export type IntegrationPreviewDiagnostic = {
  __typename?: 'IntegrationPreviewDiagnostic';
  classification: Scalars['String']['output'];
  code: Scalars['String']['output'];
  message: Scalars['String']['output'];
  path: Maybe<Scalars['String']['output']>;
  severity: Scalars['String']['output'];
  source: Maybe<Scalars['String']['output']>;
  stage: Scalars['String']['output'];
  tenantId: Scalars['ID']['output'];
};

export type IntegrationPreviewEvent = {
  __typename?: 'IntegrationPreviewEvent';
  classification: Scalars['String']['output'];
  correlationId: Scalars['ID']['output'];
  id: Scalars['ID']['output'];
  payload: Scalars['JSON']['output'];
  sourceMessageId: Maybe<Scalars['String']['output']>;
  tenantId: Scalars['ID']['output'];
  type: Scalars['String']['output'];
};

export type IntegrationPreviewResult = {
  __typename?: 'IntegrationPreviewResult';
  artifactRevisions: IntegrationExecutionArtifactRevisions;
  correlations: IntegrationPreviewCorrelations;
  deliveries: Array<IntegrationPreviewDelivery>;
  diagnostics: Array<IntegrationPreviewDiagnostic>;
  events: Array<IntegrationPreviewEvent>;
  integrationRevision: IntegrationArtifactRevision;
  mode: Scalars['String']['output'];
  routes: Array<IntegrationPreviewRoute>;
  tenantId: Scalars['ID']['output'];
};

export type IntegrationPreviewRoute = {
  __typename?: 'IntegrationPreviewRoute';
  diagnosticCodes: Array<Scalars['String']['output']>;
  eventId: Scalars['ID']['output'];
  matched: Scalars['Boolean']['output'];
  plannedActions: Array<Scalars['String']['output']>;
  route: Scalars['String']['output'];
  skipReason: Maybe<Scalars['String']['output']>;
  skipped: Scalars['Boolean']['output'];
  tenantId: Scalars['ID']['output'];
  transformCount: Scalars['Int']['output'];
};

export type IntegrationSession = {
  __typename?: 'IntegrationSession';
  archived: Scalars['Boolean']['output'];
  artifacts: Array<SessionArtifact>;
  createdAt: Scalars['DateTime']['output'];
  currentProfileDraft: Maybe<SessionArtifact>;
  currentWorkflowDraft: Maybe<SessionArtifact>;
  description: Maybe<Scalars['String']['output']>;
  diagnostics: Array<SessionDiagnostic>;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  publications: Array<SessionPublication>;
  runs: Array<SessionRun>;
  samples: Array<SessionSample>;
  updatedAt: Scalars['DateTime']['output'];
  workflowSimulations: Array<SessionWorkflowSimulation>;
};

export type IntegrationSessionEvent = {
  __typename?: 'IntegrationSessionEvent';
  id: Scalars['ID']['output'];
  message: Scalars['String']['output'];
  run: Maybe<SessionRun>;
  runId: Maybe<Scalars['ID']['output']>;
  session: Maybe<IntegrationSession>;
  sessionId: Scalars['ID']['output'];
  timestamp: Scalars['DateTime']['output'];
  type: Scalars['String']['output'];
};

export type LlmCapability = {
  __typename?: 'LLMCapability';
  configured: Scalars['Boolean']['output'];
  defaultModel: Maybe<Scalars['String']['output']>;
  enabled: Scalars['Boolean']['output'];
  features: Array<LlmFeatureCapability>;
  providerBaseURLHost: Maybe<Scalars['String']['output']>;
  qualityModel: Maybe<Scalars['String']['output']>;
  status: Scalars['String']['output'];
  warnings: Array<Scalars['String']['output']>;
};

export type LlmFeatureCapability = {
  __typename?: 'LLMFeatureCapability';
  enabled: Scalars['Boolean']['output'];
  model: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  reason: Maybe<Scalars['String']['output']>;
  status: Scalars['String']['output'];
};

export type LabResult = {
  __typename?: 'LabResult';
  interpretation: Maybe<Scalars['String']['output']>;
  referenceRange: Maybe<Scalars['String']['output']>;
  status: Maybe<Scalars['String']['output']>;
  unit: Maybe<Scalars['String']['output']>;
  value: Scalars['String']['output'];
};

export type LabResultEvent = Event & {
  __typename?: 'LabResultEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  isCritical: Scalars['Boolean']['output'];
  orderingProvider: Maybe<Provider>;
  patient: Patient;
  result: LabResult;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  test: LabTest;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type LabTest = {
  __typename?: 'LabTest';
  category: Maybe<Scalars['String']['output']>;
  description: Scalars['String']['output'];
  localCode: Maybe<Scalars['String']['output']>;
  loincCode: Maybe<Scalars['String']['output']>;
};

export type LineageLink = {
  __typename?: 'LineageLink';
  description: Maybe<Scalars['String']['output']>;
  sourcePath: Scalars['String']['output'];
  targetPath: Maybe<Scalars['String']['output']>;
};

export type ListMappingsInput = {
  createdAfter: InputMaybe<Scalars['DateTime']['input']>;
  createdBefore: InputMaybe<Scalars['DateTime']['input']>;
  equivalence: InputMaybe<MappingEquivalence>;
  first: InputMaybe<Scalars['Int']['input']>;
  offset: InputMaybe<Scalars['Int']['input']>;
  origin: InputMaybe<MappingOrigin>;
  profileId: InputMaybe<Scalars['String']['input']>;
  sourceSystem: InputMaybe<Scalars['String']['input']>;
  targetSystem: InputMaybe<Scalars['String']['input']>;
  uploadBatchId: InputMaybe<Scalars['ID']['input']>;
};

export type ListPendingAutoroutesInput = {
  first: InputMaybe<Scalars['Int']['input']>;
  minConfidence: InputMaybe<Scalars['Float']['input']>;
  offset: InputMaybe<Scalars['Int']['input']>;
  sourceSystem: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<PendingAutorouteStatus>;
  targetSystem: InputMaybe<Scalars['String']['input']>;
};

export type LiveParseInput = {
  format: SourceFormat;
  message: Scalars['String']['input'];
};

export type Location = {
  __typename?: 'Location';
  bed: Maybe<Scalars['String']['output']>;
  facility: Maybe<Scalars['String']['output']>;
  room: Maybe<Scalars['String']['output']>;
  unit: Maybe<Scalars['String']['output']>;
};

export type MappingCandidate = {
  __typename?: 'MappingCandidate';
  code: Scalars['String']['output'];
  confidence: Scalars['Float']['output'];
  display: Scalars['String']['output'];
  equivalence: Maybe<MappingEquivalence>;
  reasoning: Maybe<Scalars['String']['output']>;
  score: Maybe<Scalars['Float']['output']>;
  system: Scalars['String']['output'];
};

export type MappingEquivalence =
  | 'EQUIVALENT'
  | 'INEXACT'
  | 'NARROWER'
  | 'WIDER';

export type MappingOrigin =
  | 'APPROVED_AUTOROUTE'
  | 'CSV_UPLOAD'
  | 'MANUAL';

export type MessageClassification = {
  __typename?: 'MessageClassification';
  confidence: Scalars['Float']['output'];
  eventType: Maybe<EventType>;
  messageType: Scalars['String']['output'];
  suggestedTags: Array<Scalars['String']['output']>;
  summary: Maybe<Scalars['String']['output']>;
};

export type Mutation = {
  __typename?: 'Mutation';
  acceptDiagnosticFix: SessionDiagnostic;
  addSessionSample: SessionSample;
  approvePendingAutoroute: CodeMapping;
  approveSessionPublication: SessionDeploymentSnapshot;
  approveWorkflowVersion: WorkflowApprovalRequest;
  archiveIntegrationSession: IntegrationSession;
  archiveWorkflowDefinition: WorkflowDefinition;
  bulkApprovePendingAutoroutes: BulkApproveResult;
  cancelTemporalWorkflow: Scalars['Boolean']['output'];
  createFhirSubscription: FhirSubscription;
  createIntegrationSession: IntegrationSession;
  createMapping: CodeMapping;
  createProfile: SourceProfile;
  createWorkflowDefinition: WorkflowDefinition;
  debugContinue: Maybe<WorkflowDebugStep>;
  debugEndSession: Scalars['Boolean']['output'];
  debugRemoveBreakpoint: Scalars['Boolean']['output'];
  debugSetBreakpoint: Breakpoint;
  debugStep: Maybe<WorkflowDebugStep>;
  deleteFhirSubscription: Scalars['Boolean']['output'];
  deleteMapping: Scalars['Boolean']['output'];
  deleteMappingBatch: Scalars['Int']['output'];
  deleteProfile: Scalars['Boolean']['output'];
  deployIntegrationRelease: OperatorDeployment;
  deploySessionPublication: SessionDeploymentSnapshot;
  discardDeadLetter: OperatorControlResult;
  dryRunWorkflow: DryRunResult;
  duplicateProfile: SourceProfile;
  exportIntegrationBundle: IntegrationBundle;
  generateWorkflow: GeneratedWorkflow;
  pauseFhirSubscription: FhirSubscription;
  pauseIntegrationDeployment: OperatorDeployment;
  previewIntegrationMessage: IntegrationPreviewResult;
  publishIntegrationSession: SessionPublication;
  publishWorkflowVersion: WorkflowRelease;
  rejectPendingAutoroute: Scalars['Boolean']['output'];
  rejectWorkflowVersion: WorkflowApprovalRequest;
  replayDelivery: OperatorControlResult;
  requestWorkflowApproval: WorkflowApprovalRequest;
  resubmitMessage: OperatorControlResult;
  resumeFhirSubscription: FhirSubscription;
  resumeIntegrationDeployment: OperatorDeployment;
  retireIntegrationDeployment: OperatorDeployment;
  rollbackWorkflowVersion: WorkflowRelease;
  runSessionPreview: SessionRun;
  saveWorkflowVersion: WorkflowVersion;
  signalReviewDecision: Scalars['Boolean']['output'];
  simulateSessionWorkflow: SessionWorkflowSimulation;
  startDebugSession: DebugSession;
  startTerminologyReview: StartTerminologyReviewResult;
  submitBatch: BatchResult;
  submitEvent: SubmitResult;
  submitMessage: SubmitResult;
  triggerWorkflow: WorkflowResult;
  updateMapping: CodeMapping;
  updateProfile: SourceProfile;
  updateSessionProfileDraft: SessionArtifact;
  updateSessionWorkflowDraft: SessionArtifact;
  updateWorkflowDefinition: WorkflowDefinition;
  uploadMappingCSV: UploadMappingResult;
};


export type MutationAcceptDiagnosticFixArgs = {
  input: AcceptDiagnosticFixInput;
};


export type MutationAddSessionSampleArgs = {
  input: AddSessionSampleInput;
};


export type MutationApprovePendingAutorouteArgs = {
  input: ApprovePendingAutorouteInput;
};


export type MutationApproveSessionPublicationArgs = {
  input: PromoteSessionPublicationInput;
};


export type MutationApproveWorkflowVersionArgs = {
  input: ApproveWorkflowVersionInput;
};


export type MutationArchiveIntegrationSessionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationArchiveWorkflowDefinitionArgs = {
  input: ArchiveWorkflowDefinitionInput;
};


export type MutationBulkApprovePendingAutoroutesArgs = {
  input: InputMaybe<BulkApproveInput>;
};


export type MutationCancelTemporalWorkflowArgs = {
  reason: InputMaybe<Scalars['String']['input']>;
  workflowId: Scalars['String']['input'];
};


export type MutationCreateFhirSubscriptionArgs = {
  input: CreateSubscriptionInput;
};


export type MutationCreateIntegrationSessionArgs = {
  input: CreateIntegrationSessionInput;
};


export type MutationCreateMappingArgs = {
  input: CreateMappingInput;
};


export type MutationCreateProfileArgs = {
  input: CreateProfileInput;
};


export type MutationCreateWorkflowDefinitionArgs = {
  input: CreateWorkflowDefinitionInput;
};


export type MutationDebugContinueArgs = {
  sessionId: Scalars['ID']['input'];
};


export type MutationDebugEndSessionArgs = {
  sessionId: Scalars['ID']['input'];
};


export type MutationDebugRemoveBreakpointArgs = {
  breakpointId: Scalars['ID']['input'];
  sessionId: Scalars['ID']['input'];
};


export type MutationDebugSetBreakpointArgs = {
  input: SetBreakpointInput;
};


export type MutationDebugStepArgs = {
  sessionId: Scalars['ID']['input'];
};


export type MutationDeleteFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteMappingArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeleteMappingBatchArgs = {
  batchId: Scalars['ID']['input'];
};


export type MutationDeleteProfileArgs = {
  id: Scalars['ID']['input'];
};


export type MutationDeployIntegrationReleaseArgs = {
  input: OperatorDeploymentCommandInput;
};


export type MutationDeploySessionPublicationArgs = {
  input: PromoteSessionPublicationInput;
};


export type MutationDiscardDeadLetterArgs = {
  input: OperatorDeliveryControlInput;
};


export type MutationDryRunWorkflowArgs = {
  input: DryRunWorkflowInput;
};


export type MutationDuplicateProfileArgs = {
  id: Scalars['ID']['input'];
  newId: Scalars['ID']['input'];
  newName: Scalars['String']['input'];
};


export type MutationExportIntegrationBundleArgs = {
  input: ExportIntegrationBundleInput;
};


export type MutationGenerateWorkflowArgs = {
  input: GenerateWorkflowInput;
};


export type MutationPauseFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPauseIntegrationDeploymentArgs = {
  input: OperatorDeploymentCommandInput;
};


export type MutationPreviewIntegrationMessageArgs = {
  input: PreviewIntegrationMessageInput;
};


export type MutationPublishIntegrationSessionArgs = {
  input: PublishIntegrationSessionInput;
};


export type MutationPublishWorkflowVersionArgs = {
  input: PublishWorkflowVersionInput;
};


export type MutationRejectPendingAutorouteArgs = {
  input: RejectPendingAutorouteInput;
};


export type MutationRejectWorkflowVersionArgs = {
  input: RejectWorkflowVersionInput;
};


export type MutationReplayDeliveryArgs = {
  input: OperatorDeliveryControlInput;
};


export type MutationRequestWorkflowApprovalArgs = {
  input: RequestWorkflowApprovalInput;
};


export type MutationResubmitMessageArgs = {
  input: OperatorDeliveryControlInput;
};


export type MutationResumeFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationResumeIntegrationDeploymentArgs = {
  input: OperatorDeploymentCommandInput;
};


export type MutationRetireIntegrationDeploymentArgs = {
  input: OperatorDeploymentCommandInput;
};


export type MutationRollbackWorkflowVersionArgs = {
  input: RollbackWorkflowVersionInput;
};


export type MutationRunSessionPreviewArgs = {
  input: RunSessionPreviewInput;
};


export type MutationSaveWorkflowVersionArgs = {
  input: SaveWorkflowVersionInput;
};


export type MutationSignalReviewDecisionArgs = {
  input: SignalReviewDecisionInput;
};


export type MutationSimulateSessionWorkflowArgs = {
  input: SimulateSessionWorkflowInput;
};


export type MutationStartDebugSessionArgs = {
  input: StartDebugSessionInput;
};


export type MutationStartTerminologyReviewArgs = {
  input: StartTerminologyReviewInput;
};


export type MutationSubmitBatchArgs = {
  input: SubmitBatchInput;
};


export type MutationSubmitEventArgs = {
  input: SubmitEventInput;
};


export type MutationSubmitMessageArgs = {
  input: SubmitMessageInput;
};


export type MutationTriggerWorkflowArgs = {
  environment: InputMaybe<Scalars['String']['input']>;
  event: Scalars['JSON']['input'];
  name: Scalars['String']['input'];
  versionId: InputMaybe<Scalars['String']['input']>;
};


export type MutationUpdateMappingArgs = {
  input: UpdateMappingInput;
};


export type MutationUpdateProfileArgs = {
  id: Scalars['ID']['input'];
  input: UpdateProfileInput;
};


export type MutationUpdateSessionProfileDraftArgs = {
  input: UpdateSessionArtifactInput;
};


export type MutationUpdateSessionWorkflowDraftArgs = {
  input: UpdateSessionArtifactInput;
};


export type MutationUpdateWorkflowDefinitionArgs = {
  input: UpdateWorkflowDefinitionInput;
};


export type MutationUploadMappingCsvArgs = {
  input: UploadMappingCsvInput;
};

export type NormalizationSettingsConfig = {
  __typename?: 'NormalizationSettingsConfig';
  phoneFormat: Maybe<Scalars['String']['output']>;
  phoneNormalize: Scalars['Boolean']['output'];
  ssnRejectPatterns: Array<Scalars['String']['output']>;
  ssnStripDashes: Scalars['Boolean']['output'];
};

export type NormalizationSettingsInput = {
  phoneFormat: InputMaybe<Scalars['String']['input']>;
  phoneNormalize: InputMaybe<Scalars['Boolean']['input']>;
  ssnRejectPatterns: InputMaybe<Array<Scalars['String']['input']>>;
  ssnStripDashes: InputMaybe<Scalars['Boolean']['input']>;
};

export type OperatorAttemptFilter = {
  destinationArtifactId: InputMaybe<Scalars['ID']['input']>;
  from: InputMaybe<Scalars['DateTime']['input']>;
  receiptId: InputMaybe<Scalars['ID']['input']>;
  route: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
  to: InputMaybe<Scalars['DateTime']['input']>;
};

export type OperatorAuditConnection = {
  __typename?: 'OperatorAuditConnection';
  nodes: Array<OperatorAuditRecord>;
  pageInfo: OperatorPageInfo;
};

export type OperatorAuditRecord = {
  __typename?: 'OperatorAuditRecord';
  attemptCount: Scalars['Int']['output'];
  attemptId: Scalars['ID']['output'];
  auditId: Scalars['ID']['output'];
  detail: Scalars['JSON']['output'];
  eventKind: Scalars['String']['output'];
  principal: OperatorPrincipal;
  reason: Scalars['String']['output'];
  recordedAt: Scalars['DateTime']['output'];
};

export type OperatorCircuit = {
  __typename?: 'OperatorCircuit';
  consecutiveFailures: Scalars['Int']['output'];
  destination: IntegrationArtifactRevision;
  openUntil: Maybe<Scalars['DateTime']['output']>;
  state: Scalars['String']['output'];
  updatedAt: Scalars['DateTime']['output'];
};

export type OperatorControlResult = {
  __typename?: 'OperatorControlResult';
  actor: OperatorPrincipal;
  attempt: OperatorDeliveryAttempt;
  idempotencyKey: Scalars['String']['output'];
  kind: Scalars['String']['output'];
  reason: Scalars['String']['output'];
  resultAttemptId: Scalars['ID']['output'];
  sourceAttemptId: Scalars['ID']['output'];
};

export type OperatorDeadLetter = {
  __typename?: 'OperatorDeadLetter';
  active: Scalars['Boolean']['output'];
  attemptId: Scalars['ID']['output'];
  failedAt: Scalars['DateTime']['output'];
  failureCode: Scalars['String']['output'];
  failureDetail: Scalars['String']['output'];
  lastReplayedAt: Maybe<Scalars['DateTime']['output']>;
  replayCount: Scalars['Int']['output'];
  resolution: Scalars['String']['output'];
  resolvedAt: Maybe<Scalars['DateTime']['output']>;
};

export type OperatorDeadLetterConnection = {
  __typename?: 'OperatorDeadLetterConnection';
  nodes: Array<OperatorDeadLetter>;
  pageInfo: OperatorPageInfo;
};

export type OperatorDeliveryAttempt = {
  __typename?: 'OperatorDeliveryAttempt';
  action: Scalars['String']['output'];
  attemptCount: Scalars['Int']['output'];
  attemptId: Scalars['ID']['output'];
  completedAt: Maybe<Scalars['DateTime']['output']>;
  deadLetter: Maybe<OperatorDeadLetter>;
  destination: IntegrationPreviewDestination;
  eventId: Scalars['ID']['output'];
  lastErrorCode: Scalars['String']['output'];
  lastErrorDetail: Scalars['String']['output'];
  leaseExpiresAt: Maybe<Scalars['DateTime']['output']>;
  leaseOwner: Scalars['String']['output'];
  outboxStatus: Scalars['String']['output'];
  parentAttemptId: Maybe<Scalars['ID']['output']>;
  receiptId: Scalars['ID']['output'];
  recordedAt: Scalars['DateTime']['output'];
  route: Scalars['String']['output'];
  scheduledAt: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
  tenantId: Scalars['ID']['output'];
  topic: Scalars['String']['output'];
  traceId: Scalars['String']['output'];
};

export type OperatorDeliveryAttemptConnection = {
  __typename?: 'OperatorDeliveryAttemptConnection';
  nodes: Array<OperatorDeliveryAttempt>;
  pageInfo: OperatorPageInfo;
};

export type OperatorDeliveryControlInput = {
  attemptId: Scalars['ID']['input'];
  /** Caller-owned key that makes a repeated control action a no-op. */
  idempotencyKey: Scalars['String']['input'];
  /** Required operator justification recorded in the append-only audit trail. */
  reason: Scalars['String']['input'];
};

export type OperatorDeployment = {
  __typename?: 'OperatorDeployment';
  definitionRevision: IntegrationArtifactRevision;
  health: Scalars['String']['output'];
  releaseId: Maybe<Scalars['ID']['output']>;
  state: Scalars['String']['output'];
  updatedAt: Scalars['DateTime']['output'];
  updatedBy: OperatorPrincipal;
  updatedReason: Scalars['String']['output'];
  validationExpiresAt: Maybe<Scalars['DateTime']['output']>;
  validationPassed: Scalars['Boolean']['output'];
  version: Scalars['Int']['output'];
};

export type OperatorDeploymentCommandInput = {
  definitionId: Scalars['ID']['input'];
  /** Optimistic concurrency guard; a stale version is rejected, never retried. */
  expectedVersion: Scalars['Int']['input'];
  reason: Scalars['String']['input'];
  revisionId: Scalars['ID']['input'];
};

export type OperatorDeploymentEvent = {
  __typename?: 'OperatorDeploymentEvent';
  action: Scalars['String']['output'];
  actor: OperatorPrincipal;
  eventId: Scalars['ID']['output'];
  fromState: Scalars['String']['output'];
  health: Scalars['String']['output'];
  occurredAt: Scalars['DateTime']['output'];
  reason: Scalars['String']['output'];
  releaseId: Maybe<Scalars['ID']['output']>;
  toState: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type OperatorDiagnostic = {
  __typename?: 'OperatorDiagnostic';
  classification: Scalars['String']['output'];
  code: Scalars['String']['output'];
  path: Maybe<Scalars['String']['output']>;
  severity: Scalars['String']['output'];
  stage: Scalars['String']['output'];
};

export type OperatorEvent = {
  __typename?: 'OperatorEvent';
  classification: Scalars['String']['output'];
  correlationId: Scalars['String']['output'];
  eventId: Scalars['ID']['output'];
  eventType: Scalars['String']['output'];
  payloadFields: Array<OperatorPayloadField>;
  payloadTruncated: Scalars['Boolean']['output'];
  receiptId: Scalars['ID']['output'];
  recordedAt: Scalars['DateTime']['output'];
  sourceMessageId: Scalars['String']['output'];
};

export type OperatorLineage = {
  __typename?: 'OperatorLineage';
  artifactRevisions: IntegrationExecutionArtifactRevisions;
  correlationId: Scalars['String']['output'];
  diagnostics: Array<OperatorDiagnostic>;
  eventId: Scalars['ID']['output'];
  lineageId: Scalars['ID']['output'];
  receiptId: Scalars['ID']['output'];
  recordedAt: Scalars['DateTime']['output'];
  routes: Array<OperatorRoute>;
  sourceMessageId: Scalars['String']['output'];
  traceId: Scalars['String']['output'];
};

export type OperatorMessageTrace = {
  __typename?: 'OperatorMessageTrace';
  attempts: Array<OperatorDeliveryAttempt>;
  audit: Array<OperatorAuditRecord>;
  events: Array<OperatorEvent>;
  lineage: Array<OperatorLineage>;
  receipt: OperatorReceipt;
};

export type OperatorPageInfo = {
  __typename?: 'OperatorPageInfo';
  endCursor: Maybe<Scalars['String']['output']>;
  hasNextPage: Scalars['Boolean']['output'];
};

export type OperatorPageInput = {
  /** Opaque forward cursor returned by a previous page. */
  after: InputMaybe<Scalars['String']['input']>;
  /** Bounded page size. Omitted uses 25; the server caps every page at 100. */
  first: InputMaybe<Scalars['Int']['input']>;
};

/** One structural coordinate of a canonical event payload. Never a value. */
export type OperatorPayloadField = {
  __typename?: 'OperatorPayloadField';
  kind: Scalars['String']['output'];
  path: Scalars['String']['output'];
  repeated: Scalars['Boolean']['output'];
};

export type OperatorPrincipal = {
  __typename?: 'OperatorPrincipal';
  authMethod: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  kind: Scalars['String']['output'];
  roles: Array<Scalars['String']['output']>;
};

export type OperatorReceipt = {
  __typename?: 'OperatorReceipt';
  attemptCount: Scalars['Int']['output'];
  correlationId: Scalars['String']['output'];
  deadLetterCount: Scalars['Int']['output'];
  eventCount: Scalars['Int']['output'];
  failedAttemptCount: Scalars['Int']['output'];
  integrationRevision: IntegrationArtifactRevision;
  principal: OperatorPrincipal;
  rawRetentionMode: Scalars['String']['output'];
  reason: Scalars['String']['output'];
  receiptId: Scalars['ID']['output'];
  recordedAt: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
  tenantId: Scalars['ID']['output'];
};

export type OperatorReceiptConnection = {
  __typename?: 'OperatorReceiptConnection';
  nodes: Array<OperatorReceipt>;
  pageInfo: OperatorPageInfo;
};

export type OperatorReceiptFilter = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  from: InputMaybe<Scalars['DateTime']['input']>;
  integrationArtifactId: InputMaybe<Scalars['ID']['input']>;
  sourceMessageId: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
  to: InputMaybe<Scalars['DateTime']['input']>;
};

export type OperatorRoute = {
  __typename?: 'OperatorRoute';
  diagnosticCodes: Array<Scalars['String']['output']>;
  matched: Scalars['Boolean']['output'];
  plannedActions: Array<Scalars['String']['output']>;
  route: Scalars['String']['output'];
  skipReason: Maybe<Scalars['String']['output']>;
  skipped: Scalars['Boolean']['output'];
  transformCount: Scalars['Int']['output'];
};

export type OrderDirection =
  | 'ASC'
  | 'DESC';

export type PageInfo = {
  __typename?: 'PageInfo';
  endCursor: Maybe<Scalars['String']['output']>;
  hasNextPage: Scalars['Boolean']['output'];
  hasPreviousPage: Scalars['Boolean']['output'];
  startCursor: Maybe<Scalars['String']['output']>;
};

export type PagingInput = {
  limit: InputMaybe<Scalars['Int']['input']>;
  offset: InputMaybe<Scalars['Int']['input']>;
};

export type ParseEvent = {
  __typename?: 'ParseEvent';
  fields: Maybe<Scalars['JSON']['output']>;
  isComplete: Scalars['Boolean']['output'];
  rawSegment: Scalars['String']['output'];
  segmentIndex: Scalars['Int']['output'];
  segmentType: Scalars['String']['output'];
  warnings: Array<Scalars['String']['output']>;
};

export type ParseResult = {
  __typename?: 'ParseResult';
  errors: Array<Scalars['String']['output']>;
  events: Array<Event>;
  success: Scalars['Boolean']['output'];
  warnings: Array<ParseWarning>;
};

export type ParseWarning = {
  __typename?: 'ParseWarning';
  code: Scalars['String']['output'];
  explanation: Maybe<Scalars['String']['output']>;
  fixSuggestion: Maybe<Scalars['String']['output']>;
  fromCache: Maybe<Scalars['Boolean']['output']>;
  impact: Maybe<Scalars['String']['output']>;
  message: Scalars['String']['output'];
  path: Maybe<Scalars['String']['output']>;
  phase: Scalars['String']['output'];
  severity: Maybe<Scalars['String']['output']>;
};

export type ParseWarningInput = {
  code: Scalars['String']['input'];
  message: Scalars['String']['input'];
  path: InputMaybe<Scalars['String']['input']>;
  phase: Scalars['String']['input'];
  severity: InputMaybe<Scalars['String']['input']>;
};

export type Patient = {
  __typename?: 'Patient';
  address: Maybe<Address>;
  dateOfBirth: Maybe<Scalars['DateTime']['output']>;
  email: Maybe<Scalars['String']['output']>;
  familyName: Scalars['String']['output'];
  gender: Maybe<Scalars['String']['output']>;
  givenName: Scalars['String']['output'];
  identifiers: Array<Identifier>;
  middleName: Maybe<Scalars['String']['output']>;
  mrn: Scalars['ID']['output'];
  phone: Maybe<Scalars['String']['output']>;
};

export type PatientAdmitEvent = Event & {
  __typename?: 'PatientAdmitEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  encounter: Encounter;
  id: Scalars['ID']['output'];
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type PatientConnection = {
  __typename?: 'PatientConnection';
  edges: Array<PatientEdge>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type PatientDischargeEvent = Event & {
  __typename?: 'PatientDischargeEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  encounter: Encounter;
  id: Scalars['ID']['output'];
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type PatientEdge = {
  __typename?: 'PatientEdge';
  cursor: Scalars['String']['output'];
  node: Patient;
};

export type PatientFilter = {
  dateOfBirth: InputMaybe<Scalars['DateTime']['input']>;
  familyName: InputMaybe<Scalars['String']['input']>;
  givenName: InputMaybe<Scalars['String']['input']>;
  mrn: InputMaybe<Scalars['String']['input']>;
};

export type PatientTimeline = {
  __typename?: 'PatientTimeline';
  eventCount: Scalars['Int']['output'];
  events: Array<TimelineEvent>;
  lastUpdated: Scalars['DateTime']['output'];
  mrn: Scalars['ID']['output'];
};

export type PendingAutoroute = {
  __typename?: 'PendingAutoroute';
  alternates: Array<MappingCandidate>;
  confidence: Scalars['Float']['output'];
  createdAt: Scalars['DateTime']['output'];
  decisionTrace: Maybe<AutorouteTrace>;
  equivalence: Maybe<MappingEquivalence>;
  expiresAt: Maybe<Scalars['DateTime']['output']>;
  id: Scalars['ID']['output'];
  reasoning: Maybe<Scalars['String']['output']>;
  rejectionReason: Maybe<Scalars['String']['output']>;
  reviewedAt: Maybe<Scalars['DateTime']['output']>;
  reviewedBy: Maybe<Scalars['String']['output']>;
  sourceCode: Scalars['String']['output'];
  sourceDisplay: Maybe<Scalars['String']['output']>;
  sourceSystem: Scalars['String']['output'];
  status: PendingAutorouteStatus;
  suggestedCode: Scalars['String']['output'];
  suggestedDisplay: Maybe<Scalars['String']['output']>;
  targetSystem: Scalars['String']['output'];
};

export type PendingAutorouteConnection = {
  __typename?: 'PendingAutorouteConnection';
  nodes: Array<PendingAutoroute>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type PendingAutorouteStats = {
  __typename?: 'PendingAutorouteStats';
  approvedCount: Scalars['Int']['output'];
  avgConfidence: Maybe<Scalars['Float']['output']>;
  expiredCount: Scalars['Int']['output'];
  pendingCount: Scalars['Int']['output'];
  rejectedCount: Scalars['Int']['output'];
};

export type PendingAutorouteStatus =
  | 'APPROVED'
  | 'EXPIRED'
  | 'PENDING'
  | 'REJECTED';

export type PreviewIntegrationMessageInput = {
  correlationId: Scalars['ID']['input'];
  data: Scalars['String']['input'];
  integrationId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
};

export type Procedure = {
  __typename?: 'Procedure';
  code: Maybe<Scalars['String']['output']>;
  codeSystem: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  status: Maybe<Scalars['String']['output']>;
};

export type ProcedureEvent = Event & {
  __typename?: 'ProcedureEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  patient: Patient;
  performedDate: Maybe<Scalars['String']['output']>;
  procedure: Procedure;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
};

export type ProfileRevision = {
  __typename?: 'ProfileRevision';
  changeSummary: Maybe<Scalars['String']['output']>;
  createdAt: Scalars['DateTime']['output'];
  createdBy: Maybe<Scalars['String']['output']>;
  version: Scalars['String']['output'];
};

export type ProjectionStatus = {
  __typename?: 'ProjectionStatus';
  behind: Scalars['Int']['output'];
  checkpoint: Scalars['Int']['output'];
  lastPosition: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  status: Scalars['String']['output'];
};

export type PromoteSessionPublicationInput = {
  expectedVersion: Scalars['Int']['input'];
  publicationId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
  sessionId: Scalars['ID']['input'];
};

export type Provider = {
  __typename?: 'Provider';
  familyName: Scalars['String']['output'];
  givenName: Scalars['String']['output'];
  id: Maybe<Scalars['String']['output']>;
  npi: Maybe<Scalars['String']['output']>;
  organizationName: Maybe<Scalars['String']['output']>;
  specialty: Maybe<Scalars['String']['output']>;
};

export type PublishIntegrationSessionInput = {
  definitionId: Scalars['ID']['input'];
  definitionRevisionId: Scalars['ID']['input'];
  profileRevisionId: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
  sessionId: Scalars['ID']['input'];
  workflowSimulationId: Scalars['ID']['input'];
};

export type PublishWorkflowVersionInput = {
  environment: Scalars['String']['input'];
  publishedBy: InputMaybe<Scalars['String']['input']>;
  versionId: Scalars['ID']['input'];
  workflowId: Scalars['ID']['input'];
};

export type QualityDimensions = {
  __typename?: 'QualityDimensions';
  accuracy: Scalars['Float']['output'];
  completeness: Scalars['Float']['output'];
  conformance: Scalars['Float']['output'];
  consistency: Scalars['Float']['output'];
  timeliness: Scalars['Float']['output'];
};

export type QualityRecommendation = {
  __typename?: 'QualityRecommendation';
  category: Maybe<Scalars['String']['output']>;
  description: Scalars['String']['output'];
  impact: Maybe<Scalars['String']['output']>;
  priority: Scalars['Int']['output'];
  title: Scalars['String']['output'];
};

export type Query = {
  __typename?: 'Query';
  activeEncounter: Maybe<ActiveEncounter>;
  activeEncounterByPatient: Maybe<ActiveEncounter>;
  activeEncounters: Array<ActiveEncounter>;
  analyzeQuality: DataQualityScore;
  classifyMessage: MessageClassification;
  debugSession: Maybe<DebugSession>;
  event: Maybe<Event>;
  eventStatistics: EventStatistics;
  events: EventConnection;
  explainWarnings: Array<ExplainedWarning>;
  explainWorkflow: WorkflowExplanation;
  exportMappingsCSV: Scalars['String']['output'];
  extractEntities: ExtractionResult;
  getMapping: Maybe<CodeMapping>;
  getPendingAutoroute: Maybe<PendingAutoroute>;
  getUploadBatch: Maybe<UploadBatch>;
  health: HealthStatus;
  integrationSession: Maybe<IntegrationSession>;
  integrationSessions: Array<IntegrationSession>;
  listMappings: CodeMappingConnection;
  listPendingAutoroutes: PendingAutorouteConnection;
  llmCapability: LlmCapability;
  lookupMapping: Maybe<CodeMapping>;
  operatorAttemptAudit: OperatorAuditConnection;
  operatorCircuits: Array<OperatorCircuit>;
  operatorDeadLetters: OperatorDeadLetterConnection;
  operatorDeliveryAttempt: Maybe<OperatorDeliveryAttempt>;
  operatorDeliveryAttempts: OperatorDeliveryAttemptConnection;
  operatorDeploymentEvents: Array<OperatorDeploymentEvent>;
  operatorDeployments: Array<OperatorDeployment>;
  operatorMessageTrace: Maybe<OperatorMessageTrace>;
  operatorReceipts: OperatorReceiptConnection;
  parsePreview: ParseResult;
  parsePreviewWithProfile: ParseResult;
  patient: Maybe<Patient>;
  patientTimeline: Maybe<PatientTimeline>;
  patients: PatientConnection;
  pendingAutorouteStats: PendingAutorouteStats;
  profile: Maybe<SourceProfile>;
  profileRevisions: Array<ProfileRevision>;
  profiles: Array<SourceProfile>;
  projectionStatus: Array<ProjectionStatus>;
  quickQualityScore: DataQualityScore;
  resolveMapping: ResolveMappingResult;
  sessionArtifacts: Array<SessionArtifact>;
  sessionDiagnostics: Array<SessionDiagnostic>;
  sessionPublications: Array<SessionPublication>;
  sessionRun: Maybe<SessionRun>;
  sessionRuns: Array<SessionRun>;
  sessionSamples: Array<SessionSample>;
  sessionWorkflowSimulations: Array<SessionWorkflowSimulation>;
  suggestMappings: Array<MappingCandidate>;
  temporalWorkflow: Maybe<TemporalWorkflow>;
  temporalWorkflows: TemporalWorkflowConnection;
  workflow: Maybe<WorkflowStatus>;
  workflowApprovalRequests: Array<WorkflowApprovalRequest>;
  workflowDefinition: Maybe<WorkflowDefinition>;
  workflowDefinitions: Array<WorkflowDefinition>;
  workflowRun: Maybe<WorkflowRun>;
  workflowRunTrace: Array<TraceSpan>;
  workflowRuns: Array<WorkflowRun>;
  workflowVersion: Maybe<WorkflowVersion>;
  workflowVersions: Array<WorkflowVersion>;
  workflows: Array<WorkflowStatus>;
};


export type QueryActiveEncounterArgs = {
  id: Scalars['ID']['input'];
};


export type QueryActiveEncounterByPatientArgs = {
  mrn: Scalars['ID']['input'];
};


export type QueryActiveEncountersArgs = {
  class: InputMaybe<Scalars['String']['input']>;
  location: InputMaybe<Scalars['String']['input']>;
  unit: InputMaybe<Scalars['String']['input']>;
};


export type QueryAnalyzeQualityArgs = {
  input: AnalyzeQualityInput;
};


export type QueryClassifyMessageArgs = {
  input: ClassifyMessageInput;
};


export type QueryDebugSessionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEventArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEventsArgs = {
  after: InputMaybe<Scalars['String']['input']>;
  filter: InputMaybe<EventFilter>;
  first?: InputMaybe<Scalars['Int']['input']>;
  orderBy: InputMaybe<EventOrderBy>;
};


export type QueryExplainWarningsArgs = {
  format: SourceFormat;
  warnings: Array<ParseWarningInput>;
};


export type QueryExplainWorkflowArgs = {
  input: ExplainWorkflowInput;
};


export type QueryExportMappingsCsvArgs = {
  input: InputMaybe<ListMappingsInput>;
};


export type QueryExtractEntitiesArgs = {
  input: ExtractEntitiesInput;
};


export type QueryGetMappingArgs = {
  id: Scalars['ID']['input'];
};


export type QueryGetPendingAutorouteArgs = {
  id: Scalars['ID']['input'];
};


export type QueryGetUploadBatchArgs = {
  id: Scalars['ID']['input'];
};


export type QueryIntegrationSessionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryIntegrationSessionsArgs = {
  includeArchived: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryListMappingsArgs = {
  input: InputMaybe<ListMappingsInput>;
};


export type QueryListPendingAutoroutesArgs = {
  input: InputMaybe<ListPendingAutoroutesInput>;
};


export type QueryLookupMappingArgs = {
  profileId: InputMaybe<Scalars['String']['input']>;
  sourceCode: Scalars['String']['input'];
  sourceSystem: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
};


export type QueryOperatorAttemptAuditArgs = {
  attemptId: Scalars['ID']['input'];
  page: InputMaybe<OperatorPageInput>;
};


export type QueryOperatorDeadLettersArgs = {
  activeOnly: InputMaybe<Scalars['Boolean']['input']>;
  page: InputMaybe<OperatorPageInput>;
};


export type QueryOperatorDeliveryAttemptArgs = {
  attemptId: Scalars['ID']['input'];
};


export type QueryOperatorDeliveryAttemptsArgs = {
  filter: InputMaybe<OperatorAttemptFilter>;
  page: InputMaybe<OperatorPageInput>;
};


export type QueryOperatorDeploymentEventsArgs = {
  definitionId: Scalars['ID']['input'];
  revisionId: Scalars['ID']['input'];
};


export type QueryOperatorMessageTraceArgs = {
  receiptId: Scalars['ID']['input'];
};


export type QueryOperatorReceiptsArgs = {
  filter: InputMaybe<OperatorReceiptFilter>;
  page: InputMaybe<OperatorPageInput>;
};


export type QueryParsePreviewArgs = {
  data: Scalars['String']['input'];
  format: SourceFormat;
  source: InputMaybe<Scalars['String']['input']>;
};


export type QueryParsePreviewWithProfileArgs = {
  data: Scalars['String']['input'];
  format: SourceFormat;
  profileId: InputMaybe<Scalars['ID']['input']>;
  source: InputMaybe<Scalars['String']['input']>;
};


export type QueryPatientArgs = {
  mrn: Scalars['ID']['input'];
};


export type QueryPatientTimelineArgs = {
  fromTimestamp: InputMaybe<Scalars['DateTime']['input']>;
  limit?: InputMaybe<Scalars['Int']['input']>;
  mrn: Scalars['ID']['input'];
  toTimestamp: InputMaybe<Scalars['DateTime']['input']>;
};


export type QueryPatientsArgs = {
  after: InputMaybe<Scalars['String']['input']>;
  filter: InputMaybe<PatientFilter>;
  first?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryProfileArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProfileRevisionsArgs = {
  id: Scalars['ID']['input'];
};


export type QueryProfilesArgs = {
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
};


export type QueryQuickQualityScoreArgs = {
  event: Scalars['JSON']['input'];
};


export type QueryResolveMappingArgs = {
  input: ResolveMappingInput;
};


export type QuerySessionArtifactsArgs = {
  sessionId: Scalars['ID']['input'];
};


export type QuerySessionDiagnosticsArgs = {
  runId: InputMaybe<Scalars['ID']['input']>;
  sessionId: Scalars['ID']['input'];
};


export type QuerySessionPublicationsArgs = {
  sessionId: Scalars['ID']['input'];
};


export type QuerySessionRunArgs = {
  id: Scalars['ID']['input'];
};


export type QuerySessionRunsArgs = {
  sessionId: Scalars['ID']['input'];
};


export type QuerySessionSamplesArgs = {
  sessionId: Scalars['ID']['input'];
};


export type QuerySessionWorkflowSimulationsArgs = {
  sessionId: Scalars['ID']['input'];
};


export type QuerySuggestMappingsArgs = {
  input: SuggestMappingsInput;
};


export type QueryTemporalWorkflowArgs = {
  runId: InputMaybe<Scalars['String']['input']>;
  workflowId: Scalars['String']['input'];
};


export type QueryTemporalWorkflowsArgs = {
  after: InputMaybe<Scalars['String']['input']>;
  filter: InputMaybe<TemporalWorkflowFilter>;
  first?: InputMaybe<Scalars['Int']['input']>;
};


export type QueryWorkflowArgs = {
  name: Scalars['String']['input'];
};


export type QueryWorkflowApprovalRequestsArgs = {
  filter: InputMaybe<WorkflowApprovalRequestFilter>;
  paging: InputMaybe<PagingInput>;
};


export type QueryWorkflowDefinitionArgs = {
  nameOrId: Scalars['String']['input'];
};


export type QueryWorkflowDefinitionsArgs = {
  filter: InputMaybe<WorkflowDefinitionFilter>;
  paging: InputMaybe<PagingInput>;
};


export type QueryWorkflowRunArgs = {
  id: Scalars['ID']['input'];
};


export type QueryWorkflowRunTraceArgs = {
  runId: Scalars['ID']['input'];
};


export type QueryWorkflowRunsArgs = {
  filter: InputMaybe<WorkflowRunFilter>;
  paging: InputMaybe<PagingInput>;
};


export type QueryWorkflowVersionArgs = {
  id: Scalars['ID']['input'];
};


export type QueryWorkflowVersionsArgs = {
  paging: InputMaybe<PagingInput>;
  workflowId: Scalars['ID']['input'];
};

export type RejectPendingAutorouteInput = {
  id: Scalars['ID']['input'];
  reason: Scalars['String']['input'];
};

export type RejectWorkflowVersionInput = {
  approvalRequestId: Scalars['ID']['input'];
  comment: InputMaybe<Scalars['String']['input']>;
  reviewedBy: InputMaybe<Scalars['String']['input']>;
};

export type RequestWorkflowApprovalInput = {
  comment: InputMaybe<Scalars['String']['input']>;
  environment: Scalars['String']['input'];
  requestedBy: InputMaybe<Scalars['String']['input']>;
  targetVersionId: Scalars['ID']['input'];
  workflowId: Scalars['ID']['input'];
};

export type ResolveMappingInput = {
  allowAutoroute: InputMaybe<Scalars['Boolean']['input']>;
  minConfidence: InputMaybe<Scalars['Float']['input']>;
  profileId: InputMaybe<Scalars['String']['input']>;
  sourceCode: Scalars['String']['input'];
  sourceDisplay: InputMaybe<Scalars['String']['input']>;
  sourceSystem: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
};

export type ResolveMappingResult = {
  __typename?: 'ResolveMappingResult';
  candidates: Array<MappingCandidate>;
  confidence: Maybe<Scalars['Float']['output']>;
  decision: AutorouteDecision;
  durationMs: Scalars['Int']['output'];
  found: Scalars['Boolean']['output'];
  mapping: Maybe<CodeMapping>;
  reasoning: Maybe<Scalars['String']['output']>;
  trace: Maybe<AutorouteTrace>;
};

export type RollbackWorkflowVersionInput = {
  environment: Scalars['String']['input'];
  publishedBy: InputMaybe<Scalars['String']['input']>;
  targetVersionId: Scalars['ID']['input'];
  workflowId: Scalars['ID']['input'];
};

export type RouteExplanation = {
  __typename?: 'RouteExplanation';
  actions: Array<Scalars['String']['output']>;
  description: Scalars['String']['output'];
  name: Scalars['String']['output'];
  trigger: Scalars['String']['output'];
};

export type RunSessionPreviewInput = {
  data: InputMaybe<Scalars['String']['input']>;
  format: InputMaybe<SourceFormat>;
  sampleId: InputMaybe<Scalars['ID']['input']>;
  sessionId: Scalars['ID']['input'];
  source: InputMaybe<Scalars['String']['input']>;
};

export type RunStage = {
  __typename?: 'RunStage';
  completedAt: Maybe<Scalars['DateTime']['output']>;
  durationMs: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  startedAt: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
  summary: Maybe<Scalars['String']['output']>;
};

export type SaveWorkflowVersionInput = {
  createdBy: InputMaybe<Scalars['String']['input']>;
  notes: InputMaybe<Scalars['String']['input']>;
  workflowId: Scalars['ID']['input'];
  yaml: Scalars['String']['input'];
};

export type SessionArtifact = {
  __typename?: 'SessionArtifact';
  content: Scalars['String']['output'];
  createdAt: Scalars['DateTime']['output'];
  digest: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  kind: Scalars['String']['output'];
  name: Scalars['String']['output'];
  revisionId: Scalars['ID']['output'];
  sessionId: Scalars['ID']['output'];
  updatedAt: Scalars['DateTime']['output'];
  version: Scalars['Int']['output'];
};

export type SessionDeploymentSnapshot = {
  __typename?: 'SessionDeploymentSnapshot';
  definitionRevision: IntegrationArtifactRevision;
  health: Scalars['String']['output'];
  releaseId: Maybe<Scalars['ID']['output']>;
  state: Scalars['String']['output'];
  version: Scalars['Int']['output'];
};

export type SessionDiagnostic = {
  __typename?: 'SessionDiagnostic';
  accepted: Scalars['Boolean']['output'];
  acceptedAt: Maybe<Scalars['DateTime']['output']>;
  code: Scalars['String']['output'];
  fixSuggestion: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  lineage: Array<LineageLink>;
  message: Scalars['String']['output'];
  path: Maybe<Scalars['String']['output']>;
  runId: Maybe<Scalars['ID']['output']>;
  sampleId: Maybe<Scalars['ID']['output']>;
  sessionId: Scalars['ID']['output'];
  severity: Scalars['String']['output'];
};

export type SessionPublication = {
  __typename?: 'SessionPublication';
  createdAt: Scalars['DateTime']['output'];
  definitionRevision: IntegrationArtifactRevision;
  definitionVersion: Scalars['Int']['output'];
  id: Scalars['ID']['output'];
  manifestDigest: Scalars['String']['output'];
  productionProfile: IntegrationArtifactRevision;
  productionWorkflow: IntegrationArtifactRevision;
  publishedBy: Scalars['ID']['output'];
  reason: Scalars['String']['output'];
  sessionId: Scalars['ID']['output'];
  sessionProfile: IntegrationArtifactRevision;
  sessionWorkflow: IntegrationArtifactRevision;
  signatureAlgorithm: Scalars['String']['output'];
  signingKeyId: Scalars['ID']['output'];
  sourceRunIds: Array<Scalars['ID']['output']>;
  version: Scalars['Int']['output'];
  workflowSimulationId: Scalars['ID']['output'];
};

export type SessionRun = {
  __typename?: 'SessionRun';
  completedAt: Maybe<Scalars['DateTime']['output']>;
  createdAt: Scalars['DateTime']['output'];
  diagnostics: Array<SessionDiagnostic>;
  events: Array<Event>;
  id: Scalars['ID']['output'];
  lineage: Array<LineageLink>;
  profileRevisionDigest: Maybe<Scalars['String']['output']>;
  profileRevisionId: Maybe<Scalars['ID']['output']>;
  sampleId: Maybe<Scalars['ID']['output']>;
  sessionId: Scalars['ID']['output'];
  stages: Array<RunStage>;
  status: Scalars['String']['output'];
  warnings: Array<ParseWarning>;
};

export type SessionSample = {
  __typename?: 'SessionSample';
  createdAt: Scalars['DateTime']['output'];
  format: SourceFormat;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  payloadChecksum: Scalars['String']['output'];
  payloadRef: Maybe<Scalars['String']['output']>;
  rawPayload: Maybe<Scalars['String']['output']>;
  sessionId: Scalars['ID']['output'];
  source: Maybe<Scalars['String']['output']>;
};

export type SessionWorkflowActionTrace = {
  __typename?: 'SessionWorkflowActionTrace';
  destinationArtifactId: Maybe<Scalars['ID']['output']>;
  id: Scalars['ID']['output'];
  type: Scalars['String']['output'];
};

export type SessionWorkflowEventTrace = {
  __typename?: 'SessionWorkflowEventTrace';
  eventId: Scalars['ID']['output'];
  eventType: Scalars['String']['output'];
  routes: Array<SessionWorkflowRouteTrace>;
  runId: Scalars['ID']['output'];
};

export type SessionWorkflowRouteTrace = {
  __typename?: 'SessionWorkflowRouteTrace';
  actions: Array<SessionWorkflowActionTrace>;
  diagnosticCodes: Array<Scalars['String']['output']>;
  matched: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  skipReason: Maybe<Scalars['String']['output']>;
  transforms: Array<SessionWorkflowTransformTrace>;
};

export type SessionWorkflowSimulation = {
  __typename?: 'SessionWorkflowSimulation';
  createdAt: Scalars['DateTime']['output'];
  delta: Maybe<SessionWorkflowSimulationDelta>;
  events: Array<SessionWorkflowEventTrace>;
  id: Scalars['ID']['output'];
  sessionId: Scalars['ID']['output'];
  sourceRunIds: Array<Scalars['ID']['output']>;
  workflowArtifactId: Scalars['ID']['output'];
  workflowRevisionDigest: Scalars['String']['output'];
  workflowRevisionId: Scalars['ID']['output'];
};

export type SessionWorkflowSimulationDelta = {
  __typename?: 'SessionWorkflowSimulationDelta';
  addedActions: Array<Scalars['String']['output']>;
  addedEvents: Array<Scalars['String']['output']>;
  addedMatchedRoutes: Array<Scalars['String']['output']>;
  addedTransforms: Array<Scalars['String']['output']>;
  baselineSimulationId: Scalars['ID']['output'];
  candidateSimulationId: Scalars['ID']['output'];
  removedActions: Array<Scalars['String']['output']>;
  removedEvents: Array<Scalars['String']['output']>;
  removedMatchedRoutes: Array<Scalars['String']['output']>;
  removedTransforms: Array<Scalars['String']['output']>;
};

export type SessionWorkflowTransformTrace = {
  __typename?: 'SessionWorkflowTransformTrace';
  index: Scalars['Int']['output'];
  status: Scalars['String']['output'];
  type: Scalars['String']['output'];
};

export type SetBreakpointInput = {
  name: Scalars['String']['input'];
  sessionId: Scalars['ID']['input'];
  type: Scalars['String']['input'];
};

export type SignalReviewDecisionInput = {
  approved: Scalars['Boolean']['input'];
  comment: InputMaybe<Scalars['String']['input']>;
  decidedBy: Scalars['String']['input'];
  equivalenceOverride: InputMaybe<MappingEquivalence>;
  rejectionReason: InputMaybe<Scalars['String']['input']>;
  workflowId: Scalars['String']['input'];
};

export type SimulateSessionWorkflowInput = {
  baselineSimulationId: InputMaybe<Scalars['ID']['input']>;
  sessionId: Scalars['ID']['input'];
  sourceRunIds: Array<Scalars['ID']['input']>;
  workflowRevisionId: Scalars['ID']['input'];
};

export type SourceCount = {
  __typename?: 'SourceCount';
  count: Scalars['Int']['output'];
  source: Scalars['String']['output'];
};

export type SourceFormat =
  | 'CDA'
  | 'CSV'
  | 'EDI_835'
  | 'EDI_837'
  | 'FHIR'
  | 'HL7V2';

export type SourceProfile = {
  __typename?: 'SourceProfile';
  createdAt: Scalars['DateTime']['output'];
  createdBy: Maybe<Scalars['String']['output']>;
  hl7v2: Maybe<Hl7v2Config>;
  id: Scalars['ID']['output'];
  identifiers: Maybe<IdentifierConfig>;
  isActive: Scalars['Boolean']['output'];
  name: Scalars['String']['output'];
  terminology: Maybe<TerminologyConfig>;
  updatedAt: Scalars['DateTime']['output'];
  version: Scalars['String']['output'];
};

export type StartDebugSessionInput = {
  event: Scalars['JSON']['input'];
  workflowYaml: Scalars['String']['input'];
};

export type StartTerminologyReviewInput = {
  autoApproveThreshold: InputMaybe<Scalars['Float']['input']>;
  profileId: InputMaybe<Scalars['String']['input']>;
  reviewTimeoutDays: InputMaybe<Scalars['Int']['input']>;
  sourceCode: Scalars['String']['input'];
  sourceDisplay: InputMaybe<Scalars['String']['input']>;
  sourceSystem: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
};

export type StartTerminologyReviewResult = {
  __typename?: 'StartTerminologyReviewResult';
  runId: Scalars['String']['output'];
  started: Scalars['Boolean']['output'];
  workflowId: Scalars['String']['output'];
};

export type SubmitBatchInput = {
  events: InputMaybe<Array<BatchEventItem>>;
  messages: InputMaybe<Array<BatchMessageItem>>;
  parallel: InputMaybe<Scalars['Boolean']['input']>;
  stopOnError: InputMaybe<Scalars['Boolean']['input']>;
};

export type SubmitEventInput = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  data: Scalars['JSON']['input'];
  source: Scalars['String']['input'];
  type: EventType;
};

export type SubmitMessageInput = {
  correlationId: InputMaybe<Scalars['String']['input']>;
  data: Scalars['String']['input'];
  format: SourceFormat;
  source: Scalars['String']['input'];
};

export type SubmitResult = {
  __typename?: 'SubmitResult';
  errors: Array<Scalars['String']['output']>;
  eventId: Maybe<Scalars['ID']['output']>;
  success: Scalars['Boolean']['output'];
  warnings: Array<ParseWarning>;
  workflowResults: Array<WorkflowResult>;
};

export type Subscription = {
  __typename?: 'Subscription';
  debugStepEvent: WorkflowDebugStep;
  eventStream: Event;
  integrationSessionEvents: IntegrationSessionEvent;
  liveParseStream: ParseEvent;
  patientEvents: Event;
  sessionRunEvents: IntegrationSessionEvent;
  workflowEvents: WorkflowEventNotification;
};


export type SubscriptionDebugStepEventArgs = {
  sessionId: Scalars['ID']['input'];
};


export type SubscriptionEventStreamArgs = {
  filter: InputMaybe<EventFilter>;
};


export type SubscriptionIntegrationSessionEventsArgs = {
  sessionId: Scalars['ID']['input'];
};


export type SubscriptionLiveParseStreamArgs = {
  input: LiveParseInput;
};


export type SubscriptionPatientEventsArgs = {
  mrn: Scalars['ID']['input'];
};


export type SubscriptionSessionRunEventsArgs = {
  runId: InputMaybe<Scalars['ID']['input']>;
  sessionId: Scalars['ID']['input'];
};


export type SubscriptionWorkflowEventsArgs = {
  workflowName: Scalars['String']['input'];
};

export type SuggestMappingsInput = {
  maxCandidates: InputMaybe<Scalars['Int']['input']>;
  sourceCode: Scalars['String']['input'];
  sourceDisplay: InputMaybe<Scalars['String']['input']>;
  sourceSystem: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
};

export type TemporalWorkflow = {
  __typename?: 'TemporalWorkflow';
  closeTime: Maybe<Scalars['DateTime']['output']>;
  durationMs: Maybe<Scalars['Int']['output']>;
  error: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  input: Maybe<Scalars['JSON']['output']>;
  result: Maybe<Scalars['JSON']['output']>;
  runId: Scalars['String']['output'];
  startTime: Scalars['DateTime']['output'];
  status: TemporalWorkflowStatus;
  taskQueue: Scalars['String']['output'];
  workflowType: Scalars['String']['output'];
};

export type TemporalWorkflowConnection = {
  __typename?: 'TemporalWorkflowConnection';
  nodes: Array<TemporalWorkflow>;
  pageInfo: PageInfo;
  totalCount: Scalars['Int']['output'];
};

export type TemporalWorkflowFilter = {
  startTimeAfter: InputMaybe<Scalars['DateTime']['input']>;
  startTimeBefore: InputMaybe<Scalars['DateTime']['input']>;
  status: InputMaybe<TemporalWorkflowStatus>;
  workflowType: InputMaybe<Scalars['String']['input']>;
};

export type TemporalWorkflowStatus =
  | 'CANCELED'
  | 'COMPLETED'
  | 'CONTINUED_AS_NEW'
  | 'FAILED'
  | 'RUNNING'
  | 'TERMINATED'
  | 'TIMED_OUT';

export type TerminologyConfig = {
  __typename?: 'TerminologyConfig';
  mappings: Array<TerminologyMappingTable>;
};

export type TerminologyConfigInput = {
  mappings: InputMaybe<Array<TerminologyMappingTableInput>>;
};

export type TerminologyMappingEntry = {
  __typename?: 'TerminologyMappingEntry';
  display: Maybe<Scalars['String']['output']>;
  sourceCode: Scalars['String']['output'];
  targetCode: Scalars['String']['output'];
};

export type TerminologyMappingEntryInput = {
  display: InputMaybe<Scalars['String']['input']>;
  sourceCode: Scalars['String']['input'];
  targetCode: Scalars['String']['input'];
};

export type TerminologyMappingTable = {
  __typename?: 'TerminologyMappingTable';
  entries: Array<TerminologyMappingEntry>;
  id: Scalars['ID']['output'];
  sourceSystem: Scalars['String']['output'];
  targetSystem: Scalars['String']['output'];
};

export type TerminologyMappingTableInput = {
  entries: InputMaybe<Array<TerminologyMappingEntryInput>>;
  id: Scalars['ID']['input'];
  sourceSystem: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
};

export type TimelineEvent = {
  __typename?: 'TimelineEvent';
  eventType: Scalars['String']['output'];
  position: Scalars['Int']['output'];
  source: Maybe<Scalars['String']['output']>;
  streamId: Scalars['String']['output'];
  summary: Scalars['String']['output'];
  timestamp: Scalars['DateTime']['output'];
};

export type ToleranceConfig = {
  __typename?: 'ToleranceConfig';
  extraComponents: Scalars['Boolean']['output'];
  missingSegments: Array<Scalars['String']['output']>;
  nonStandardDelimiters: Scalars['Boolean']['output'];
  nteAnywhere: Scalars['Boolean']['output'];
  unknownSegments: Scalars['Boolean']['output'];
};

export type ToleranceConfigInput = {
  extraComponents: InputMaybe<Scalars['Boolean']['input']>;
  missingSegments: InputMaybe<Array<Scalars['String']['input']>>;
  nonStandardDelimiters: InputMaybe<Scalars['Boolean']['input']>;
  nteAnywhere: InputMaybe<Scalars['Boolean']['input']>;
  unknownSegments: InputMaybe<Scalars['Boolean']['input']>;
};

export type TraceSpan = {
  __typename?: 'TraceSpan';
  attributes: Maybe<Scalars['JSON']['output']>;
  endTime: Maybe<Scalars['DateTime']['output']>;
  events: Array<TraceSpanEvent>;
  id: Scalars['ID']['output'];
  name: Scalars['String']['output'];
  parentId: Maybe<Scalars['ID']['output']>;
  startTime: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
};

export type TraceSpanEvent = {
  __typename?: 'TraceSpanEvent';
  attributes: Maybe<Scalars['JSON']['output']>;
  name: Scalars['String']['output'];
  timestamp: Scalars['DateTime']['output'];
};

export type UpdateMappingInput = {
  comment: InputMaybe<Scalars['String']['input']>;
  confidence: InputMaybe<Scalars['Float']['input']>;
  equivalence: InputMaybe<MappingEquivalence>;
  id: Scalars['ID']['input'];
  sourceDisplay: InputMaybe<Scalars['String']['input']>;
  targetDisplay: InputMaybe<Scalars['String']['input']>;
};

export type UpdateProfileInput = {
  changeSummary: Scalars['String']['input'];
  hl7v2: InputMaybe<Hl7v2ConfigInput>;
  identifiers: InputMaybe<IdentifierConfigInput>;
  name: InputMaybe<Scalars['String']['input']>;
  terminology: InputMaybe<TerminologyConfigInput>;
};

export type UpdateSessionArtifactInput = {
  content: Scalars['String']['input'];
  name: InputMaybe<Scalars['String']['input']>;
  sessionId: Scalars['ID']['input'];
};

export type UpdateWorkflowDefinitionInput = {
  description: InputMaybe<Scalars['String']['input']>;
  id: Scalars['ID']['input'];
  name: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
  updatedBy: InputMaybe<Scalars['String']['input']>;
};

export type UploadBatch = {
  __typename?: 'UploadBatch';
  duplicateRows: Scalars['Int']['output'];
  errorRows: Scalars['Int']['output'];
  filename: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  profileId: Maybe<Scalars['String']['output']>;
  sourceSystem: Maybe<Scalars['String']['output']>;
  targetSystem: Maybe<Scalars['String']['output']>;
  totalRows: Scalars['Int']['output'];
  uploadedAt: Scalars['DateTime']['output'];
  uploadedBy: Maybe<Scalars['String']['output']>;
  validRows: Scalars['Int']['output'];
  validationErrors: Array<UploadValidationError>;
};

export type UploadMappingCsvInput = {
  csv: Scalars['String']['input'];
  defaultSourceSystem: InputMaybe<Scalars['String']['input']>;
  defaultTargetSystem: InputMaybe<Scalars['String']['input']>;
  dryRun: InputMaybe<Scalars['Boolean']['input']>;
  filename: Scalars['String']['input'];
  profileId: InputMaybe<Scalars['String']['input']>;
};

export type UploadMappingResult = {
  __typename?: 'UploadMappingResult';
  batch: UploadBatch;
  mappingsCreated: Scalars['Int']['output'];
  mappingsSkipped: Scalars['Int']['output'];
  preview: Array<CodeMapping>;
};

export type UploadValidationError = {
  __typename?: 'UploadValidationError';
  column: Maybe<Scalars['String']['output']>;
  message: Scalars['String']['output'];
  row: Scalars['Int']['output'];
};

export type ValidationSettingsConfig = {
  __typename?: 'ValidationSettingsConfig';
  mbi: Maybe<ValidatorSetting>;
  npi: Maybe<ValidatorSetting>;
  ssn: Maybe<ValidatorSetting>;
};

export type ValidationSettingsInput = {
  mbi: InputMaybe<ValidatorSettingInput>;
  npi: InputMaybe<ValidatorSettingInput>;
  ssn: InputMaybe<ValidatorSettingInput>;
};

export type ValidatorSetting = {
  __typename?: 'ValidatorSetting';
  enabled: Scalars['Boolean']['output'];
  onInvalid: Scalars['String']['output'];
};

export type ValidatorSettingInput = {
  enabled: Scalars['Boolean']['input'];
  onInvalid: Scalars['String']['input'];
};

export type VitalSign = {
  __typename?: 'VitalSign';
  interpretation: Maybe<Scalars['String']['output']>;
  loincCode: Maybe<Scalars['String']['output']>;
  name: Scalars['String']['output'];
  unit: Maybe<Scalars['String']['output']>;
  value: Scalars['String']['output'];
};

export type VitalSignEvent = Event & {
  __typename?: 'VitalSignEvent';
  correlationId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  patient: Patient;
  source: Scalars['String']['output'];
  sourceFormat: Maybe<SourceFormat>;
  timestamp: Scalars['DateTime']['output'];
  type: EventType;
  vitalSign: VitalSign;
};

export type WorkflowApprovalRequest = {
  __typename?: 'WorkflowApprovalRequest';
  comment: Maybe<Scalars['String']['output']>;
  environment: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  requestedBy: Scalars['String']['output'];
  reviewedAt: Maybe<Scalars['DateTime']['output']>;
  reviewedBy: Maybe<Scalars['String']['output']>;
  status: Scalars['String']['output'];
  targetVersionId: Scalars['ID']['output'];
  workflowId: Scalars['ID']['output'];
};

export type WorkflowApprovalRequestFilter = {
  environment: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
  workflowId: InputMaybe<Scalars['ID']['input']>;
};

export type WorkflowDebugStep = {
  __typename?: 'WorkflowDebugStep';
  kind: Scalars['String']['output'];
  name: Scalars['String']['output'];
  spanName: Scalars['String']['output'];
  stepNumber: Scalars['Int']['output'];
  timestamp: Scalars['DateTime']['output'];
  variables: Maybe<Scalars['JSON']['output']>;
};

export type WorkflowDefinition = {
  __typename?: 'WorkflowDefinition';
  createdAt: Scalars['DateTime']['output'];
  description: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  latestVersion: Maybe<WorkflowVersion>;
  name: Scalars['String']['output'];
  publishedVersionsByEnv: Scalars['JSON']['output'];
  status: Scalars['String']['output'];
  updatedAt: Scalars['DateTime']['output'];
};

export type WorkflowDefinitionFilter = {
  name: InputMaybe<Scalars['String']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
};

export type WorkflowEventNotification = {
  __typename?: 'WorkflowEventNotification';
  actionsExecuted: Array<Scalars['String']['output']>;
  duration: Scalars['Int']['output'];
  event: Event;
  routesMatched: Array<Scalars['String']['output']>;
  workflow: Scalars['String']['output'];
};

export type WorkflowExplanation = {
  __typename?: 'WorkflowExplanation';
  description: Scalars['String']['output'];
  diagram: Maybe<Scalars['String']['output']>;
  routeExplanations: Array<RouteExplanation>;
  summary: Scalars['String']['output'];
  warnings: Array<Scalars['String']['output']>;
};

export type WorkflowRelease = {
  __typename?: 'WorkflowRelease';
  environment: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  publishedAt: Scalars['DateTime']['output'];
  publishedBy: Scalars['String']['output'];
  rollbackFromReleaseId: Maybe<Scalars['ID']['output']>;
  versionId: Scalars['ID']['output'];
  workflowId: Scalars['ID']['output'];
};

export type WorkflowResult = {
  __typename?: 'WorkflowResult';
  actionsExecuted: Scalars['Int']['output'];
  duration: Scalars['Int']['output'];
  environment: Maybe<Scalars['String']['output']>;
  errors: Array<Scalars['String']['output']>;
  routesMatched: Scalars['Int']['output'];
  runId: Maybe<Scalars['ID']['output']>;
  versionId: Maybe<Scalars['ID']['output']>;
  workflowName: Scalars['String']['output'];
};

export type WorkflowRun = {
  __typename?: 'WorkflowRun';
  actionsExecuted: Scalars['Int']['output'];
  durationMs: Scalars['Int']['output'];
  environment: Scalars['String']['output'];
  errors: Array<Scalars['String']['output']>;
  eventId: Maybe<Scalars['String']['output']>;
  id: Scalars['ID']['output'];
  routesMatched: Scalars['Int']['output'];
  startedAt: Scalars['DateTime']['output'];
  status: Scalars['String']['output'];
  versionId: Maybe<Scalars['ID']['output']>;
  workflowName: Scalars['String']['output'];
};

export type WorkflowRunFilter = {
  environment: InputMaybe<Scalars['String']['input']>;
  fromStartedAt: InputMaybe<Scalars['DateTime']['input']>;
  status: InputMaybe<Scalars['String']['input']>;
  toStartedAt: InputMaybe<Scalars['DateTime']['input']>;
  workflowName: InputMaybe<Scalars['String']['input']>;
};

export type WorkflowStatus = {
  __typename?: 'WorkflowStatus';
  enabled: Scalars['Boolean']['output'];
  errors: Scalars['Int']['output'];
  eventsProcessed: Scalars['Int']['output'];
  lastEventTime: Maybe<Scalars['DateTime']['output']>;
  name: Scalars['String']['output'];
  routeCount: Scalars['Int']['output'];
};

export type WorkflowValidation = {
  __typename?: 'WorkflowValidation';
  errors: Array<Scalars['String']['output']>;
  info: Array<Scalars['String']['output']>;
  valid: Scalars['Boolean']['output'];
  warnings: Array<Scalars['String']['output']>;
};

export type WorkflowVersion = {
  __typename?: 'WorkflowVersion';
  createdAt: Scalars['DateTime']['output'];
  createdBy: Scalars['String']['output'];
  id: Scalars['ID']['output'];
  notes: Maybe<Scalars['String']['output']>;
  validation: WorkflowValidation;
  versionNumber: Scalars['Int']['output'];
  workflowId: Scalars['ID']['output'];
  yaml: Scalars['String']['output'];
};

export type StartDebugSessionMutationVariables = Exact<{
  input: StartDebugSessionInput;
}>;


export type StartDebugSessionMutation = { __typename?: 'Mutation', startDebugSession: { __typename?: 'DebugSession', id: string, workflowId: string, state: string, createdAt: string, breakpoints: Array<{ __typename?: 'Breakpoint', id: string, type: string, name: string, enabled: boolean }>, steps: Array<{ __typename?: 'WorkflowDebugStep', stepNumber: number, kind: string, name: string, variables: unknown | null, timestamp: string, spanName: string }> } };

export type DebugStepMutationVariables = Exact<{
  sessionId: Scalars['ID']['input'];
}>;


export type DebugStepMutation = { __typename?: 'Mutation', debugStep: { __typename?: 'WorkflowDebugStep', stepNumber: number, kind: string, name: string, variables: unknown | null, timestamp: string, spanName: string } | null };

export type DebugContinueMutationVariables = Exact<{
  sessionId: Scalars['ID']['input'];
}>;


export type DebugContinueMutation = { __typename?: 'Mutation', debugContinue: { __typename?: 'WorkflowDebugStep', stepNumber: number, kind: string, name: string, variables: unknown | null, timestamp: string, spanName: string } | null };

export type DebugSetBreakpointMutationVariables = Exact<{
  input: SetBreakpointInput;
}>;


export type DebugSetBreakpointMutation = { __typename?: 'Mutation', debugSetBreakpoint: { __typename?: 'Breakpoint', id: string, type: string, name: string, enabled: boolean } };

export type DebugRemoveBreakpointMutationVariables = Exact<{
  sessionId: Scalars['ID']['input'];
  breakpointId: Scalars['ID']['input'];
}>;


export type DebugRemoveBreakpointMutation = { __typename?: 'Mutation', debugRemoveBreakpoint: boolean };

export type DebugEndSessionMutationVariables = Exact<{
  sessionId: Scalars['ID']['input'];
}>;


export type DebugEndSessionMutation = { __typename?: 'Mutation', debugEndSession: boolean };

export type LiveParseStreamSubscriptionVariables = Exact<{
  input: LiveParseInput;
}>;


export type LiveParseStreamSubscription = { __typename?: 'Subscription', liveParseStream: { __typename?: 'ParseEvent', segmentIndex: number, segmentType: string, rawSegment: string, fields: unknown | null, warnings: Array<string>, isComplete: boolean } };

export type DebugStepEventSubscriptionVariables = Exact<{
  sessionId: Scalars['ID']['input'];
}>;


export type DebugStepEventSubscription = { __typename?: 'Subscription', debugStepEvent: { __typename?: 'WorkflowDebugStep', stepNumber: number, kind: string, name: string, variables: unknown | null, timestamp: string, spanName: string } };

export type DebugSessionQueryQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type DebugSessionQueryQuery = { __typename?: 'Query', debugSession: { __typename?: 'DebugSession', id: string, workflowId: string, state: string, createdAt: string, breakpoints: Array<{ __typename?: 'Breakpoint', id: string, type: string, name: string, enabled: boolean }>, steps: Array<{ __typename?: 'WorkflowDebugStep', stepNumber: number, kind: string, name: string, variables: unknown | null, timestamp: string, spanName: string }> } | null };

export type WorkflowRunTraceQueryVariables = Exact<{
  runId: Scalars['ID']['input'];
}>;


export type WorkflowRunTraceQuery = { __typename?: 'Query', workflowRunTrace: Array<{ __typename?: 'TraceSpan', id: string, name: string, parentId: string | null, startTime: string, endTime: string | null, status: string, attributes: unknown | null, events: Array<{ __typename?: 'TraceSpanEvent', name: string, timestamp: string, attributes: unknown | null }> }> };

export type EventStreamSubscriptionVariables = Exact<{
  filter: InputMaybe<EventFilter>;
}>;


export type EventStreamSubscription = { __typename?: 'Subscription', eventStream: { __typename?: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } };

export type WorkflowEventsSubscriptionVariables = Exact<{
  workflowName: Scalars['String']['input'];
}>;


export type WorkflowEventsSubscription = { __typename?: 'Subscription', workflowEvents: { __typename?: 'WorkflowEventNotification', workflow: string, routesMatched: Array<string>, actionsExecuted: Array<string>, duration: number, event: { __typename?: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string } | { __typename?: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string } } };

export type PatientEventsSubscriptionVariables = Exact<{
  mrn: Scalars['ID']['input'];
}>;


export type PatientEventsSubscription = { __typename?: 'Subscription', patientEvents: { __typename?: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } };

export type EventsQueryVariables = Exact<{
  filter: InputMaybe<EventFilter>;
  first: InputMaybe<Scalars['Int']['input']>;
  after: InputMaybe<Scalars['String']['input']>;
  orderBy: InputMaybe<EventOrderBy>;
}>;


export type EventsQuery = { __typename?: 'Query', events: { __typename?: 'EventConnection', totalCount: number, edges: Array<{ __typename?: 'EventEdge', cursor: string, node: { __typename?: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, hasPreviousPage: boolean, startCursor: string | null, endCursor: string | null } } };

export type EventByIdQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type EventByIdQuery = { __typename?: 'Query', event: { __typename?: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename?: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | null };

export type EventStatisticsQueryVariables = Exact<{ [key: string]: never; }>;


export type EventStatisticsQuery = { __typename?: 'Query', eventStatistics: { __typename?: 'EventStatistics', totalEvents: number, byType: Array<{ __typename?: 'EventTypeCount', eventType: string, count: number }>, bySource: Array<{ __typename?: 'SourceCount', source: string, count: number }> } };

export type PatientTimelineQueryVariables = Exact<{
  mrn: Scalars['ID']['input'];
  fromTimestamp: InputMaybe<Scalars['DateTime']['input']>;
  toTimestamp: InputMaybe<Scalars['DateTime']['input']>;
  limit: InputMaybe<Scalars['Int']['input']>;
}>;


export type PatientTimelineQuery = { __typename?: 'Query', patientTimeline: { __typename?: 'PatientTimeline', mrn: string, lastUpdated: string, eventCount: number, events: Array<{ __typename?: 'TimelineEvent', position: number, timestamp: string, eventType: string, summary: string, streamId: string, source: string | null }> } | null };

export type PatientsQueryVariables = Exact<{
  filter: InputMaybe<PatientFilter>;
  first: InputMaybe<Scalars['Int']['input']>;
  after: InputMaybe<Scalars['String']['input']>;
}>;


export type PatientsQuery = { __typename?: 'Query', patients: { __typename?: 'PatientConnection', totalCount: number, edges: Array<{ __typename?: 'PatientEdge', cursor: string, node: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, middleName: string | null, dateOfBirth: string | null, gender: string | null } }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, endCursor: string | null } } };

export type ExplainWarningsQueryVariables = Exact<{
  warnings: Array<ParseWarningInput> | ParseWarningInput;
  format: SourceFormat;
}>;


export type ExplainWarningsQuery = { __typename?: 'Query', explainWarnings: Array<{ __typename?: 'ExplainedWarning', code: string, explanation: string, fixSuggestion: string | null, impact: string | null, fromCache: boolean }> };

export type HealthQueryVariables = Exact<{ [key: string]: never; }>;


export type HealthQuery = { __typename?: 'Query', health: { __typename?: 'HealthStatus', status: string, version: string } };

export type PreviewIntegrationMessageMutationVariables = Exact<{
  input: PreviewIntegrationMessageInput;
}>;


export type PreviewIntegrationMessageMutation = { __typename?: 'Mutation', previewIntegrationMessage: { __typename?: 'IntegrationPreviewResult', mode: string, tenantId: string, integrationRevision: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, artifactRevisions: { __typename?: 'IntegrationExecutionArtifactRevisions', source: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, profile: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, workflow: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string } }, events: Array<{ __typename?: 'IntegrationPreviewEvent', tenantId: string, id: string, type: string, sourceMessageId: string | null, correlationId: string, classification: string, payload: unknown }>, diagnostics: Array<{ __typename?: 'IntegrationPreviewDiagnostic', tenantId: string, severity: string, stage: string, code: string, message: string, path: string | null, source: string | null, classification: string }>, routes: Array<{ __typename?: 'IntegrationPreviewRoute', tenantId: string, eventId: string, route: string, matched: boolean, skipped: boolean, skipReason: string | null, transformCount: number, plannedActions: Array<string>, diagnosticCodes: Array<string> }>, deliveries: Array<{ __typename?: 'IntegrationPreviewDelivery', tenantId: string, eventId: string, route: string, action: string, status: string, diagnosticCodes: Array<string>, destination: { __typename?: 'IntegrationPreviewDestination', artifactId: string, revisionId: string, digest: string, class: string } }>, correlations: { __typename?: 'IntegrationPreviewCorrelations', tenantId: string, correlationId: string, traceId: string | null, sourceMessageId: string | null, eventIds: Array<string>, workflowRunId: string | null } } };

export type IntegrationSessionRunFieldsFragment = { __typename?: 'SessionRun', id: string, sessionId: string, sampleId: string | null, status: string, profileRevisionId: string | null, profileRevisionDigest: string | null, createdAt: string, completedAt: string | null, stages: Array<{ __typename?: 'RunStage', id: string, name: string, status: string, startedAt: string, completedAt: string | null, durationMs: number, summary: string | null }>, diagnostics: Array<{ __typename?: 'SessionDiagnostic', id: string, sessionId: string, runId: string | null, sampleId: string | null, severity: string, code: string, message: string, path: string | null, fixSuggestion: string | null, accepted: boolean, acceptedAt: string | null, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }> }>, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, appointment: { __typename?: 'Appointment', id: string, status: string, startTime: string, endTime: string | null, reason: string | null, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null, provider: { __typename?: 'Provider', familyName: string, givenName: string, npi: string | null } | null } } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', documentType: string, title: string | null, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', isCritical: boolean, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, test: { __typename?: 'LabTest', loincCode: string | null, localCode: string | null, description: string }, result: { __typename?: 'LabResult', value: string, unit: string | null, status: string | null } } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, explanation: string | null, fixSuggestion: string | null, impact: string | null, severity: string | null, fromCache: boolean | null }> };

export type CreateStreamingIntegrationSessionMutationVariables = Exact<{
  input: CreateIntegrationSessionInput;
}>;


export type CreateStreamingIntegrationSessionMutation = { __typename?: 'Mutation', createIntegrationSession: { __typename?: 'IntegrationSession', id: string } };

export type AddStreamingSessionSampleMutationVariables = Exact<{
  input: AddSessionSampleInput;
}>;


export type AddStreamingSessionSampleMutation = { __typename?: 'Mutation', addSessionSample: { __typename?: 'SessionSample', id: string, sessionId: string } };

export type UpdateStreamingSessionProfileMutationVariables = Exact<{
  input: UpdateSessionArtifactInput;
}>;


export type UpdateStreamingSessionProfileMutation = { __typename?: 'Mutation', updateSessionProfileDraft: { __typename?: 'SessionArtifact', revisionId: string, digest: string } };

export type RunStreamingSessionPreviewMutationVariables = Exact<{
  input: RunSessionPreviewInput;
}>;


export type RunStreamingSessionPreviewMutation = { __typename?: 'Mutation', runSessionPreview: { __typename?: 'SessionRun', id: string, sessionId: string, sampleId: string | null, status: string, profileRevisionId: string | null, profileRevisionDigest: string | null, createdAt: string, completedAt: string | null, stages: Array<{ __typename?: 'RunStage', id: string, name: string, status: string, startedAt: string, completedAt: string | null, durationMs: number, summary: string | null }>, diagnostics: Array<{ __typename?: 'SessionDiagnostic', id: string, sessionId: string, runId: string | null, sampleId: string | null, severity: string, code: string, message: string, path: string | null, fixSuggestion: string | null, accepted: boolean, acceptedAt: string | null, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }> }>, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, appointment: { __typename?: 'Appointment', id: string, status: string, startTime: string, endTime: string | null, reason: string | null, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null, provider: { __typename?: 'Provider', familyName: string, givenName: string, npi: string | null } | null } } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', documentType: string, title: string | null, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', isCritical: boolean, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, test: { __typename?: 'LabTest', loincCode: string | null, localCode: string | null, description: string }, result: { __typename?: 'LabResult', value: string, unit: string | null, status: string | null } } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, explanation: string | null, fixSuggestion: string | null, impact: string | null, severity: string | null, fromCache: boolean | null }> } };

export type StreamIntegrationSessionEventsSubscriptionVariables = Exact<{
  sessionId: Scalars['ID']['input'];
}>;


export type StreamIntegrationSessionEventsSubscription = { __typename?: 'Subscription', integrationSessionEvents: { __typename?: 'IntegrationSessionEvent', id: string, type: string, sessionId: string, runId: string | null, message: string, timestamp: string, run: { __typename?: 'SessionRun', id: string, sessionId: string, sampleId: string | null, status: string, profileRevisionId: string | null, profileRevisionDigest: string | null, createdAt: string, completedAt: string | null, stages: Array<{ __typename?: 'RunStage', id: string, name: string, status: string, startedAt: string, completedAt: string | null, durationMs: number, summary: string | null }>, diagnostics: Array<{ __typename?: 'SessionDiagnostic', id: string, sessionId: string, runId: string | null, sampleId: string | null, severity: string, code: string, message: string, path: string | null, fixSuggestion: string | null, accepted: boolean, acceptedAt: string | null, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }> }>, lineage: Array<{ __typename?: 'LineageLink', sourcePath: string, targetPath: string | null, description: string | null }>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, appointment: { __typename?: 'Appointment', id: string, status: string, startTime: string, endTime: string | null, reason: string | null, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null, provider: { __typename?: 'Provider', familyName: string, givenName: string, npi: string | null } | null } } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', documentType: string, title: string | null, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', isCritical: boolean, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, test: { __typename?: 'LabTest', loincCode: string | null, localCode: string | null, description: string }, result: { __typename?: 'LabResult', value: string, unit: string | null, status: string | null } } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, explanation: string | null, fixSuggestion: string | null, impact: string | null, severity: string | null, fromCache: boolean | null }> } | null } };

export type SessionWorkflowSimulationFieldsFragment = { __typename?: 'SessionWorkflowSimulation', id: string, sessionId: string, workflowArtifactId: string, workflowRevisionId: string, workflowRevisionDigest: string, sourceRunIds: Array<string>, createdAt: string, events: Array<{ __typename?: 'SessionWorkflowEventTrace', runId: string, eventId: string, eventType: string, routes: Array<{ __typename?: 'SessionWorkflowRouteTrace', name: string, matched: boolean, skipReason: string | null, diagnosticCodes: Array<string>, transforms: Array<{ __typename?: 'SessionWorkflowTransformTrace', index: number, type: string, status: string }>, actions: Array<{ __typename?: 'SessionWorkflowActionTrace', id: string, type: string, destinationArtifactId: string | null }> }> }>, delta: { __typename?: 'SessionWorkflowSimulationDelta', baselineSimulationId: string, candidateSimulationId: string, addedEvents: Array<string>, removedEvents: Array<string>, addedMatchedRoutes: Array<string>, removedMatchedRoutes: Array<string>, addedTransforms: Array<string>, removedTransforms: Array<string>, addedActions: Array<string>, removedActions: Array<string> } | null };

export type ListWorkflowSimulationSessionsQueryVariables = Exact<{ [key: string]: never; }>;


export type ListWorkflowSimulationSessionsQuery = { __typename?: 'Query', integrationSessions: Array<{ __typename?: 'IntegrationSession', id: string, name: string, archived: boolean, runs: Array<{ __typename?: 'SessionRun', id: string, status: string, profileRevisionId: string | null, events: Array<{ __typename?: 'AppointmentEvent', id: string, type: EventType } | { __typename?: 'ConditionEvent', id: string, type: EventType } | { __typename?: 'DocumentEvent', id: string, type: EventType } | { __typename?: 'ImmunizationEvent', id: string, type: EventType } | { __typename?: 'LabResultEvent', id: string, type: EventType } | { __typename?: 'PatientAdmitEvent', id: string, type: EventType } | { __typename?: 'PatientDischargeEvent', id: string, type: EventType } | { __typename?: 'ProcedureEvent', id: string, type: EventType } | { __typename?: 'VitalSignEvent', id: string, type: EventType }> }>, workflowSimulations: Array<{ __typename?: 'SessionWorkflowSimulation', id: string, workflowRevisionId: string, workflowRevisionDigest: string, sourceRunIds: Array<string>, createdAt: string }> }> };

export type SaveSessionWorkflowDraftMutationVariables = Exact<{
  input: UpdateSessionArtifactInput;
}>;


export type SaveSessionWorkflowDraftMutation = { __typename?: 'Mutation', updateSessionWorkflowDraft: { __typename?: 'SessionArtifact', id: string, revisionId: string, digest: string, version: number } };

export type SimulateSessionWorkflowMutationVariables = Exact<{
  input: SimulateSessionWorkflowInput;
}>;


export type SimulateSessionWorkflowMutation = { __typename?: 'Mutation', simulateSessionWorkflow: { __typename?: 'SessionWorkflowSimulation', id: string, sessionId: string, workflowArtifactId: string, workflowRevisionId: string, workflowRevisionDigest: string, sourceRunIds: Array<string>, createdAt: string, events: Array<{ __typename?: 'SessionWorkflowEventTrace', runId: string, eventId: string, eventType: string, routes: Array<{ __typename?: 'SessionWorkflowRouteTrace', name: string, matched: boolean, skipReason: string | null, diagnosticCodes: Array<string>, transforms: Array<{ __typename?: 'SessionWorkflowTransformTrace', index: number, type: string, status: string }>, actions: Array<{ __typename?: 'SessionWorkflowActionTrace', id: string, type: string, destinationArtifactId: string | null }> }> }>, delta: { __typename?: 'SessionWorkflowSimulationDelta', baselineSimulationId: string, candidateSimulationId: string, addedEvents: Array<string>, removedEvents: Array<string>, addedMatchedRoutes: Array<string>, removedMatchedRoutes: Array<string>, addedTransforms: Array<string>, removedTransforms: Array<string>, addedActions: Array<string>, removedActions: Array<string> } | null } };

export type SessionPublicationFieldsFragment = { __typename?: 'SessionPublication', id: string, sessionId: string, version: number, workflowSimulationId: string, definitionVersion: number, sourceRunIds: Array<string>, manifestDigest: string, signatureAlgorithm: string, signingKeyId: string, publishedBy: string, reason: string, createdAt: string, sessionProfile: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, sessionWorkflow: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, definitionRevision: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, productionProfile: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, productionWorkflow: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string } };

export type PublishIntegrationSessionMutationVariables = Exact<{
  input: PublishIntegrationSessionInput;
}>;


export type PublishIntegrationSessionMutation = { __typename?: 'Mutation', publishIntegrationSession: { __typename?: 'SessionPublication', id: string, sessionId: string, version: number, workflowSimulationId: string, definitionVersion: number, sourceRunIds: Array<string>, manifestDigest: string, signatureAlgorithm: string, signingKeyId: string, publishedBy: string, reason: string, createdAt: string, sessionProfile: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, sessionWorkflow: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, definitionRevision: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, productionProfile: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string }, productionWorkflow: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string } } };

export type ApproveSessionPublicationMutationVariables = Exact<{
  input: PromoteSessionPublicationInput;
}>;


export type ApproveSessionPublicationMutation = { __typename?: 'Mutation', approveSessionPublication: { __typename?: 'SessionDeploymentSnapshot', state: string, version: number, releaseId: string | null, health: string, definitionRevision: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string } } };

export type DeploySessionPublicationMutationVariables = Exact<{
  input: PromoteSessionPublicationInput;
}>;


export type DeploySessionPublicationMutation = { __typename?: 'Mutation', deploySessionPublication: { __typename?: 'SessionDeploymentSnapshot', state: string, version: number, releaseId: string | null, health: string, definitionRevision: { __typename?: 'IntegrationArtifactRevision', artifactId: string, revisionId: string, digest: string } } };

export type ExtractEntitiesQueryVariables = Exact<{
  input: ExtractEntitiesInput;
}>;


export type ExtractEntitiesQuery = { __typename?: 'Query', extractEntities: { __typename?: 'ExtractionResult', overallConfidence: number, processingTimeMs: number, model: string | null, conditions: Array<{ __typename?: 'ExtractedCondition', name: string, code: string | null, codeSystem: string | null, confidence: number, negated: boolean | null, textSpan: string | null, status: string | null }>, medications: Array<{ __typename?: 'ExtractedMedication', name: string, code: string | null, codeSystem: string | null, dose: string | null, route: string | null, frequency: string | null, confidence: number, negated: boolean | null, textSpan: string | null }>, vitalSigns: Array<{ __typename?: 'ExtractedVitalSign', name: string, loincCode: string | null, value: string, unit: string | null, confidence: number, interpretation: string | null, textSpan: string | null }>, allergies: Array<{ __typename?: 'ExtractedAllergy', substance: string, code: string | null, codeSystem: string | null, severity: string | null, reaction: string | null, confidence: number, negated: boolean | null, textSpan: string | null }>, procedures: Array<{ __typename?: 'ExtractedProcedure', name: string, code: string | null, codeSystem: string | null, status: string | null, confidence: number, negated: boolean | null, textSpan: string | null }> } };

export type AnalyzeQualityQueryVariables = Exact<{
  input: AnalyzeQualityInput;
}>;


export type AnalyzeQualityQuery = { __typename?: 'Query', analyzeQuality: { __typename?: 'DataQualityScore', overallScore: number, processingTimeMs: number | null, model: string | null, dimensions: { __typename?: 'QualityDimensions', completeness: number, accuracy: number, consistency: number, conformance: number, timeliness: number }, issues: Array<{ __typename?: 'DataQualityIssue', dimension: string, severity: string, field: string | null, description: string, actualValue: string | null, expectedValue: string | null }>, recommendations: Array<{ __typename?: 'QualityRecommendation', priority: number, category: string | null, title: string, description: string, impact: string | null }> } };

export type GenerateWorkflowMutationVariables = Exact<{
  input: GenerateWorkflowInput;
}>;


export type GenerateWorkflowMutation = { __typename?: 'Mutation', generateWorkflow: { __typename?: 'GeneratedWorkflow', yaml: string, explanation: string, warnings: Array<string> } };

export type ExplainWorkflowQueryVariables = Exact<{
  input: ExplainWorkflowInput;
}>;


export type ExplainWorkflowQuery = { __typename?: 'Query', explainWorkflow: { __typename?: 'WorkflowExplanation', summary: string, description: string, diagram: string | null, warnings: Array<string>, routeExplanations: Array<{ __typename?: 'RouteExplanation', name: string, trigger: string, actions: Array<string>, description: string }> } };

export type LlmCapabilityQueryVariables = Exact<{ [key: string]: never; }>;


export type LlmCapabilityQuery = { __typename?: 'Query', llmCapability: { __typename?: 'LLMCapability', enabled: boolean, configured: boolean, providerBaseURLHost: string | null, defaultModel: string | null, qualityModel: string | null, status: string, warnings: Array<string>, features: Array<{ __typename?: 'LLMFeatureCapability', name: string, enabled: boolean, status: string, reason: string | null, model: string | null }> } };

export type ClassifyMessageQueryVariables = Exact<{
  input: ClassifyMessageInput;
}>;


export type ClassifyMessageQuery = { __typename?: 'Query', classifyMessage: { __typename?: 'MessageClassification', messageType: string, eventType: EventType | null, suggestedTags: Array<string>, confidence: number, summary: string | null } };

export type ParsePreviewQueryVariables = Exact<{
  format: SourceFormat;
  data: Scalars['String']['input'];
  source: InputMaybe<Scalars['String']['input']>;
}>;


export type ParsePreviewQuery = { __typename?: 'Query', parsePreview: { __typename?: 'ParseResult', success: boolean, errors: Array<string>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, appointment: { __typename?: 'Appointment', id: string, status: string, startTime: string, endTime: string | null, reason: string | null, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null, provider: { __typename?: 'Provider', familyName: string, givenName: string, npi: string | null } | null } } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', documentType: string, title: string | null, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', isCritical: boolean, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, test: { __typename?: 'LabTest', loincCode: string | null, localCode: string | null, description: string }, result: { __typename?: 'LabResult', value: string, unit: string | null, status: string | null } } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, explanation: string | null, fixSuggestion: string | null, impact: string | null, severity: string | null, fromCache: boolean | null }> } };

export type ParsePreviewWithProfileQueryVariables = Exact<{
  format: SourceFormat;
  data: Scalars['String']['input'];
  source: InputMaybe<Scalars['String']['input']>;
  profileId: InputMaybe<Scalars['ID']['input']>;
}>;


export type ParsePreviewWithProfileQuery = { __typename?: 'Query', parsePreviewWithProfile: { __typename?: 'ParseResult', success: boolean, errors: Array<string>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, appointment: { __typename?: 'Appointment', id: string, status: string, startTime: string, endTime: string | null, reason: string | null, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null, provider: { __typename?: 'Provider', familyName: string, givenName: string, npi: string | null } | null } } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', documentType: string, title: string | null, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', isCritical: boolean, id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, test: { __typename?: 'LabTest', loincCode: string | null, localCode: string | null, description: string }, result: { __typename?: 'LabResult', value: string, unit: string | null, status: string | null } } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null, patient: { __typename?: 'Patient', mrn: string, familyName: string, givenName: string, dateOfBirth: string | null, gender: string | null }, encounter: { __typename?: 'Encounter', class: string, location: { __typename?: 'Location', facility: string | null, unit: string | null, room: string | null, bed: string | null } | null } } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, explanation: string | null, fixSuggestion: string | null, impact: string | null, severity: string | null, fromCache: boolean | null }> } };

export type ProfileFieldsFragment = { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean };

export type ToleranceFieldsFragment = { __typename?: 'ToleranceConfig', missingSegments: Array<string>, nteAnywhere: boolean, extraComponents: boolean, unknownSegments: boolean, nonStandardDelimiters: boolean };

export type Hl7v2ConfigFieldsFragment = { __typename?: 'HL7v2Config', defaultVersion: string, timezone: string, tolerance: { __typename?: 'ToleranceConfig', missingSegments: Array<string>, nteAnywhere: boolean, extraComponents: boolean, unknownSegments: boolean, nonStandardDelimiters: boolean } | null, eventClassifications: Array<{ __typename?: 'EventClassificationRule', messageType: string, condition: string | null, eventType: string, priority: number }> };

export type IdentifierConfigFieldsFragment = { __typename?: 'IdentifierConfig', assigningAuthorities: Array<{ __typename?: 'AssigningAuthority', code: string, system: string, name: string | null }>, primaryIdPreference: Array<{ __typename?: 'IDPreferenceRule', type: string, assignerContains: string | null, priority: number }>, validation: { __typename?: 'ValidationSettingsConfig', npi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, mbi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, ssn: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null } | null, normalization: { __typename?: 'NormalizationSettingsConfig', ssnStripDashes: boolean, ssnRejectPatterns: Array<string>, phoneNormalize: boolean, phoneFormat: string | null } | null };

export type TerminologyConfigFieldsFragment = { __typename?: 'TerminologyConfig', mappings: Array<{ __typename?: 'TerminologyMappingTable', id: string, sourceSystem: string, targetSystem: string, entries: Array<{ __typename?: 'TerminologyMappingEntry', sourceCode: string, targetCode: string, display: string | null }> }> };

export type FullProfileFieldsFragment = { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean, hl7v2: { __typename?: 'HL7v2Config', defaultVersion: string, timezone: string, tolerance: { __typename?: 'ToleranceConfig', missingSegments: Array<string>, nteAnywhere: boolean, extraComponents: boolean, unknownSegments: boolean, nonStandardDelimiters: boolean } | null, eventClassifications: Array<{ __typename?: 'EventClassificationRule', messageType: string, condition: string | null, eventType: string, priority: number }> } | null, identifiers: { __typename?: 'IdentifierConfig', assigningAuthorities: Array<{ __typename?: 'AssigningAuthority', code: string, system: string, name: string | null }>, primaryIdPreference: Array<{ __typename?: 'IDPreferenceRule', type: string, assignerContains: string | null, priority: number }>, validation: { __typename?: 'ValidationSettingsConfig', npi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, mbi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, ssn: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null } | null, normalization: { __typename?: 'NormalizationSettingsConfig', ssnStripDashes: boolean, ssnRejectPatterns: Array<string>, phoneNormalize: boolean, phoneFormat: string | null } | null } | null, terminology: { __typename?: 'TerminologyConfig', mappings: Array<{ __typename?: 'TerminologyMappingTable', id: string, sourceSystem: string, targetSystem: string, entries: Array<{ __typename?: 'TerminologyMappingEntry', sourceCode: string, targetCode: string, display: string | null }> }> } | null };

export type ListProfilesQueryVariables = Exact<{
  activeOnly?: InputMaybe<Scalars['Boolean']['input']>;
}>;


export type ListProfilesQuery = { __typename?: 'Query', profiles: Array<{ __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean }> };

export type GetProfileQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetProfileQuery = { __typename?: 'Query', profile: { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean, hl7v2: { __typename?: 'HL7v2Config', defaultVersion: string, timezone: string, tolerance: { __typename?: 'ToleranceConfig', missingSegments: Array<string>, nteAnywhere: boolean, extraComponents: boolean, unknownSegments: boolean, nonStandardDelimiters: boolean } | null, eventClassifications: Array<{ __typename?: 'EventClassificationRule', messageType: string, condition: string | null, eventType: string, priority: number }> } | null, identifiers: { __typename?: 'IdentifierConfig', assigningAuthorities: Array<{ __typename?: 'AssigningAuthority', code: string, system: string, name: string | null }>, primaryIdPreference: Array<{ __typename?: 'IDPreferenceRule', type: string, assignerContains: string | null, priority: number }>, validation: { __typename?: 'ValidationSettingsConfig', npi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, mbi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, ssn: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null } | null, normalization: { __typename?: 'NormalizationSettingsConfig', ssnStripDashes: boolean, ssnRejectPatterns: Array<string>, phoneNormalize: boolean, phoneFormat: string | null } | null } | null, terminology: { __typename?: 'TerminologyConfig', mappings: Array<{ __typename?: 'TerminologyMappingTable', id: string, sourceSystem: string, targetSystem: string, entries: Array<{ __typename?: 'TerminologyMappingEntry', sourceCode: string, targetCode: string, display: string | null }> }> } | null } | null };

export type GetProfileRevisionsQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetProfileRevisionsQuery = { __typename?: 'Query', profileRevisions: Array<{ __typename?: 'ProfileRevision', version: string, createdAt: string, createdBy: string | null, changeSummary: string | null }> };

export type CreateProfileMutationVariables = Exact<{
  input: CreateProfileInput;
}>;


export type CreateProfileMutation = { __typename?: 'Mutation', createProfile: { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean } };

export type UpdateProfileMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  input: UpdateProfileInput;
}>;


export type UpdateProfileMutation = { __typename?: 'Mutation', updateProfile: { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean, hl7v2: { __typename?: 'HL7v2Config', defaultVersion: string, timezone: string, tolerance: { __typename?: 'ToleranceConfig', missingSegments: Array<string>, nteAnywhere: boolean, extraComponents: boolean, unknownSegments: boolean, nonStandardDelimiters: boolean } | null, eventClassifications: Array<{ __typename?: 'EventClassificationRule', messageType: string, condition: string | null, eventType: string, priority: number }> } | null, identifiers: { __typename?: 'IdentifierConfig', assigningAuthorities: Array<{ __typename?: 'AssigningAuthority', code: string, system: string, name: string | null }>, primaryIdPreference: Array<{ __typename?: 'IDPreferenceRule', type: string, assignerContains: string | null, priority: number }>, validation: { __typename?: 'ValidationSettingsConfig', npi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, mbi: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null, ssn: { __typename?: 'ValidatorSetting', enabled: boolean, onInvalid: string } | null } | null, normalization: { __typename?: 'NormalizationSettingsConfig', ssnStripDashes: boolean, ssnRejectPatterns: Array<string>, phoneNormalize: boolean, phoneFormat: string | null } | null } | null, terminology: { __typename?: 'TerminologyConfig', mappings: Array<{ __typename?: 'TerminologyMappingTable', id: string, sourceSystem: string, targetSystem: string, entries: Array<{ __typename?: 'TerminologyMappingEntry', sourceCode: string, targetCode: string, display: string | null }> }> } | null } };

export type DeleteProfileMutationVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type DeleteProfileMutation = { __typename?: 'Mutation', deleteProfile: boolean };

export type DuplicateProfileMutationVariables = Exact<{
  id: Scalars['ID']['input'];
  newId: Scalars['ID']['input'];
  newName: Scalars['String']['input'];
}>;


export type DuplicateProfileMutation = { __typename?: 'Mutation', duplicateProfile: { __typename?: 'SourceProfile', id: string, name: string, version: string, createdAt: string, updatedAt: string, createdBy: string | null, isActive: boolean } };

export type SubmitMessageMutationVariables = Exact<{
  input: SubmitMessageInput;
}>;


export type SubmitMessageMutation = { __typename?: 'Mutation', submitMessage: { __typename?: 'SubmitResult', success: boolean, eventId: string | null, errors: Array<string>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null, severity: string | null }>, workflowResults: Array<{ __typename?: 'WorkflowResult', workflowName: string, routesMatched: number, actionsExecuted: number, errors: Array<string>, duration: number }> } };

export type TemporalWorkflowFieldsFragment = { __typename?: 'TemporalWorkflow', id: string, runId: string, workflowType: string, status: TemporalWorkflowStatus, taskQueue: string, startTime: string, closeTime: string | null, durationMs: number | null };

export type ListTemporalWorkflowsQueryVariables = Exact<{
  filter: InputMaybe<TemporalWorkflowFilter>;
  first: InputMaybe<Scalars['Int']['input']>;
  after: InputMaybe<Scalars['String']['input']>;
}>;


export type ListTemporalWorkflowsQuery = { __typename?: 'Query', temporalWorkflows: { __typename?: 'TemporalWorkflowConnection', totalCount: number, nodes: Array<{ __typename?: 'TemporalWorkflow', id: string, runId: string, workflowType: string, status: TemporalWorkflowStatus, taskQueue: string, startTime: string, closeTime: string | null, durationMs: number | null }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, hasPreviousPage: boolean, endCursor: string | null } } };

export type GetTemporalWorkflowQueryVariables = Exact<{
  workflowId: Scalars['String']['input'];
  runId: InputMaybe<Scalars['String']['input']>;
}>;


export type GetTemporalWorkflowQuery = { __typename?: 'Query', temporalWorkflow: { __typename?: 'TemporalWorkflow', input: unknown | null, result: unknown | null, id: string, runId: string, workflowType: string, status: TemporalWorkflowStatus, taskQueue: string, startTime: string, closeTime: string | null, durationMs: number | null } | null };

export type CancelTemporalWorkflowMutationVariables = Exact<{
  workflowId: Scalars['String']['input'];
  reason: InputMaybe<Scalars['String']['input']>;
}>;


export type CancelTemporalWorkflowMutation = { __typename?: 'Mutation', cancelTemporalWorkflow: boolean };

export type SignalReviewDecisionMutationVariables = Exact<{
  input: SignalReviewDecisionInput;
}>;


export type SignalReviewDecisionMutation = { __typename?: 'Mutation', signalReviewDecision: boolean };

export type MappingFieldsFragment = { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null };

export type BatchFieldsFragment = { __typename?: 'UploadBatch', id: string, filename: string, sourceSystem: string | null, targetSystem: string | null, profileId: string | null, totalRows: number, validRows: number, duplicateRows: number, errorRows: number, uploadedAt: string, uploadedBy: string | null, validationErrors: Array<{ __typename?: 'UploadValidationError', row: number, column: string | null, message: string }> };

export type ListMappingsQueryVariables = Exact<{
  input: InputMaybe<ListMappingsInput>;
}>;


export type ListMappingsQuery = { __typename?: 'Query', listMappings: { __typename?: 'CodeMappingConnection', totalCount: number, nodes: Array<{ __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, hasPreviousPage: boolean } } };

export type GetMappingQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetMappingQuery = { __typename?: 'Query', getMapping: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } | null };

export type LookupMappingQueryVariables = Exact<{
  sourceSystem: Scalars['String']['input'];
  sourceCode: Scalars['String']['input'];
  targetSystem: Scalars['String']['input'];
  profileId: InputMaybe<Scalars['String']['input']>;
}>;


export type LookupMappingQuery = { __typename?: 'Query', lookupMapping: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } | null };

export type GetUploadBatchQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetUploadBatchQuery = { __typename?: 'Query', getUploadBatch: { __typename?: 'UploadBatch', id: string, filename: string, sourceSystem: string | null, targetSystem: string | null, profileId: string | null, totalRows: number, validRows: number, duplicateRows: number, errorRows: number, uploadedAt: string, uploadedBy: string | null, validationErrors: Array<{ __typename?: 'UploadValidationError', row: number, column: string | null, message: string }> } | null };

export type UploadMappingCsvMutationVariables = Exact<{
  input: UploadMappingCsvInput;
}>;


export type UploadMappingCsvMutation = { __typename?: 'Mutation', uploadMappingCSV: { __typename?: 'UploadMappingResult', mappingsCreated: number, mappingsSkipped: number, batch: { __typename?: 'UploadBatch', id: string, filename: string, sourceSystem: string | null, targetSystem: string | null, profileId: string | null, totalRows: number, validRows: number, duplicateRows: number, errorRows: number, uploadedAt: string, uploadedBy: string | null, validationErrors: Array<{ __typename?: 'UploadValidationError', row: number, column: string | null, message: string }> }, preview: Array<{ __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null }> } };

export type CreateMappingMutationVariables = Exact<{
  input: CreateMappingInput;
}>;


export type CreateMappingMutation = { __typename?: 'Mutation', createMapping: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } };

export type DeleteMappingMutationVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type DeleteMappingMutation = { __typename?: 'Mutation', deleteMapping: boolean };

export type DeleteMappingBatchMutationVariables = Exact<{
  batchId: Scalars['ID']['input'];
}>;


export type DeleteMappingBatchMutation = { __typename?: 'Mutation', deleteMappingBatch: number };

export type UpdateMappingMutationVariables = Exact<{
  input: UpdateMappingInput;
}>;


export type UpdateMappingMutation = { __typename?: 'Mutation', updateMapping: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } };

export type ExportMappingsCsvQueryVariables = Exact<{
  input: InputMaybe<ListMappingsInput>;
}>;


export type ExportMappingsCsvQuery = { __typename?: 'Query', exportMappingsCSV: string };

export type CandidateFieldsFragment = { __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null };

export type AutorouteTraceFieldsFragment = { __typename?: 'AutorouteTrace', traceId: string, timestamp: string, totalDurationMs: number, steps: Array<{ __typename?: 'AutorouteStep', step: string, result: string, durationMs: number, metadata: unknown | null }> };

export type ResolveMappingQueryVariables = Exact<{
  input: ResolveMappingInput;
}>;


export type ResolveMappingQuery = { __typename?: 'Query', resolveMapping: { __typename?: 'ResolveMappingResult', found: boolean, decision: AutorouteDecision, confidence: number | null, reasoning: string | null, durationMs: number, mapping: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } | null, candidates: Array<{ __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null }>, trace: { __typename?: 'AutorouteTrace', traceId: string, timestamp: string, totalDurationMs: number, steps: Array<{ __typename?: 'AutorouteStep', step: string, result: string, durationMs: number, metadata: unknown | null }> } | null } };

export type SuggestMappingsQueryVariables = Exact<{
  input: SuggestMappingsInput;
}>;


export type SuggestMappingsQuery = { __typename?: 'Query', suggestMappings: Array<{ __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null }> };

export type PendingAutorouteFieldsFragment = { __typename?: 'PendingAutoroute', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, suggestedCode: string, suggestedDisplay: string | null, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, status: PendingAutorouteStatus, createdAt: string, expiresAt: string | null, reviewedAt: string | null, reviewedBy: string | null, rejectionReason: string | null, alternates: Array<{ __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null }>, decisionTrace: { __typename?: 'AutorouteTrace', traceId: string, timestamp: string, totalDurationMs: number, steps: Array<{ __typename?: 'AutorouteStep', step: string, result: string, durationMs: number, metadata: unknown | null }> } | null };

export type ListPendingAutoroutesQueryVariables = Exact<{
  input: InputMaybe<ListPendingAutoroutesInput>;
}>;


export type ListPendingAutoroutesQuery = { __typename?: 'Query', listPendingAutoroutes: { __typename?: 'PendingAutorouteConnection', totalCount: number, nodes: Array<{ __typename?: 'PendingAutoroute', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, suggestedCode: string, suggestedDisplay: string | null, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, status: PendingAutorouteStatus, createdAt: string, expiresAt: string | null, reviewedAt: string | null, reviewedBy: string | null, rejectionReason: string | null, alternates: Array<{ __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null }>, decisionTrace: { __typename?: 'AutorouteTrace', traceId: string, timestamp: string, totalDurationMs: number, steps: Array<{ __typename?: 'AutorouteStep', step: string, result: string, durationMs: number, metadata: unknown | null }> } | null }>, pageInfo: { __typename?: 'PageInfo', hasNextPage: boolean, hasPreviousPage: boolean } } };

export type GetPendingAutorouteQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetPendingAutorouteQuery = { __typename?: 'Query', getPendingAutoroute: { __typename?: 'PendingAutoroute', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, suggestedCode: string, suggestedDisplay: string | null, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, status: PendingAutorouteStatus, createdAt: string, expiresAt: string | null, reviewedAt: string | null, reviewedBy: string | null, rejectionReason: string | null, alternates: Array<{ __typename?: 'MappingCandidate', code: string, display: string, system: string, confidence: number, equivalence: MappingEquivalence | null, reasoning: string | null, score: number | null }>, decisionTrace: { __typename?: 'AutorouteTrace', traceId: string, timestamp: string, totalDurationMs: number, steps: Array<{ __typename?: 'AutorouteStep', step: string, result: string, durationMs: number, metadata: unknown | null }> } | null } | null };

export type PendingAutorouteStatsQueryVariables = Exact<{ [key: string]: never; }>;


export type PendingAutorouteStatsQuery = { __typename?: 'Query', pendingAutorouteStats: { __typename?: 'PendingAutorouteStats', pendingCount: number, approvedCount: number, rejectedCount: number, expiredCount: number, avgConfidence: number | null } };

export type ApprovePendingAutorouteMutationVariables = Exact<{
  input: ApprovePendingAutorouteInput;
}>;


export type ApprovePendingAutorouteMutation = { __typename?: 'Mutation', approvePendingAutoroute: { __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null } };

export type RejectPendingAutorouteMutationVariables = Exact<{
  input: RejectPendingAutorouteInput;
}>;


export type RejectPendingAutorouteMutation = { __typename?: 'Mutation', rejectPendingAutoroute: boolean };

export type BulkApprovePendingAutoroutesMutationVariables = Exact<{
  input: InputMaybe<BulkApproveInput>;
}>;


export type BulkApprovePendingAutoroutesMutation = { __typename?: 'Mutation', bulkApprovePendingAutoroutes: { __typename?: 'BulkApproveResult', approved: number, skipped: number, mappings: Array<{ __typename?: 'CodeMapping', id: string, sourceSystem: string, sourceCode: string, sourceDisplay: string | null, targetSystem: string, targetCode: string, targetDisplay: string | null, equivalence: MappingEquivalence, confidence: number | null, comment: string | null, origin: MappingOrigin, profileId: string | null, uploadBatchId: string | null, createdAt: string, createdBy: string | null }> } };

export type StartTerminologyReviewMutationVariables = Exact<{
  input: StartTerminologyReviewInput;
}>;


export type StartTerminologyReviewMutation = { __typename?: 'Mutation', startTerminologyReview: { __typename?: 'StartTerminologyReviewResult', workflowId: string, runId: string, started: boolean } };

export type ListWorkflowsQueryVariables = Exact<{ [key: string]: never; }>;


export type ListWorkflowsQuery = { __typename?: 'Query', workflows: Array<{ __typename?: 'WorkflowStatus', name: string, enabled: boolean, routeCount: number, eventsProcessed: number, lastEventTime: string | null, errors: number }> };

export type ListWorkflowDefinitionsQueryVariables = Exact<{
  filter: InputMaybe<WorkflowDefinitionFilter>;
  paging: InputMaybe<PagingInput>;
}>;


export type ListWorkflowDefinitionsQuery = { __typename?: 'Query', workflowDefinitions: Array<{ __typename?: 'WorkflowDefinition', id: string, name: string, description: string | null, status: string, createdAt: string, updatedAt: string, publishedVersionsByEnv: unknown, latestVersion: { __typename?: 'WorkflowVersion', id: string, workflowId: string, versionNumber: number, createdAt: string, createdBy: string, notes: string | null, validation: { __typename?: 'WorkflowValidation', valid: boolean, errors: Array<string>, warnings: Array<string>, info: Array<string> } } | null }> };

export type GetWorkflowVersionsQueryVariables = Exact<{
  workflowId: Scalars['ID']['input'];
  paging: InputMaybe<PagingInput>;
}>;


export type GetWorkflowVersionsQuery = { __typename?: 'Query', workflowVersions: Array<{ __typename?: 'WorkflowVersion', id: string, workflowId: string, versionNumber: number, yaml: string, createdBy: string, createdAt: string, notes: string | null, validation: { __typename?: 'WorkflowValidation', valid: boolean, errors: Array<string>, warnings: Array<string>, info: Array<string> } }> };

export type GetWorkflowVersionByIdQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetWorkflowVersionByIdQuery = { __typename?: 'Query', workflowVersion: { __typename?: 'WorkflowVersion', id: string, workflowId: string, versionNumber: number, yaml: string, createdBy: string, createdAt: string, notes: string | null, validation: { __typename?: 'WorkflowValidation', valid: boolean, errors: Array<string>, warnings: Array<string>, info: Array<string> } } | null };

export type ListWorkflowRunsQueryVariables = Exact<{
  filter: InputMaybe<WorkflowRunFilter>;
  paging: InputMaybe<PagingInput>;
}>;


export type ListWorkflowRunsQuery = { __typename?: 'Query', workflowRuns: Array<{ __typename?: 'WorkflowRun', id: string, workflowName: string, environment: string, versionId: string | null, eventId: string | null, routesMatched: number, actionsExecuted: number, errors: Array<string>, durationMs: number, startedAt: string, status: string }> };

export type GetWorkflowRunQueryVariables = Exact<{
  id: Scalars['ID']['input'];
}>;


export type GetWorkflowRunQuery = { __typename?: 'Query', workflowRun: { __typename?: 'WorkflowRun', id: string, workflowName: string, environment: string, versionId: string | null, eventId: string | null, routesMatched: number, actionsExecuted: number, errors: Array<string>, durationMs: number, startedAt: string, status: string } | null };

export type ListWorkflowApprovalRequestsQueryVariables = Exact<{
  filter: InputMaybe<WorkflowApprovalRequestFilter>;
  paging: InputMaybe<PagingInput>;
}>;


export type ListWorkflowApprovalRequestsQuery = { __typename?: 'Query', workflowApprovalRequests: Array<{ __typename?: 'WorkflowApprovalRequest', id: string, workflowId: string, targetVersionId: string, environment: string, status: string, requestedBy: string, reviewedBy: string | null, reviewedAt: string | null, comment: string | null }> };

export type GetWorkflowQueryVariables = Exact<{
  name: Scalars['String']['input'];
}>;


export type GetWorkflowQuery = { __typename?: 'Query', workflow: { __typename?: 'WorkflowStatus', name: string, enabled: boolean, routeCount: number, eventsProcessed: number, lastEventTime: string | null, errors: number } | null };

export type CreateWorkflowDefinitionMutationVariables = Exact<{
  input: CreateWorkflowDefinitionInput;
}>;


export type CreateWorkflowDefinitionMutation = { __typename?: 'Mutation', createWorkflowDefinition: { __typename?: 'WorkflowDefinition', id: string, name: string, description: string | null, status: string, createdAt: string, updatedAt: string, publishedVersionsByEnv: unknown, latestVersion: { __typename?: 'WorkflowVersion', id: string, workflowId: string, versionNumber: number, createdAt: string, createdBy: string, notes: string | null, validation: { __typename?: 'WorkflowValidation', valid: boolean, errors: Array<string>, warnings: Array<string>, info: Array<string> } } | null } };

export type SaveWorkflowVersionMutationVariables = Exact<{
  input: SaveWorkflowVersionInput;
}>;


export type SaveWorkflowVersionMutation = { __typename?: 'Mutation', saveWorkflowVersion: { __typename?: 'WorkflowVersion', id: string, workflowId: string, versionNumber: number, yaml: string, createdBy: string, createdAt: string, notes: string | null, validation: { __typename?: 'WorkflowValidation', valid: boolean, errors: Array<string>, warnings: Array<string>, info: Array<string> } } };

export type PublishWorkflowVersionMutationVariables = Exact<{
  input: PublishWorkflowVersionInput;
}>;


export type PublishWorkflowVersionMutation = { __typename?: 'Mutation', publishWorkflowVersion: { __typename?: 'WorkflowRelease', id: string, workflowId: string, environment: string, versionId: string, publishedBy: string, publishedAt: string, rollbackFromReleaseId: string | null } };

export type RollbackWorkflowVersionMutationVariables = Exact<{
  input: RollbackWorkflowVersionInput;
}>;


export type RollbackWorkflowVersionMutation = { __typename?: 'Mutation', rollbackWorkflowVersion: { __typename?: 'WorkflowRelease', id: string, workflowId: string, environment: string, versionId: string, publishedBy: string, publishedAt: string, rollbackFromReleaseId: string | null } };

export type RequestWorkflowApprovalMutationVariables = Exact<{
  input: RequestWorkflowApprovalInput;
}>;


export type RequestWorkflowApprovalMutation = { __typename?: 'Mutation', requestWorkflowApproval: { __typename?: 'WorkflowApprovalRequest', id: string, workflowId: string, targetVersionId: string, environment: string, status: string, requestedBy: string, reviewedBy: string | null, reviewedAt: string | null, comment: string | null } };

export type ApproveWorkflowVersionMutationVariables = Exact<{
  input: ApproveWorkflowVersionInput;
}>;


export type ApproveWorkflowVersionMutation = { __typename?: 'Mutation', approveWorkflowVersion: { __typename?: 'WorkflowApprovalRequest', id: string, workflowId: string, targetVersionId: string, environment: string, status: string, requestedBy: string, reviewedBy: string | null, reviewedAt: string | null, comment: string | null } };

export type RejectWorkflowVersionMutationVariables = Exact<{
  input: RejectWorkflowVersionInput;
}>;


export type RejectWorkflowVersionMutation = { __typename?: 'Mutation', rejectWorkflowVersion: { __typename?: 'WorkflowApprovalRequest', id: string, workflowId: string, targetVersionId: string, environment: string, status: string, requestedBy: string, reviewedBy: string | null, reviewedAt: string | null, comment: string | null } };

export type TriggerWorkflowMutationVariables = Exact<{
  name: Scalars['String']['input'];
  event: Scalars['JSON']['input'];
  environment: InputMaybe<Scalars['String']['input']>;
  versionId: InputMaybe<Scalars['String']['input']>;
}>;


export type TriggerWorkflowMutation = { __typename?: 'Mutation', triggerWorkflow: { __typename?: 'WorkflowResult', workflowName: string, routesMatched: number, actionsExecuted: number, errors: Array<string>, duration: number, runId: string | null, environment: string | null, versionId: string | null } };

export type DryRunWorkflowMutationVariables = Exact<{
  input: DryRunWorkflowInput;
}>;


export type DryRunWorkflowMutation = { __typename?: 'Mutation', dryRunWorkflow: { __typename?: 'DryRunResult', warnings: Array<string>, validationErrors: Array<string>, routeResults: Array<{ __typename?: 'DryRunRouteResult', routeName: string, matched: boolean, actionsWouldRun: number, skipReason: string | null }> } };

export const IntegrationSessionRunFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IntegrationSessionRunFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionRun"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stages"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}}]}},{"kind":"Field","name":{"kind":"Name","value":"diagnostics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"accepted"}},{"kind":"Field","name":{"kind":"Name","value":"acceptedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientAdmitEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientDischargeEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LabResultEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"test"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"localCode"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"result"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isCritical"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AppointmentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appointment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"npi"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DocumentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"documentType"}},{"kind":"Field","name":{"kind":"Name","value":"title"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]} as unknown as DocumentNode<IntegrationSessionRunFieldsFragment, unknown>;
export const SessionWorkflowSimulationFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SessionWorkflowSimulationFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionWorkflowSimulation"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowArtifactId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"sourceRunIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"routes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"matched"}},{"kind":"Field","name":{"kind":"Name","value":"skipReason"}},{"kind":"Field","name":{"kind":"Name","value":"diagnosticCodes"}},{"kind":"Field","name":{"kind":"Name","value":"transforms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"index"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"actions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"destinationArtifactId"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"delta"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"baselineSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"candidateSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"addedEvents"}},{"kind":"Field","name":{"kind":"Name","value":"removedEvents"}},{"kind":"Field","name":{"kind":"Name","value":"addedMatchedRoutes"}},{"kind":"Field","name":{"kind":"Name","value":"removedMatchedRoutes"}},{"kind":"Field","name":{"kind":"Name","value":"addedTransforms"}},{"kind":"Field","name":{"kind":"Name","value":"removedTransforms"}},{"kind":"Field","name":{"kind":"Name","value":"addedActions"}},{"kind":"Field","name":{"kind":"Name","value":"removedActions"}}]}}]}}]} as unknown as DocumentNode<SessionWorkflowSimulationFieldsFragment, unknown>;
export const SessionPublicationFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SessionPublicationFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionPublication"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"sessionProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sessionWorkflow"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflowSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"definitionRevision"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"definitionVersion"}},{"kind":"Field","name":{"kind":"Name","value":"productionProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"productionWorkflow"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceRunIds"}},{"kind":"Field","name":{"kind":"Name","value":"manifestDigest"}},{"kind":"Field","name":{"kind":"Name","value":"signatureAlgorithm"}},{"kind":"Field","name":{"kind":"Name","value":"signingKeyId"}},{"kind":"Field","name":{"kind":"Name","value":"publishedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]} as unknown as DocumentNode<SessionPublicationFieldsFragment, unknown>;
export const ProfileFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]} as unknown as DocumentNode<ProfileFieldsFragment, unknown>;
export const ToleranceFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ToleranceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ToleranceConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"missingSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nteAnywhere"}},{"kind":"Field","name":{"kind":"Name","value":"extraComponents"}},{"kind":"Field","name":{"kind":"Name","value":"unknownSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nonStandardDelimiters"}}]}}]} as unknown as DocumentNode<ToleranceFieldsFragment, unknown>;
export const Hl7v2ConfigFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HL7v2ConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HL7v2Config"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"defaultVersion"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"tolerance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ToleranceFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"eventClassifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"messageType"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ToleranceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ToleranceConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"missingSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nteAnywhere"}},{"kind":"Field","name":{"kind":"Name","value":"extraComponents"}},{"kind":"Field","name":{"kind":"Name","value":"unknownSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nonStandardDelimiters"}}]}}]} as unknown as DocumentNode<Hl7v2ConfigFieldsFragment, unknown>;
export const IdentifierConfigFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IdentifierConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"IdentifierConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assigningAuthorities"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"primaryIdPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"assignerContains"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"npi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"mbi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"ssn"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"normalization"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ssnStripDashes"}},{"kind":"Field","name":{"kind":"Name","value":"ssnRejectPatterns"}},{"kind":"Field","name":{"kind":"Name","value":"phoneNormalize"}},{"kind":"Field","name":{"kind":"Name","value":"phoneFormat"}}]}}]}}]} as unknown as DocumentNode<IdentifierConfigFieldsFragment, unknown>;
export const TerminologyConfigFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TerminologyConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TerminologyConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mappings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"entries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"display"}}]}}]}}]}}]} as unknown as DocumentNode<TerminologyConfigFieldsFragment, unknown>;
export const FullProfileFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FullProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}},{"kind":"Field","name":{"kind":"Name","value":"hl7v2"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HL7v2ConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"identifiers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"IdentifierConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"terminology"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TerminologyConfigFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ToleranceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ToleranceConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"missingSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nteAnywhere"}},{"kind":"Field","name":{"kind":"Name","value":"extraComponents"}},{"kind":"Field","name":{"kind":"Name","value":"unknownSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nonStandardDelimiters"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HL7v2ConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HL7v2Config"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"defaultVersion"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"tolerance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ToleranceFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"eventClassifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"messageType"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IdentifierConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"IdentifierConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assigningAuthorities"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"primaryIdPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"assignerContains"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"npi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"mbi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"ssn"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"normalization"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ssnStripDashes"}},{"kind":"Field","name":{"kind":"Name","value":"ssnRejectPatterns"}},{"kind":"Field","name":{"kind":"Name","value":"phoneNormalize"}},{"kind":"Field","name":{"kind":"Name","value":"phoneFormat"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TerminologyConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TerminologyConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mappings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"entries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"display"}}]}}]}}]}}]} as unknown as DocumentNode<FullProfileFieldsFragment, unknown>;
export const TemporalWorkflowFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TemporalWorkflowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TemporalWorkflow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowType"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"taskQueue"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"closeTime"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}}]}}]} as unknown as DocumentNode<TemporalWorkflowFieldsFragment, unknown>;
export const MappingFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<MappingFieldsFragment, unknown>;
export const BatchFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"BatchFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UploadBatch"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"totalRows"}},{"kind":"Field","name":{"kind":"Name","value":"validRows"}},{"kind":"Field","name":{"kind":"Name","value":"duplicateRows"}},{"kind":"Field","name":{"kind":"Name","value":"errorRows"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedAt"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedBy"}},{"kind":"Field","name":{"kind":"Name","value":"validationErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"row"}},{"kind":"Field","name":{"kind":"Name","value":"column"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]} as unknown as DocumentNode<BatchFieldsFragment, unknown>;
export const CandidateFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}}]} as unknown as DocumentNode<CandidateFieldsFragment, unknown>;
export const AutorouteTraceFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AutorouteTraceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AutorouteTrace"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"step"}},{"kind":"Field","name":{"kind":"Name","value":"result"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalDurationMs"}}]}}]} as unknown as DocumentNode<AutorouteTraceFieldsFragment, unknown>;
export const PendingAutorouteFieldsFragmentDoc = {"kind":"Document","definitions":[{"kind":"FragmentDefinition","name":{"kind":"Name","value":"PendingAutorouteFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PendingAutoroute"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedCode"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"rejectionReason"}},{"kind":"Field","name":{"kind":"Name","value":"alternates"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CandidateFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"decisionTrace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AutorouteTraceFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AutorouteTraceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AutorouteTrace"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"step"}},{"kind":"Field","name":{"kind":"Name","value":"result"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalDurationMs"}}]}}]} as unknown as DocumentNode<PendingAutorouteFieldsFragment, unknown>;
export const StartDebugSessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartDebugSession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartDebugSessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startDebugSession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"breakpoints"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}}]}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stepNumber"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"variables"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"spanName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<StartDebugSessionMutation, StartDebugSessionMutationVariables>;
export const DebugStepDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DebugStep"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugStep"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stepNumber"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"variables"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"spanName"}}]}}]}}]} as unknown as DocumentNode<DebugStepMutation, DebugStepMutationVariables>;
export const DebugContinueDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DebugContinue"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugContinue"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stepNumber"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"variables"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"spanName"}}]}}]}}]} as unknown as DocumentNode<DebugContinueMutation, DebugContinueMutationVariables>;
export const DebugSetBreakpointDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DebugSetBreakpoint"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SetBreakpointInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugSetBreakpoint"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}}]}}]}}]} as unknown as DocumentNode<DebugSetBreakpointMutation, DebugSetBreakpointMutationVariables>;
export const DebugRemoveBreakpointDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DebugRemoveBreakpoint"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"breakpointId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugRemoveBreakpoint"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}},{"kind":"Argument","name":{"kind":"Name","value":"breakpointId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"breakpointId"}}}]}]}}]} as unknown as DocumentNode<DebugRemoveBreakpointMutation, DebugRemoveBreakpointMutationVariables>;
export const DebugEndSessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DebugEndSession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugEndSession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}}]}]}}]} as unknown as DocumentNode<DebugEndSessionMutation, DebugEndSessionMutationVariables>;
export const LiveParseStreamDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"LiveParseStream"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"LiveParseInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"liveParseStream"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"segmentIndex"}},{"kind":"Field","name":{"kind":"Name","value":"segmentType"}},{"kind":"Field","name":{"kind":"Name","value":"rawSegment"}},{"kind":"Field","name":{"kind":"Name","value":"fields"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"isComplete"}}]}}]}}]} as unknown as DocumentNode<LiveParseStreamSubscription, LiveParseStreamSubscriptionVariables>;
export const DebugStepEventDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"DebugStepEvent"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugStepEvent"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stepNumber"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"variables"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"spanName"}}]}}]}}]} as unknown as DocumentNode<DebugStepEventSubscription, DebugStepEventSubscriptionVariables>;
export const DebugSessionQueryDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"DebugSessionQuery"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"debugSession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"breakpoints"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}}]}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"stepNumber"}},{"kind":"Field","name":{"kind":"Name","value":"kind"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"variables"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"spanName"}}]}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]} as unknown as DocumentNode<DebugSessionQueryQuery, DebugSessionQueryQueryVariables>;
export const WorkflowRunTraceDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"WorkflowRunTrace"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowRunTrace"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"parentId"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"attributes"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"attributes"}}]}}]}}]}}]} as unknown as DocumentNode<WorkflowRunTraceQuery, WorkflowRunTraceQueryVariables>;
export const EventStreamDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"EventStream"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"EventFilter"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventStream"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}}]}}]}}]} as unknown as DocumentNode<EventStreamSubscription, EventStreamSubscriptionVariables>;
export const WorkflowEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"WorkflowEvents"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"workflowName"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"workflowName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"workflowName"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflow"}},{"kind":"Field","name":{"kind":"Name","value":"routesMatched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsExecuted"}},{"kind":"Field","name":{"kind":"Name","value":"duration"}}]}}]}}]} as unknown as DocumentNode<WorkflowEventsSubscription, WorkflowEventsSubscriptionVariables>;
export const PatientEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"PatientEvents"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"mrn"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patientEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"mrn"},"value":{"kind":"Variable","name":{"kind":"Name","value":"mrn"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}}]}}]}}]} as unknown as DocumentNode<PatientEventsSubscription, PatientEventsSubscriptionVariables>;
export const EventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Events"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"EventFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"orderBy"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"EventOrderBy"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"events"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}},{"kind":"Argument","name":{"kind":"Name","value":"orderBy"},"value":{"kind":"Variable","name":{"kind":"Name","value":"orderBy"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"hasPreviousPage"}},{"kind":"Field","name":{"kind":"Name","value":"startCursor"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<EventsQuery, EventsQueryVariables>;
export const EventByIdDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"EventById"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"event"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}}]}}]}}]} as unknown as DocumentNode<EventByIdQuery, EventByIdQueryVariables>;
export const EventStatisticsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"EventStatistics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventStatistics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"totalEvents"}},{"kind":"Field","name":{"kind":"Name","value":"byType"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"count"}}]}},{"kind":"Field","name":{"kind":"Name","value":"bySource"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"count"}}]}}]}}]}}]} as unknown as DocumentNode<EventStatisticsQuery, EventStatisticsQueryVariables>;
export const PatientTimelineDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PatientTimeline"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"mrn"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"fromTimestamp"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"DateTime"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"toTimestamp"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"DateTime"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"limit"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patientTimeline"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"mrn"},"value":{"kind":"Variable","name":{"kind":"Name","value":"mrn"}}},{"kind":"Argument","name":{"kind":"Name","value":"fromTimestamp"},"value":{"kind":"Variable","name":{"kind":"Name","value":"fromTimestamp"}}},{"kind":"Argument","name":{"kind":"Name","value":"toTimestamp"},"value":{"kind":"Variable","name":{"kind":"Name","value":"toTimestamp"}}},{"kind":"Argument","name":{"kind":"Name","value":"limit"},"value":{"kind":"Variable","name":{"kind":"Name","value":"limit"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"position"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"streamId"}},{"kind":"Field","name":{"kind":"Name","value":"source"}}]}},{"kind":"Field","name":{"kind":"Name","value":"lastUpdated"}},{"kind":"Field","name":{"kind":"Name","value":"eventCount"}}]}}]}}]} as unknown as DocumentNode<PatientTimelineQuery, PatientTimelineQueryVariables>;
export const PatientsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Patients"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PatientFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patients"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"edges"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cursor"}},{"kind":"Field","name":{"kind":"Name","value":"node"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"middleName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}}]}}]}}]} as unknown as DocumentNode<PatientsQuery, PatientsQueryVariables>;
export const ExplainWarningsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExplainWarnings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"warnings"}},"type":{"kind":"NonNullType","type":{"kind":"ListType","type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ParseWarningInput"}}}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"format"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SourceFormat"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"explainWarnings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"warnings"},"value":{"kind":"Variable","name":{"kind":"Name","value":"warnings"}}},{"kind":"Argument","name":{"kind":"Name","value":"format"},"value":{"kind":"Variable","name":{"kind":"Name","value":"format"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]} as unknown as DocumentNode<ExplainWarningsQuery, ExplainWarningsQueryVariables>;
export const HealthDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Health"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"health"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}}]}}]} as unknown as DocumentNode<HealthQuery, HealthQueryVariables>;
export const PreviewIntegrationMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PreviewIntegrationMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PreviewIntegrationMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"previewIntegrationMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mode"}},{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"integrationRevision"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"artifactRevisions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"source"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"profile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflow"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"sourceMessageId"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"Field","name":{"kind":"Name","value":"classification"}},{"kind":"Field","name":{"kind":"Name","value":"payload"}}]}},{"kind":"Field","name":{"kind":"Name","value":"diagnostics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"stage"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"classification"}}]}},{"kind":"Field","name":{"kind":"Name","value":"routes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"route"}},{"kind":"Field","name":{"kind":"Name","value":"matched"}},{"kind":"Field","name":{"kind":"Name","value":"skipped"}},{"kind":"Field","name":{"kind":"Name","value":"skipReason"}},{"kind":"Field","name":{"kind":"Name","value":"transformCount"}},{"kind":"Field","name":{"kind":"Name","value":"plannedActions"}},{"kind":"Field","name":{"kind":"Name","value":"diagnosticCodes"}}]}},{"kind":"Field","name":{"kind":"Name","value":"deliveries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"destination"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}},{"kind":"Field","name":{"kind":"Name","value":"class"}}]}},{"kind":"Field","name":{"kind":"Name","value":"route"}},{"kind":"Field","name":{"kind":"Name","value":"action"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"diagnosticCodes"}}]}},{"kind":"Field","name":{"kind":"Name","value":"correlations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"tenantId"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"sourceMessageId"}},{"kind":"Field","name":{"kind":"Name","value":"eventIds"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRunId"}}]}}]}}]}}]} as unknown as DocumentNode<PreviewIntegrationMessageMutation, PreviewIntegrationMessageMutationVariables>;
export const CreateStreamingIntegrationSessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateStreamingIntegrationSession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateIntegrationSessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createIntegrationSession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}}]}}]}}]} as unknown as DocumentNode<CreateStreamingIntegrationSessionMutation, CreateStreamingIntegrationSessionMutationVariables>;
export const AddStreamingSessionSampleDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"AddStreamingSessionSample"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AddSessionSampleInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"addSessionSample"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}}]}}]}}]} as unknown as DocumentNode<AddStreamingSessionSampleMutation, AddStreamingSessionSampleMutationVariables>;
export const UpdateStreamingSessionProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateStreamingSessionProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateSessionArtifactInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateSessionProfileDraft"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}}]}}]} as unknown as DocumentNode<UpdateStreamingSessionProfileMutation, UpdateStreamingSessionProfileMutationVariables>;
export const RunStreamingSessionPreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RunStreamingSessionPreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RunSessionPreviewInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"runSessionPreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"IntegrationSessionRunFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IntegrationSessionRunFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionRun"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stages"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}}]}},{"kind":"Field","name":{"kind":"Name","value":"diagnostics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"accepted"}},{"kind":"Field","name":{"kind":"Name","value":"acceptedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientAdmitEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientDischargeEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LabResultEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"test"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"localCode"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"result"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isCritical"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AppointmentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appointment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"npi"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DocumentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"documentType"}},{"kind":"Field","name":{"kind":"Name","value":"title"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]} as unknown as DocumentNode<RunStreamingSessionPreviewMutation, RunStreamingSessionPreviewMutationVariables>;
export const StreamIntegrationSessionEventsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"subscription","name":{"kind":"Name","value":"StreamIntegrationSessionEvents"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"integrationSessionEvents"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sessionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sessionId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"run"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"IntegrationSessionRunFields"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IntegrationSessionRunFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionRun"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"stages"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"completedAt"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}}]}},{"kind":"Field","name":{"kind":"Name","value":"diagnostics"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"sampleId"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"accepted"}},{"kind":"Field","name":{"kind":"Name","value":"acceptedAt"}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"lineage"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourcePath"}},{"kind":"Field","name":{"kind":"Name","value":"targetPath"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientAdmitEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientDischargeEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LabResultEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"test"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"localCode"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"result"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isCritical"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AppointmentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appointment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"npi"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DocumentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"documentType"}},{"kind":"Field","name":{"kind":"Name","value":"title"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]} as unknown as DocumentNode<StreamIntegrationSessionEventsSubscription, StreamIntegrationSessionEventsSubscriptionVariables>;
export const ListWorkflowSimulationSessionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListWorkflowSimulationSessions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"integrationSessions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"archived"}},{"kind":"Field","name":{"kind":"Name","value":"runs"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"profileRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflowSimulations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"sourceRunIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]}}]}}]} as unknown as DocumentNode<ListWorkflowSimulationSessionsQuery, ListWorkflowSimulationSessionsQueryVariables>;
export const SaveSessionWorkflowDraftDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SaveSessionWorkflowDraft"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateSessionArtifactInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateSessionWorkflowDraft"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}}]}}]} as unknown as DocumentNode<SaveSessionWorkflowDraftMutation, SaveSessionWorkflowDraftMutationVariables>;
export const SimulateSessionWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SimulateSessionWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SimulateSessionWorkflowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"simulateSessionWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SessionWorkflowSimulationFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SessionWorkflowSimulationFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionWorkflowSimulation"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowArtifactId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowRevisionDigest"}},{"kind":"Field","name":{"kind":"Name","value":"sourceRunIds"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"routes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"matched"}},{"kind":"Field","name":{"kind":"Name","value":"skipReason"}},{"kind":"Field","name":{"kind":"Name","value":"diagnosticCodes"}},{"kind":"Field","name":{"kind":"Name","value":"transforms"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"index"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"actions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"destinationArtifactId"}}]}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"delta"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"baselineSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"candidateSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"addedEvents"}},{"kind":"Field","name":{"kind":"Name","value":"removedEvents"}},{"kind":"Field","name":{"kind":"Name","value":"addedMatchedRoutes"}},{"kind":"Field","name":{"kind":"Name","value":"removedMatchedRoutes"}},{"kind":"Field","name":{"kind":"Name","value":"addedTransforms"}},{"kind":"Field","name":{"kind":"Name","value":"removedTransforms"}},{"kind":"Field","name":{"kind":"Name","value":"addedActions"}},{"kind":"Field","name":{"kind":"Name","value":"removedActions"}}]}}]}}]} as unknown as DocumentNode<SimulateSessionWorkflowMutation, SimulateSessionWorkflowMutationVariables>;
export const PublishIntegrationSessionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PublishIntegrationSession"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PublishIntegrationSessionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"publishIntegrationSession"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"SessionPublicationFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"SessionPublicationFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SessionPublication"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sessionId"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"sessionProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sessionWorkflow"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflowSimulationId"}},{"kind":"Field","name":{"kind":"Name","value":"definitionRevision"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"definitionVersion"}},{"kind":"Field","name":{"kind":"Name","value":"productionProfile"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"productionWorkflow"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"sourceRunIds"}},{"kind":"Field","name":{"kind":"Name","value":"manifestDigest"}},{"kind":"Field","name":{"kind":"Name","value":"signatureAlgorithm"}},{"kind":"Field","name":{"kind":"Name","value":"signingKeyId"}},{"kind":"Field","name":{"kind":"Name","value":"publishedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}}]}}]} as unknown as DocumentNode<PublishIntegrationSessionMutation, PublishIntegrationSessionMutationVariables>;
export const ApproveSessionPublicationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApproveSessionPublication"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PromoteSessionPublicationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approveSessionPublication"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"definitionRevision"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"releaseId"}},{"kind":"Field","name":{"kind":"Name","value":"health"}}]}}]}}]} as unknown as DocumentNode<ApproveSessionPublicationMutation, ApproveSessionPublicationMutationVariables>;
export const DeploySessionPublicationDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeploySessionPublication"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PromoteSessionPublicationInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deploySessionPublication"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"definitionRevision"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"artifactId"}},{"kind":"Field","name":{"kind":"Name","value":"revisionId"}},{"kind":"Field","name":{"kind":"Name","value":"digest"}}]}},{"kind":"Field","name":{"kind":"Name","value":"state"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"releaseId"}},{"kind":"Field","name":{"kind":"Name","value":"health"}}]}}]}}]} as unknown as DocumentNode<DeploySessionPublicationMutation, DeploySessionPublicationMutationVariables>;
export const ExtractEntitiesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExtractEntities"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ExtractEntitiesInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"extractEntities"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"conditions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"codeSystem"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"negated"}},{"kind":"Field","name":{"kind":"Name","value":"textSpan"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"medications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"codeSystem"}},{"kind":"Field","name":{"kind":"Name","value":"dose"}},{"kind":"Field","name":{"kind":"Name","value":"route"}},{"kind":"Field","name":{"kind":"Name","value":"frequency"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"negated"}},{"kind":"Field","name":{"kind":"Name","value":"textSpan"}}]}},{"kind":"Field","name":{"kind":"Name","value":"vitalSigns"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"interpretation"}},{"kind":"Field","name":{"kind":"Name","value":"textSpan"}}]}},{"kind":"Field","name":{"kind":"Name","value":"allergies"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"substance"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"codeSystem"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"reaction"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"negated"}},{"kind":"Field","name":{"kind":"Name","value":"textSpan"}}]}},{"kind":"Field","name":{"kind":"Name","value":"procedures"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"codeSystem"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"negated"}},{"kind":"Field","name":{"kind":"Name","value":"textSpan"}}]}},{"kind":"Field","name":{"kind":"Name","value":"overallConfidence"}},{"kind":"Field","name":{"kind":"Name","value":"processingTimeMs"}},{"kind":"Field","name":{"kind":"Name","value":"model"}}]}}]}}]} as unknown as DocumentNode<ExtractEntitiesQuery, ExtractEntitiesQueryVariables>;
export const AnalyzeQualityDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"AnalyzeQuality"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"AnalyzeQualityInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"analyzeQuality"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"overallScore"}},{"kind":"Field","name":{"kind":"Name","value":"dimensions"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"completeness"}},{"kind":"Field","name":{"kind":"Name","value":"accuracy"}},{"kind":"Field","name":{"kind":"Name","value":"consistency"}},{"kind":"Field","name":{"kind":"Name","value":"conformance"}},{"kind":"Field","name":{"kind":"Name","value":"timeliness"}}]}},{"kind":"Field","name":{"kind":"Name","value":"issues"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dimension"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"field"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"actualValue"}},{"kind":"Field","name":{"kind":"Name","value":"expectedValue"}}]}},{"kind":"Field","name":{"kind":"Name","value":"recommendations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"priority"}},{"kind":"Field","name":{"kind":"Name","value":"category"}},{"kind":"Field","name":{"kind":"Name","value":"title"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}}]}},{"kind":"Field","name":{"kind":"Name","value":"processingTimeMs"}},{"kind":"Field","name":{"kind":"Name","value":"model"}}]}}]}}]} as unknown as DocumentNode<AnalyzeQualityQuery, AnalyzeQualityQueryVariables>;
export const GenerateWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"GenerateWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"GenerateWorkflowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"generateWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"yaml"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}}]}}]}}]} as unknown as DocumentNode<GenerateWorkflowMutation, GenerateWorkflowMutationVariables>;
export const ExplainWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExplainWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ExplainWorkflowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"explainWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"summary"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"routeExplanations"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"trigger"}},{"kind":"Field","name":{"kind":"Name","value":"actions"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"diagram"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}}]}}]}}]} as unknown as DocumentNode<ExplainWorkflowQuery, ExplainWorkflowQueryVariables>;
export const LlmCapabilityDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LlmCapability"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"llmCapability"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"configured"}},{"kind":"Field","name":{"kind":"Name","value":"providerBaseURLHost"}},{"kind":"Field","name":{"kind":"Name","value":"defaultModel"}},{"kind":"Field","name":{"kind":"Name","value":"qualityModel"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"features"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"model"}}]}}]}}]}}]} as unknown as DocumentNode<LlmCapabilityQuery, LlmCapabilityQueryVariables>;
export const ClassifyMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ClassifyMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ClassifyMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"classifyMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"messageType"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedTags"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"summary"}}]}}]}}]} as unknown as DocumentNode<ClassifyMessageQuery, ClassifyMessageQueryVariables>;
export const ParsePreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ParsePreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"format"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SourceFormat"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"data"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"source"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"parsePreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"format"},"value":{"kind":"Variable","name":{"kind":"Name","value":"format"}}},{"kind":"Argument","name":{"kind":"Name","value":"data"},"value":{"kind":"Variable","name":{"kind":"Name","value":"data"}}},{"kind":"Argument","name":{"kind":"Name","value":"source"},"value":{"kind":"Variable","name":{"kind":"Name","value":"source"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"success"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientAdmitEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientDischargeEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LabResultEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"test"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"localCode"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"result"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isCritical"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AppointmentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appointment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"npi"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DocumentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"documentType"}},{"kind":"Field","name":{"kind":"Name","value":"title"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]}}]} as unknown as DocumentNode<ParsePreviewQuery, ParsePreviewQueryVariables>;
export const ParsePreviewWithProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ParsePreviewWithProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"format"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SourceFormat"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"data"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"source"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"profileId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"parsePreviewWithProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"format"},"value":{"kind":"Variable","name":{"kind":"Name","value":"format"}}},{"kind":"Argument","name":{"kind":"Name","value":"data"},"value":{"kind":"Variable","name":{"kind":"Name","value":"data"}}},{"kind":"Argument","name":{"kind":"Name","value":"source"},"value":{"kind":"Variable","name":{"kind":"Name","value":"source"}}},{"kind":"Argument","name":{"kind":"Name","value":"profileId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"profileId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"success"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientAdmitEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PatientDischargeEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"encounter"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"class"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"LabResultEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"test"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"loincCode"}},{"kind":"Field","name":{"kind":"Name","value":"localCode"}},{"kind":"Field","name":{"kind":"Name","value":"description"}}]}},{"kind":"Field","name":{"kind":"Name","value":"result"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"value"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}},{"kind":"Field","name":{"kind":"Name","value":"isCritical"}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AppointmentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"patient"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mrn"}},{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"dateOfBirth"}},{"kind":"Field","name":{"kind":"Name","value":"gender"}}]}},{"kind":"Field","name":{"kind":"Name","value":"appointment"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"endTime"}},{"kind":"Field","name":{"kind":"Name","value":"reason"}},{"kind":"Field","name":{"kind":"Name","value":"location"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"facility"}},{"kind":"Field","name":{"kind":"Name","value":"unit"}},{"kind":"Field","name":{"kind":"Name","value":"room"}},{"kind":"Field","name":{"kind":"Name","value":"bed"}}]}},{"kind":"Field","name":{"kind":"Name","value":"provider"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"familyName"}},{"kind":"Field","name":{"kind":"Name","value":"givenName"}},{"kind":"Field","name":{"kind":"Name","value":"npi"}}]}}]}}]}},{"kind":"InlineFragment","typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"DocumentEvent"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"documentType"}},{"kind":"Field","name":{"kind":"Name","value":"title"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"explanation"}},{"kind":"Field","name":{"kind":"Name","value":"fixSuggestion"}},{"kind":"Field","name":{"kind":"Name","value":"impact"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}},{"kind":"Field","name":{"kind":"Name","value":"fromCache"}}]}}]}}]}}]} as unknown as DocumentNode<ParsePreviewWithProfileQuery, ParsePreviewWithProfileQueryVariables>;
export const ListProfilesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListProfiles"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"activeOnly"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Boolean"}},"defaultValue":{"kind":"BooleanValue","value":true}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profiles"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"activeOnly"},"value":{"kind":"Variable","name":{"kind":"Name","value":"activeOnly"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]} as unknown as DocumentNode<ListProfilesQuery, ListProfilesQueryVariables>;
export const GetProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FullProfileFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ToleranceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ToleranceConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"missingSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nteAnywhere"}},{"kind":"Field","name":{"kind":"Name","value":"extraComponents"}},{"kind":"Field","name":{"kind":"Name","value":"unknownSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nonStandardDelimiters"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HL7v2ConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HL7v2Config"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"defaultVersion"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"tolerance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ToleranceFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"eventClassifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"messageType"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IdentifierConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"IdentifierConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assigningAuthorities"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"primaryIdPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"assignerContains"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"npi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"mbi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"ssn"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"normalization"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ssnStripDashes"}},{"kind":"Field","name":{"kind":"Name","value":"ssnRejectPatterns"}},{"kind":"Field","name":{"kind":"Name","value":"phoneNormalize"}},{"kind":"Field","name":{"kind":"Name","value":"phoneFormat"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TerminologyConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TerminologyConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mappings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"entries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"display"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FullProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}},{"kind":"Field","name":{"kind":"Name","value":"hl7v2"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HL7v2ConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"identifiers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"IdentifierConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"terminology"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TerminologyConfigFields"}}]}}]}}]} as unknown as DocumentNode<GetProfileQuery, GetProfileQueryVariables>;
export const GetProfileRevisionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetProfileRevisions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"profileRevisions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"changeSummary"}}]}}]}}]} as unknown as DocumentNode<GetProfileRevisionsQuery, GetProfileRevisionsQueryVariables>;
export const CreateProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateProfileInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]} as unknown as DocumentNode<CreateProfileMutation, CreateProfileMutationVariables>;
export const UpdateProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateProfileInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"FullProfileFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ToleranceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"ToleranceConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"missingSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nteAnywhere"}},{"kind":"Field","name":{"kind":"Name","value":"extraComponents"}},{"kind":"Field","name":{"kind":"Name","value":"unknownSegments"}},{"kind":"Field","name":{"kind":"Name","value":"nonStandardDelimiters"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"HL7v2ConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"HL7v2Config"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"defaultVersion"}},{"kind":"Field","name":{"kind":"Name","value":"timezone"}},{"kind":"Field","name":{"kind":"Name","value":"tolerance"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ToleranceFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"eventClassifications"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"messageType"}},{"kind":"Field","name":{"kind":"Name","value":"condition"}},{"kind":"Field","name":{"kind":"Name","value":"eventType"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"IdentifierConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"IdentifierConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"assigningAuthorities"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"name"}}]}},{"kind":"Field","name":{"kind":"Name","value":"primaryIdPreference"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"assignerContains"}},{"kind":"Field","name":{"kind":"Name","value":"priority"}}]}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"npi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"mbi"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}},{"kind":"Field","name":{"kind":"Name","value":"ssn"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"onInvalid"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"normalization"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"ssnStripDashes"}},{"kind":"Field","name":{"kind":"Name","value":"ssnRejectPatterns"}},{"kind":"Field","name":{"kind":"Name","value":"phoneNormalize"}},{"kind":"Field","name":{"kind":"Name","value":"phoneFormat"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TerminologyConfigFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TerminologyConfig"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"mappings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"entries"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"display"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"FullProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}},{"kind":"Field","name":{"kind":"Name","value":"hl7v2"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"HL7v2ConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"identifiers"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"IdentifierConfigFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"terminology"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TerminologyConfigFields"}}]}}]}}]} as unknown as DocumentNode<UpdateProfileMutation, UpdateProfileMutationVariables>;
export const DeleteProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteProfileMutation, DeleteProfileMutationVariables>;
export const DuplicateProfileDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DuplicateProfile"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"newId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"newName"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"duplicateProfile"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}},{"kind":"Argument","name":{"kind":"Name","value":"newId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"newId"}}},{"kind":"Argument","name":{"kind":"Name","value":"newName"},"value":{"kind":"Variable","name":{"kind":"Name","value":"newName"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"ProfileFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"ProfileFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"SourceProfile"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"version"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"isActive"}}]}}]} as unknown as DocumentNode<DuplicateProfileMutation, DuplicateProfileMutationVariables>;
export const SubmitMessageDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SubmitMessage"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SubmitMessageInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"submitMessage"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"success"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}},{"kind":"Field","name":{"kind":"Name","value":"severity"}}]}},{"kind":"Field","name":{"kind":"Name","value":"workflowResults"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowName"}},{"kind":"Field","name":{"kind":"Name","value":"routesMatched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsExecuted"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"duration"}}]}}]}}]}}]} as unknown as DocumentNode<SubmitMessageMutation, SubmitMessageMutationVariables>;
export const ListTemporalWorkflowsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListTemporalWorkflows"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"TemporalWorkflowFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"first"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"Int"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"after"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"temporalWorkflows"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"first"},"value":{"kind":"Variable","name":{"kind":"Name","value":"first"}}},{"kind":"Argument","name":{"kind":"Name","value":"after"},"value":{"kind":"Variable","name":{"kind":"Name","value":"after"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TemporalWorkflowFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"hasPreviousPage"}},{"kind":"Field","name":{"kind":"Name","value":"endCursor"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TemporalWorkflowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TemporalWorkflow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowType"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"taskQueue"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"closeTime"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}}]}}]} as unknown as DocumentNode<ListTemporalWorkflowsQuery, ListTemporalWorkflowsQueryVariables>;
export const GetTemporalWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetTemporalWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"runId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"temporalWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"workflowId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}}},{"kind":"Argument","name":{"kind":"Name","value":"runId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"runId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"TemporalWorkflowFields"}},{"kind":"Field","name":{"kind":"Name","value":"input"}},{"kind":"Field","name":{"kind":"Name","value":"result"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"TemporalWorkflowFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"TemporalWorkflow"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"workflowType"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"taskQueue"}},{"kind":"Field","name":{"kind":"Name","value":"startTime"}},{"kind":"Field","name":{"kind":"Name","value":"closeTime"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}}]}}]} as unknown as DocumentNode<GetTemporalWorkflowQuery, GetTemporalWorkflowQueryVariables>;
export const CancelTemporalWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CancelTemporalWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"reason"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"cancelTemporalWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"workflowId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}}},{"kind":"Argument","name":{"kind":"Name","value":"reason"},"value":{"kind":"Variable","name":{"kind":"Name","value":"reason"}}}]}]}}]} as unknown as DocumentNode<CancelTemporalWorkflowMutation, CancelTemporalWorkflowMutationVariables>;
export const SignalReviewDecisionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SignalReviewDecision"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SignalReviewDecisionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"signalReviewDecision"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<SignalReviewDecisionMutation, SignalReviewDecisionMutationVariables>;
export const ListMappingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListMappings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ListMappingsInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"listMappings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"hasPreviousPage"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<ListMappingsQuery, ListMappingsQueryVariables>;
export const GetMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"getMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<GetMappingQuery, GetMappingQueryVariables>;
export const LookupMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"LookupMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sourceSystem"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"sourceCode"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"targetSystem"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"profileId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"lookupMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"sourceSystem"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sourceSystem"}}},{"kind":"Argument","name":{"kind":"Name","value":"sourceCode"},"value":{"kind":"Variable","name":{"kind":"Name","value":"sourceCode"}}},{"kind":"Argument","name":{"kind":"Name","value":"targetSystem"},"value":{"kind":"Variable","name":{"kind":"Name","value":"targetSystem"}}},{"kind":"Argument","name":{"kind":"Name","value":"profileId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"profileId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<LookupMappingQuery, LookupMappingQueryVariables>;
export const GetUploadBatchDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetUploadBatch"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"getUploadBatch"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"BatchFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"BatchFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UploadBatch"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"totalRows"}},{"kind":"Field","name":{"kind":"Name","value":"validRows"}},{"kind":"Field","name":{"kind":"Name","value":"duplicateRows"}},{"kind":"Field","name":{"kind":"Name","value":"errorRows"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedAt"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedBy"}},{"kind":"Field","name":{"kind":"Name","value":"validationErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"row"}},{"kind":"Field","name":{"kind":"Name","value":"column"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}}]} as unknown as DocumentNode<GetUploadBatchQuery, GetUploadBatchQueryVariables>;
export const UploadMappingCsvDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UploadMappingCSV"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UploadMappingCSVInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"uploadMappingCSV"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"batch"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"BatchFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"mappingsCreated"}},{"kind":"Field","name":{"kind":"Name","value":"mappingsSkipped"}},{"kind":"Field","name":{"kind":"Name","value":"preview"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"BatchFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"UploadBatch"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"filename"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"totalRows"}},{"kind":"Field","name":{"kind":"Name","value":"validRows"}},{"kind":"Field","name":{"kind":"Name","value":"duplicateRows"}},{"kind":"Field","name":{"kind":"Name","value":"errorRows"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedAt"}},{"kind":"Field","name":{"kind":"Name","value":"uploadedBy"}},{"kind":"Field","name":{"kind":"Name","value":"validationErrors"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"row"}},{"kind":"Field","name":{"kind":"Name","value":"column"}},{"kind":"Field","name":{"kind":"Name","value":"message"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<UploadMappingCsvMutation, UploadMappingCsvMutationVariables>;
export const CreateMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateMappingInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<CreateMappingMutation, CreateMappingMutationVariables>;
export const DeleteMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}]}]}}]} as unknown as DocumentNode<DeleteMappingMutation, DeleteMappingMutationVariables>;
export const DeleteMappingBatchDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DeleteMappingBatch"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"batchId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"deleteMappingBatch"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"batchId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"batchId"}}}]}]}}]} as unknown as DocumentNode<DeleteMappingBatchMutation, DeleteMappingBatchMutationVariables>;
export const UpdateMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"UpdateMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"UpdateMappingInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"updateMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<UpdateMappingMutation, UpdateMappingMutationVariables>;
export const ExportMappingsCsvDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ExportMappingsCSV"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ListMappingsInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"exportMappingsCSV"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<ExportMappingsCsvQuery, ExportMappingsCsvQueryVariables>;
export const ResolveMappingDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ResolveMapping"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ResolveMappingInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"resolveMapping"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"found"}},{"kind":"Field","name":{"kind":"Name","value":"decision"}},{"kind":"Field","name":{"kind":"Name","value":"mapping"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"candidates"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CandidateFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"trace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AutorouteTraceFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AutorouteTraceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AutorouteTrace"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"step"}},{"kind":"Field","name":{"kind":"Name","value":"result"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalDurationMs"}}]}}]} as unknown as DocumentNode<ResolveMappingQuery, ResolveMappingQueryVariables>;
export const SuggestMappingsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"SuggestMappings"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SuggestMappingsInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"suggestMappings"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CandidateFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}}]} as unknown as DocumentNode<SuggestMappingsQuery, SuggestMappingsQueryVariables>;
export const ListPendingAutoroutesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListPendingAutoroutes"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"ListPendingAutoroutesInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"listPendingAutoroutes"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"nodes"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"PendingAutorouteFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalCount"}},{"kind":"Field","name":{"kind":"Name","value":"pageInfo"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"hasNextPage"}},{"kind":"Field","name":{"kind":"Name","value":"hasPreviousPage"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AutorouteTraceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AutorouteTrace"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"step"}},{"kind":"Field","name":{"kind":"Name","value":"result"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalDurationMs"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"PendingAutorouteFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PendingAutoroute"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedCode"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"rejectionReason"}},{"kind":"Field","name":{"kind":"Name","value":"alternates"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CandidateFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"decisionTrace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AutorouteTraceFields"}}]}}]}}]} as unknown as DocumentNode<ListPendingAutoroutesQuery, ListPendingAutoroutesQueryVariables>;
export const GetPendingAutorouteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetPendingAutoroute"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"getPendingAutoroute"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"PendingAutorouteFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"CandidateFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"MappingCandidate"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"display"}},{"kind":"Field","name":{"kind":"Name","value":"system"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"score"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"AutorouteTraceFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"AutorouteTrace"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"traceId"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"steps"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"step"}},{"kind":"Field","name":{"kind":"Name","value":"result"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"metadata"}}]}},{"kind":"Field","name":{"kind":"Name","value":"totalDurationMs"}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"PendingAutorouteFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"PendingAutoroute"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedCode"}},{"kind":"Field","name":{"kind":"Name","value":"suggestedDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"reasoning"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"expiresAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"rejectionReason"}},{"kind":"Field","name":{"kind":"Name","value":"alternates"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"CandidateFields"}}]}},{"kind":"Field","name":{"kind":"Name","value":"decisionTrace"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"AutorouteTraceFields"}}]}}]}}]} as unknown as DocumentNode<GetPendingAutorouteQuery, GetPendingAutorouteQueryVariables>;
export const PendingAutorouteStatsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"PendingAutorouteStats"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"pendingAutorouteStats"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"pendingCount"}},{"kind":"Field","name":{"kind":"Name","value":"approvedCount"}},{"kind":"Field","name":{"kind":"Name","value":"rejectedCount"}},{"kind":"Field","name":{"kind":"Name","value":"expiredCount"}},{"kind":"Field","name":{"kind":"Name","value":"avgConfidence"}}]}}]}}]} as unknown as DocumentNode<PendingAutorouteStatsQuery, PendingAutorouteStatsQueryVariables>;
export const ApprovePendingAutorouteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApprovePendingAutoroute"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ApprovePendingAutorouteInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approvePendingAutoroute"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<ApprovePendingAutorouteMutation, ApprovePendingAutorouteMutationVariables>;
export const RejectPendingAutorouteDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RejectPendingAutoroute"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RejectPendingAutorouteInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rejectPendingAutoroute"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}]}]}}]} as unknown as DocumentNode<RejectPendingAutorouteMutation, RejectPendingAutorouteMutationVariables>;
export const BulkApprovePendingAutoroutesDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"BulkApprovePendingAutoroutes"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"BulkApproveInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"bulkApprovePendingAutoroutes"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approved"}},{"kind":"Field","name":{"kind":"Name","value":"skipped"}},{"kind":"Field","name":{"kind":"Name","value":"mappings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"FragmentSpread","name":{"kind":"Name","value":"MappingFields"}}]}}]}}]}},{"kind":"FragmentDefinition","name":{"kind":"Name","value":"MappingFields"},"typeCondition":{"kind":"NamedType","name":{"kind":"Name","value":"CodeMapping"}},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"sourceSystem"}},{"kind":"Field","name":{"kind":"Name","value":"sourceCode"}},{"kind":"Field","name":{"kind":"Name","value":"sourceDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"targetSystem"}},{"kind":"Field","name":{"kind":"Name","value":"targetCode"}},{"kind":"Field","name":{"kind":"Name","value":"targetDisplay"}},{"kind":"Field","name":{"kind":"Name","value":"equivalence"}},{"kind":"Field","name":{"kind":"Name","value":"confidence"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}},{"kind":"Field","name":{"kind":"Name","value":"origin"}},{"kind":"Field","name":{"kind":"Name","value":"profileId"}},{"kind":"Field","name":{"kind":"Name","value":"uploadBatchId"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}}]}}]} as unknown as DocumentNode<BulkApprovePendingAutoroutesMutation, BulkApprovePendingAutoroutesMutationVariables>;
export const StartTerminologyReviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"StartTerminologyReview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"StartTerminologyReviewInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"startTerminologyReview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"started"}}]}}]}}]} as unknown as DocumentNode<StartTerminologyReviewMutation, StartTerminologyReviewMutationVariables>;
export const ListWorkflowsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListWorkflows"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflows"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"routeCount"}},{"kind":"Field","name":{"kind":"Name","value":"eventsProcessed"}},{"kind":"Field","name":{"kind":"Name","value":"lastEventTime"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}}]}}]}}]} as unknown as DocumentNode<ListWorkflowsQuery, ListWorkflowsQueryVariables>;
export const ListWorkflowDefinitionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListWorkflowDefinitions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"WorkflowDefinitionFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paging"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PagingInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowDefinitions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"paging"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paging"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"latestVersion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"versionNumber"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"notes"}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"info"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"publishedVersionsByEnv"}}]}}]}}]} as unknown as DocumentNode<ListWorkflowDefinitionsQuery, ListWorkflowDefinitionsQueryVariables>;
export const GetWorkflowVersionsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetWorkflowVersions"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paging"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PagingInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowVersions"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"workflowId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"workflowId"}}},{"kind":"Argument","name":{"kind":"Name","value":"paging"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paging"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"versionNumber"}},{"kind":"Field","name":{"kind":"Name","value":"yaml"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"notes"}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"info"}}]}}]}}]}}]} as unknown as DocumentNode<GetWorkflowVersionsQuery, GetWorkflowVersionsQueryVariables>;
export const GetWorkflowVersionByIdDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetWorkflowVersionById"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"versionNumber"}},{"kind":"Field","name":{"kind":"Name","value":"yaml"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"notes"}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"info"}}]}}]}}]}}]} as unknown as DocumentNode<GetWorkflowVersionByIdQuery, GetWorkflowVersionByIdQueryVariables>;
export const ListWorkflowRunsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListWorkflowRuns"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"WorkflowRunFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paging"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PagingInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowRuns"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"paging"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paging"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowName"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"versionId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"routesMatched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsExecuted"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<ListWorkflowRunsQuery, ListWorkflowRunsQueryVariables>;
export const GetWorkflowRunDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetWorkflowRun"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"id"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ID"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowRun"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"id"},"value":{"kind":"Variable","name":{"kind":"Name","value":"id"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowName"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"versionId"}},{"kind":"Field","name":{"kind":"Name","value":"eventId"}},{"kind":"Field","name":{"kind":"Name","value":"routesMatched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsExecuted"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"durationMs"}},{"kind":"Field","name":{"kind":"Name","value":"startedAt"}},{"kind":"Field","name":{"kind":"Name","value":"status"}}]}}]}}]} as unknown as DocumentNode<GetWorkflowRunQuery, GetWorkflowRunQueryVariables>;
export const ListWorkflowApprovalRequestsDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ListWorkflowApprovalRequests"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"filter"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"WorkflowApprovalRequestFilter"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"paging"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"PagingInput"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowApprovalRequests"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"filter"},"value":{"kind":"Variable","name":{"kind":"Name","value":"filter"}}},{"kind":"Argument","name":{"kind":"Name","value":"paging"},"value":{"kind":"Variable","name":{"kind":"Name","value":"paging"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"targetVersionId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"requestedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}}]}}]}}]} as unknown as DocumentNode<ListWorkflowApprovalRequestsQuery, ListWorkflowApprovalRequestsQueryVariables>;
export const GetWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"GetWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"enabled"}},{"kind":"Field","name":{"kind":"Name","value":"routeCount"}},{"kind":"Field","name":{"kind":"Name","value":"eventsProcessed"}},{"kind":"Field","name":{"kind":"Name","value":"lastEventTime"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}}]}}]}}]} as unknown as DocumentNode<GetWorkflowQuery, GetWorkflowQueryVariables>;
export const CreateWorkflowDefinitionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"CreateWorkflowDefinition"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"CreateWorkflowDefinitionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"createWorkflowDefinition"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"name"}},{"kind":"Field","name":{"kind":"Name","value":"description"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"updatedAt"}},{"kind":"Field","name":{"kind":"Name","value":"latestVersion"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"versionNumber"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"notes"}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"info"}}]}}]}},{"kind":"Field","name":{"kind":"Name","value":"publishedVersionsByEnv"}}]}}]}}]} as unknown as DocumentNode<CreateWorkflowDefinitionMutation, CreateWorkflowDefinitionMutationVariables>;
export const SaveWorkflowVersionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"SaveWorkflowVersion"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SaveWorkflowVersionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"saveWorkflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"versionNumber"}},{"kind":"Field","name":{"kind":"Name","value":"yaml"}},{"kind":"Field","name":{"kind":"Name","value":"createdBy"}},{"kind":"Field","name":{"kind":"Name","value":"createdAt"}},{"kind":"Field","name":{"kind":"Name","value":"notes"}},{"kind":"Field","name":{"kind":"Name","value":"validation"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"valid"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"info"}}]}}]}}]}}]} as unknown as DocumentNode<SaveWorkflowVersionMutation, SaveWorkflowVersionMutationVariables>;
export const PublishWorkflowVersionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"PublishWorkflowVersion"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"PublishWorkflowVersionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"publishWorkflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"versionId"}},{"kind":"Field","name":{"kind":"Name","value":"publishedBy"}},{"kind":"Field","name":{"kind":"Name","value":"publishedAt"}},{"kind":"Field","name":{"kind":"Name","value":"rollbackFromReleaseId"}}]}}]}}]} as unknown as DocumentNode<PublishWorkflowVersionMutation, PublishWorkflowVersionMutationVariables>;
export const RollbackWorkflowVersionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RollbackWorkflowVersion"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RollbackWorkflowVersionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rollbackWorkflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"versionId"}},{"kind":"Field","name":{"kind":"Name","value":"publishedBy"}},{"kind":"Field","name":{"kind":"Name","value":"publishedAt"}},{"kind":"Field","name":{"kind":"Name","value":"rollbackFromReleaseId"}}]}}]}}]} as unknown as DocumentNode<RollbackWorkflowVersionMutation, RollbackWorkflowVersionMutationVariables>;
export const RequestWorkflowApprovalDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RequestWorkflowApproval"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RequestWorkflowApprovalInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"requestWorkflowApproval"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"targetVersionId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"requestedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}}]}}]}}]} as unknown as DocumentNode<RequestWorkflowApprovalMutation, RequestWorkflowApprovalMutationVariables>;
export const ApproveWorkflowVersionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"ApproveWorkflowVersion"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"ApproveWorkflowVersionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"approveWorkflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"targetVersionId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"requestedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}}]}}]}}]} as unknown as DocumentNode<ApproveWorkflowVersionMutation, ApproveWorkflowVersionMutationVariables>;
export const RejectWorkflowVersionDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"RejectWorkflowVersion"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"RejectWorkflowVersionInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"rejectWorkflowVersion"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"workflowId"}},{"kind":"Field","name":{"kind":"Name","value":"targetVersionId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"requestedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedBy"}},{"kind":"Field","name":{"kind":"Name","value":"reviewedAt"}},{"kind":"Field","name":{"kind":"Name","value":"comment"}}]}}]}}]} as unknown as DocumentNode<RejectWorkflowVersionMutation, RejectWorkflowVersionMutationVariables>;
export const TriggerWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"TriggerWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"name"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"event"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"JSON"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"environment"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"versionId"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"triggerWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"name"},"value":{"kind":"Variable","name":{"kind":"Name","value":"name"}}},{"kind":"Argument","name":{"kind":"Name","value":"event"},"value":{"kind":"Variable","name":{"kind":"Name","value":"event"}}},{"kind":"Argument","name":{"kind":"Name","value":"environment"},"value":{"kind":"Variable","name":{"kind":"Name","value":"environment"}}},{"kind":"Argument","name":{"kind":"Name","value":"versionId"},"value":{"kind":"Variable","name":{"kind":"Name","value":"versionId"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"workflowName"}},{"kind":"Field","name":{"kind":"Name","value":"routesMatched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsExecuted"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"duration"}},{"kind":"Field","name":{"kind":"Name","value":"runId"}},{"kind":"Field","name":{"kind":"Name","value":"environment"}},{"kind":"Field","name":{"kind":"Name","value":"versionId"}}]}}]}}]} as unknown as DocumentNode<TriggerWorkflowMutation, TriggerWorkflowMutationVariables>;
export const DryRunWorkflowDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"mutation","name":{"kind":"Name","value":"DryRunWorkflow"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"input"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"DryRunWorkflowInput"}}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"dryRunWorkflow"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"input"},"value":{"kind":"Variable","name":{"kind":"Name","value":"input"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"routeResults"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"routeName"}},{"kind":"Field","name":{"kind":"Name","value":"matched"}},{"kind":"Field","name":{"kind":"Name","value":"actionsWouldRun"}},{"kind":"Field","name":{"kind":"Name","value":"skipReason"}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"}},{"kind":"Field","name":{"kind":"Name","value":"validationErrors"}}]}}]}}]} as unknown as DocumentNode<DryRunWorkflowMutation, DryRunWorkflowMutationVariables>;