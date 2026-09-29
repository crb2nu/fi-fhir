<!--
  WindowPicker — the Statistics and Retention window: last hour, last 24
  hours, last 7 days, or a custom range. A preset applies at once; a custom
  range applies on Apply and says why when it cannot be counted.
-->
<script lang="ts">
  import { Button, Input, Select } from '$lib/ui/primitives';
  import { WINDOW_PRESETS, resolveWindow, type WindowChoice, type WindowPreset } from './statisticsWindow';

  interface Props {
    choice: WindowChoice;
    disabled?: boolean;
    /** Called with the choice to count; the caller resolves it against now. */
    onapply: (choice: WindowChoice) => void;
  }

  let { choice = $bindable(), disabled = false, onapply }: Props = $props();

  let error = $state<string | null>(null);
  let customFrom = $state(choice.customFrom);
  let customTo = $state(choice.customTo);

  function choosePreset(value: string): void {
    const preset = value as WindowPreset;
    error = null;
    if (preset === 'custom') {
      choice = { ...choice, preset };
      return;
    }
    choice = { preset, customFrom, customTo };
    onapply(choice);
  }

  function applyCustom(event: Event): void {
    event.preventDefault();
    const next: WindowChoice = { preset: 'custom', customFrom, customTo };
    const resolved = resolveWindow(next);
    if (!resolved.ok) {
      error = resolved.message;
      return;
    }
    error = null;
    choice = next;
    onapply(next);
  }
</script>

<form class="picker" aria-label="Statistics window" onsubmit={applyCustom} data-testid="statistics-window-picker">
  <div class="preset">
    <Select
      aria-label="Window"
      value={choice.preset}
      options={WINDOW_PRESETS}
      {disabled}
      onchange={(event) => choosePreset((event.currentTarget as HTMLSelectElement).value)}
    />
  </div>
  {#if choice.preset === 'custom'}
    <div class="time">
      <Input aria-label="Window from" type="datetime-local" bind:value={customFrom} />
    </div>
    <div class="time">
      <Input aria-label="Window to" type="datetime-local" bind:value={customTo} />
    </div>
    <Button type="submit" {disabled}>Apply</Button>
  {/if}
</form>
{#if error}
  <p class="error" role="alert">{error}</p>
{/if}

<style>
  .picker {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-2);
  }

  .preset {
    width: 148px;
  }

  .time {
    width: 188px;
  }

  .error {
    margin: var(--space-1) 0 0;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }
</style>
