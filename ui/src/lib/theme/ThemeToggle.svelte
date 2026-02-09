<script lang="ts">
  import IconButton from '$lib/ui/IconButton.svelte';
  import { themePreference, setThemePreference, type ThemePreference } from './theme';

  const order: ThemePreference[] = ['system', 'light', 'dark'];

  function nextPreference(current: ThemePreference): ThemePreference {
    const idx = Math.max(0, order.indexOf(current));
    return order[(idx + 1) % order.length] ?? 'system';
  }

  function labelFor(pref: ThemePreference): string {
    if (pref === 'system') return 'Theme: system';
    if (pref === 'light') return 'Theme: light';
    return 'Theme: dark';
  }

  function iconFor(pref: ThemePreference): 'system' | 'light' | 'dark' {
    if (pref === 'system') return 'system';
    if (pref === 'light') return 'light';
    return 'dark';
  }

  function onToggle(): void {
    setThemePreference(nextPreference($themePreference));
  }
</script>

<IconButton
  size="md"
  variant="default"
  label={labelFor($themePreference)}
  on:click={onToggle}
>
  {#if iconFor($themePreference) === 'light'}
    <!-- Sun -->
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path d="M12 4V2" />
      <path d="M12 22v-2" />
      <path d="M4 12H2" />
      <path d="M22 12h-2" />
      <path d="M5 5l-1.5-1.5" />
      <path d="M20.5 20.5L19 19" />
      <path d="M5 19l-1.5 1.5" />
      <path d="M20.5 3.5L19 5" />
      <circle cx="12" cy="12" r="4" />
    </svg>
  {:else if iconFor($themePreference) === 'dark'}
    <!-- Moon -->
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <path
        d="M21 12.8A8.5 8.5 0 0 1 11.2 3 6.5 6.5 0 1 0 21 12.8Z"
        stroke-linejoin="round"
      />
    </svg>
  {:else}
    <!-- System / monitor -->
    <svg viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" aria-hidden="true">
      <rect x="3" y="4" width="18" height="12" rx="2" />
      <path d="M8 20h8" />
      <path d="M12 16v4" />
    </svg>
  {/if}
</IconButton>
