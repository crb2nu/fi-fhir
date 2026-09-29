/** Display tokens for the Definitions tab. */
import type { BadgeTone } from '$lib/ui/primitives';
import type { ValidationLabel } from './definitionDraft';

export function stateTone(state: string): BadgeTone {
  switch (state) {
    case 'deployed':
      return 'success';
    case 'published':
    case 'approved':
    case 'validated':
      return 'info';
    case 'paused':
      return 'warning';
    case 'retired':
      return 'neutral';
    default:
      return 'accent';
  }
}

export function validationTone(label: ValidationLabel): BadgeTone {
  switch (label) {
    case 'current':
      return 'success';
    case 'expired':
      return 'warning';
    case 'failed':
      return 'danger';
    default:
      return 'neutral';
  }
}

export const VALIDATION_TEXT: Record<ValidationLabel, string> = {
  none: 'Not validated',
  current: 'Current',
  expired: 'Expired',
  failed: 'Failed'
};

/** `2026-09-29 04:11:07Z`: second precision for evidence times. */
export function formatSecond(value: string | null | undefined): string {
  if (!value) return '—';
  const parsed = new Date(value);
  if (Number.isNaN(parsed.getTime())) return value;
  return parsed.toISOString().slice(0, 19).replace('T', ' ') + 'Z';
}
