/**
 * Collaboration Feature — barrel export.
 *
 * Provides presence awareness, task management, handoffs, and advisory
 * file claims for team-based integration work.
 */

// Store + types
export {
  collaborationState,
  activePresence,
  pendingHandoffs,
  myTasks,
  activeClaims,
  fetchPresence,
  createTask,
  updateTaskStatus,
  assignTask,
  createHandoff,
  acceptHandoff,
  rejectHandoff,
  claimFile,
  releaseFile,
  sortTasks,
  avatarColorForAgent,
  CURRENT_AGENT_ID
} from './collaborationStore';

export type {
  AgentPresence,
  IntegrationTask,
  Handoff,
  FileClaim,
  CollaborationState
} from './collaborationStore';

// Components
export { default as PresenceBar } from './PresenceBar.svelte';
export { default as TaskPanel } from './TaskPanel.svelte';
export { default as HandoffDialog } from './HandoffDialog.svelte';
