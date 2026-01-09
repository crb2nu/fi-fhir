/**
 * fi-fhir TypeScript SDK
 *
 * Transform legacy healthcare formats (HL7v2, CSV, EDI) into semantic events.
 *
 * @example
 * ```typescript
 * import { parseHL7, parseCSV, Workflow } from '@fi-fhir/sdk';
 *
 * // Parse HL7v2 message
 * const event = await parseHL7(hl7Message, { source: 'epic_adt' });
 * console.log(event.type, event.patient.mrn);
 *
 * // Parse CSV patient data
 * const events = await parseCSV(csvContent, {
 *   eventType: 'patient',
 *   hasHeader: true
 * });
 *
 * // Run workflow
 * const workflow = new Workflow('./workflow.yaml');
 * const result = await workflow.run(events);
 * ```
 *
 * @packageDocumentation
 */

// Parser exports
export {
  parse,
  parseHL7,
  parseCSV,
  parseCSVWithSchema,
  FiFhirError,
  type ParseOptions,
  type CSVParseOptions,
  type ParseResultWithSchema,
  type InferredSchema,
  type ColumnInfo,
} from './parser';

// Workflow exports
export {
  Workflow,
  type WorkflowResult,
  type DryRunResult,
  type RouteMatchResult,
  type ValidationResult,
  type RouteInfo,
} from './workflow';

// Type exports
export type {
  EventType,
  SourceFormat,
  EventMeta,
  Coding,
  CodeableConcept,
  Identifier,
  IdentifierSet,
  Address,
  Patient,
  Location,
  Encounter,
  Provider,
  LabTest,
  LabValue,
  Appointment,
  ParseWarning,
  PatientAdmitEvent,
  PatientUpdateEvent,
  PatientDischargeEvent,
  PatientTransferEvent,
  LabResultEvent,
  AppointmentEvent,
  GenericEvent,
  HealthcareEvent,
} from './types/events';

// Type guards
export {
  isPatientEvent,
  isLabEvent,
  isAppointmentEvent,
} from './types/events';

// Utility exports
export {
  isFiFhirAvailable,
  getFiFhirVersion,
} from './utils/cli';
