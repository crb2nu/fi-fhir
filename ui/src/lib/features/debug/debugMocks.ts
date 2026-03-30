import type {
  DebugSession,
  DebugStep,
  TraceSpan,
  EventLineageNode,
  Breakpoint,
  ParseEvent
} from './types';

export const mockBreakpoints: Breakpoint[] = [
  { id: 'bp-1', type: 'route', name: 'lab-critical', enabled: true },
  { id: 'bp-2', type: 'action', name: 'webhook', enabled: true },
  { id: 'bp-3', type: 'transform', name: 'map_terminology', enabled: false }
];

export const mockSteps: DebugStep[] = [
  {
    stepNumber: 1,
    kind: 'route',
    name: 'lab-critical',
    variables: {
      'event.type': 'LAB_RESULT',
      'event.source': 'epic',
      'event.isCritical': true,
      'route.matched': true
    },
    timestamp: new Date().toISOString(),
    spanName: 'workflow.route'
  },
  {
    stepNumber: 2,
    kind: 'transform',
    name: 'map_terminology',
    variables: {
      'event.type': 'LAB_RESULT',
      'event.source': 'epic',
      'event.isCritical': true,
      'transform.type': 'map_terminology',
      'transform.source_system': 'LOINC',
      'transform.target_system': 'SNOMED'
    },
    timestamp: new Date().toISOString(),
    spanName: 'workflow.transform'
  },
  {
    stepNumber: 3,
    kind: 'action',
    name: 'webhook',
    variables: {
      'event.type': 'LAB_RESULT',
      'event.source': 'epic',
      'action.type': 'webhook',
      'action.url': 'https://fhir.hospital.org/Observation',
      'action.method': 'POST'
    },
    timestamp: new Date().toISOString(),
    spanName: 'workflow.action'
  }
];

export const mockSession: DebugSession = {
  id: 'debug-session-1',
  workflowId: 'lab-routing-v2',
  state: 'paused',
  breakpoints: mockBreakpoints,
  steps: mockSteps,
  createdAt: new Date().toISOString()
};

export const mockTraceSpans: TraceSpan[] = [
  {
    id: 'span-1',
    name: 'workflow.process',
    parentId: null,
    startTime: new Date(Date.now() - 500).toISOString(),
    endTime: new Date().toISOString(),
    status: 'ok',
    attributes: { 'event.type': 'LAB_RESULT', 'event.source': 'epic' },
    events: []
  },
  {
    id: 'span-2',
    name: 'workflow.route',
    parentId: 'span-1',
    startTime: new Date(Date.now() - 450).toISOString(),
    endTime: new Date(Date.now() - 100).toISOString(),
    status: 'ok',
    attributes: { 'route.name': 'lab-critical', 'route.matched': true },
    events: []
  },
  {
    id: 'span-3',
    name: 'workflow.transform',
    parentId: 'span-2',
    startTime: new Date(Date.now() - 400).toISOString(),
    endTime: new Date(Date.now() - 300).toISOString(),
    status: 'ok',
    attributes: { 'transform.type': 'map_terminology' },
    events: []
  },
  {
    id: 'span-4',
    name: 'workflow.action',
    parentId: 'span-2',
    startTime: new Date(Date.now() - 280).toISOString(),
    endTime: new Date(Date.now() - 120).toISOString(),
    status: 'ok',
    attributes: { 'action.type': 'webhook', 'action.success': true },
    events: [
      {
        name: 'http.request',
        timestamp: new Date(Date.now() - 250).toISOString(),
        attributes: { 'http.method': 'POST', 'http.status_code': 201 }
      }
    ]
  }
];

export const mockEventLineage: EventLineageNode[] = [
  {
    stage: 'source',
    label: 'Epic HL7 Feed',
    detail: 'ADT^A01 message received',
    status: 'success'
  },
  {
    stage: 'parse',
    label: 'HL7v2 Parser',
    detail: '12 segments, 2 warnings',
    status: 'warning'
  },
  {
    stage: 'events',
    label: 'LAB_RESULT',
    detail: 'Critical flag: true',
    status: 'success'
  },
  {
    stage: 'workflow',
    label: 'lab-routing-v2',
    detail: '2/3 routes matched',
    status: 'success'
  },
  {
    stage: 'actions',
    label: '3 Actions',
    detail: 'webhook, fhir, log',
    status: 'success'
  }
];

export const mockParseEvents: ParseEvent[] = [
  {
    segmentIndex: 0,
    segmentType: 'MSH',
    rawSegment:
      'MSH|^~\\&|EPIC|HOSP|FHIR|DEST|20240101120000||ADT^A01|MSG001|P|2.5.1',
    fields: { 'MSH-9': 'ADT^A01', 'MSH-10': 'MSG001', 'MSH-12': '2.5.1' },
    warnings: [],
    isComplete: false
  },
  {
    segmentIndex: 1,
    segmentType: 'PID',
    rawSegment: 'PID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M',
    fields: {
      'PID-3': 'MRN123^^^HOSP^MR',
      'PID-5': 'DOE^JOHN',
      'PID-8': 'M'
    },
    warnings: [],
    isComplete: false
  },
  {
    segmentIndex: 2,
    segmentType: 'PV1',
    rawSegment: 'PV1|1|I|ICU^101^A|||ATTENDING^DOCTOR',
    fields: { 'PV1-2': 'I', 'PV1-3': 'ICU^101^A' },
    warnings: ['PV1-7 (attending) not in standard format'],
    isComplete: true
  }
];
