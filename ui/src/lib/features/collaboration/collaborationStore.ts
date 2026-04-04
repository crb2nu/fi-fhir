/**
 * Collaboration Store
 *
 * Manages presence awareness, integration tasks, handoffs, and advisory
 * file claims. Currently uses mock data; will be wired to MCP tools
 * (agent-context) in a future milestone.
 */
import { writable, derived } from 'svelte/store';

// ---------------------------------------------------------------------------
// Types
// ---------------------------------------------------------------------------

export interface AgentPresence {
  agentId: string;
  agentType: 'human' | 'claude-code' | 'codex' | 'gemini' | 'kilocode';
  displayName: string;
  status: 'active' | 'idle' | 'away';
  currentFile?: string | undefined;
  currentStage?: string | undefined;
  lastSeen: number;
  avatarColor: string;
}

export interface IntegrationTask {
  id: string;
  title: string;
  description?: string | undefined;
  status: 'pending' | 'in_progress' | 'completed' | 'blocked';
  priority: 'low' | 'medium' | 'high' | 'critical';
  assignee?: string | undefined;
  creator: string;
  stage?: string | undefined;
  blockedBy?: string[] | undefined;
  createdAt: number;
  updatedAt: number;
}

export interface Handoff {
  id: string;
  fromAgent: string;
  toAgent?: string | undefined;
  status: 'pending' | 'accepted' | 'rejected';
  summary: string;
  context: {
    openDocuments: string[];
    decisions: string[];
    diagnosticCount: number;
    currentStage?: string | undefined;
  };
  createdAt: number;
}

export interface FileClaim {
  filePath: string;
  claimedBy: string;
  claimedAt: number;
  agentType: string;
}

export interface CollaborationState {
  presence: AgentPresence[];
  tasks: IntegrationTask[];
  handoffs: Handoff[];
  fileClaims: FileClaim[];
  isLoading: boolean;
  error: string | null;
}

// ---------------------------------------------------------------------------
// Avatar color palette
// ---------------------------------------------------------------------------

const AVATAR_COLORS = [
  '#6366f1',
  '#ec4899',
  '#f59e0b',
  '#10b981',
  '#0ea5e9',
  '#8b5cf6',
  '#ef4444',
  '#14b8a6'
] as const;

/** Deterministic color from agentId using a simple hash. */
export function avatarColorForAgent(agentId: string): string {
  let hash = 0;
  for (let i = 0; i < agentId.length; i++) {
    hash = (hash * 31 + agentId.charCodeAt(i)) | 0;
  }
  return AVATAR_COLORS[Math.abs(hash) % AVATAR_COLORS.length]!;
}

// ---------------------------------------------------------------------------
// Current agent identity
// ---------------------------------------------------------------------------

export const CURRENT_AGENT_ID = 'mapping-studio';

// ---------------------------------------------------------------------------
// Mock data
// ---------------------------------------------------------------------------

const now = Date.now();

function buildMockPresence(): AgentPresence[] {
  return [
    {
      agentId: 'mapping-studio',
      agentType: 'human',
      displayName: 'You',
      status: 'active',
      currentFile: '/workflows/adt-a01.yaml',
      currentStage: 'Translation',
      lastSeen: now,
      avatarColor: avatarColorForAgent('mapping-studio')
    },
    {
      agentId: 'claude-code',
      agentType: 'claude-code',
      displayName: 'Claude Code',
      status: 'active',
      currentFile: '/profiles/us-core-patient.json',
      currentStage: 'Delivery',
      lastSeen: now - 12_000,
      avatarColor: avatarColorForAgent('claude-code')
    },
    {
      agentId: 'codex-agent',
      agentType: 'codex',
      displayName: 'Codex',
      status: 'idle',
      currentStage: 'Intake',
      lastSeen: now - 180_000,
      avatarColor: avatarColorForAgent('codex-agent')
    },
    {
      agentId: 'dr-smith',
      agentType: 'human',
      displayName: 'Dr. Smith',
      status: 'away',
      lastSeen: now - 3_600_000,
      avatarColor: avatarColorForAgent('dr-smith')
    }
  ];
}

