# GraphQL API Layer Design

This document describes the GraphQL API layer for fi-fhir, providing a flexible query interface for healthcare events and resources.

## Overview

The GraphQL API provides:
- **Queries**: Retrieve events, patients, workflows, and configurations
- **Mutations**: Submit events, trigger workflows, manage subscriptions
- **Subscriptions**: Real-time event streaming via WebSocket

```
┌─────────────────────────────────────────────────────────────┐
│                      GraphQL API                             │
├─────────────────────────────────────────────────────────────┤
│  Queries              Mutations            Subscriptions     │
│  ────────             ─────────            ─────────────     │
│  events()             submitEvent()        eventStream()     │
│  event(id)            parseMessage()       workflowEvents()  │
│  patient(mrn)         triggerWorkflow()                      │
│  workflow(name)       createSubscription()                   │
│  health()             deleteSubscription()                   │
└─────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌─────────────────────────────────────────────────────────────┐
│                    Event Store / Router                      │
│  ┌─────────────┐  ┌─────────────┐  ┌─────────────┐          │
│  │   Parser    │  │  Workflow   │  │    FHIR     │          │
│  │   Engine    │  │   Engine    │  │  Subscriptions│        │
│  └─────────────┘  └─────────────┘  └─────────────┘          │
└─────────────────────────────────────────────────────────────┘
```

## Schema Design

### Core Types

```graphql
# Scalar types
scalar DateTime
scalar JSON

# Event interface - all events implement this
interface Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
}

# Event type enum
enum EventType {
  PATIENT_ADMIT
  PATIENT_DISCHARGE
  PATIENT_TRANSFER
  PATIENT_UPDATE
  LAB_RESULT
  LAB_ORDERED
  APPOINTMENT_SCHEDULED
  APPOINTMENT_CANCELLED
  APPOINTMENT_NOSHOW
  CLAIM_SUBMITTED
  CLAIM_ADJUDICATED
  VITAL_SIGN
  CONDITION
  PROCEDURE
  IMMUNIZATION
  DOCUMENT
}

# Source format enum
enum SourceFormat {
  HL7V2
  FHIR
  CSV
  EDI_837
  EDI_835
  CDA
}

# Patient type
type Patient {
  mrn: ID!
  identifiers: [Identifier!]!
  familyName: String!
  givenName: String!
  middleName: String
  dateOfBirth: DateTime
  gender: String
  address: Address
  phone: String
  email: String
}

# Identifier type
type Identifier {
  value: String!
  type: String!
  system: String
  assigner: String
}

# Address type
type Address {
  line1: String
  line2: String
  city: String
  state: String
  postalCode: String
  country: String
}

# Provider type
type Provider {
  npi: String
  id: String
  familyName: String!
  givenName: String!
  specialty: String
  organizationName: String
}

# Location type
type Location {
  facility: String
  unit: String
  room: String
  bed: String
}

# Encounter type
type Encounter {
  id: ID!
  class: String!
  status: String
  admitDateTime: DateTime
  dischargeDateTime: DateTime
  location: Location
  attendingProvider: Provider
}
```

### Event Types

```graphql
# Patient admit event
type PatientAdmitEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  encounter: Encounter!
}

# Patient discharge event
type PatientDischargeEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  encounter: Encounter!
}

# Lab result event
type LabResultEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  test: LabTest!
  result: LabResult!
  isCritical: Boolean!
  orderingProvider: Provider
}

type LabTest {
  loincCode: String
  localCode: String
  description: String!
  category: String
}

type LabResult {
  value: String!
  unit: String
  referenceRange: String
  interpretation: String
  status: String
}

# Vital sign event
type VitalSignEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  vitalSign: VitalSign!
}

type VitalSign {
  name: String!
  loincCode: String
  value: String!
  unit: String
  interpretation: String
}

# Condition event
type ConditionEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  condition: Condition!
  clinicalStatus: String
  onsetDate: String
}

type Condition {
  name: String!
  code: String
  codeSystem: String
  category: String
}

# Appointment event
type AppointmentEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient!
  appointment: Appointment!
}

type Appointment {
  id: ID!
  status: String!
  startTime: DateTime!
  endTime: DateTime
  location: Location
  provider: Provider
  reason: String
}

# Document event
type DocumentEvent implements Event {
  id: ID!
  type: EventType!
  timestamp: DateTime!
  source: String!
  sourceFormat: SourceFormat!
  correlationId: String
  patient: Patient
  documentType: String!
  title: String
}
```

