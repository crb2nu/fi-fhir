<script lang="ts">
  import { resolve } from '$app/paths';
  import Panel from '$lib/ui/Panel.svelte';
  import SystemStatusPanel from '$lib/features/system/SystemStatusPanel.svelte';
</script>

<h1>fi-fhir Mapping Studio</h1>
<p class="sub">
  A workbench for turning messy interface data into typed, testable semantic events (and eventually Source Profiles).
</p>

<div class="grid">
  <SystemStatusPanel />

  <Panel title="Tools">
    <div class="tools">
      <a class="cta" href={resolve('/hl7')}>HL7 Preview & Triage</a>
      <div class="hint">
        Paste or load sample HL7v2 messages, preview extraction, and review warnings by phase.
      </div>

      <a class="cta" href={resolve('/profiles')}>Profiles (YAML)</a>
      <div class="hint">
        Browse profiles and edit raw Source Profile YAML via the API.
      </div>

      <a class="cta" href={resolve('/terminology')}>Terminology Mapping</a>
      <div class="hint">
        Manage custom code mappings with LLM-powered autoroute suggestions.
      </div>

      <a class="cta" href={resolve('/workflows')}>Workflow Builder</a>
      <div class="hint">
        Design event routing workflows visually. Configure filters, actions, and dry-run against sample events.
      </div>
    </div>
  </Panel>

  <Panel title="Next">
    <ul class="list">
      <li>Generate Source Profile YAML from UI edits</li>
      <li>Track profile changes and re-run previews deterministically</li>
      <li>Expand mapping helpers beyond HL7 (CSV, EDI)</li>
    </ul>
  </Panel>

  <Panel title="Contracts">
    <div class="contract">
      <div class="label">API surface</div>
      <div class="value">GraphQL + OpenAPI (generated client types, validated in CI)</div>
    </div>
  </Panel>
</div>

<style>
  h1 {
    color: #f9fafb;
    margin: 0 0 8px;
  }

  .sub {
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
    margin: 0 0 16px;
    max-width: 70ch;
  }

  .grid {
    display: grid;
    gap: 14px;
    grid-template-columns: 1fr;
  }

  @media (min-width: 980px) {
    .grid {
      grid-template-columns: 1.3fr 0.7fr;
      align-items: start;
    }
  }

  .tools {
    display: grid;
    gap: 10px;
  }

  .cta {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    padding: 10px 12px;
    border-radius: 10px;
    border: 1px solid rgba(59, 130, 246, 0.35);
    background: rgba(59, 130, 246, 0.12);
    color: rgba(219, 234, 254, 0.95);
    text-decoration: none;
    font-weight: 700;
  }

  .cta:hover {
    background: rgba(59, 130, 246, 0.18);
  }

  .hint {
    color: rgba(229, 231, 235, 0.8);
    line-height: 1.55;
  }

  .list {
    margin: 0;
    padding-left: 18px;
    color: rgba(229, 231, 235, 0.86);
    line-height: 1.55;
  }

  .contract {
    display: grid;
    gap: 6px;
  }

  .label {
    color: rgba(229, 231, 235, 0.7);
    font-size: 0.9rem;
    font-weight: 700;
  }

  .value {
    color: rgba(229, 231, 235, 0.88);
    line-height: 1.55;
  }
</style>
