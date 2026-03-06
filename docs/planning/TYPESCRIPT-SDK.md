# TypeScript SDK Planning

This document describes the TypeScript/JavaScript SDK for fi-fhir.

## Overview

The TypeScript SDK provides a native JavaScript interface to fi-fhir functionality, enabling:
- Healthcare message parsing in Node.js applications
- Type-safe event handling with TypeScript definitions
- Easy integration with existing JavaScript/TypeScript codebases

## Architecture Options

| Approach | Pros | Cons |
|----------|------|------|
| **CLI Wrapper** | Simple, no build complexity | Process spawn overhead |
| **HTTP Server** | Language agnostic, scalable | Requires running service |
| **WebAssembly** | Browser support, fast | WASM size, Go WASM limitations |
| **Native Addon** | Best performance | Complex build, platform-specific |

**Chosen: CLI Wrapper** - Simplest to maintain, acceptable performance for most use cases.

## Package Structure

```
sdk/typescript/
├── package.json
├── tsconfig.json
├── src/
│   ├── index.ts           # Main exports
│   ├── parser.ts          # Parsing functions
│   ├── workflow.ts        # Workflow execution
│   ├── types/
│   │   ├── events.ts      # Event type definitions
│   │   ├── patient.ts     # Patient types
│   │   ├── encounter.ts   # Encounter types
│   │   └── lab.ts         # Lab result types
│   └── utils/
│       └── cli.ts         # CLI wrapper utilities
├── tests/
│   ├── parser.test.ts
│   └── workflow.test.ts
└── examples/
    ├── parse-hl7.ts
    └── csv-workflow.ts
```

## API Design

### Parsing

```typescript
import { parse, parseHL7, parseCSV } from '@fi-fhir/sdk';

// Auto-detect format
const events = await parse(messageContent);

// Explicit format
const hl7Event = await parseHL7(hl7Message, {
  source: 'epic_adt',
  profile: './profiles/epic.yaml'
});

// CSV with options
const csvEvents = await parseCSV(csvContent, {
  hasHeader: true,
  delimiter: ',',
  eventType: 'patient'
});
```

### Event Types

```typescript
interface EventMeta {
  id: string;
  type: EventType;
  timestamp: string;
  receivedAt: string;
  source: string;
  sourceFormat: SourceFormat;
  sourceMessageId?: string;
}

interface PatientAdmitEvent extends EventMeta {
  type: 'patient_admit';
  patient: Patient;
  encounter: Encounter;
}

interface LabResultEvent extends EventMeta {
  type: 'lab_result';
  patient: Patient;
  test: LabTest;
  result: LabValue;
  isCritical: boolean;
}

interface EligibilityInquiryEvent extends EventMeta {
  type: 'eligibility_inquiry';
  informationSource: Provider;
  informationReceiver: Provider;
  subscriber: Patient;
  dependent?: Patient;
  inquiry: EligibilityInquiry;
  traceNumber?: string;
}

interface EligibilityResponseEvent extends EventMeta {
  type: 'eligibility_response';
  informationSource: Provider;
  informationReceiver: Provider;
  subscriber: Patient;
  dependent?: Patient;
  status: EligibilityStatus;
  benefits?: EligibilityBenefit[];
  errors?: EligibilityValidationError[];
  traceNumber?: string;
  planBeginDate?: string;
  planEndDate?: string;
}

interface ClaimStatusRequestEvent extends EventMeta {
  type: 'claim_status_request';
  payer: Provider;
  provider: Provider;
  subscriber: Patient;
  dependent?: Patient;
  inquiry: ClaimStatusInquiry;
  traceNumber?: string;
}

interface ClaimStatusResponseEvent extends EventMeta {
  type: 'claim_status_response';
  payer: Provider;
  provider: Provider;
  subscriber: Patient;
  dependent?: Patient;
  claimSubmitterID?: string;
  payerClaimID?: string;
  statuses: ClaimStatusInfo[];
  serviceLines?: ClaimServiceLineStatus[];
  traceNumber?: string;
}

type HealthcareEvent =
  | PatientAdmitEvent
  | PatientUpdateEvent
  | PatientDischargeEvent
  | LabResultEvent
  | AppointmentEvent
  | ClaimSubmittedEvent
  | ClaimAdjudicatedEvent
  | EligibilityInquiryEvent
  | EligibilityResponseEvent
  | ClaimStatusRequestEvent
  | ClaimStatusResponseEvent;
```

### Workflow

```typescript
import { Workflow } from '@fi-fhir/sdk';

const workflow = await Workflow.load('./workflow.yaml');

// Process events
const results = await workflow.process(events);

// Dry run
const dryRunResults = await workflow.dryRun(events);
```

Workflow YAML files support CEL expressions for complex filtering:

```yaml
routes:
  - name: critical_labs
    filter:
      event_type: lab_result
      condition: event.result.interpretation == "critical"  # CEL expression
    actions:
      - type: webhook
        url: https://alerts.example.com
```

See [WORKFLOW-DSL.md](WORKFLOW-DSL.md) for the full CEL Quick Reference.

### Streaming

```typescript
import { createHL7Parser } from '@fi-fhir/sdk';

const parser = createHL7Parser({ source: 'lab_system' });

// Stream processing
inputStream
  .pipe(parser)
  .on('event', (event) => console.log(event))
  .on('warning', (warning) => console.warn(warning))
  .on('error', (error) => console.error(error));
```

## Implementation

### CLI Wrapper

