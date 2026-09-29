/**
 * The one command registry behind the one palette (Cmd/Ctrl+K, the header's
 * Commands button). The shell registers navigation and workspace commands; a
 * route registers its own while it is mounted and unregisters on destroy.
 *
 *   const unregister = registerCommands('hl7', commands, { priority: 10 });
 *   onDestroy(unregister);
 *
 * Registering the same source again replaces its list, so a route can
 * re-register whenever its commands change (a selection appears, a
 * pre-flight changes a hint). Higher `priority` sources list first; the
 * current route's commands sit above the shell's.
 */
import { derived, get, writable, type Readable } from 'svelte/store';

export interface Command {
  /** Unique within its source. */
  id: string;
  label: string;
  /** Heading the palette groups this command under ("Navigation", "HL7"). */
  group: string;
  /** Secondary text on the right (a route, a one-line reason). */
  hint?: string | undefined;
  /** Shortcut label in the platform's notation ("⌘B", "Ctrl+B"); display only. */
  shortcut?: string | undefined;
  /** Extra words the filter matches. */
  keywords?: readonly string[] | undefined;
  /** Listed only while this returns true; evaluated when the palette opens and filters. */
  when?: (() => boolean) | undefined;
  run: () => void | Promise<void>;
}

export interface RegisterOptions {
  /** Higher lists first. The shell registers at 0; a route at 10. */
  priority?: number;
}

interface Source {
  id: string;
  priority: number;
  order: number;
  commands: readonly Command[];
}

const sources = writable<Source[]>([]);
let registrations = 0;

/** Registers (or replaces) a source's commands; returns its unregister. */
export function registerCommands(
  source: string,
  commands: readonly Command[],
  options: RegisterOptions = {}
): () => void {
  const priority = options.priority ?? 0;
  sources.update((list) => {
    const existing = list.find((entry) => entry.id === source);
    const next: Source = {
      id: source,
      priority,
      order: existing?.order ?? registrations++,
      commands: [...commands],
    };
    return existing ? list.map((entry) => (entry.id === source ? next : entry)) : [...list, next];
  });
  return () => unregisterCommands(source);
}

/** Registers one command into a source, keeping the source's other commands. */
export function registerCommand(source: string, command: Command, options: RegisterOptions = {}): () => void {
  const current = get(sources).find((entry) => entry.id === source);
  const rest = (current?.commands ?? []).filter((entry) => entry.id !== command.id);
  registerCommands(source, [...rest, command], { priority: options.priority ?? current?.priority ?? 0 });
  return () => unregisterCommand(source, command.id);
}

/** Removes one command from a source. */
export function unregisterCommand(source: string, id: string): void {
  sources.update((list) =>
    list.map((entry) =>
      entry.id === source ? { ...entry, commands: entry.commands.filter((cmd) => cmd.id !== id) } : entry
    )
  );
}

/** Removes a source and all its commands. */
export function unregisterCommands(source: string): void {
  sources.update((list) => list.filter((entry) => entry.id !== source));
}

/**
 * Every registered command, highest-priority source first, then in
 * registration order. `when` is not applied here; see `visibleCommands`.
 */
export const commands: Readable<Command[]> = derived(sources, ($sources) =>
  [...$sources]
    .sort((a, b) => b.priority - a.priority || a.order - b.order)
    .flatMap((source) => source.commands)
);

/** The commands whose `when` holds now. A `when` that throws hides its command. */
export function visibleCommands(list: readonly Command[]): Command[] {
  return list.filter((command) => {
    if (!command.when) return true;
    try {
      return command.when();
    } catch {
      return false;
    }
  });
}

/** Case-insensitive substring match over label, hint, group and keywords. */
export function filterCommands(list: readonly Command[], query: string): Command[] {
  const q = query.trim().toLowerCase();
  if (!q) return [...list];
  return list.filter((command) =>
    [command.label, command.hint ?? '', command.group, ...(command.keywords ?? [])]
      .join(' ')
      .toLowerCase()
      .includes(q)
  );
}

const open = writable(false);

/** Whether the palette is open. */
export const paletteOpen: Readable<boolean> = { subscribe: open.subscribe };

export function openPalette(): void {
  open.set(true);
}

export function closePalette(): void {
  open.set(false);
}

/** Tests only: forget every source and close the palette. */
export function resetCommandRegistry(): void {
  sources.set([]);
  open.set(false);
  registrations = 0;
}
