import { describe, expect, it } from 'vitest';
import { parseHL7Path } from './hl7Path';
import { getHL7Value } from './hl7Access';
import { parseHL7Message } from './hl7v2';

// Synthetic message: two PID-3 repetitions, two OBX segments. No patient data.
const MESSAGE = parseHL7Message(
  [
    'MSH|^~\\&|E2E|FAC|FI_FHIR|FAC|20260101090000||ORU^R01|MSG-1|T|2.5.1',
    'PID|1||SYN-0001^^^E2E^MR~SYN-0002^^^ALT^PI||SYNTHETIC^PATIENT||20000101|U',
    'OBX|1|NM|GLU^Glucose||98|mg/dL',
    'OBX|2|NM|NA^Sodium||140|mmol/L'
  ].join('\r')
);

describe('parseHL7Path', () => {
  it('parses segment-only paths, with and without an occurrence', () => {
    expect(parseHL7Path('PV1')).toEqual({ kind: 'segment', segmentId: 'PV1' });
    expect(parseHL7Path('OBX[1]')).toEqual({ kind: 'segment', segmentId: 'OBX', segmentOccurrence: 1 });
  });

  describe('dot notation', () => {
    it('parses field, component, repetition and repetition component', () => {
      expect(parseHL7Path('PV1.2')).toEqual({ kind: 'field', segmentId: 'PV1', field: 2 });
      expect(parseHL7Path('PID.5.1')).toEqual({ kind: 'component', segmentId: 'PID', field: 5, component: 1 });
      expect(parseHL7Path('PID.3[1]')).toEqual({ kind: 'repetition', segmentId: 'PID', field: 3, repetition: 1 });
      expect(parseHL7Path('PID.3[0].1')).toEqual({
        kind: 'repetition_component',
        segmentId: 'PID',
        field: 3,
        repetition: 0,
        component: 1
      });
    });

    it('keeps a segment occurrence', () => {
      expect(parseHL7Path('OBX[1].5')).toEqual({ kind: 'field', segmentId: 'OBX', segmentOccurrence: 1, field: 5 });
    });
  });

  describe('dash notation', () => {
    it('parses field and component', () => {
      expect(parseHL7Path('PV1-2')).toEqual({ kind: 'field', segmentId: 'PV1', field: 2 });
      expect(parseHL7Path('PV1-2.1')).toEqual({ kind: 'component', segmentId: 'PV1', field: 2, component: 1 });
    });

    it('parses a field repetition (the form the lineage panel uses)', () => {
      expect(parseHL7Path('PID-3[1]')).toEqual({ kind: 'repetition', segmentId: 'PID', field: 3, repetition: 1 });
      expect(parseHL7Path('PID-3[0].1')).toEqual({
        kind: 'repetition_component',
        segmentId: 'PID',
        field: 3,
        repetition: 0,
        component: 1
      });
    });

    it('parses a segment occurrence with a field', () => {
      expect(parseHL7Path('OBX[1]-5')).toEqual({ kind: 'field', segmentId: 'OBX', segmentOccurrence: 1, field: 5 });
    });

    it('addresses the same value as the dot form', () => {
      for (const [dash, dot] of [
        ['PID-3[0].1', 'PID.3[0].1'],
        ['PID-3[1].1', 'PID.3[1].1'],
        ['PID-5.2', 'PID.5.2'],
        ['OBX[1]-5', 'OBX[1].5']
      ] as const) {
        expect(parseHL7Path(dash)).toEqual(parseHL7Path(dot));
      }
    });

    it('reads the value it names from a message', () => {
      expect(getHL7Value(MESSAGE, parseHL7Path('PID-3[0].1'))).toBe('SYN-0001');
      expect(getHL7Value(MESSAGE, parseHL7Path('PID-3[1].1'))).toBe('SYN-0002');
      expect(getHL7Value(MESSAGE, parseHL7Path('PID.3[1].1'))).toBe('SYN-0002');
      expect(getHL7Value(MESSAGE, parseHL7Path('OBX[1]-5'))).toBe('140');
    });
  });

  it('rejects malformed paths', () => {
    for (const bad of ['', '   ', 'PID-0', 'PID.0', 'pid-3', 'PID-3[x]', 'PID-3[0]-1', 'PIDX-3', 'PID-3.']) {
      expect(parseHL7Path(bad), bad).toBeNull();
    }
    expect(parseHL7Path(null)).toBeNull();
    expect(parseHL7Path(undefined)).toBeNull();
  });
});
