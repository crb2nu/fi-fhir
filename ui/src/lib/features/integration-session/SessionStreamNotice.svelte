<script lang="ts">
  /**
   * HL7 intake's honest note for a UI built with the session engine
   * (`VITE_FI_FHIR_INTEGRATION_SESSION_ENABLED=true`) running against an API
   * that cannot stream Integration Session runs. Preview still works — it
   * takes the stateless path — so this is a compact note, not an error.
   * Renders nothing when the build did not opt in or availability is unknown.
   */
  import StreamingUnavailable from '$lib/ui/StreamingUnavailable.svelte';
  import { streamStatus } from '$lib/graphql/streamAvailability';
  import { isIntegrationSessionBuildEnabled } from './api';

  const sessionStream = streamStatus('integrationSessionEvents');
  const buildEnabled = isIntegrationSessionBuildEnabled();

  $: unavailable =
    buildEnabled && $sessionStream.availability === 'unavailable' ? $sessionStream : null;
</script>

{#if unavailable}
  <StreamingUnavailable
    compact
    root="integrationSessionEvents"
    subject="Integration Session runs"
    reason={unavailable.reason}
    alternative="Preview runs on the stateless path instead; results appear below when you press Preview."
  />
{/if}
