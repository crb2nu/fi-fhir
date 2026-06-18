import { describe, it, expect, vi, beforeEach } from 'vitest';

vi.mock('$lib/graphql/client', () => ({
  graphqlFetch: vi.fn(),
  isErrorToasted: vi.fn(() => false)
}));

import { graphqlFetch } from '$lib/graphql/client';
import {
  GenerateWorkflowDocument,
  ExplainWorkflowDocument,
  SuggestMappingsDocument,
  AnalyzeQualityDocument
} from '$lib/gen/graphql';
import {
  buildSuggestInput,
  buildReviewInput,
  formatGenerate,
  formatExplain,
  formatSuggest,
  formatReview,
  dispatchCopilotAction
} from './copilotDispatch';

const mockFetch = graphqlFetch as unknown as ReturnType<typeof vi.fn>;

beforeEach(() => {
  mockFetch.mockReset();
});

describe('buildSuggestInput', () => {
  it('parses a trailing "to <system>" clause into targetSystem + sourceCode', () => {
    const input = buildSuggestInput('8867-4 to http://snomed.info/sct', {});
    expect(input.sourceCode).toBe('8867-4');
    expect(input.targetSystem).toBe('http://snomed.info/sct');
  });

  it('defaults sourceSystem to "unknown" and targetSystem to LOINC when unspecified', () => {
    const input = buildSuggestInput('OBX-3', {});
    expect(input.sourceCode).toBe('OBX-3');
    expect(input.sourceSystem).toBe('unknown');
    expect(input.targetSystem).toBe('http://loinc.org');
  });

  it('prefers context metadata over parsed/default systems', () => {
    const input = buildSuggestInput('8867-4 to http://loinc.org', {
      metadata: { sourceSystem: 'urn:oid:1.2.3', targetSystem: 'http://snomed.info/sct' }
    });
    expect(input.sourceSystem).toBe('urn:oid:1.2.3');
    expect(input.targetSystem).toBe('http://snomed.info/sct');
  });

  it('strips a leading "map " verb from the source code', () => {
    const input = buildSuggestInput('map 8867-4', {});
    expect(input.sourceCode).toBe('8867-4');
  });
});

describe('buildReviewInput', () => {
  it('passes JSON input through as the event payload', () => {
    const input = buildReviewInput('{"patientId":"P1"}', {});
    expect(input.event).toEqual({ patientId: 'P1' });
    expect(input.eventType).toBe('DOCUMENT');
  });

  it('wraps non-JSON input as { raw } and honors a valid eventType hint', () => {
    const input = buildReviewInput('free text note', { metadata: { eventType: 'lab_result' } });
    expect(input.event).toEqual({ raw: 'free text note' });
    expect(input.eventType).toBe('LAB_RESULT');
  });

  it('falls back to DOCUMENT for an unrecognized eventType hint', () => {
    const input = buildReviewInput('x', { metadata: { eventType: 'NOT_A_TYPE' } });
    expect(input.eventType).toBe('DOCUMENT');
  });
});

