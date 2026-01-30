/**
 * Tests for warning domain utilities.
 */
import { describe, it, expect } from 'vitest';
import {
  hasExplanation,
  updateWarningWithExplanation,
  groupWarningsByPhase,
  type WarningLike
} from './warnings';

describe('warnings utilities', () => {
  describe('hasExplanation', () => {
    it('should return true when explanation is present', () => {
      const warning: WarningLike = {
        phase: 'syntactic',
        code: 'W001',
        message: 'Test',
        explanation: 'This is an explanation'
      };

      expect(hasExplanation(warning)).toBe(true);
    });

    it('should return false when explanation is null', () => {
      const warning: WarningLike = {
        phase: 'syntactic',
        code: 'W001',
        message: 'Test',
        explanation: null
      };

      expect(hasExplanation(warning)).toBe(false);
    });

    it('should return false when explanation is undefined', () => {
      const warning: WarningLike = {
        phase: 'syntactic',
        code: 'W001',
        message: 'Test'
      };

      expect(hasExplanation(warning)).toBe(false);
    });

    it('should return false for empty string explanation', () => {
      const warning: WarningLike = {
        phase: 'syntactic',
        code: 'W001',
        message: 'Test',
        explanation: ''
      };

      expect(hasExplanation(warning)).toBe(false);
    });
  });

  describe('updateWarningWithExplanation', () => {
    it('should update warning with matching code', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test 1' },
        { phase: 'syntactic', code: 'W002', message: 'Test 2' }
      ];

      const result = updateWarningWithExplanation(warnings, 'W001', {
        explanation: 'New explanation',
        fixSuggestion: 'Fix suggestion',
        impact: 'High impact',
        fromCache: true
      });

      expect(result[0]).toEqual({
        phase: 'syntactic',
        code: 'W001',
        message: 'Test 1',
        explanation: 'New explanation',
        fixSuggestion: 'Fix suggestion',
        impact: 'High impact',
        fromCache: true
      });
    });

    it('should not modify warnings with non-matching code', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test 1' },
        { phase: 'syntactic', code: 'W002', message: 'Test 2' }
      ];

      const result = updateWarningWithExplanation(warnings, 'W001', {
        explanation: 'New explanation',
        fromCache: false
      });

      expect(result[1]).toEqual({
        phase: 'syntactic',
        code: 'W002',
        message: 'Test 2'
      });
    });

    it('should return new array (immutable)', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test' }
      ];

      const result = updateWarningWithExplanation(warnings, 'W001', {
        explanation: 'Explanation',
        fromCache: false
      });

      expect(result).not.toBe(warnings);
    });

    it('should handle null fixSuggestion and impact', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test' }
      ];

      const result = updateWarningWithExplanation(warnings, 'W001', {
        explanation: 'Explanation',
        fixSuggestion: null,
        impact: null,
        fromCache: false
      });

      expect(result[0]!.fixSuggestion).toBeNull();
      expect(result[0]!.impact).toBeNull();
    });
  });

  describe('groupWarningsByPhase', () => {
    it('should group warnings by phase', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Syntactic warning' },
        { phase: 'semantic', code: 'W002', message: 'Semantic warning' },
        { phase: 'syntactic', code: 'W003', message: 'Another syntactic' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups).toHaveLength(2);
      expect(groups[0]!.phase).toBe('syntactic');
      expect(groups[0]!.items).toHaveLength(2);
      expect(groups[1]!.phase).toBe('semantic');
      expect(groups[1]!.items).toHaveLength(1);
    });

    it('should sort groups by phase order (byte, syntactic, semantic, edi_companion)', () => {
      const warnings: WarningLike[] = [
        { phase: 'semantic', code: 'W001', message: 'Test' },
        { phase: 'byte', code: 'W002', message: 'Test' },
        { phase: 'syntactic', code: 'W003', message: 'Test' },
        { phase: 'edi_companion', code: 'W004', message: 'Test' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups.map((g) => g.phase)).toEqual(['byte', 'syntactic', 'semantic', 'edi_companion']);
    });

    it('should place unknown phases at the end', () => {
      const warnings: WarningLike[] = [
        { phase: 'custom_phase', code: 'W001', message: 'Test' },
        { phase: 'syntactic', code: 'W002', message: 'Test' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups[0]!.phase).toBe('syntactic');
      expect(groups[1]!.phase).toBe('custom_phase');
    });

    it('should sort warnings within groups by code', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W003', message: 'Test' },
        { phase: 'syntactic', code: 'W001', message: 'Test' },
        { phase: 'syntactic', code: 'W002', message: 'Test' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups[0]!.items.map((w) => w.code)).toEqual(['W001', 'W002', 'W003']);
    });

    it('should sort by path as secondary sort key', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test', path: 'PID.3' },
        { phase: 'syntactic', code: 'W001', message: 'Test', path: 'MSH.1' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups[0]!.items[0]!.path).toBe('MSH.1');
      expect(groups[0]!.items[1]!.path).toBe('PID.3');
    });

    it('should handle empty array', () => {
      const groups = groupWarningsByPhase([]);

      expect(groups).toEqual([]);
    });

    it('should handle warnings with empty phase as "unknown"', () => {
      const warnings: WarningLike[] = [
        { phase: '', code: 'W001', message: 'Test' }
      ];

      const groups = groupWarningsByPhase(warnings);

      expect(groups[0]!.phase).toBe('unknown');
    });

    it('should handle null paths in sorting', () => {
      const warnings: WarningLike[] = [
        { phase: 'syntactic', code: 'W001', message: 'Test', path: null },
        { phase: 'syntactic', code: 'W001', message: 'Test', path: 'MSH.1' }
      ];

      const groups = groupWarningsByPhase(warnings);

      // null path should sort before 'MSH.1'
      expect(groups[0]!.items[0]!.path).toBeNull();
      expect(groups[0]!.items[1]!.path).toBe('MSH.1');
    });
  });
});
