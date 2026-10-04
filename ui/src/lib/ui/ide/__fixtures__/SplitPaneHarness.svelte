<script lang="ts">
  import type { ComponentProps } from 'svelte';
  import SplitPane from '../SplitPane.svelte';

  export let settings: ComponentProps<typeof SplitPane> = {};
</script>

<SplitPane {...settings}>
  <label>Primary draft <input aria-label="Primary draft" /></label>
  <div slot="secondary" role="region" aria-label="Secondary results">Results stay available.</div>
</SplitPane>
