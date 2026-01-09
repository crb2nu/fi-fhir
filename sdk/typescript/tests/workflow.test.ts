import { describe, it, expect, beforeAll, afterAll } from 'vitest';
import { writeFileSync, unlinkSync, mkdirSync } from 'fs';
import { join } from 'path';
import { tmpdir } from 'os';
import { Workflow, isFiFhirAvailable } from '../src';

describe('Workflow', () => {
  let fiFhirAvailable: boolean;
  let tempDir: string;
  let workflowPath: string;

  beforeAll(async () => {
    fiFhirAvailable = await isFiFhirAvailable();

    // Create temp workflow file
    tempDir = join(tmpdir(), 'fi-fhir-test-' + Date.now());
    try {
      mkdirSync(tempDir, { recursive: true });
    } catch {
      // Directory may already exist
    }

    workflowPath = join(tempDir, 'test-workflow.yaml');
    writeFileSync(workflowPath, `
workflow:
  name: test_workflow
  version: "1.0"
  routes:
    - name: patient_events
      filter:
        event_type: patient_update
      actions:
        - type: log
          level: info
          message: "Patient event processed"
    - name: all_events
      actions:
        - type: log
          level: debug
`);
  });

  afterAll(() => {
    try {
      unlinkSync(workflowPath);
    } catch {
      // File may not exist
    }
  });

  describe('validate', () => {
    it.skipIf(!fiFhirAvailable)('validates valid workflow', async () => {
      const workflow = new Workflow(workflowPath);
      const result = await workflow.validate();

      expect(result.valid).toBe(true);
      expect(result.name).toBe('test_workflow');
      expect(result.version).toBe('1.0');
      expect(result.routes).toHaveLength(2);
      expect(result.routes?.[0].name).toBe('patient_events');
    });

    it.skipIf(!fiFhirAvailable)('returns error for invalid workflow', async () => {
      const invalidPath = join(tempDir, 'invalid.yaml');
      writeFileSync(invalidPath, `
workflow:
  routes: []
`);

      const workflow = new Workflow(invalidPath);
      const result = await workflow.validate();

      expect(result.valid).toBe(false);
      expect(result.errors).toBeDefined();

      unlinkSync(invalidPath);
    });
  });

  describe('run', () => {
    it.skipIf(!fiFhirAvailable)('processes events through workflow', async () => {
      const workflow = new Workflow(workflowPath);

      const events = [
        {
          type: 'patient_update',
          id: '1',
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
          source: 'test',
          source_format: 'csv',
          patient: { mrn: '123456' }
        }
      ];

      const result = await workflow.run(events);

      expect(result.eventsProcessed).toBe(1);
      expect(result.routeMatches).toBeGreaterThan(0);
      expect(result.errors).toBe(0);
    });

    it.skipIf(!fiFhirAvailable)('accepts JSON string input', async () => {
      const workflow = new Workflow(workflowPath);

      const json = JSON.stringify([
        {
          type: 'patient_update',
          id: '1',
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
          source: 'test',
          source_format: 'csv',
          patient: { mrn: '123456' }
        }
      ]);

      const result = await workflow.run(json);

      expect(result.eventsProcessed).toBe(1);
    });
  });

  describe('dryRun', () => {
    it.skipIf(!fiFhirAvailable)('shows which routes would match', async () => {
      const workflow = new Workflow(workflowPath);

      const events = [
        {
          type: 'patient_update',
          id: '1',
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
          source: 'test',
          source_format: 'csv',
          patient: { mrn: '123456' }
        },
        {
          type: 'lab_result',
          id: '2',
          timestamp: new Date().toISOString(),
          received_at: new Date().toISOString(),
          source: 'test',
          source_format: 'csv',
          patient: { mrn: '123456' },
          test: { code: 'GLU' },
          result: { value: '95' }
        }
      ];

      const results = await workflow.dryRun(events);

      expect(results).toHaveLength(2);

      // First event (patient_update) should match both routes
      const event0Routes = results[0].routes;
      const patientRoute = event0Routes.find(r => r.name === 'patient_events');
      expect(patientRoute?.matched).toBe(true);

      // Second event (lab_result) should only match all_events
      const event1Routes = results[1].routes;
      const patientRoute2 = event1Routes.find(r => r.name === 'patient_events');
      expect(patientRoute2?.matched).toBe(false);

      const allRoute = event1Routes.find(r => r.name === 'all_events');
      expect(allRoute?.matched).toBe(true);
    });
  });

  describe('static load', () => {
    it('creates workflow instance', () => {
      const workflow = Workflow.load(workflowPath);
      expect(workflow).toBeInstanceOf(Workflow);
    });
  });
});
