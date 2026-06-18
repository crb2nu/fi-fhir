import { describe, expect, it } from 'vitest';
import { validateCsvFile } from './csvFileValidation';

const REQUIRED_MESSAGE = 'Please select a CSV file';

describe('validateCsvFile', () => {
  it('returns null for a .csv file', () => {
    expect(validateCsvFile('mappings.csv')).toBeNull();
  });

  it('returns null for a name with dots before the .csv extension', () => {
    expect(validateCsvFile('loinc.export.2026.csv')).toBeNull();
  });

  it('flags a non-CSV extension with an inline-ready message', () => {
    expect(validateCsvFile('mappings.txt')).toBe(REQUIRED_MESSAGE);
  });

  it('flags a file with no extension', () => {
    expect(validateCsvFile('mappings')).toBe(REQUIRED_MESSAGE);
  });

  it('flags an uppercase extension (matches the prior case-sensitive guard)', () => {
    expect(validateCsvFile('MAPPINGS.CSV')).toBe(REQUIRED_MESSAGE);
  });
});
