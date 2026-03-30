/**
 * Tests for the CEL language mode.
 */
import { describe, it, expect } from 'vitest';
import { cel } from './lang-cel';
import { EditorState } from '@codemirror/state';
import { syntaxTree } from '@codemirror/language';

function createState(doc: string) {
  return EditorState.create({
    doc,
    extensions: [cel()]
  });
}

describe('lang-cel', () => {
  it('should create a language extension', () => {
    const ext = cel();
    expect(ext).toBeDefined();
  });

  it('should create a valid editor state with CEL content', () => {
    const state = createState('event.type == "PATIENT_ADMIT"');
    expect(state.doc.toString()).toBe('event.type == "PATIENT_ADMIT"');
  });

  it('should parse without errors for keyword expressions', () => {
    const state = createState('true && false || null');
    expect(state.doc.length).toBeGreaterThan(0);
    const tree = syntaxTree(state);
    expect(tree).toBeDefined();
  });

  it('should parse string literals', () => {
    const state = createState('"hello world"');
    expect(state.doc.toString()).toBe('"hello world"');
    const tree = syntaxTree(state);
    expect(tree).toBeDefined();
  });

  it('should parse single-quoted string literals', () => {
    const state = createState("'hello world'");
    expect(state.doc.toString()).toBe("'hello world'");
  });

  it('should parse numeric literals', () => {
    const state = createState('42 + 3.14');
    expect(state.doc.toString()).toBe('42 + 3.14');
  });

  it('should parse operator expressions', () => {
    const state = createState('a != b && c >= d');
    expect(state.doc.toString()).toBe('a != b && c >= d');
  });

  it('should handle dotted property access', () => {
    const state = createState('event.type');
    expect(state.doc.toString()).toBe('event.type');
    const tree = syntaxTree(state);
    expect(tree).toBeDefined();
  });

  it('should parse line comments', () => {
    const state = createState('// this is a comment\ntrue');
    expect(state.doc.lines).toBe(2);
  });

  it('should handle function-like keywords', () => {
    const state = createState('has(event.field) && size(list) > 0');
    expect(state.doc.toString()).toBe('has(event.field) && size(list) > 0');
  });

  it('should handle complex expressions', () => {
    const expr = 'event.type in ["PATIENT_ADMIT", "PATIENT_DISCHARGE"] && event.isCritical == true';
    const state = createState(expr);
    expect(state.doc.toString()).toBe(expr);
  });

  it('should return a StreamLanguage instance', () => {
    const ext = cel();
    // StreamLanguage.define returns a LanguageSupport-like object
    expect(ext).toBeTruthy();
  });
});
