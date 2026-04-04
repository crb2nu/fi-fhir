/**
 * HUD SSE event subscription for ambient platform awareness.
 * Connects to the HUD event stream and maintains a rolling buffer
 * of recent events. Falls back to simulated events when SSE is unavailable.
 */
import { writable, get } from 'svelte/store';
import { browser } from '$app/environment';
import { PLATFORM_CONFIG } from './config';
import { platformState } from './platformStore';

// ─── Types ────────────────────────────────────────────────────────────────────

export interface HudEvent {
  type: 'agent_presence' | 'system_health' | 'task_notification' | 'session_update';
  payload: Record<string, unknown>;
  timestamp: number;
}

// ─── Store ────────────────────────────────────────────────────────────────────

const MAX_EVENTS = 100;

export const hudEvents = writable<HudEvent[]>([]);

// ─── Simulated events ─────────────────────────────────────────────────────────

const SIMULATED_EVENTS: Array<() => HudEvent> = [
  () => ({
    type: 'agent_presence',
    payload: {
      agent_id: 'mapping-studio',
      agent_type: 'ui',
      status: 'active',
      namespace: 'mapping-studio/operator',
    },
    timestamp: Date.now(),
  }),
  () => ({
    type: 'system_health',
    payload: {
      component: 'fi-fhir-engine',
      status: 'healthy',
      uptime_hours: 72.4,
      active_workflows: 4,
    },
    timestamp: Date.now(),
  }),
  () => ({
    type: 'task_notification',
    payload: {
      task_id: `task-${Math.floor(Math.random() * 1000)}`,
      action: ['completed', 'created', 'updated'][Math.floor(Math.random() * 3)],
      title: [
        'ADT-to-FHIR mapping review',
        'Terminology valueset refresh',
        'Lab pipeline error investigation',
        'Pharmacy-feed route update',
      ][Math.floor(Math.random() * 4)],
    },
    timestamp: Date.now(),
  }),
  () => ({
    type: 'session_update',
    payload: {
      session_id: `session-${Math.floor(Math.random() * 100)}`,
      agent_id: ['claude-code', 'gemini', 'codex'][Math.floor(Math.random() * 3)],
      status: ['active', 'ended'][Math.floor(Math.random() * 2)],
    },
    timestamp: Date.now(),
  }),
  () => ({
    type: 'system_health',
    payload: {
      component: 'terminology-svc',
      status: ['healthy', 'degraded'][Math.floor(Math.random() * 2)],
      avg_latency_ms: 80 + Math.floor(Math.random() * 260),
    },
    timestamp: Date.now(),
  }),
];

function pushEvent(event: HudEvent): void {
  hudEvents.update((events) => {
    const next = [event, ...events];
    return next.length > MAX_EVENTS ? next.slice(0, MAX_EVENTS) : next;
  });
}

// ─── SSE subscription ─────────────────────────────────────────────────────────

export function subscribeHudEvents(): () => void {
  if (!browser) return () => {};

  const endpoint = PLATFORM_CONFIG.endpoint.replace(/\/mcp$/, '');
  const sseUrl = `${endpoint}/api/events`;
  let eventSource: EventSource | null = null;
  let simulatedTimer: ReturnType<typeof setInterval> | null = null;
  let disposed = false;

  function connectSSE() {
    if (disposed) return;

    try {
      eventSource = new EventSource(sseUrl);

      eventSource.onmessage = (msg) => {
        try {
          const data = JSON.parse(msg.data) as Partial<HudEvent>;
          if (data.type && data.payload) {
            pushEvent({
              type: data.type as HudEvent['type'],
              payload: data.payload as Record<string, unknown>,
              timestamp: data.timestamp ?? Date.now(),
            });
          }
        } catch {
          // Ignore malformed events
        }
      };

      eventSource.onerror = () => {
        // SSE connection failed — close and fall back to simulation
        eventSource?.close();
        eventSource = null;
        startSimulation();
      };
    } catch {
      // EventSource constructor failed — fall back to simulation
      startSimulation();
    }
  }

  function startSimulation() {
    if (disposed || simulatedTimer) return;

    simulatedTimer = setInterval(() => {
      if (disposed) return;
      // Generate 1-2 events per tick
      const count = Math.random() > 0.5 ? 2 : 1;
      for (let i = 0; i < count; i++) {
        const generator = SIMULATED_EVENTS[Math.floor(Math.random() * SIMULATED_EVENTS.length)]!;
        pushEvent(generator());
      }
    }, 5000);
  }

  // Try real SSE first if platform is connected
  const state = get(platformState);
  if (state.connected && PLATFORM_CONFIG.enabled) {
    connectSSE();
  } else {
    startSimulation();
  }

  // Cleanup function
  return () => {
    disposed = true;
    eventSource?.close();
    eventSource = null;
    if (simulatedTimer) {
      clearInterval(simulatedTimer);
      simulatedTimer = null;
    }
  };
}
