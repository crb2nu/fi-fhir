import { describe, expect, it } from 'vitest';
import {
  ADAPTER_PRESENTATION,
  adapterPresentation,
  listedKey,
  listedKeys,
  secretDisplay,
  yesNo
} from './engineProperties';

describe('engineProperties', () => {
  it('renders a secret property as exactly unset or set, never the string sent', () => {
    expect(secretDisplay('unset')).toBe('unset');
    expect(secretDisplay('set')).toBe('set');
    // A server that broke its contract still cannot put a value on screen.
    expect(secretDisplay('synthetic-bearer-value-0000')).toBe('set');
    expect(secretDisplay('')).toBe('set');
  });

  it('names a variable only where the runtime lists it', () => {
    const listed = listedKeys({
      properties: [{ key: 'FI_FHIR_MLLP_DEFINITION_ID', value: '', secret: false, source: 'default' }]
    });
    expect(listedKey(listed, 'FI_FHIR_MLLP_DEFINITION_ID')).toBe('FI_FHIR_MLLP_DEFINITION_ID');
    expect(listedKey(listed, 'FI_FHIR_DELIVERY_IDENTITY_MODE')).toBeUndefined();
    expect(listedKey(listed, undefined)).toBeUndefined();
  });

  it('presents the four adapters in the runtime order, and an unexpected kind plainly', () => {
    expect(Object.keys(ADAPTER_PRESENTATION)).toEqual(['http', 'mllp', 'batch', 'delivery']);
    expect(adapterPresentation('mllp').title).toBe('MLLP listener');
    expect(adapterPresentation('sftp-push')).toEqual({ title: 'sftp-push', enableKeys: [], fields: [] });
    expect(yesNo(null)).toBeNull();
    expect(yesNo(false)).toBe('No');
  });
});
