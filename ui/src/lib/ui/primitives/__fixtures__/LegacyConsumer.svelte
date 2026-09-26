<!--
  Interop fixture: a legacy-syntax (non-runes) component — like most of the
  app today — consuming the runes primitives. Proves the three things page
  lanes rely on: `onclick` props (not `on:click`), default content as
  children, and `{#snippet}` for named regions.
-->
<script lang="ts">
  import { Button, Panel, Tabs, Toolbar } from '../index';

  export let onRun: () => void = () => {};
  export let onTab: (id: string) => void = () => {};

  let view = 'browse';
  $: label = `View: ${view}`;
</script>

<Toolbar title="Legacy page">
  {#snippet tabs()}
    <Tabs
      label="Views"
      value={view}
      onchange={(id) => {
        view = id;
        onTab(id);
      }}
      items={[
        { id: 'browse', label: 'Browse' },
        { id: 'stats', label: 'Statistics' }
      ]}
    />
  {/snippet}
  {#snippet actions()}
    <Button variant="primary" onclick={onRun}>Run</Button>
  {/snippet}
</Toolbar>

<Panel title="Summary">
  <p data-testid="legacy-body">{label}</p>
</Panel>
