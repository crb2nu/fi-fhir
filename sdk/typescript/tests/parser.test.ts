import { describe, it, expect, beforeAll } from 'vitest';
import { parse, parseHL7, parseCSV, parseCSVWithSchema, FiFhirError, isFiFhirAvailable } from '../src';

describe('Parser', () => {
  let fiFhirAvailable: boolean;

  beforeAll(async () => {
    fiFhirAvailable = await isFiFhirAvailable();
  });

  describe('parseHL7', () => {
    it.skipIf(!fiFhirAvailable)('parses ADT^A01 message', async () => {
      const message = `MSH|^~\\&|EPIC|HOSPITAL|DEST|DEST|20240115120000||ADT^A01|MSG001|P|2.5
EVN|A01|20240115120000
PID|1||123456^^^HOSP^MR||DOE^JOHN^W||19800315|M|||123 MAIN ST^^ANYTOWN^VA^24101||555-123-4567
PV1|1|I|ICU^101^A^HOSP||||12345^SMITH^ROBERT|||MED||||||||V123456^^^HOSP^VN|||||||||||||||||||||||||20240115110000`;

      const event = await parseHL7(message, { source: 'test_system' });

      expect(event.type).toBe('patient_admit');
      expect(event.source).toBe('test_system');
      expect(event.source_format).toBe('hl7v2');

      if (event.type === 'patient_admit') {
        expect(event.patient.mrn).toBe('123456');
        expect(event.patient.family_name).toBe('DOE');
        expect(event.patient.given_name).toBe('JOHN');
        expect(event.patient.gender).toBe('M');
      }
    });

    it.skipIf(!fiFhirAvailable)('parses ORU^R01 lab result', async () => {
      const message = `MSH|^~\\&|LAB|HOSPITAL|DEST|DEST|20240115140000||ORU^R01|MSG002|P|2.5
PID|1||123456^^^HOSP^MR||DOE^JOHN
OBR|1|ORD001|ACC001|CBC^Complete Blood Count|||20240115130000
OBX|1|NM|WBC^White Blood Cell Count||12.5|10*3/uL|4.5-11.0|H|||F`;

      const event = await parseHL7(message);

      expect(event.type).toBe('lab_result');

      if (event.type === 'lab_result') {
        expect(event.patient.mrn).toBe('123456');
        expect(event.test.local_code).toBe('WBC');
        expect(event.result.value).toBe('12.5');
        expect(event.result.interpretation).toBe('H');
      }
    });
  });

  describe('parseCSV', () => {
    it.skipIf(!fiFhirAvailable)('parses patient CSV', async () => {
      const csv = `mrn,first_name,last_name,dob,gender
123456,John,Doe,1980-03-15,M
789012,Jane,Smith,1992-07-22,F`;

      const events = await parseCSV(csv, { eventType: 'patient' });

      expect(events).toHaveLength(2);
      expect(events[0].type).toBe('patient_update');

      if (events[0].type === 'patient_update') {
        expect(events[0].patient.mrn).toBe('123456');
        expect(events[0].patient.given_name).toBe('John');
        expect(events[0].patient.family_name).toBe('Doe');
      }
    });

    it.skipIf(!fiFhirAvailable)('parses lab result CSV', async () => {
      const csv = `mrn,test_code,test_name,result,unit,interpretation
123456,GLU,Glucose,95,mg/dL,N
123456,HGB,Hemoglobin,14.2,g/dL,N`;

      const events = await parseCSV(csv, { eventType: 'lab' });

      expect(events).toHaveLength(2);
      expect(events[0].type).toBe('lab_result');

      if (events[0].type === 'lab_result') {
        expect(events[0].test.local_code).toBe('GLU');
        expect(events[0].result.value).toBe('95');
      }
    });

    it.skipIf(!fiFhirAvailable)('handles custom delimiter', async () => {
      const tsv = `mrn\tfirst_name\tlast_name
123456\tJohn\tDoe`;

      const events = await parseCSV(tsv, {
        eventType: 'patient',
        delimiter: 'tab'
      });

      expect(events).toHaveLength(1);
      if (events[0].type === 'patient_update') {
        expect(events[0].patient.mrn).toBe('123456');
      }
    });
  });

  describe('parseCSVWithSchema', () => {
    it.skipIf(!fiFhirAvailable)('infers schema from CSV', async () => {
      const csv = `mrn,first_name,last_name,dob,gender,ssn,phone,email
123456,John,Doe,1980-03-15,M,123-45-6789,555-123-4567,john@example.com
789012,Jane,Smith,1992-07-22,F,987-65-4321,555-987-6543,jane@example.com`;

      const result = await parseCSVWithSchema(csv);

      expect(result.schema).toBeDefined();
      expect(result.schema?.columns).toHaveLength(8);

      const mrnCol = result.schema?.columns.find(c => c.name === 'mrn');
      expect(mrnCol?.inferred_type).toBe('mrn');
      expect(mrnCol?.semantic_hint).toBe('patient_mrn');

      const ssnCol = result.schema?.columns.find(c => c.name === 'ssn');
      expect(ssnCol?.inferred_type).toBe('ssn');

      const emailCol = result.schema?.columns.find(c => c.name === 'email');
      expect(emailCol?.inferred_type).toBe('email');
    });
  });

  describe('error handling', () => {
    it.skipIf(!fiFhirAvailable)('throws FiFhirError for invalid input', async () => {
      await expect(parseCSV('', { eventType: 'patient' }))
        .rejects.toThrow(FiFhirError);
    });

    it('throws FiFhirError when binary not found', async () => {
      // This test will only work if fi-fhir is NOT installed
      // Skip in CI where it's available
      if (fiFhirAvailable) {
        return;
      }

      await expect(parseHL7('invalid'))
        .rejects.toThrow(/fi-fhir binary not found/);
    });
  });
});
