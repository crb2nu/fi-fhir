# @fi-fhir/sdk

TypeScript SDK for the fi-fhir healthcare integration library.

## Installation

```bash
npm install @fi-fhir/sdk
```

**Prerequisites:** The `fi-fhir` CLI must be installed and available in your PATH.

```bash
# Install fi-fhir (from source)
go install gitlab.flexinfer.ai/libs/fi-fhir/cmd/fi-fhir@latest

# Or set custom path
export FI_FHIR_PATH=/path/to/fi-fhir
```

## Usage

### Parsing HL7v2 Messages

```typescript
import { parseHL7 } from '@fi-fhir/sdk';

const message = `MSH|^~\\&|EPIC|HOSPITAL|...`;

const event = await parseHL7(message, { source: 'epic_adt' });

if (event.type === 'patient_admit') {
  console.log(`Patient ${event.patient.mrn} admitted`);
  console.log(`Location: ${event.encounter.location.room}`);
}
```

### Parsing CSV Data

```typescript
import { parseCSV } from '@fi-fhir/sdk';

const csv = `mrn,first_name,last_name,dob,gender
123456,John,Doe,1980-03-15,M`;

const events = await parseCSV(csv, {
  eventType: 'patient',
  hasHeader: true,
});

for (const event of events) {
  console.log(`Parsed patient: ${event.patient.mrn}`);
}
```

### Schema Inference

```typescript
import { parseCSVWithSchema } from '@fi-fhir/sdk';

const result = await parseCSVWithSchema(csvContent);

console.log('Inferred columns:');
for (const col of result.schema.columns) {
  console.log(`  ${col.name}: ${col.inferred_type} (${col.semantic_hint})`);
}
```

### Workflow Processing

```typescript
import { Workflow, parseCSV } from '@fi-fhir/sdk';

// Load workflow configuration
const workflow = new Workflow('./workflow.yaml');

// Validate configuration
const validation = await workflow.validate();
if (!validation.valid) {
  console.error('Invalid workflow:', validation.errors);
  process.exit(1);
}

// Parse events
const events = await parseCSV(csvContent, { eventType: 'patient' });

// Process through workflow
const result = await workflow.run(events);
console.log(`Processed ${result.eventsProcessed} events`);
console.log(`Matched ${result.routeMatches} routes`);

// Or dry-run to see what would match
const dryRun = await workflow.dryRun(events);
for (const eventResult of dryRun) {
  console.log(`Event ${eventResult.eventIndex}:`);
  for (const route of eventResult.routes) {
    const status = route.matched ? 'MATCH' : 'NO MATCH';
    console.log(`  ${route.name}: ${status}`);
  }
}
```

## API Reference

### Parsing Functions

#### `parse(content, options?)`

Parse any supported format (auto-detects format).

#### `parseHL7(content, options?)`

Parse an HL7v2 message. Returns a single event.

#### `parseCSV(content, options?)`

Parse CSV/flatfile data. Returns an array of events.

Options:
- `hasHeader`: First row is header (default: true)
- `delimiter`: Field delimiter (default: ',')
- `eventType`: 'patient' or 'lab'

#### `parseCSVWithSchema(content, options?)`

Parse CSV with schema inference. Returns events and inferred schema.

### Workflow Class

#### `new Workflow(configPath)`

Create a workflow instance from a YAML configuration file.

#### `workflow.validate()`

Validate the workflow configuration.

#### `workflow.run(events)`

Process events through the workflow routes.

#### `workflow.dryRun(events)`

Simulate processing without executing actions.

### Type Guards

```typescript
import { isPatientEvent, isLabEvent, isAppointmentEvent } from '@fi-fhir/sdk';

if (isPatientEvent(event)) {
  // event is PatientAdmitEvent | PatientUpdateEvent | etc.
  console.log(event.patient.mrn);
}

if (isLabEvent(event)) {
  // event is LabResultEvent
  console.log(event.test.local_code, event.result.value);
}
```

## Event Types

The SDK provides full TypeScript types for all healthcare events:

- `PatientAdmitEvent`
- `PatientUpdateEvent`
- `PatientDischargeEvent`
- `PatientTransferEvent`
- `LabResultEvent`
- `AppointmentEvent`

See `src/types/events.ts` for complete type definitions.

## Error Handling

```typescript
import { parseHL7, FiFhirError } from '@fi-fhir/sdk';

try {
  const event = await parseHL7(invalidMessage);
} catch (error) {
  if (error instanceof FiFhirError) {
    console.error(`Parse error (exit code ${error.exitCode}):`, error.message);
    console.error('stderr:', error.stderr);
  }
}
```

## Environment Variables

- `FI_FHIR_PATH`: Path to fi-fhir binary (default: looks in PATH)

## License

MIT