function buildMockTasks(): IntegrationTask[] {
  return [
    {
      id: 'task-1',
      title: 'Map ADT^A01 to Patient resource',
      description: 'Create FHIR Patient resource mapping from HL7v2 ADT^A01 message segments PID, PD1, NK1.',
      status: 'in_progress',
      priority: 'high',
      assignee: 'mapping-studio',
      creator: 'mapping-studio',
      stage: 'Translation',
      createdAt: now - 86_400_000,
      updatedAt: now - 3_600_000
    },
    {
      id: 'task-2',
      title: 'Review ORM routing rules',
      description: 'Validate the ORM^O01 routing rules against the latest facility requirements.',
      status: 'pending',
      priority: 'medium',
      creator: 'dr-smith',
      createdAt: now - 172_800_000,
      updatedAt: now - 172_800_000
    },
    {
      id: 'task-3',
      title: 'Fix HL7 date parsing for ADT feed',
      description: 'The DG1-5 date field contains a non-standard format from the upstream system. Parser rejects it.',
      status: 'blocked',
      priority: 'high',
      assignee: 'claude-code',
      creator: 'mapping-studio',
      stage: 'Intake',
      blockedBy: ['task-1'],
      createdAt: now - 259_200_000,
      updatedAt: now - 86_400_000
    },
    {
      id: 'task-4',
      title: 'Update profile tolerances for OBX',
      description: 'Adjust OBX segment tolerance to allow missing OBX-6 units for vital signs.',
      status: 'completed',
      priority: 'low',
      assignee: 'dr-smith',
      creator: 'dr-smith',
      stage: 'Delivery',
      createdAt: now - 432_000_000,
      updatedAt: now - 86_400_000
    },
    {
      id: 'task-5',
      title: 'Add US Core R4 terminology bindings',
      description: 'Bind required value sets for Patient.race, Patient.ethnicity, and Encounter.type.',
      status: 'pending',
      priority: 'medium',
      creator: 'claude-code',
      stage: 'Translation',
      createdAt: now - 43_200_000,
      updatedAt: now - 43_200_000
    },
    {
      id: 'task-6',
      title: 'Validate ORU^R01 output against facility spec',
      description: 'Run dry-run validation against Acme Health ORU specification document.',
      status: 'pending',
      priority: 'critical',
      assignee: 'codex-agent',
      creator: 'mapping-studio',
      stage: 'Delivery',
      createdAt: now - 21_600_000,
      updatedAt: now - 21_600_000
    }
  ];
}

function buildMockHandoffs(): Handoff[] {
  return [
    {
      id: 'handoff-1',
      fromAgent: 'claude-code',
      toAgent: 'mapping-studio',
      status: 'pending',
      summary: 'Completed FHIR mapping for ADT segments PID and NK1. Terminology review needed for 3 race/ethnicity codes that could not be auto-mapped.',
      context: {
        openDocuments: ['/workflows/adt-a01.yaml', '/profiles/us-core-patient.json'],
        decisions: ['Use US Core R4 profiles', 'Map PID-10 to Patient.race extension'],
        diagnosticCount: 12,
        currentStage: 'Translation'
      },
      createdAt: now - 1_800_000
    },
    {
      id: 'handoff-2',
      fromAgent: 'mapping-studio',
      toAgent: 'codex-agent',
      status: 'accepted',
      summary: 'Initial ORM routing configuration ready for automated testing pass.',
      context: {
        openDocuments: ['/workflows/orm-o01.yaml'],
        decisions: ['Route ORM to Lab workflow when ORC-1=NW'],
        diagnosticCount: 3,
        currentStage: 'Delivery'
      },
      createdAt: now - 7_200_000
    }
  ];
}

function buildMockFileClaims(): FileClaim[] {
  return [
    {
      filePath: '/profiles/us-core-patient.json',
      claimedBy: 'claude-code',
      claimedAt: now - 600_000,
      agentType: 'claude-code'
    },
    {
      filePath: '/workflows/orm-o01.yaml',
      claimedBy: 'codex-agent',
      claimedAt: now - 3_600_000,
      agentType: 'codex'
    },
    {
      filePath: '/terminology/race-ethnicity.csv',
      claimedBy: 'mapping-studio',
      claimedAt: now - 1_200_000,
      agentType: 'human'
    }
  ];
}

// ---------------------------------------------------------------------------
// Writable store
// ---------------------------------------------------------------------------

const initialState: CollaborationState = {
  presence: [],
  tasks: [],
  handoffs: [],
  fileClaims: [],
  isLoading: false,
  error: null
};

export const collaborationState = writable<CollaborationState>(initialState);

// ---------------------------------------------------------------------------
// Derived stores
// ---------------------------------------------------------------------------

