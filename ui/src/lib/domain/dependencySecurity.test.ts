import { createRequire } from 'node:module';
import { realpathSync } from 'node:fs';
import { runInNewContext } from 'node:vm';
import { describe, expect, it } from 'vitest';

// Exercise the copy actually loaded by the glob consumer, not an independent
// direct dependency which could leave a vulnerable nested installation behind.
const load = createRequire(import.meta.url);
const micromatchLoad = createRequire(load.resolve('micromatch'));
const braces = micromatchLoad('braces');
const micromatch = load('micromatch');
const cookie = createRequire(load.resolve('@sveltejs/kit/package.json'))('cookie');

type AST = { type: string; value?: string; nodes?: AST[]; parent?: AST };
function nestedAST(depth: number): AST {
  let ast: AST = { type: 'text', value: 'a' };
  for (let i = 0; i < depth; i++) ast = { type: 'brace', nodes: [ast] };
  return { type: 'root', nodes: [ast] };
}

const methods = ['parse', 'compile', 'expand', 'stringify'] as const;

describe('braces security backport (CVE-2026-93687)', () => {
  it('resolves micromatch to the reviewed local fork', () => {
    expect(realpathSync(micromatchLoad.resolve('braces'))).toBe(
      realpathSync(load.resolve('../../../vendor/braces/index.js'))
    );
    expect(micromatchLoad('braces/package.json').name).toBe('@fi-fhir/braces');
  });

  it.each(methods)('%s accepts depth 100 and rejects 101 and attack-sized input', (method) => {
    for (const [open, close] of [['{', '}'], ['(', ')']]) {
      const pattern = (depth: number) => open!.repeat(depth) + 'a' + close!.repeat(depth);
      expect(() => braces[method](pattern(100))).not.toThrow();
      for (const depth of [101, 2500]) {
        expect(() => braces[method](pattern(depth))).toThrow(/exceeds max depth/);
      }
      expect(() => braces[method](pattern(101), { maxDepth: 1e9 })).toThrow(/exceeds max depth/);
      expect(() => braces[method](pattern(101), { maxDepth: Infinity })).toThrow(/exceeds max depth/);
      expect(() => braces[method](pattern(1), { maxDepth: 1.5 })).not.toThrow();
      expect(() => braces[method](pattern(2), { maxDepth: 1.5 })).toThrow(/exceeds max depth/);
    }
  });

  it.each(['compile', 'expand', 'stringify'])('%s bounds caller-provided ASTs', (method) => {
    expect(() => braces[method](nestedAST(100))).not.toThrow();
    expect(() => braces[method](nestedAST(101))).toThrow(/exceeds max depth/);
    const cycle: AST = { type: 'brace', nodes: [] };
    cycle.nodes!.push(cycle);
    expect(() => braces[method](cycle)).toThrow(/exceeds max depth/);
  });

  it('rejects cyclic parent chains without hanging the build', () => {
    const self: AST = { type: 'paren', nodes: [{ type: 'text', value: 'a' }] };
    self.parent = self;
    const first: AST = { type: 'paren', nodes: [{ type: 'text', value: 'a' }] };
    first.parent = { type: 'paren', parent: first };
    for (const ast of [self, first]) {
      expect(() => runInNewContext('expand(ast)', { expand: braces.expand, ast }, { timeout: 500 }))
        .toThrow(/parent chain contains a cycle/);
    }
  });

  it('preserves the patterns used by code generation and linting', () => {
    expect(micromatch(['src/a.svelte', 'src/b.ts', 'src/c.go'], 'src/*.{svelte,ts}'))
      .toEqual(['src/a.svelte', 'src/b.ts']);
    expect(braces.expand('foo/({a,b})')).toEqual(['foo/(a)', 'foo/(b)']);
    expect(braces.stringify('{{a,b},{c,d}}', { escapeInvalid: true })).toBe('{{a,b},{c,d}}');
    expect(braces.expand('file-{01..03}.{js,ts}')).toEqual([
      'file-01.js', 'file-01.ts', 'file-02.js', 'file-02.ts', 'file-03.js', 'file-03.ts'
    ]);
  });
});

describe('SvelteKit cookie security override', () => {
  it('round-trips ordinary cookies while rejecting injected names, paths and domains', () => {
    const value = cookie.serialize('session', 'abc 123', { path: '/', httpOnly: true, sameSite: 'lax' });
    expect(value).toBe('session=abc%20123; Path=/; HttpOnly; SameSite=Lax');
    expect(cookie.parse(value).session).toBe('abc 123');
    expect(() => cookie.serialize('user; injected', 'value')).toThrow();
    expect(() => cookie.serialize('user', 'value', { path: '/; injected' })).toThrow();
    expect(() => cookie.serialize('user', 'value', { domain: 'example.com; injected' })).toThrow();
  });
});
