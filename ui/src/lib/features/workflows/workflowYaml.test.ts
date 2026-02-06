import { describe, it, expect } from 'vitest';
import { draftToYaml, yamlToDraft } from './workflowYaml';
import type { WorkflowDraft } from './workflowTypes';

describe('workflowYaml', () => {
  const sampleDraft: WorkflowDraft = {
    name: 'adt-routing',
    version: '1.0',
    routes: [
      {
        _key: 'r1',
        name: 'patient_admits',
        filter: {
          eventTypes: ['PATIENT_ADMIT', 'PATIENT_DISCHARGE'],
          sources: ['epic'],
          condition: ''
        },
        transforms: [],
        actions: [
          { _key: 'a1', type: 'fhir', config: { server: 'https://fhir.example.com' } },
          { _key: 'a2', type: 'log', config: { level: 'info', message: 'Processed' } }
        ],
        expanded: false
      },
      {
        _key: 'r2',
        name: 'critical_labs',
        filter: {
          eventTypes: ['LAB_RESULT'],
          sources: [],
          condition: 'event.isCritical == true'
        },
        transforms: [],
        actions: [{ _key: 'a3', type: 'webhook', config: { url: 'https://alerts.example.com' } }],
        expanded: true
      }
    ]
  };

  describe('draftToYaml', () => {
    it('generates valid YAML from a draft', () => {
      const yaml = draftToYaml(sampleDraft);
      expect(yaml).toContain('name: adt-routing');
      expect(yaml).toMatch(/version: ['"]?1\.0['"]?/);
      expect(yaml).toContain('name: patient_admits');
      expect(yaml).toContain('name: critical_labs');
    });

    it('includes event types as array for multiple types', () => {
      const yaml = draftToYaml(sampleDraft);
      expect(yaml).toContain('PATIENT_ADMIT');
      expect(yaml).toContain('PATIENT_DISCHARGE');
    });

    it('includes action config fields', () => {
      const yaml = draftToYaml(sampleDraft);
      expect(yaml).toContain('https://fhir.example.com');
      expect(yaml).toContain('type: log');
    });

    it('includes CEL condition', () => {
      const yaml = draftToYaml(sampleDraft);
      expect(yaml).toContain('condition: event.isCritical == true');
    });

    it('handles empty draft', () => {
      const yaml = draftToYaml({ name: '', version: '1.0', routes: [] });
      expect(yaml).toContain('name: untitled');
      expect(yaml).toContain('routes: []');
    });
  });

  describe('yamlToDraft', () => {
    it('parses YAML back to draft structure', () => {
      const yaml = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml);

      expect(parsed.name).toBe('adt-routing');
      expect(parsed.version).toBe('1.0');
      expect(parsed.routes).toHaveLength(2);
    });

    it('preserves route names', () => {
      const yaml = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml);

      expect(parsed.routes[0]!.name).toBe('patient_admits');
      expect(parsed.routes[1]!.name).toBe('critical_labs');
    });

    it('preserves filter event types', () => {
      const yaml = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml);

      expect(parsed.routes[0]!.filter.eventTypes).toEqual([
        'PATIENT_ADMIT',
        'PATIENT_DISCHARGE'
      ]);
    });

    it('preserves actions', () => {
      const yaml = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml);

      expect(parsed.routes[0]!.actions).toHaveLength(2);
      expect(parsed.routes[0]!.actions[0]!.type).toBe('fhir');
      expect(parsed.routes[0]!.actions[0]!.config.server).toBe('https://fhir.example.com');
    });

    it('handles single event type', () => {
      const yaml = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml);

      expect(parsed.routes[1]!.filter.eventTypes).toEqual(['LAB_RESULT']);
    });

    it('round-trips correctly', () => {
      const yaml1 = draftToYaml(sampleDraft);
      const parsed = yamlToDraft(yaml1);
      const yaml2 = draftToYaml(parsed);

      // The YAML output should be structurally equivalent
      const reparsed = yamlToDraft(yaml2);
      expect(reparsed.name).toBe(sampleDraft.name);
      expect(reparsed.routes.length).toBe(sampleDraft.routes.length);
      expect(reparsed.routes[0]!.filter.eventTypes).toEqual(
        sampleDraft.routes[0]!.filter.eventTypes
      );
      expect(reparsed.routes[0]!.actions.length).toBe(sampleDraft.routes[0]!.actions.length);
    });

    it('handles workflow wrapper format', () => {
      const yaml = `workflow:\n  name: test\n  version: "1.0"\n  routes: []`;
      const parsed = yamlToDraft(yaml);
      expect(parsed.name).toBe('test');
    });

    it('throws on invalid YAML', () => {
      expect(() => yamlToDraft('not: [valid: yaml')).toThrow();
    });
  });
});
