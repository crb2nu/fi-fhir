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
></textarea>

<style>
  .ta {
    width: 100%;
    resize: vertical;
    min-height: 160px;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
    line-height: 1.45;
    outline: none;
  }

  .ta:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .ta:disabled {
    opacity: 0.65;
  }
</style>
