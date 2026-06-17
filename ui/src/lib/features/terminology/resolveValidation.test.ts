import { describe, expect, it } from 'vitest';
import { validateResolveInputs } from './resolveValidation';

const REQUIRED_MESSAGE =
  'Source code, source system, and target system are required';

describe('validateResolveInputs', () => {
  it('returns null when all three required fields are present', () => {
    expect(
      validateResolveInputs({
        sourceCode: 'LAB001',
        sourceSystem: 'epic_custom_labs',
        targetSystem: 'http://loinc.org',
      }),
    ).toBeNull();
  });

  it('flags a missing source code with an inline-ready message', () => {
    expect(
      validateResolveInputs({
        sourceCode: '',
        sourceSystem: 'epic_custom_labs',
        targetSystem: 'http://loinc.org',
      }),
    ).toBe(REQUIRED_MESSAGE);
  });

  it('flags a missing source system', () => {
    expect(
      validateResolveInputs({
        sourceCode: 'LAB001',
        sourceSystem: '',
        targetSystem: 'http://loinc.org',
      }),
    ).toBe(REQUIRED_MESSAGE);
  });

  it('flags a missing target system', () => {
    expect(
      validateResolveInputs({
        sourceCode: 'LAB001',
        sourceSystem: 'epic_custom_labs',
        targetSystem: '',
      }),
    ).toBe(REQUIRED_MESSAGE);
  });

  it('flags an entirely empty form', () => {
    expect(
      validateResolveInputs({
        sourceCode: '',
        sourceSystem: '',
        targetSystem: '',
      }),
    ).toBe(REQUIRED_MESSAGE);
  });
});
