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

export type Address = {
  __typename?: 'Address';
  city: Maybe<Scalars['String']['output']>;
  country: Maybe<Scalars['String']['output']>;
  line1: Maybe<Scalars['String']['output']>;
  line2: Maybe<Scalars['String']['output']>;
  postalCode: Maybe<Scalars['String']['output']>;
  state: Maybe<Scalars['String']['output']>;
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

export type CreateSubscriptionInput = {
  criteria: Scalars['String']['input'];
  endpoint: Scalars['String']['input'];
  name: Scalars['String']['input'];
  server: Scalars['String']['input'];
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
  | 'APPOINTMENT_CANCELLED'
  | 'APPOINTMENT_NOSHOW'
  | 'APPOINTMENT_SCHEDULED'
  | 'CLAIM_ADJUDICATED'
  | 'CLAIM_SUBMITTED'
  | 'CONDITION'
  | 'DOCUMENT'
  | 'IMMUNIZATION'
  | 'LAB_ORDERED'
  | 'LAB_RESULT'
  | 'PATIENT_ADMIT'
  | 'PATIENT_DISCHARGE'
  | 'PATIENT_TRANSFER'
  | 'PATIENT_UPDATE'
  | 'PROCEDURE'
  | 'VITAL_SIGN';

export type EventTypeCount = {
  __typename?: 'EventTypeCount';
  count: Scalars['Int']['output'];
  eventType: Scalars['String']['output'];
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

export type HealthStatus = {
  __typename?: 'HealthStatus';
  components: Array<ComponentHealth>;
  status: Scalars['String']['output'];
  uptime: Scalars['Int']['output'];
  version: Scalars['String']['output'];
};

export type Identifier = {
  __typename?: 'Identifier';
  assigner: Maybe<Scalars['String']['output']>;
  system: Maybe<Scalars['String']['output']>;
  type: Scalars['String']['output'];
  value: Scalars['String']['output'];
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

export type Location = {
  __typename?: 'Location';
  bed: Maybe<Scalars['String']['output']>;
  facility: Maybe<Scalars['String']['output']>;
  room: Maybe<Scalars['String']['output']>;
  unit: Maybe<Scalars['String']['output']>;
};

export type Mutation = {
  __typename?: 'Mutation';
  createFhirSubscription: FhirSubscription;
  deleteFhirSubscription: Scalars['Boolean']['output'];
  pauseFhirSubscription: FhirSubscription;
  resumeFhirSubscription: FhirSubscription;
  submitBatch: BatchResult;
  submitEvent: SubmitResult;
  submitMessage: SubmitResult;
  triggerWorkflow: WorkflowResult;
};


export type MutationCreateFhirSubscriptionArgs = {
  input: CreateSubscriptionInput;
};


export type MutationDeleteFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationPauseFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
};


export type MutationResumeFhirSubscriptionArgs = {
  id: Scalars['ID']['input'];
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
  event: Scalars['JSON']['input'];
  name: Scalars['String']['input'];
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
  message: Scalars['String']['output'];
  path: Maybe<Scalars['String']['output']>;
  phase: Scalars['String']['output'];
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

export type ProjectionStatus = {
  __typename?: 'ProjectionStatus';
  behind: Scalars['Int']['output'];
  checkpoint: Scalars['Int']['output'];
  lastPosition: Scalars['Int']['output'];
  name: Scalars['String']['output'];
  status: Scalars['String']['output'];
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

export type Query = {
  __typename?: 'Query';
  activeEncounter: Maybe<ActiveEncounter>;
  activeEncounterByPatient: Maybe<ActiveEncounter>;
  activeEncounters: Array<ActiveEncounter>;
  event: Maybe<Event>;
  eventStatistics: EventStatistics;
  events: EventConnection;
  health: HealthStatus;
  parsePreview: ParseResult;
  patient: Maybe<Patient>;
  patientTimeline: Maybe<PatientTimeline>;
  patients: PatientConnection;
  projectionStatus: Array<ProjectionStatus>;
  workflow: Maybe<WorkflowStatus>;
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


export type QueryEventArgs = {
  id: Scalars['ID']['input'];
};


export type QueryEventsArgs = {
  after: InputMaybe<Scalars['String']['input']>;
  filter: InputMaybe<EventFilter>;
  first?: InputMaybe<Scalars['Int']['input']>;
  orderBy: InputMaybe<EventOrderBy>;
};


export type QueryParsePreviewArgs = {
  data: Scalars['String']['input'];
  format: SourceFormat;
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


export type QueryWorkflowArgs = {
  name: Scalars['String']['input'];
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
  eventStream: Event;
  patientEvents: Event;
  workflowEvents: WorkflowEventNotification;
};


export type SubscriptionEventStreamArgs = {
  filter: InputMaybe<EventFilter>;
};


export type SubscriptionPatientEventsArgs = {
  mrn: Scalars['ID']['input'];
};


export type SubscriptionWorkflowEventsArgs = {
  workflowName: Scalars['String']['input'];
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

export type WorkflowEventNotification = {
  __typename?: 'WorkflowEventNotification';
  actionsExecuted: Array<Scalars['String']['output']>;
  duration: Scalars['Int']['output'];
  event: Event;
  routesMatched: Array<Scalars['String']['output']>;
  workflow: Scalars['String']['output'];
};

export type WorkflowResult = {
  __typename?: 'WorkflowResult';
  actionsExecuted: Scalars['Int']['output'];
  duration: Scalars['Int']['output'];
  errors: Array<Scalars['String']['output']>;
  routesMatched: Scalars['Int']['output'];
  workflowName: Scalars['String']['output'];
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

export type HealthQueryVariables = Exact<{ [key: string]: never; }>;


export type HealthQuery = { __typename?: 'Query', health: { __typename?: 'HealthStatus', status: string, version: string } };

export type ParsePreviewQueryVariables = Exact<{
  format: SourceFormat;
  data: Scalars['String']['input'];
  source: InputMaybe<Scalars['String']['input']>;
}>;


export type ParsePreviewQuery = { __typename?: 'Query', parsePreview: { __typename?: 'ParseResult', success: boolean, errors: Array<string>, events: Array<{ __typename: 'AppointmentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ConditionEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'DocumentEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ImmunizationEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'LabResultEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'PatientAdmitEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'PatientDischargeEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'ProcedureEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null } | { __typename: 'VitalSignEvent', id: string, type: EventType, timestamp: string, source: string, sourceFormat: SourceFormat | null, correlationId: string | null }>, warnings: Array<{ __typename?: 'ParseWarning', phase: string, code: string, message: string, path: string | null }> } };


export const HealthDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"Health"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"health"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"status"}},{"kind":"Field","name":{"kind":"Name","value":"version"}}]}}]}}]} as unknown as DocumentNode<HealthQuery, HealthQueryVariables>;
export const ParsePreviewDocument = {"kind":"Document","definitions":[{"kind":"OperationDefinition","operation":"query","name":{"kind":"Name","value":"ParsePreview"},"variableDefinitions":[{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"format"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"SourceFormat"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"data"}},"type":{"kind":"NonNullType","type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}},{"kind":"VariableDefinition","variable":{"kind":"Variable","name":{"kind":"Name","value":"source"}},"type":{"kind":"NamedType","name":{"kind":"Name","value":"String"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"parsePreview"},"arguments":[{"kind":"Argument","name":{"kind":"Name","value":"format"},"value":{"kind":"Variable","name":{"kind":"Name","value":"format"}}},{"kind":"Argument","name":{"kind":"Name","value":"data"},"value":{"kind":"Variable","name":{"kind":"Name","value":"data"}}},{"kind":"Argument","name":{"kind":"Name","value":"source"},"value":{"kind":"Variable","name":{"kind":"Name","value":"source"}}}],"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"success"}},{"kind":"Field","name":{"kind":"Name","value":"errors"}},{"kind":"Field","name":{"kind":"Name","value":"events"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"__typename"}},{"kind":"Field","name":{"kind":"Name","value":"id"}},{"kind":"Field","name":{"kind":"Name","value":"type"}},{"kind":"Field","name":{"kind":"Name","value":"timestamp"}},{"kind":"Field","name":{"kind":"Name","value":"source"}},{"kind":"Field","name":{"kind":"Name","value":"sourceFormat"}},{"kind":"Field","name":{"kind":"Name","value":"correlationId"}}]}},{"kind":"Field","name":{"kind":"Name","value":"warnings"},"selectionSet":{"kind":"SelectionSet","selections":[{"kind":"Field","name":{"kind":"Name","value":"phase"}},{"kind":"Field","name":{"kind":"Name","value":"code"}},{"kind":"Field","name":{"kind":"Name","value":"message"}},{"kind":"Field","name":{"kind":"Name","value":"path"}}]}}]}}]}}]} as unknown as DocumentNode<ParsePreviewQuery, ParsePreviewQueryVariables>;