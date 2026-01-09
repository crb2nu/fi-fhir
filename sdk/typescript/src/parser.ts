import { execFiFhir, FiFhirError } from './utils/cli';
import type { HealthcareEvent, ParseWarning, SourceFormat } from './types/events';

/**
 * Options for parsing healthcare messages
 */
export interface ParseOptions {
  /** Input format (auto-detected if not specified) */
  format?: SourceFormat;
  /** Source system identifier */
  source?: string;
  /** Path to Source Profile YAML file */
  profile?: string;
  /** Show parse warnings */
  warnings?: boolean;
  /** Timeout in milliseconds */
  timeout?: number;
}

/**
 * Options specific to CSV parsing
 */
export interface CSVParseOptions extends ParseOptions {
  /** First row contains column headers (default: true) */
  hasHeader?: boolean;
  /** Field delimiter (default: ',') */
  delimiter?: string;
  /** Event type to produce: 'patient' or 'lab' */
  eventType?: 'patient' | 'lab';
  /** Infer schema from data */
  inferSchema?: boolean;
}

/**
 * Result from parsing with schema inference
 */
export interface ParseResultWithSchema {
  events: HealthcareEvent[];
  schema?: InferredSchema;
  warnings?: ParseWarning[];
}

/**
 * Inferred schema from CSV data
 */
export interface InferredSchema {
  columns: ColumnInfo[];
}

/**
 * Column information from schema inference
 */
export interface ColumnInfo {
  index: number;
  name: string;
  inferred_type: string;
  sample_values?: string[];
  semantic_hint?: string;
}

/**
 * Parse a healthcare message and return events
 *
 * @param content - Message content (HL7v2, CSV, etc.)
 * @param options - Parse options
 * @returns Array of healthcare events
 *
 * @example
 * ```typescript
 * const events = await parse(hl7Message, { format: 'hl7v2' });
 * console.log(events[0].type); // 'patient_admit'
 * ```
 */
export async function parse(
  content: string,
  options: ParseOptions = {}
): Promise<HealthcareEvent[]> {
  const args = buildParseArgs(options);
  args.push('-'); // Read from stdin

  const { stdout } = await execFiFhir(args, content, { timeout: options.timeout });

  const result = JSON.parse(stdout);

  // Normalize to array (HL7v2 returns single event, CSV returns array)
  return Array.isArray(result) ? result : [result];
}

/**
 * Parse an HL7v2 message
 *
 * @param content - HL7v2 message content
 * @param options - Parse options (format is automatically set to 'hl7v2')
 * @returns Single healthcare event
 *
 * @example
 * ```typescript
 * const event = await parseHL7(message, { source: 'epic_adt' });
 * if (event.type === 'patient_admit') {
 *   console.log(event.patient.mrn);
 * }
 * ```
 */
export async function parseHL7(
  content: string,
  options: Omit<ParseOptions, 'format'> = {}
): Promise<HealthcareEvent> {
  const events = await parse(content, { ...options, format: 'hl7v2' });
  return events[0];
}

/**
 * Parse CSV/flatfile data
 *
 * @param content - CSV content
 * @param options - CSV-specific parse options
 * @returns Array of healthcare events
 *
 * @example
 * ```typescript
 * const events = await parseCSV(csvContent, {
 *   eventType: 'patient',
 *   hasHeader: true
 * });
 * ```
 */
export async function parseCSV(
  content: string,
  options: CSVParseOptions = {}
): Promise<HealthcareEvent[]> {
  const args = ['parse', '-f', 'csv'];

  if (options.source) args.push('-s', options.source);
  if (options.profile) args.push('--profile', options.profile);
  if (options.eventType) args.push('-t', options.eventType);
  if (options.delimiter) args.push('-d', options.delimiter);
  if (options.hasHeader === false) args.push('--no-header');
  if (options.inferSchema) args.push('--infer-schema');

  args.push('-');

  const { stdout } = await execFiFhir(args, content, { timeout: options.timeout });
  const result = JSON.parse(stdout);

  // If inferSchema is true, result includes schema
  if (options.inferSchema && result.events) {
    return result.events;
  }

  return Array.isArray(result) ? result : [result];
}

/**
 * Parse CSV with schema inference
 *
 * @param content - CSV content
 * @param options - CSV parse options
 * @returns Parse result with events and inferred schema
 */
export async function parseCSVWithSchema(
  content: string,
  options: Omit<CSVParseOptions, 'inferSchema'> = {}
): Promise<ParseResultWithSchema> {
  const args = ['parse', '-f', 'csv', '--infer-schema'];

  if (options.source) args.push('-s', options.source);
  if (options.eventType) args.push('-t', options.eventType);
  if (options.delimiter) args.push('-d', options.delimiter);
  if (options.hasHeader === false) args.push('--no-header');

  args.push('-');

  const { stdout } = await execFiFhir(args, content, { timeout: options.timeout });
  return JSON.parse(stdout);
}

/**
 * Build CLI arguments from parse options
 */
function buildParseArgs(options: ParseOptions): string[] {
  const args = ['parse'];

  if (options.format) {
    args.push('-f', options.format);
  }
  if (options.source) {
    args.push('-s', options.source);
  }
  if (options.profile) {
    args.push('--profile', options.profile);
  }
  if (options.warnings) {
    args.push('-w');
  }

  return args;
}

// Re-export error type
export { FiFhirError };
