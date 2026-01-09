/**
 * Event type identifiers matching Go pkg/events constants
 */
export type EventType =
  | 'patient_admit'
  | 'patient_update'
  | 'patient_discharge'
  | 'patient_transfer'
  | 'patient_merge'
  | 'lab_result'
  | 'lab_order'
  | 'appointment_scheduled'
  | 'appointment_modified'
  | 'appointment_cancelled'
  | 'appointment_noshow'
  | 'claim_submitted'
  | 'claim_adjudicated'
  | 'csv_record';

/**
 * Source format identifiers
 */
export type SourceFormat = 'hl7v2' | 'csv' | 'edi' | 'fhir' | 'json';

/**
 * Base event metadata present on all events
 */
export interface EventMeta {
  id: string;
  type: EventType;
  timestamp: string;
  received_at: string;
  source: string;
  source_format: SourceFormat;
  source_message_id?: string;
  profile_id?: string;
  raw_payload?: string;
}

/**
 * Coding represents a code from a terminology system
 */
export interface Coding {
  system: string;
  code: string;
  display?: string;
  version?: string;
}

/**
 * CodeableConcept with text and codings
 */
export interface CodeableConcept {
  text?: string;
  coding?: Coding[];
}

/**
 * Single identifier value
 */
export interface Identifier {
  value: string;
  type?: string;
  system?: string;
  assigning_authority?: string;
}

/**
 * Set of identifiers
 */
export interface IdentifierSet {
  identifiers: Identifier[] | null;
  mrn?: string;
  ssn?: string;
  drivers_license?: string;
}

/**
 * Physical address
 */
export interface Address {
  line1?: string;
  line2?: string;
  city?: string;
  state?: string;
  postal_code?: string;
  country?: string;
  type?: string;
}

/**
 * Patient demographics
 */
export interface Patient {
  mrn: string;
  identifiers: IdentifierSet;
  family_name?: string;
  given_name?: string;
  middle_name?: string;
  prefix?: string;
  suffix?: string;
  date_of_birth?: string;
  gender?: string;
  race?: string;
  ethnicity?: string;
  marital_status?: string;
  language?: string;
  address?: Address;
  phone?: string;
  email?: string;
  deceased?: boolean;
  deceased_date?: string;
}

/**
 * Healthcare facility location
 */
export interface Location {
  facility?: string;
  building?: string;
  floor?: string;
  point_of_care?: string;
  room?: string;
  bed?: string;
  location_type?: string;
}

/**
 * Patient encounter/visit
 */
export interface Encounter {
  id: string;
  identifiers: IdentifierSet;
  class: string;
  type?: string;
  status?: string;
  admit_datetime: string;
  discharge_datetime?: string;
  location: Location;
  attending_provider?: Provider;
  admitting_provider?: Provider;
  referring_provider?: Provider;
  admit_source?: string;
  discharge_disposition?: string;
}

/**
 * Healthcare provider
 */
export interface Provider {
  id?: string;
  npi?: string;
  family_name?: string;
  given_name?: string;
  prefix?: string;
  suffix?: string;
  specialty?: string;
  identifiers?: IdentifierSet;
}

/**
 * Laboratory test information
 */
export interface LabTest {
  code: CodeableConcept;
  local_code?: string;
  loinc_code?: string;
  description?: string;
  category?: string;
}

/**
 * Laboratory result value
 */
export interface LabValue {
  value: string;
  unit?: string;
  reference_range?: string;
  interpretation?: string;
  status?: string;
  observation_time?: string;
  numeric_value?: number;
  comparator?: string;
}

/**
 * Appointment information
 */
export interface Appointment {
  id: string;
  identifiers?: IdentifierSet;
  status?: string;
  type?: string;
  reason?: string;
  start_time?: string;
  end_time?: string;
  duration_minutes?: number;
  location?: Location;
  provider?: Provider;
  previous_status?: string;
  cancellation_reason?: string;
  noshow?: boolean;
}

/**
 * Parse warning from the parser
 */
export interface ParseWarning {
  phase: string;
  code: string;
  message: string;
  path?: string;
  severity: string;
}

// Event type interfaces

export interface PatientAdmitEvent extends EventMeta {
  type: 'patient_admit';
  patient: Patient;
  encounter: Encounter;
}

export interface PatientUpdateEvent extends EventMeta {
  type: 'patient_update';
  patient: Patient;
  encounter: Encounter;
}

export interface PatientDischargeEvent extends EventMeta {
  type: 'patient_discharge';
  patient: Patient;
  encounter: Encounter;
}

export interface PatientTransferEvent extends EventMeta {
  type: 'patient_transfer';
  patient: Patient;
  encounter: Encounter;
  prior_location?: Location;
}

export interface LabResultEvent extends EventMeta {
  type: 'lab_result';
  patient: Patient;
  test: LabTest;
  result: LabValue;
  order_id?: string;
  specimen_id?: string;
  performing_lab?: string;
  is_critical: boolean;
}

export interface AppointmentEvent extends EventMeta {
  type: 'appointment_scheduled' | 'appointment_modified' | 'appointment_cancelled' | 'appointment_noshow';
  patient: Patient;
  appointment: Appointment;
}

export interface GenericEvent extends EventMeta {
  type: 'csv_record';
  data?: Record<string, unknown>;
}

/**
 * Union type for all healthcare events
 */
export type HealthcareEvent =
  | PatientAdmitEvent
  | PatientUpdateEvent
  | PatientDischargeEvent
  | PatientTransferEvent
  | LabResultEvent
  | AppointmentEvent
  | GenericEvent;

/**
 * Type guard for patient events
 */
export function isPatientEvent(event: HealthcareEvent): event is PatientAdmitEvent | PatientUpdateEvent | PatientDischargeEvent | PatientTransferEvent {
  return event.type.startsWith('patient_');
}

/**
 * Type guard for lab events
 */
export function isLabEvent(event: HealthcareEvent): event is LabResultEvent {
  return event.type === 'lab_result';
}

/**
 * Type guard for appointment events
 */
export function isAppointmentEvent(event: HealthcareEvent): event is AppointmentEvent {
  return event.type.startsWith('appointment_');
}
