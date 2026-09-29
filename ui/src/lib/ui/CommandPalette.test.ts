/**
 * The legacy `<CommandPalette>` is a registration shim: a route that still
 * renders it (HL7 intake) feeds the shell's one palette and opens it.
 */
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { render } from '@testing-library/svelte';
import { get } from 'svelte/store';
import CommandPalette from './CommandPalette.svelte';
import { commands, paletteOpen, registerCommands, resetCommandRegistry } from './ide/commandRegistry';

describe('CommandPalette registration shim', () => {
  beforeEach(() => resetCommandRegistry());

  it('renders nothing and registers its commands above the shell, grouped by its title', () => {
    registerCommands('shell', [{ id: 'nav', label: 'Go to Home', group: 'Navigation', run: vi.fn() }]);
    const { container } = render(CommandPalette, {
      props: {
        title: 'HL7 commands',
        commands: [
          { id: 'preview', label: 'Preview (parse)', hint: 'Cmd/Ctrl+Enter', run: vi.fn() },
          { id: 'copy', label: 'Copy', category: 'Clipboard', run: vi.fn() }
        ]
      }
    });

    expect(container.querySelector('[role="dialog"]')).toBeNull();
    expect(get(commands).map((c) => [c.id, c.group])).toEqual([
      ['preview', 'HL7'],
      ['copy', 'Clipboard'],
      ['nav', 'Navigation']
    ]);
  });

  it('opens the shell palette when the route sets open, and resets its own flag', async () => {
    const { rerender } = render(CommandPalette, { props: { open: false, commands: [] } });
    expect(get(paletteOpen)).toBe(false);
    await rerender({ open: true });
    expect(get(paletteOpen)).toBe(true);
  });

  it('unregisters when the route unmounts', () => {
    const { unmount } = render(CommandPalette, {
      props: { commands: [{ id: 'preview', label: 'Preview', run: vi.fn() }] }
    });
    expect(get(commands)).toHaveLength(1);
    unmount();
    expect(get(commands)).toHaveLength(0);
  });
});