/** Agents that are active or idle (not away). */
export const activePresence = derived(
  collaborationState,
  ($s) => $s.presence.filter((a) => a.status !== 'away')
);

/** Handoffs that are still pending acceptance. */
export const pendingHandoffs = derived(
  collaborationState,
  ($s) => $s.handoffs.filter((h) => h.status === 'pending')
);

/** Tasks assigned to the current operator. */
export const myTasks = derived(
  collaborationState,
  ($s) => $s.tasks.filter((t) => t.assignee === CURRENT_AGENT_ID)
);

/** File claims that are currently active. */
export const activeClaims = derived(
  collaborationState,
  ($s) => $s.fileClaims
);

// ---------------------------------------------------------------------------
// Priority / status sort helpers
// ---------------------------------------------------------------------------

const PRIORITY_RANK: Record<IntegrationTask['priority'], number> = {
  critical: 0,
  high: 1,
  medium: 2,
  low: 3
};

const STATUS_RANK: Record<IntegrationTask['status'], number> = {
  blocked: 0,
  in_progress: 1,
  pending: 2,
  completed: 3
};

export function sortTasks(tasks: IntegrationTask[]): IntegrationTask[] {
  return [...tasks].sort((a, b) => {
    const pd = PRIORITY_RANK[a.priority] - PRIORITY_RANK[b.priority];
    if (pd !== 0) return pd;
    return STATUS_RANK[a.status] - STATUS_RANK[b.status];
  });
}

// ---------------------------------------------------------------------------
// Actions — presence
// ---------------------------------------------------------------------------

export async function fetchPresence(): Promise<void> {
  collaborationState.update((s) => ({ ...s, isLoading: true, error: null }));

  // Simulate MCP call
  await new Promise((r) => setTimeout(r, 120));

  collaborationState.update((s) => ({
    ...s,
    presence: buildMockPresence(),
    tasks: buildMockTasks(),
    handoffs: buildMockHandoffs(),
    fileClaims: buildMockFileClaims(),
    isLoading: false
  }));
}

// ---------------------------------------------------------------------------
// Actions — tasks
// ---------------------------------------------------------------------------

export async function createTask(
  task: Omit<IntegrationTask, 'id' | 'createdAt' | 'updatedAt'>
): Promise<void> {
  const ts = Date.now();
  const newTask: IntegrationTask = {
    ...task,
    id: `task-${ts}`,
    createdAt: ts,
    updatedAt: ts
  };
  collaborationState.update((s) => ({
    ...s,
    tasks: [...s.tasks, newTask]
  }));
}

export async function updateTaskStatus(
  id: string,
  status: IntegrationTask['status']
): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    tasks: s.tasks.map((t) =>
      t.id === id ? { ...t, status, updatedAt: Date.now() } : t
    )
  }));
}

export async function assignTask(id: string, agentId: string): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    tasks: s.tasks.map((t) =>
      t.id === id ? { ...t, assignee: agentId, updatedAt: Date.now() } : t
    )
  }));
}

// ---------------------------------------------------------------------------
// Actions — handoffs
// ---------------------------------------------------------------------------

export async function createHandoff(
  handoff: Omit<Handoff, 'id' | 'status' | 'createdAt'>
): Promise<void> {
  const ts = Date.now();
  const newHandoff: Handoff = {
    ...handoff,
    id: `handoff-${ts}`,
    status: 'pending',
    createdAt: ts
  };
  collaborationState.update((s) => ({
    ...s,
    handoffs: [...s.handoffs, newHandoff]
  }));
}

export async function acceptHandoff(id: string): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    handoffs: s.handoffs.map((h) =>
      h.id === id ? { ...h, status: 'accepted' as const } : h
    )
  }));
}

export async function rejectHandoff(id: string): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    handoffs: s.handoffs.map((h) =>
      h.id === id ? { ...h, status: 'rejected' as const } : h
    )
  }));
}

// ---------------------------------------------------------------------------
// Actions — file claims
// ---------------------------------------------------------------------------

export async function claimFile(filePath: string): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    fileClaims: [
      ...s.fileClaims.filter((c) => c.filePath !== filePath),
      {
        filePath,
        claimedBy: CURRENT_AGENT_ID,
        claimedAt: Date.now(),
        agentType: 'human'
      }
    ]
  }));
}

export async function releaseFile(filePath: string): Promise<void> {
  collaborationState.update((s) => ({
    ...s,
    fileClaims: s.fileClaims.filter((c) => c.filePath !== filePath)
  }));
}
