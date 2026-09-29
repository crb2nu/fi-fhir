import { beforeEach, describe, expect, it, vi } from 'vitest';
import { get } from 'svelte/store';
import {
  closePalette,
  commands,
  filterCommands,
  openPalette,
  paletteOpen,
  registerCommand,
  registerCommands,
  resetCommandRegistry,
  unregisterCommand,
  unregisterCommands,
  visibleCommands,
  type Command
} from './commandRegistry';

function cmd(id: string, group = 'Workspace', extra: Partial<Command> = {}): Command {
  return { id, label: `Label ${id}`, group, run: vi.fn(), ...extra };
}

describe('commandRegistry', () => {
  beforeEach(() => resetCommandRegistry());

  it('lists sources by priority, then registration order', () => {
    registerCommands('shell', [cmd('a'), cmd('b')]);
    registerCommands('other', [cmd('c')]);
    registerCommands('hl7', [cmd('h', 'HL7')], { priority: 10 });

    expect(get(commands).map((c) => c.id)).toEqual(['h', 'a', 'b', 'c']);
  });

  it('replaces a source on re-registration and keeps its place', () => {
    registerCommands('shell', [cmd('a')]);
    registerCommands('other', [cmd('c')]);
    registerCommands('shell', [cmd('a2'), cmd('a3')]);

    expect(get(commands).map((c) => c.id)).toEqual(['a2', 'a3', 'c']);
  });

  it('unregisters a source through the returned function or by name', () => {
    const off = registerCommands('hl7', [cmd('h')], { priority: 10 });
    registerCommands('shell', [cmd('a')]);
    off();
    expect(get(commands).map((c) => c.id)).toEqual(['a']);
    unregisterCommands('shell');
    expect(get(commands)).toEqual([]);
  });

  it('registers and unregisters single commands inside a source', () => {
    registerCommands('shell', [cmd('a')]);
    const off = registerCommand('shell', cmd('b'));
    expect(get(commands).map((c) => c.id)).toEqual(['a', 'b']);
    off();
    expect(get(commands).map((c) => c.id)).toEqual(['a']);
    registerCommand('shell', cmd('z'));
    unregisterCommand('shell', 'a');
    expect(get(commands).map((c) => c.id)).toEqual(['z']);
  });

  it('hides commands whose when predicate is false or throws', () => {
    const list = [
      cmd('shown'),
      cmd('hidden', 'Workspace', { when: () => false }),
      cmd('broken', 'Workspace', {
        when: () => {
          throw new Error('no state');
        }
      }),
      cmd('conditional', 'Workspace', { when: () => true })
    ];
    expect(visibleCommands(list).map((c) => c.id)).toEqual(['shown', 'conditional']);
  });

  it('filters over label, hint, group and keywords', () => {
    const list = [
      cmd('a', 'Navigation', { label: 'Go to Operator', hint: '/operator' }),
      cmd('b', 'HL7', { label: 'Preview (parse)', keywords: ['run'] }),
      cmd('c', 'Workspace', { label: 'Toggle sidebar' })
    ];
    expect(filterCommands(list, 'operator').map((c) => c.id)).toEqual(['a']);
    expect(filterCommands(list, 'RUN').map((c) => c.id)).toEqual(['b']);
    expect(filterCommands(list, 'hl7').map((c) => c.id)).toEqual(['b']);
    expect(filterCommands(list, '  ').map((c) => c.id)).toEqual(['a', 'b', 'c']);
  });

  it('opens and closes the one palette', () => {
    expect(get(paletteOpen)).toBe(false);
    openPalette();
    expect(get(paletteOpen)).toBe(true);
    closePalette();
    expect(get(paletteOpen)).toBe(false);
  });
});
