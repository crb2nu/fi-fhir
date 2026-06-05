/**
 * Tests for collaborationStore presentation helpers.
 */
import { describe, it, expect } from 'vitest';
import { priorityLabel, type IntegrationTask } from './collaborationStore';

describe('priorityLabel', () => {
  it('returns a human-readable label for each priority', () => {
    expect(priorityLabel('critical')).toBe('Critical');
    expect(priorityLabel('high')).toBe('High');
    expect(priorityLabel('medium')).toBe('Medium');
    expect(priorityLabel('low')).toBe('Low');
  });

  it('provides a non-color text cue for every task priority value (WCAG 1.4.1)', () => {
    const priorities: Array<IntegrationTask['priority']> = ['critical', 'high', 'medium', 'low'];
    const labels = priorities.map(priorityLabel);
    // Every priority maps to a distinct, non-empty label.
    expect(new Set(labels).size).toBe(priorities.length);
    for (const label of labels) {
      expect(label.length).toBeGreaterThan(0);
      expect(label).not.toMatch(/^#|rgb|var\(/);
    }
  });

  it('falls back to "Low" for an unexpected value', () => {
    expect(priorityLabel('unknown' as IntegrationTask['priority'])).toBe('Low');
  });
});
