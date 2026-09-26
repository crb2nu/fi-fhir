import { describe, expect, it } from 'vitest';
import {
  confidenceTone,
  decisionTone,
  equivalenceLabel,
  equivalenceTone,
  formatDurationMs,
  formatPercent,
  formatTimestamp,
  pendingStatusTone,
  workflowStatusTone
} from './terminologyFormat';

describe('terminologyFormat', () => {
  it('formats timestamps as local YYYY-MM-DD HH:mm and leaves missing values empty', () => {
    const local = new Date(2026, 8, 26, 9, 5);
    expect(formatTimestamp(local.toISOString())).toBe('2026-09-26 09:05');
    expect(formatTimestamp(local.toISOString(), false)).toBe('2026-09-26');
    expect(formatTimestamp(null)).toBe('');
    expect(formatTimestamp('not a date')).toBe('not a date');
  });

  it('formats confidence as a percentage and durations with units', () => {
    expect(formatPercent(0.953)).toBe('95%');
    expect(formatPercent(0.953, 1)).toBe('95.3%');
    expect(formatPercent(null)).toBe('');
    expect(formatDurationMs(42)).toBe('42 ms');
    expect(formatDurationMs(1500)).toBe('1.5 s');
    expect(formatDurationMs(125000)).toBe('2m 5s');
    expect(formatDurationMs(null)).toBe('');
  });

  it('keeps colour for state: only inexact matches and low confidence are tinted', () => {
    expect(equivalenceLabel('EQUIVALENT')).toBe('Equivalent');
    expect(equivalenceLabel(null)).toBe('');
    expect(equivalenceTone('EQUIVALENT')).toBe('neutral');
    expect(equivalenceTone('INEXACT')).toBe('warning');
    expect(confidenceTone(0.95)).toBe('success');
    expect(confidenceTone(0.75)).toBe('neutral');
    expect(confidenceTone(0.55)).toBe('warning');
    expect(confidenceTone(0.2)).toBe('danger');
  });

  it('maps review, decision and workflow states to badge tones', () => {
    expect(pendingStatusTone('PENDING')).toBe('warning');
    expect(pendingStatusTone('REJECTED')).toBe('danger');
    expect(decisionTone('PERSISTENT_HIT')).toBe('success');
    expect(decisionTone('NO_MATCH')).toBe('danger');
    expect(workflowStatusTone('RUNNING')).toBe('info');
    expect(workflowStatusTone('CANCELED')).toBe('neutral');
  });
});
