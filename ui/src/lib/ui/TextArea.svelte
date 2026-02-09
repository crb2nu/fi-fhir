<script lang="ts">
  export let value: string;
  export let placeholder: string | undefined = undefined;
  export let rows = 10;
  export let disabled = false;
  export let readOnly = false;

  function onInput(e: Event) {
    value = (e.currentTarget as HTMLTextAreaElement).value;
  }
</script>

<textarea
  class="ta"
  bind:value
  {placeholder}
  {rows}
  {disabled}
  readonly={readOnly}
  on:input={onInput}
  {...$$restProps}
></textarea>

<style>
  .ta {
    width: 100%;
    resize: vertical;
    min-height: 160px;
    padding: 10px 12px;
    border-radius: var(--radius-xl);
    border: 1px solid var(--color-border-default);
    background: var(--color-bg-input);
    color: var(--color-text-primary);
    font-family: var(--font-mono);
    line-height: 1.45;
    outline: none;
  }

  .ta:focus {
    border-color: var(--color-border-focus);
    box-shadow: var(--shadow-focus);
  }

  .ta:disabled {
    opacity: 0.65;
  }
</style>
