<!--
  AutoRefreshToggle — re-reads a list on a fixed interval while on. Off by
  default: an operator opts in, and the page never polls a view nobody asked
  to watch. The interval stops when the toggle is turned off or the view
  unmounts.
-->
<script lang="ts">
  import Timer from '@lucide/svelte/icons/timer';
  import TimerOff from '@lucide/svelte/icons/timer-off';
  import { Button } from '$lib/ui/primitives';

  interface Props {
    /** Called on every tick while on. */
    onrefresh: () => void;
    /** What is refreshed, for the accessible name ("messages", "delivery"). */
    subject: string;
    intervalMs?: number;
  }

  let { onrefresh, subject, intervalMs = 15_000 }: Props = $props();

  let on = $state(false);
  const seconds = $derived(Math.round(intervalMs / 1000));

  $effect(() => {
    if (!on) return;
    const timer = setInterval(() => onrefresh(), intervalMs);
    return () => clearInterval(timer);
  });
</script>

<Button
  variant="ghost"
  icon={on ? Timer : TimerOff}
  aria-pressed={on ? 'true' : 'false'}
  aria-label={`Auto-refresh ${subject} every ${seconds} s`}
  title={on
    ? `Re-reading ${subject} every ${seconds} s. Click to stop.`
    : `Off. Click to re-read ${subject} every ${seconds} s.`}
  data-testid="auto-refresh"
  data-state={on ? 'on' : 'off'}
  onclick={() => (on = !on)}
>
  {on ? `Auto ${seconds} s` : 'Auto off'}
</Button>
