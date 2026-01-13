<script lang="ts">
  import type { HealthQuery, ParsePreviewQuery, ParsePreviewQueryVariables } from '$lib/gen/graphql';
  import { HealthDocument, ParsePreviewDocument } from '$lib/gen/graphql';
  import { graphqlFetch } from '$lib/graphql/client';

  let health: HealthQuery | null = null;
  let parsePreview: ParsePreviewQuery | null = null;
  let error: string | null = null;

  const sample = `MSH|^~\\&|EPIC|HOSPITAL|FI-FHIR|DEST|20240115103000||ADT^A01|MSG001|P|2.5\rPID|1||MRN123^^^HOSP^MR||DOE^JOHN||19800101|M\rPV1|1|I|ICU^101^A^HOSPITAL`;

  async function run() {
    error = null;
    health = null;
    parsePreview = null;

    try {
      health = await graphqlFetch(HealthDocument);
      const vars: ParsePreviewQueryVariables = { format: 'HL7V2', data: sample, source: 'ui_preview' };
      parsePreview = await graphqlFetch(ParsePreviewDocument, vars);
    } catch (e) {
      error = e instanceof Error ? e.message : String(e);
    }
  }
</script>

<h1>HL7 Mapping (Preview)</h1>
<p>Smoke-test of strictly typed GraphQL operations against <code>/graphql</code>.</p>

<button class="btn" on:click={run}>Run preview</button>

{#if error}
  <pre class="panel error">{error}</pre>
{/if}

{#if health}
  <pre class="panel">{JSON.stringify(health, null, 2)}</pre>
{/if}

{#if parsePreview}
  <pre class="panel">{JSON.stringify(parsePreview, null, 2)}</pre>
{/if}

<style>
  h1 {
    color: #f9fafb;
    margin: 0 0 10px;
  }
  p {
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
  }
  .btn {
    margin-top: 12px;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.14);
    background: rgba(255, 255, 255, 0.06);
    color: #f3f4f6;
    cursor: pointer;
  }
  .btn:hover {
    background: rgba(255, 255, 255, 0.1);
  }
  .panel {
    margin-top: 14px;
    padding: 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.9);
    overflow: auto;
    max-height: 420px;
  }
  .panel.error {
    border-color: rgba(239, 68, 68, 0.5);
  }
</style>