### Query Types

```graphql
type Query {
  # Get a single event by ID
  event(id: ID!): Event

  # Query events with filters
  events(
    filter: EventFilter
    first: Int = 100
    after: String
    orderBy: EventOrderBy
  ): EventConnection!

  # Get patient by MRN
  patient(mrn: ID!): Patient

  # Get patients with filter
  patients(
    filter: PatientFilter
    first: Int = 100
    after: String
  ): PatientConnection!

  # Get workflow status
  workflow(name: String!): WorkflowStatus

  # List all workflows
  workflows: [WorkflowStatus!]!

  # Health check
  health: HealthStatus!

  # Get parse result without persisting
  parsePreview(
    format: SourceFormat!
    data: String!
    source: String
  ): ParseResult!
}

# Event filter input
input EventFilter {
  types: [EventType!]
  sources: [String!]
  patientMrn: String
  fromTimestamp: DateTime
  toTimestamp: DateTime
  correlationId: String
}

# Event ordering
input EventOrderBy {
  field: EventOrderField!
  direction: OrderDirection!
}

enum EventOrderField {
  TIMESTAMP
  TYPE
  SOURCE
}

enum OrderDirection {
  ASC
  DESC
}

# Pagination connection types
type EventConnection {
  edges: [EventEdge!]!
  pageInfo: PageInfo!
  totalCount: Int!
}

type EventEdge {
  cursor: String!
  node: Event!
}

type PageInfo {
  hasNextPage: Boolean!
  hasPreviousPage: Boolean!
  startCursor: String
  endCursor: String
}

type PatientConnection {
  edges: [PatientEdge!]!
  pageInfo: PageInfo!
  totalCount: Int!
}

type PatientEdge {
  cursor: String!
  node: Patient!
}

input PatientFilter {
  mrn: String
  familyName: String
  givenName: String
  dateOfBirth: DateTime
}

type WorkflowStatus {
  name: String!
  enabled: Boolean!
  routeCount: Int!
  eventsProcessed: Int!
  lastEventTime: DateTime
  errors: Int!
}

type HealthStatus {
  status: String!
  version: String!
  uptime: Int!
  components: [ComponentHealth!]!
}

type ComponentHealth {
  name: String!
  status: String!
  message: String
}

type ParseResult {
  success: Boolean!
  events: [Event!]!
  warnings: [ParseWarning!]!
  errors: [String!]!
}

type ParseWarning {
  phase: String!
  code: String!
  message: String!
  path: String
}
```

### Mutation Types

```graphql
type Mutation {
  # Submit a raw message for parsing and processing
  submitMessage(input: SubmitMessageInput!): SubmitResult!

  # Submit a pre-parsed event directly
  submitEvent(input: SubmitEventInput!): SubmitResult!

  # Trigger a workflow manually
  triggerWorkflow(
    name: String!
    event: JSON!
  ): WorkflowResult!

  # Create a FHIR subscription
  createFhirSubscription(input: CreateSubscriptionInput!): FhirSubscription!

  # Delete a FHIR subscription
  deleteFhirSubscription(id: ID!): Boolean!

  # Pause a FHIR subscription
  pauseFhirSubscription(id: ID!): FhirSubscription!

  # Resume a FHIR subscription
  resumeFhirSubscription(id: ID!): FhirSubscription!
}

input SubmitMessageInput {
  format: SourceFormat!
  data: String!
  source: String!
  correlationId: String
}

input SubmitEventInput {
  type: EventType!
  data: JSON!
  source: String!
  correlationId: String
}

type SubmitResult {
  success: Boolean!
  eventId: ID
  warnings: [ParseWarning!]!
  errors: [String!]!
  workflowResults: [WorkflowResult!]!
}

type WorkflowResult {
  workflowName: String!
  routesMatched: Int!
  actionsExecuted: Int!
  errors: [String!]!
  duration: Int! # milliseconds
}

input CreateSubscriptionInput {
  name: String!
  server: String!
  criteria: String!
  endpoint: String!
}

type FhirSubscription {
  id: ID!
  name: String!
  status: String!
  criteria: String!
  server: String!
  endpoint: String!
  createdAt: DateTime!
}
```

### Subscription Types (Real-time)

