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

  describe('transform serialization', () => {
    const draftWithTransforms: WorkflowDraft = {
      name: 'transform-test',
      version: '1.0',
      routes: [
        {
          _key: 'r1',
          name: 'route_with_transforms',
          filter: { eventTypes: ['PATIENT_ADMIT'], sources: [], condition: '' },
          transforms: [
            { _key: 't1', type: 'set_field', config: { expression: 'event.status = "processed"' } },
            {
              _key: 't2',
              type: 'map_terminology',
              config: { field: 'code', from: 'ICD-10', to: 'SNOMED-CT' }
            },
            { _key: 't3', type: 'redact', config: { fields: 'ssn, dob' } },
            {
              _key: 't4',
              type: 'explain_warnings',
              config: { model: 'gpt-4', include_fix: 'true', cache_ttl: '24h' }
            }
          ],
          actions: [{ _key: 'a1', type: 'log', config: {} }],
          expanded: false
        }
      ]
    };

    it('serializes transforms to YAML', () => {
      const yamlStr = draftToYaml(draftWithTransforms);
      expect(yamlStr).toContain('transform:');
      expect(yamlStr).toContain('set_field:');
      expect(yamlStr).toContain('map_terminology:');
      expect(yamlStr).toContain('redact:');
      expect(yamlStr).toContain('explain_warnings:');
    });

    it('round-trips set_field transform', () => {
      const yamlStr = draftToYaml(draftWithTransforms);
      const parsed = yamlToDraft(yamlStr);
      const t = parsed.routes[0]!.transforms[0]!;
      expect(t.type).toBe('set_field');
      expect(t.config.expression).toBe('event.status = "processed"');
    });

    it('round-trips map_terminology transform', () => {
      const yamlStr = draftToYaml(draftWithTransforms);
      const parsed = yamlToDraft(yamlStr);
      const t = parsed.routes[0]!.transforms[1]!;
      expect(t.type).toBe('map_terminology');
      expect(t.config.field).toBe('code');
      expect(t.config.from).toBe('ICD-10');
      expect(t.config.to).toBe('SNOMED-CT');
    });

    it('round-trips redact transform', () => {
      const yamlStr = draftToYaml(draftWithTransforms);
      const parsed = yamlToDraft(yamlStr);
      const t = parsed.routes[0]!.transforms[2]!;
      expect(t.type).toBe('redact');
      expect(t.config.fields).toContain('ssn');
      expect(t.config.fields).toContain('dob');
    });

    it('round-trips explain_warnings transform', () => {
      const yamlStr = draftToYaml(draftWithTransforms);
      const parsed = yamlToDraft(yamlStr);
      const t = parsed.routes[0]!.transforms[3]!;
      expect(t.type).toBe('explain_warnings');
      expect(t.config.model).toBe('gpt-4');
      expect(t.config.include_fix).toBe('true');
      expect(t.config.cache_ttl).toBe('24h');
    });

    it('omits transform key when no transforms', () => {
      const draft: WorkflowDraft = {
        name: 'no-transforms',
        version: '1.0',
        routes: [
          {
            _key: 'r1',
            name: 'basic',
            filter: { eventTypes: [], sources: [], condition: '' },
            transforms: [],
            actions: [{ _key: 'a1', type: 'log', config: {} }],
            expanded: false
          }
        ]
      };
      const yamlStr = draftToYaml(draft);
      expect(yamlStr).not.toContain('transform:');
    });

    it('parses routes without transforms as empty array', () => {
      const yamlStr = `name: test\nversion: "1.0"\nroutes:\n  - name: basic\n    filter: {}\n    actions:\n      - type: log`;
      const parsed = yamlToDraft(yamlStr);
      expect(parsed.routes[0]!.transforms).toEqual([]);
    });
  });
});