```typescript
// src/utils/cli.ts
import { spawn } from 'child_process';
import { join } from 'path';

const BINARY_PATH = process.env.FI_FHIR_PATH || 'fi-fhir';

export async function execFiFhir(
  args: string[],
  input?: string
): Promise<{ stdout: string; stderr: string }> {
  return new Promise((resolve, reject) => {
    const proc = spawn(BINARY_PATH, args, {
      stdio: ['pipe', 'pipe', 'pipe']
    });

    let stdout = '';
    let stderr = '';

    proc.stdout.on('data', (data) => { stdout += data; });
    proc.stderr.on('data', (data) => { stderr += data; });

    if (input) {
      proc.stdin.write(input);
      proc.stdin.end();
    }

    proc.on('close', (code) => {
      if (code === 0) {
        resolve({ stdout, stderr });
      } else {
        reject(new Error(`fi-fhir exited with code ${code}: ${stderr}`));
      }
    });
  });
}
```

### Parser Implementation

```typescript
// src/parser.ts
import { execFiFhir } from './utils/cli';
import type { HealthcareEvent, ParseOptions } from './types';

export async function parse(
  content: string,
  options: ParseOptions = {}
): Promise<HealthcareEvent[]> {
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
  if (options.eventType) {
    args.push('-t', options.eventType);
  }

  args.push('-'); // Read from stdin

  const { stdout } = await execFiFhir(args, content);

  const result = JSON.parse(stdout);

  // Normalize to array
  return Array.isArray(result) ? result : [result];
}

export async function parseHL7(
  content: string,
  options: Omit<ParseOptions, 'format'> = {}
): Promise<HealthcareEvent> {
  const events = await parse(content, { ...options, format: 'hl7v2' });
  return events[0];
}

export async function parseCSV(
  content: string,
  options: CSVParseOptions = {}
): Promise<HealthcareEvent[]> {
  const args = ['parse', '-f', 'csv'];

  if (options.source) args.push('-s', options.source);
  if (options.eventType) args.push('-t', options.eventType);
  if (options.delimiter) args.push('-d', options.delimiter);
  if (options.hasHeader === false) args.push('--no-header');
  if (options.inferSchema) args.push('--infer-schema');

  args.push('-');

  const { stdout } = await execFiFhir(args, content);
  return JSON.parse(stdout);
}
```

## Type Generation

Types are manually maintained to match Go structs in `pkg/events/events.go`.

Future: Consider generating TypeScript types from Go structs using:
- `go-typescript` tool
- JSON Schema generation from Go → TypeScript from JSON Schema

## Binary Distribution

Options for distributing the fi-fhir binary:
1. **npm postinstall** - Download platform-specific binary during install
2. **Bundled binaries** - Include all platform binaries in npm package
3. **External dependency** - Require user to install fi-fhir separately

Recommended: platform-specific npm packages (one per OS/arch) installed as optional dependencies.

```json
{
  "optionalDependencies": {
    "@fi-fhir/fi-fhir-darwin-arm64": "0.1.0",
    "@fi-fhir/fi-fhir-darwin-x64": "0.1.0",
    "@fi-fhir/fi-fhir-linux-arm64": "0.1.0",
    "@fi-fhir/fi-fhir-linux-x64": "0.1.0",
    "@fi-fhir/fi-fhir-win32-x64": "0.1.0"
  }
}
```

CI publishes these packages on tags, using the binaries built by `release:binaries`.

## Testing Strategy

```typescript
// tests/parser.test.ts
import { describe, it, expect } from 'vitest';
import { parseHL7, parseCSV } from '../src';

describe('parseHL7', () => {
  it('parses ADT^A01 message', async () => {
    const message = `MSH|^~\\&|EPIC|...`;
    const event = await parseHL7(message);

    expect(event.type).toBe('patient_admit');
    expect(event.patient.mrn).toBeDefined();
  });
});

describe('parseCSV', () => {
  it('parses patient CSV', async () => {
    const csv = `mrn,first_name,last_name\n123,John,Doe`;
    const events = await parseCSV(csv, { eventType: 'patient' });

    expect(events).toHaveLength(1);
    expect(events[0].patient.mrn).toBe('123');
  });
});
```

## Implementation Plan

### Phase 1: Core SDK ✅
- [x] Package setup (package.json, tsconfig, vitest) - see `sdk/typescript/`
- [x] CLI wrapper utility with timeout support - see `src/utils/cli.ts`
- [x] Parse functions (HL7, CSV, auto-detect) - see `src/parser.ts`
- [x] Event type definitions with type guards - see `src/types/events.ts`
- [x] Basic tests - see `tests/parser.test.ts`
- [x] FiFhirError class with structured error info

### Phase 2: Workflow Support ✅
- [x] Workflow class with load(), validate(), run() - see `src/workflow.ts`
- [x] Dry-run mode with route matching info
- [x] Error handling with FiFhirError
- [x] Workflow tests - see `tests/workflow.test.ts`
- [ ] Streaming API (future) — tracked in [libs/fi-fhir#3](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/3)

### Phase 3: Distribution 🔲
- [x] Platform-specific packages (optional dependencies)
- [x] npm publish workflow (GitLab npm registry) on tags
- [ ] Streaming API (future) — tracked in [libs/fi-fhir#3](https://gitlab.flexinfer.ai/libs/fi-fhir/-/issues/3)

> **Note**: SDK is functional and distribution automation is implemented via GitLab npm publish on tags.

## See Also

- [WORKFLOW-DSL.md](WORKFLOW-DSL.md) - Workflow YAML format processed by SDK Workflow class
- [SOURCE-PROFILES.md](SOURCE-PROFILES.md) - Profile option passed to parse functions
- [HL7V2-QUIRKS.md](HL7V2-QUIRKS.md) - HL7v2 parsing behavior exposed via parseHL7()
- [EDI-COMPLEXITIES.md](EDI-COMPLEXITIES.md) - EDI X12 parsing (837, 835, 270/271, 276/277) for claims, eligibility, and claim status
