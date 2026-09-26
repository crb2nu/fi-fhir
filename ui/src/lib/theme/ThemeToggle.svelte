<script lang="ts">
  import Monitor from '@lucide/svelte/icons/monitor';
  import Moon from '@lucide/svelte/icons/moon';
  import Sun from '@lucide/svelte/icons/sun';
  import { IconButton } from '$lib/ui/primitives';
  import { themePreference, setThemePreference, type ThemePreference } from './theme';

  /** Header theme switch: cycles system → light → dark and persists the choice. */

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

  function onToggle(): void {
    setThemePreference(nextPreference($themePreference));
  }
</script>

<IconButton
  icon={$themePreference === 'light' ? Sun : $themePreference === 'dark' ? Moon : Monitor}
  label={labelFor($themePreference)}
  onclick={onToggle}
/>
