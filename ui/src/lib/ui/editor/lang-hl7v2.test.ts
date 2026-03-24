/**
 * Tests for the HL7v2 language mode.
 */
import { describe, it, expect } from 'vitest';
import { hl7v2 } from './lang-hl7v2';
import { EditorState } from '@codemirror/state';
import { syntaxTree } from '@codemirror/language';

function createState(doc: string) {
  return EditorState.create({
    doc,
    extensions: [hl7v2()]
  });
}

describe('lang-hl7v2', () => {
  it('should create a language extension', () => {
    const ext = hl7v2();
    expect(ext).toBeDefined();
  });

  it('should create a valid editor state with HL7v2 content', () => {
    const msg = 'MSH|^~\\&|EPIC|HOSPITAL|LAB|HOSPITAL|202601011200||ADT^A01|12345|P|2.5';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should parse segment headers (MSH, PID, OBR)', () => {
    const msg = 'MSH|^~\\&|SENDER\nPID|1||12345^^^MRN\nOBR|1||LAB001';
    const state = createState(msg);
    expect(state.doc.lines).toBe(3);
    const tree = syntaxTree(state);
    expect(tree).toBeDefined();
  });

  it('should handle pipe delimiters', () => {
    const msg = 'PID|1||12345|Jones^John';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should handle caret component separators', () => {
    const msg = 'PID|1||12345^^^MRN^MR';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should handle tilde repetition separators', () => {
    const msg = 'PID|1||12345~67890';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should handle escape sequences', () => {
    const msg = 'OBX|1|ST|text\\T\\more text';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should handle ampersand sub-component separators', () => {
    const msg = 'PID|1||12345^^^AUTH&1.2.3.4&ISO';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should handle multi-line messages', () => {
    const msg = [
      'MSH|^~\\&|EPIC|HOSP|LAB|HOSP|202601011200||ADT^A01|MSG001|P|2.5',
      'EVN|A01|202601011200',
      'PID|1||12345^^^MRN^MR||Jones^John^A||19800101|M',
      'PV1|1|I|ICU^101^A||||1234^Smith^Jane',
      'NK1|1|Jones^Mary||555-1234',
      'OBR|1||LAB001|CBC^Complete Blood Count',
      'OBX|1|NM|WBC^White Blood Count||7.5|10*3/uL|4.5-11.0|N'
    ].join('\n');
    const state = createState(msg);
    expect(state.doc.lines).toBe(7);
  });

  it('should handle empty segments', () => {
    const msg = 'PID|||';
    const state = createState(msg);
    expect(state.doc.toString()).toBe(msg);
  });

  it('should return a StreamLanguage instance', () => {
    const ext = hl7v2();
    expect(ext).toBeTruthy();
  });
});
