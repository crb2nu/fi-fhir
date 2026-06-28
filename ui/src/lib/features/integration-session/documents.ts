import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { parse } from 'graphql';
import type { ParsePreviewQuery } from '$lib/gen/graphql';
import type { IntegrationSessionDiagnostic, IntegrationSessionStage } from './types';

export type CreateIntegrationSessionMutationVariables = {
  input: {
    name: string;
    description?: string | null;
  };
};

export type CreateIntegrationSessionMutation = {
  createIntegrationSession: {
    id: string;
    name: string;
  };
};

export type AddSessionSampleMutationVariables = {
  input: {
    sessionId: string;
    name: string;
    source?: string | null;
    format: 'HL7V2';
    data: string;
    retainRawPayload?: boolean | null;
  };
};

export type AddSessionSampleMutation = {
  addSessionSample: {
    id: string;
    source: string | null;
    payloadChecksum: string;
    rawPayload: string | null;
  };
};

export type RunSessionPreviewMutationVariables = {
  input: {
    sessionId: string;
    sampleId: string;
    source?: string | null;
  };
};

export type RunSessionPreviewMutation = {
  runSessionPreview: {
    id: string;
    status: string;
    events: ParsePreviewQuery['parsePreview']['events'];
    warnings: ParsePreviewQuery['parsePreview']['warnings'];
    diagnostics: IntegrationSessionDiagnostic[];
    stages: IntegrationSessionStage[];
  };
};

export type SessionRunEventsSubscriptionVariables = {
  sessionId: string;
  runId?: string | null;
};

export type SessionRunEventsSubscription = {
  sessionRunEvents: {
    sessionId: string;
    runId: string | null;
    type: string;
    message: string;
  };
};

const eventSelection = `
  __typename
  id
  type
  timestamp
  source
  sourceFormat
  correlationId
  ... on PatientAdmitEvent {
    patient { mrn familyName givenName dateOfBirth gender }
    encounter { class location { facility unit room bed } }
  }
  ... on PatientDischargeEvent {
    patient { mrn familyName givenName dateOfBirth gender }
    encounter { class location { facility unit room bed } }
  }
  ... on LabResultEvent {
    patient { mrn familyName givenName dateOfBirth gender }
    test { loincCode localCode description }
    result { value unit status }
    isCritical
  }
  ... on AppointmentEvent {
    patient { mrn familyName givenName dateOfBirth gender }
    appointment {
      id
      status
      startTime
      endTime
      reason
      location { facility unit room bed }
      provider { familyName givenName npi }
    }
  }
  ... on DocumentEvent {
    documentType
    title
  }
`;

const warningSelection = `
  phase
  code
  message
  path
  explanation
  fixSuggestion
  impact
  severity
  fromCache
`;

const diagnosticSelection = `
  id
  code
  message
  path
  severity
  fixSuggestion
  accepted
  acceptedAt
`;

const stageSelection = `
  id
  name
  status
  startedAt
  completedAt
  durationMs
`;

export const CreateIntegrationSessionDocument = parse(`
  mutation CreateIntegrationSession($input: CreateIntegrationSessionInput!) {
    createIntegrationSession(input: $input) {
      id
      name
    }
  }
`) as unknown as TypedDocumentNode<
  CreateIntegrationSessionMutation,
  CreateIntegrationSessionMutationVariables
>;

export const AddSessionSampleDocument = parse(`
  mutation AddSessionSample($input: AddSessionSampleInput!) {
    addSessionSample(input: $input) {
      id
      source
      payloadChecksum
      rawPayload
    }
  }
`) as unknown as TypedDocumentNode<AddSessionSampleMutation, AddSessionSampleMutationVariables>;

export const RunSessionPreviewDocument = parse(`
  mutation RunSessionPreview($input: RunSessionPreviewInput!) {
    runSessionPreview(input: $input) {
      id
      status
      events {
        ${eventSelection}
      }
      warnings {
        ${warningSelection}
      }
      diagnostics {
        ${diagnosticSelection}
      }
      stages {
        ${stageSelection}
      }
    }
  }
`) as unknown as TypedDocumentNode<RunSessionPreviewMutation, RunSessionPreviewMutationVariables>;

export const SessionRunEventsDocument = parse(`
  subscription SessionRunEvents($sessionId: ID!, $runId: ID) {
    sessionRunEvents(sessionId: $sessionId, runId: $runId) {
      sessionId
      runId
      type
      message
    }
  }
`) as unknown as TypedDocumentNode<
  SessionRunEventsSubscription,
  SessionRunEventsSubscriptionVariables
>;