describe('formatters', () => {
  it('formatGenerate renders explanation, a yaml block, and warnings; no model', () => {
    const r = formatGenerate({ yaml: 'routes: []', explanation: 'Routes ADT', warnings: ['heads up'] });
    expect(r.content).toContain('Routes ADT');
    expect(r.content).toContain('```yaml');
    expect(r.content).toContain('routes: []');
    expect(r.content).toContain('heads up');
    expect(r.model).toBeNull();
  });

  it('formatExplain renders summary, routes, and a mermaid diagram; no model', () => {
    const r = formatExplain({
      summary: 'Two routes',
      description: 'Handles admits',
      routeExplanations: [{ name: 'admit', trigger: 'A01', actions: ['fhir'], description: 'maps' }],
      diagram: 'graph TD;',
      warnings: []
    });
    expect(r.content).toContain('**Two routes**');
    expect(r.content).toContain('**admit**');
    expect(r.content).toContain('```mermaid');
    expect(r.model).toBeNull();
  });

  it('formatSuggest renders a candidate table with confidence percentages', () => {
    const r = formatSuggest([
      { code: '8867-4', display: 'Heart rate', system: 'LOINC', confidence: 0.94, equivalence: null, reasoning: 'strong match', score: null }
    ]);
    expect(r.content).toContain('| `8867-4` | Heart rate | LOINC | 94% |');
    expect(r.content).toContain('strong match');
    expect(r.model).toBeNull();
  });

  it('formatSuggest reports an empty result honestly', () => {
    expect(formatSuggest([]).content).toContain('No mapping candidates');
  });

  it('formatReview surfaces the real model name', () => {
    const r = formatReview({
      overallScore: 0.8,
      dimensions: { completeness: 0.9, accuracy: 0.8, consistency: 0.7, conformance: 1, timeliness: 0.6 },
      issues: [{ dimension: 'completeness', severity: 'high', field: 'PID-3', description: 'missing MRN', actualValue: null, expectedValue: null }],
      recommendations: [{ priority: 1, category: null, title: 'Add MRN', description: 'populate PID-3', impact: null }],
      processingTimeMs: 42,
      model: 'gemma4-26b-a4b-gptq'
    });
    expect(r.content).toContain('Data Quality — overall 80%');
    expect(r.content).toContain('missing MRN');
    expect(r.model).toBe('gemma4-26b-a4b-gptq');
  });
});

describe('dispatchCopilotAction', () => {
  it('generate → GenerateWorkflow with the description', async () => {
    mockFetch.mockResolvedValue({ generateWorkflow: { yaml: 'r: 1', explanation: 'ok', warnings: [] } });
    const res = await dispatchCopilotAction('generate', 'route admits', {});
    expect(mockFetch).toHaveBeenCalledWith(GenerateWorkflowDocument, {
      input: { description: 'route admits', eventTypes: null, actionTypes: null }
    });
    expect(res.content).toContain('```yaml');
  });

  it('explain → ExplainWorkflow with the input as workflowYaml', async () => {
    mockFetch.mockResolvedValue({
      explainWorkflow: { summary: 'S', description: 'D', routeExplanations: [], diagram: null, warnings: [] }
    });
    await dispatchCopilotAction('explain', 'routes:\n  - x', {});
    expect(mockFetch).toHaveBeenCalledWith(ExplainWorkflowDocument, {
      input: { workflowYaml: 'routes:\n  - x', audience: null }
    });
  });

  it('suggest → SuggestMappings with coerced input', async () => {
    mockFetch.mockResolvedValue({ suggestMappings: [] });
    await dispatchCopilotAction('suggest', '8867-4 to http://loinc.org', {});
    expect(mockFetch).toHaveBeenCalledWith(SuggestMappingsDocument, expect.anything());
    const arg = mockFetch.mock.calls[0]?.[1] as { input: { sourceCode: string } };
    expect(arg.input.sourceCode).toBe('8867-4');
  });

  it('review → AnalyzeQuality and surfaces the model', async () => {
    mockFetch.mockResolvedValue({
      analyzeQuality: {
        overallScore: 0.5,
        dimensions: { completeness: 0.5, accuracy: 0.5, consistency: 0.5, conformance: 0.5, timeliness: 0.5 },
        issues: [],
        recommendations: [],
        model: 'gemma4-e4b-radeonvii'
      }
    });
    const res = await dispatchCopilotAction('review', '{"x":1}', {});
    expect(mockFetch).toHaveBeenCalledWith(AnalyzeQualityDocument, expect.anything());
    expect(res.model).toBe('gemma4-e4b-radeonvii');
  });

  it('propagates errors from graphqlFetch', async () => {
    mockFetch.mockRejectedValue(new Error('GraphQL HTTP 500'));
    await expect(dispatchCopilotAction('generate', 'x', {})).rejects.toThrow('GraphQL HTTP 500');
  });
});
