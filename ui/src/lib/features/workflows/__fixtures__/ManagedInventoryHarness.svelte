<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  const dispatch = createEventDispatcher();
  function open(workflowId: string) {
    dispatch('openBuilder', {
      workflowId, name: workflowId, description: null, status: 'draft',
      versionId: `${workflowId}-v1`, versionNumber: 1
    });
  }
</script>
<button onclick={() => open('workflow-a')}>Design workflow A</button>
<button onclick={() => open('workflow-b')}>Design workflow B</button>
