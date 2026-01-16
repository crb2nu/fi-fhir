import type { ProfileFix } from './types';
import type { WarningLike } from '$lib/domain/warnings';
import type { SourceProfile, UpdateProfileInput } from '$lib/gen/graphql';

/**
 * Suggest profile fixes based on warnings from parsing.
 * Now returns fixes with UpdateProfileInput changes that can be applied via profileStore.updateLocal.
 *
 * @param warnings - Warnings from the parse result
 * @param currentProfile - Optional current profile to merge changes with
 */
export function suggestFixes(
  warnings: readonly WarningLike[],
  currentProfile?: SourceProfile | null
): ProfileFix[] {
  const fixes: ProfileFix[] = [];

  // Get current missing segments if profile exists
  const currentMissingSegments = currentProfile?.hl7v2?.tolerance?.missingSegments ?? [];

  // Missing segments (semantic warnings): code like MISSING_PV1, path like PV1
  for (const w of warnings) {
    const m = /^MISSING_([A-Z0-9]{3})$/.exec(w.code);
    const seg = m?.[1];
    if (!seg) continue;

    // Skip if already tolerating this segment
    if (currentMissingSegments.includes(seg)) continue;

    const newMissingSegments = [...currentMissingSegments, seg].sort();

    // Use type assertion for partial update - profileStore.updateLocal handles merging
    fixes.push({
      id: `tolerate-missing-${seg}`,
      title: `Tolerate missing ${seg}`,
      description: `Allow parsing to continue when ${seg} is absent (records a warning instead of failing).`,
      changes: {
        hl7v2: {
          tolerance: {
            missingSegments: newMissingSegments
          }
        }
      } as UpdateProfileInput
    });
  }

  // Identifier validation policy suggestions based on validator warning codes.
  // These codes originate from `pkg/validate/identifiers.go`.
  for (const w of warnings) {
    if (w.code.startsWith('INVALID_NPI_')) {
      fixes.push({
        id: 'idval-npi-warn',
        title: 'Enable NPI validation (warn)',
        description: 'Validate NPI values and record invalid NPIs as warnings (recommended default).',
        changes: {
          identifiers: {
            validation: {
              npi: { enabled: true, onInvalid: 'warn' }
            }
          }
        } as UpdateProfileInput
      });
    }

    if (w.code.startsWith('INVALID_MBI_')) {
      fixes.push({
        id: 'idval-mbi-warn',
        title: 'Enable MBI validation (warn)',
        description: 'Validate MBI values and record invalid MBIs as warnings (recommended default).',
        changes: {
          identifiers: {
            validation: {
              mbi: { enabled: true, onInvalid: 'warn' }
            }
          }
        } as UpdateProfileInput
      });
    }

    if (w.code.startsWith('INVALID_SSN_')) {
      fixes.push({
        id: 'idval-ssn-warn',
        title: 'Enable SSN validation (warn)',
        description: 'Validate SSNs and record invalid SSNs as warnings (recommended default).',
        changes: {
          identifiers: {
            validation: {
              ssn: { enabled: true, onInvalid: 'warn' }
            }
          }
        } as UpdateProfileInput
      });
    }
  }

  // Dedupe by id
  const byId = new Map<string, ProfileFix>();
  for (const f of fixes) byId.set(f.id, f);
  return Array.from(byId.values()).sort((a, b) => a.id.localeCompare(b.id));
}
