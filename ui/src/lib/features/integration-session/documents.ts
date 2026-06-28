import type { TypedDocumentNode } from '@graphql-typed-document-node/core';
import { parse } from 'graphql';
import type { ParsePreviewQuery } from '$lib/gen/graphql';
import type { IntegrationSessionDiagnostic, IntegrationSessionStage } from './types';

export type CreateIntegrationSessionMutationVariables = {
  input: {
    source: string;
    profileId?: string | null;
    title?: string | null;
  };
};

export type CreateIntegrationSessionMutation = {
  createIntegrationSession: {
    id: string;
    source: string | null;
    profileId: string | null;
    title: string | null;
  };
};

export type AddSessionSampleMutationVariables = {
  sessionId: string;
  input: {
    source: string;
    format: 'HL7V2';
    data: string;
  };
};

export type AddSessionSampleMutation = {
  addSessionSample: {
    id: string;
    source: string;
    checksum: string | null;
    rawRetained: boolean | null;
  };
};

export type RunSessionPreviewMutationVariables = {
  sessionId: string;
  sampleId: string;
  input: {
    profileId?: string | null;
  };
};

export type RunSessionPreviewMutation = {
  runSessionPreview: {
    id: string;
    state: string;
    preview: ParsePreviewQuery['parsePreview'] | null;
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
    runId: string;
    type: string;
    state: string | null;
    message: string | null;
    preview: ParsePreviewQuery['parsePreview'] | null;
    diagnostics: IntegrationSessionDiagnostic[];
  };
};

const parseResultSelection = `
  success
  errors
  events {
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
  }
  warnings {
    phase
    code
    message
    path
    explanation
    fixSuggestion
    impact
    severity
    fromCache
  }
`;

const diagnosticSelection = `
  id
  phase
  code
  message
  path
  severity
  status
  fixSuggestion
`;

const stageSelection = `
  id
  name
  state
  startedAt
  completedAt
  durationMs
`;

export const CreateIntegrationSessionDocument = parse(`
  mutation CreateIntegrationSession($input: CreateIntegrationSessionInput!) {
    createIntegrationSession(input: $input) {
      id
      source
      profileId
      title
    }
  }
`) as unknown as TypedDocumentNode<
  CreateIntegrationSessionMutation,
  CreateIntegrationSessionMutationVariables
>;

export const AddSessionSampleDocument = parse(`
  mutation AddSessionSample($sessionId: ID!, $input: AddSessionSampleInput!) {
    addSessionSample(sessionId: $sessionId, input: $input) {
      id
      source
      checksum
      rawRetained
    }
  }
`) as unknown as TypedDocumentNode<AddSessionSampleMutation, AddSessionSampleMutationVariables>;

export const RunSessionPreviewDocument = parse(`
  mutation RunSessionPreview($sessionId: ID!, $sampleId: ID!, $input: RunSessionPreviewInput!) {
    runSessionPreview(sessionId: $sessionId, sampleId: $sampleId, input: $input) {
      id
      state
      preview {
        ${parseResultSelection}
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
      state
      message
      preview {
        ${parseResultSelection}
      }
      diagnostics {
        ${diagnosticSelection}
      }
    }
  }
`) as unknown as TypedDocumentNode<
  SessionRunEventsSubscription,
  SessionRunEventsSubscriptionVariables
>;
