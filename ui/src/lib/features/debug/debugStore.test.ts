/**
 * Tests for the debugStore module.
 */
import { describe, it, expect, beforeEach } from 'vitest';
import { get } from 'svelte/store';
import {
  debugSession,
  traceSpans,
  eventLineage,
  sessionState,
  currentStep,
  breakpoints,
  stepHistory,
  startSession,
  updateSessionState,
  addStep,
  addBreakpoint,
  removeBreakpoint,
  toggleBreakpoint,
  endSession,
  loadMockData
} from './debugStore';
import { mockSession, mockTraceSpans, mockEventLineage } from './debugMocks';
import type { DebugSession, DebugStep, Breakpoint } from './types';

describe('debugStore', () => {
  beforeEach(() => {
    endSession();
  });

  describe('initial state', () => {
    it('should have null session initially', () => {
      expect(get(debugSession)).toBeNull();
    });

    it('should have idle session state when no session', () => {
      expect(get(sessionState)).toBe('idle');
    });

    it('should have null current step when no session', () => {
      expect(get(currentStep)).toBeNull();
    });

    it('should have empty breakpoints when no session', () => {
      expect(get(breakpoints)).toEqual([]);
    });

    it('should have empty step history when no session', () => {
      expect(get(stepHistory)).toEqual([]);
    });
  });

  describe('startSession', () => {
    it('should set the session', () => {
      startSession(mockSession);
      const session = get(debugSession);
      expect(session).not.toBeNull();
      expect(session!.id).toBe('debug-session-1');
      expect(session!.workflowId).toBe('lab-routing-v2');
    });

    it('should update derived sessionState', () => {
      startSession(mockSession);
      expect(get(sessionState)).toBe('paused');
    });

    it('should populate breakpoints from session', () => {
      startSession(mockSession);
      expect(get(breakpoints)).toHaveLength(3);
    });

    it('should populate step history from session', () => {
      startSession(mockSession);
      expect(get(stepHistory)).toHaveLength(3);
    });

    it('should set current step to last step', () => {
      startSession(mockSession);
      const step = get(currentStep);
      expect(step).not.toBeNull();
      expect(step!.stepNumber).toBe(3);
      expect(step!.name).toBe('webhook');
    });
  });

  describe('addStep', () => {
    it('should append step and set state to paused', () => {
      const session: DebugSession = {
        ...mockSession,
        steps: [],
        state: 'running'
      };
      startSession(session);
      expect(get(stepHistory)).toHaveLength(0);

      const newStep: DebugStep = {
        stepNumber: 1,
        kind: 'route',
        name: 'test-route',
        variables: { key: 'value' },
        timestamp: new Date().toISOString(),
        spanName: 'workflow.route'
      };
      addStep(newStep);

      expect(get(stepHistory)).toHaveLength(1);
      expect(get(currentStep)!.name).toBe('test-route');
      expect(get(sessionState)).toBe('paused');
    });

    it('should not modify state when no session exists', () => {
      const step: DebugStep = {
        stepNumber: 1,
        kind: 'route',
        name: 'test',
        variables: {},
        timestamp: new Date().toISOString(),
        spanName: 'test'
      };
      addStep(step);
      expect(get(debugSession)).toBeNull();
    });
  });

  describe('addBreakpoint', () => {
    it('should add breakpoint to session', () => {
      startSession({ ...mockSession, breakpoints: [] });
      expect(get(breakpoints)).toHaveLength(0);

      const bp: Breakpoint = {
        id: 'bp-new',
        type: 'route',
        name: 'new-bp',
        enabled: true
      };
      addBreakpoint(bp);

      const currentBreakpoints = get(breakpoints);
      expect(currentBreakpoints).toHaveLength(1);
      expect(currentBreakpoints[0]?.name).toBe('new-bp');
    });
  });

  describe('removeBreakpoint', () => {
    it('should remove breakpoint by id', () => {
      startSession(mockSession);
      expect(get(breakpoints)).toHaveLength(3);

      removeBreakpoint('bp-1');
      expect(get(breakpoints)).toHaveLength(2);
      expect(get(breakpoints).find((bp) => bp.id === 'bp-1')).toBeUndefined();
    });

    it('should not error when removing non-existent breakpoint', () => {
      startSession(mockSession);
      removeBreakpoint('non-existent');
      expect(get(breakpoints)).toHaveLength(3);
    });
  });

  describe('toggleBreakpoint', () => {
    it('should toggle breakpoint enabled state', () => {
      startSession(mockSession);
      const bp = get(breakpoints).find((b) => b.id === 'bp-1');
      expect(bp!.enabled).toBe(true);

      toggleBreakpoint('bp-1');
      const toggled = get(breakpoints).find((b) => b.id === 'bp-1');
      expect(toggled!.enabled).toBe(false);

      toggleBreakpoint('bp-1');
      const toggledBack = get(breakpoints).find((b) => b.id === 'bp-1');
      expect(toggledBack!.enabled).toBe(true);
    });
  });

  describe('updateSessionState', () => {
    it('should update session state', () => {
      startSession(mockSession);
      expect(get(sessionState)).toBe('paused');

      updateSessionState('running');
      expect(get(sessionState)).toBe('running');

      updateSessionState('completed');
      expect(get(sessionState)).toBe('completed');
    });
  });

  describe('endSession', () => {
    it('should clear all state', () => {
      startSession(mockSession);
      traceSpans.set(mockTraceSpans);
      eventLineage.set(mockEventLineage);

      expect(get(debugSession)).not.toBeNull();
      expect(get(traceSpans)).toHaveLength(4);
      expect(get(eventLineage)).toHaveLength(5);

      endSession();

      expect(get(debugSession)).toBeNull();
      expect(get(traceSpans)).toHaveLength(0);
      expect(get(eventLineage)).toHaveLength(0);
      expect(get(sessionState)).toBe('idle');
      expect(get(currentStep)).toBeNull();
    });
  });

  describe('loadMockData', () => {
    it('should populate with mock data', () => {
      loadMockData();

      expect(get(debugSession)).not.toBeNull();
      expect(get(debugSession)!.id).toBe('debug-session-1');
      expect(get(traceSpans)).toHaveLength(4);
      expect(get(eventLineage)).toHaveLength(5);
      expect(get(breakpoints)).toHaveLength(3);
      expect(get(stepHistory)).toHaveLength(3);
    });
  });
});
