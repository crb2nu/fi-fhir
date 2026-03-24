export type DebugSessionState = 'idle' | 'running' | 'paused' | 'completed' | 'stopped';

export type BreakpointType = 'route' | 'action' | 'transform';

export interface Breakpoint {
  id: string;
  type: BreakpointType;
  name: string;
  enabled: boolean;
}

export interface DebugStep {
  stepNumber: number;
  kind: 'route' | 'action' | 'transform';
  name: string;
  variables: Record<string, unknown>;
  timestamp: string;
  spanName: string;
}

export interface DebugSession {
  id: string;
  workflowId: string;
  state: DebugSessionState;
  breakpoints: Breakpoint[];
  steps: DebugStep[];
  createdAt: string;
}

export interface TraceSpan {
  id: string;
  name: string;
  parentId: string | null;
  startTime: string;
  endTime: string | null;
  status: 'ok' | 'error' | 'unset';
  attributes: Record<string, unknown>;
  events: TraceSpanEvent[];
}

export interface TraceSpanEvent {
  name: string;
  timestamp: string;
  attributes: Record<string, unknown>;
}

export interface ParseEvent {
  segmentIndex: number;
  segmentType: string;
  rawSegment: string;
  fields: Record<string, unknown>;
  warnings: string[];
  isComplete: boolean;
}

export type EventLineageStage = 'source' | 'parse' | 'events' | 'workflow' | 'actions';

export interface EventLineageNode {
  stage: EventLineageStage;
  label: string;
  detail: string;
  status: 'success' | 'warning' | 'error' | 'pending';
}
