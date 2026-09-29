<!--
  Registration shim for a route that still renders its own
  `<CommandPalette bind:open title commands />` (HL7 intake). It renders
  nothing: its commands go into the shell's one command registry
  (`$lib/ui/ide/commandRegistry`) under the title's group, above the shell's,
  for as long as the route is mounted; setting `open` opens the shell
  palette. Delete this file once HL7PreviewPage registers through
  `registerCommands` directly (.loom/42 E-3/E-4).
-->
<script context="module" lang="ts">
  export type PaletteCommand = {
    id: string;
    label: string;
    hint?: string;
    keywords?: string[];
    category?: string;
    /** Keyboard shortcut shown right-aligned, in the platform's notation (e.g. "⌘B", "Ctrl+B"). */
    shortcut?: string;
    run: () => void | Promise<void>;
  };

  let shims = 0;
</script>

<script lang="ts">
  import { onDestroy } from 'svelte';
  import { openPalette, registerCommands, unregisterCommands } from '$lib/ui/ide/commandRegistry';

  export let open = false;
  export let title = 'Commands';
  export let commands: readonly PaletteCommand[] = [];

  const source = `route-commands-${++shims}`;

  // "HL7 commands" → group "HL7".
  $: group = title.replace(/\s+commands$/i, '').trim() || title;
  $: registerCommands(
    source,
    commands.map((command) => ({
      id: command.id,
      label: command.label,
      hint: command.hint,
      shortcut: command.shortcut,
      keywords: command.keywords,
      group: command.category ?? group,
      run: command.run
    })),
    { priority: 10 }
  );

  $: if (open) {
    openPalette();
    open = false;
  }

  onDestroy(() => unregisterCommands(source));
</script>