```graphql
type Subscription {
  # Stream all events
  eventStream(filter: EventFilter): Event!

  # Stream events for a specific workflow
  workflowEvents(workflowName: String!): WorkflowEventNotification!

  # Stream events for a specific patient
  patientEvents(mrn: ID!): Event!
}

type WorkflowEventNotification {
  event: Event!
  workflow: String!
  routesMatched: [String!]!
  actionsExecuted: [String!]!
  duration: Int!
}
```

## Architecture

### Package Structure

```
internal/api/graphql/
├── schema.go          # Schema definitions using gqlgen
├── resolver.go        # Root resolver
├── resolvers/
│   ├── query.go       # Query resolvers
│   ├── mutation.go    # Mutation resolvers
│   ├── subscription.go # Subscription resolvers
│   └── types.go       # Type resolvers
├── model/
│   └── models.go      # Generated models
├── dataloaders/
│   └── loaders.go     # DataLoader for N+1 prevention
└── server.go          # HTTP/WebSocket server
```

### Implementation Strategy

1. **Schema-First Development**: Define GraphQL schema, generate Go code
2. **gqlgen**: Use gqlgen for type-safe resolver generation
3. **DataLoaders**: Batch and cache lookups to prevent N+1 queries
4. **WebSocket Subscriptions**: Use graphql-ws protocol for real-time events

### Server Configuration

```yaml
graphql:
  # Server settings
  enabled: true
  host: "0.0.0.0"
  port: 8081
  path: "/graphql"
  playground: true  # Enable GraphQL Playground in dev

  # WebSocket settings
  websocket:
    enabled: true
    path: "/graphql/ws"
    keepalive: 30s

  # Query complexity limits
  complexity:
    max_depth: 10
    max_complexity: 1000

  # Authentication
  auth:
    enabled: false
    type: "bearer"  # bearer, basic, api_key

  # CORS settings
  cors:
    allowed_origins: ["*"]
    allowed_methods: ["GET", "POST", "OPTIONS"]
    allowed_headers: ["Authorization", "Content-Type"]
```

## CLI Integration

```bash
# Start GraphQL server
fi-fhir serve --graphql

# Start with specific config
fi-fhir serve --graphql --config config.yaml

# Query via CLI (convenience wrapper)
fi-fhir graphql query '{ health { status version } }'

# Subscribe to events
fi-fhir graphql subscribe 'subscription { eventStream { id type timestamp } }'
```

## Example Queries

### Get Recent Lab Results

```graphql
query RecentLabResults {
  events(
    filter: { types: [LAB_RESULT], patientMrn: "MRN12345" }
    first: 10
    orderBy: { field: TIMESTAMP, direction: DESC }
  ) {
    edges {
      node {
        ... on LabResultEvent {
          id
          timestamp
          test {
            loincCode
            description
          }
          result {
            value
            unit
            interpretation
          }
          isCritical
        }
      }
    }
    pageInfo {
      hasNextPage
      endCursor
    }
  }
}
```

### Submit HL7v2 Message

```graphql
mutation SubmitHL7 {
  submitMessage(input: {
    format: HL7V2
    data: "MSH|^~\\&|LAB|FACILITY|..."
    source: "lab_interface"
  }) {
    success
    eventId
    warnings {
      code
      message
    }
    workflowResults {
      workflowName
      routesMatched
    }
  }
}
```

### Subscribe to Patient Events

```graphql
subscription PatientMonitor {
  patientEvents(mrn: "MRN12345") {
    id
    type
    timestamp
    ... on LabResultEvent {
      isCritical
      test { description }
      result { value unit }
    }
    ... on VitalSignEvent {
      vitalSign { name value unit }
    }
  }
}
```

## Implementation Plan

### Phase 1: Core API
- [ ] Schema definition with gqlgen
- [ ] Query resolvers (events, patients, health)
- [ ] Basic HTTP server

### Phase 2: Mutations
- [ ] submitMessage mutation
- [ ] submitEvent mutation
- [ ] FHIR subscription management

### Phase 3: Real-time
- [ ] WebSocket subscriptions
- [ ] Event stream filtering
- [ ] Workflow event notifications

### Phase 4: Production
- [ ] Authentication/Authorization
- [ ] Query complexity limiting
- [ ] Metrics and tracing
- [ ] Rate limiting

## See Also

- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Workflow engine integration
- [FHIR-SUBSCRIPTIONS.md](FHIR-SUBSCRIPTIONS.md) - FHIR subscription integration
- [gqlgen Documentation](https://gqlgen.com/)
