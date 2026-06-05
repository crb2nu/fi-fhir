/**
 * Tests for TaskPanel — focused on the non-color priority cue (WCAG 1.4.1).
 *
 * Priority must be conveyed by a visible text label, not the colored dot alone.
 */
import { describe, it, expect, afterEach } from 'vitest';
import { render, cleanup } from '@testing-library/svelte';
import TaskPanel from './TaskPanel.svelte';
import { collaborationState, type IntegrationTask } from './collaborationStore';

function makeTask(overrides: Partial<IntegrationTask>): IntegrationTask {
  return {
    id: overrides.id ?? 't1',
    title: overrides.title ?? 'Task',
    status: overrides.status ?? 'pending',
    priority: overrides.priority ?? 'medium',
    creator: 'tester',
    createdAt: 1_700_000_000_000,
    updatedAt: 1_700_000_000_000,
    ...overrides
  };
}

const tasks: IntegrationTask[] = [
  makeTask({ id: 'c', title: 'Critical task', priority: 'critical' }),
  makeTask({ id: 'h', title: 'High task', priority: 'high' }),
  makeTask({ id: 'm', title: 'Medium task', priority: 'medium' }),
  makeTask({ id: 'l', title: 'Low task', priority: 'low' })
];

function seed(): void {
  // Seeding tasks suppresses the onMount fetch (guarded by tasks.length === 0).
  collaborationState.set({
    presence: [],
    tasks,
    handoffs: [],
    fileClaims: [],
    isLoading: false,
    error: null
  });
}

afterEach(() => {
  cleanup();
  collaborationState.set({
    presence: [],
    tasks: [],
    handoffs: [],
    fileClaims: [],
    isLoading: false,
    error: null
  });
});

describe('TaskPanel priority cue', () => {
  it('renders a visible priority text label for every task', () => {
    seed();
    const { container } = render(TaskPanel);

    const tags = container.querySelectorAll('.task-priority-tag');
    expect(tags.length).toBe(tasks.length);

    const labels = Array.from(tags, (t) => t.textContent?.trim());
    expect(labels).toEqual(expect.arrayContaining(['Critical', 'High', 'Medium', 'Low']));
  });

  it('applies a per-priority class so the cue is not color-only', () => {
    seed();
    const { container } = render(TaskPanel);

    expect(container.querySelector('.task-priority-tag.priority-critical')).not.toBeNull();
    expect(container.querySelector('.task-priority-tag.priority-high')).not.toBeNull();
    expect(container.querySelector('.task-priority-tag.priority-medium')).not.toBeNull();
    expect(container.querySelector('.task-priority-tag.priority-low')).not.toBeNull();
  });

  it('marks the decorative colored dot aria-hidden (text label carries the meaning)', () => {
    seed();
    const { container } = render(TaskPanel);

    const dots = container.querySelectorAll('.task-priority-dot');
    expect(dots.length).toBe(tasks.length);
    for (const dot of dots) {
      expect(dot.getAttribute('aria-hidden')).toBe('true');
      expect(dot.getAttribute('title')).toMatch(/priority$/);
    }
  });
});
