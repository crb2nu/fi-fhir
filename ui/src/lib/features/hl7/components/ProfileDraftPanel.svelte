<script lang="ts">
  import Panel from '$lib/ui/Panel.svelte';
  import Button from '$lib/ui/Button.svelte';
  import TextArea from '$lib/ui/TextArea.svelte';
  import type { HL7ProfileDraft, ProfileFix } from '$lib/features/hl7/profile/types';
  import { toSourceProfileYAML } from '$lib/features/hl7/profile/yaml';

  export let draft: HL7ProfileDraft;
  export let fixes: readonly ProfileFix[];
  export let onApplyFix: (fix: ProfileFix) => void;
  export let onReset: () => void;

  let copied = false;

  $: yaml = toSourceProfileYAML(draft);
  $: missingSegmentsText = draft.tolerate.missingSegments.join(', ');
  $: ssnRejectText = draft.identifiers.normalization.ssn.rejectPatterns.join(', ');

  function setMissingSegments(text: string) {
    const items = text
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    draft = {
      ...draft,
      tolerate: {
        ...draft.tolerate,
        missingSegments: Array.from(new Set(items)).sort((a, b) => a.localeCompare(b))
      }
    };
  }

  function setSSNRejectPatterns(text: string) {
    const items = text
      .split(',')
      .map((s) => s.trim())
      .filter((s) => s.length > 0);
    draft = {
      ...draft,
      identifiers: {
        ...draft.identifiers,
        normalization: {
          ...draft.identifiers.normalization,
          ssn: {
            ...draft.identifiers.normalization.ssn,
            rejectPatterns: Array.from(new Set(items)).sort((a, b) => a.localeCompare(b))
          }
        }
      }
    };
  }

  async function copyYaml() {
    copied = false;
    try {
      await navigator.clipboard.writeText(yaml);
      copied = true;
      setTimeout(() => (copied = false), 1200);
    } catch {
      // no-op; user can still select/copy manually
    }
  }
</script>

<div class="stack">
  <Panel title="Suggested fixes">
    {#if fixes.length === 0}
      <div class="empty">No automatic fixes suggested for current warnings.</div>
    {:else}
      <div class="fixes">
        {#each fixes as fix (fix.id)}
          <div class="fix">
            <div class="fix-text">
              <div class="fix-title">{fix.title}</div>
              <div class="fix-desc">{fix.description}</div>
            </div>
            <div class="fix-action">
              <Button variant="secondary" on:click={() => onApplyFix(fix)}>Apply</Button>
            </div>
          </div>
        {/each}
      </div>
    {/if}
  </Panel>

  <Panel title="Profile draft (HL7v2)">
    <div class="grid">
      <label class="label">
        Profile ID
        <input class="input" type="text" bind:value={draft.id} />
      </label>
      <label class="label">
        Name
        <input class="input" type="text" bind:value={draft.name} />
      </label>
      <label class="label">
        Version
        <input class="input" type="text" bind:value={draft.version} />
      </label>
      <label class="label">
        HL7 default version
        <input class="input" type="text" bind:value={draft.defaultVersion} />
      </label>
      <label class="label">
        Timezone
        <input class="input" type="text" bind:value={draft.timezone} placeholder="America/New_York" />
      </label>
      <label class="label">
        Tolerate missing segments (comma-separated)
        <input
          class="input mono"
          type="text"
          value={missingSegmentsText}
          on:input={(e) => setMissingSegments((e.currentTarget as HTMLInputElement).value)}
        />
      </label>
    </div>

    <div class="toggles">
      <label class="toggle">
        <input type="checkbox" bind:checked={draft.tolerate.nteAnywhere} />
        <span>NTE anywhere</span>
      </label>
      <label class="toggle">
        <input type="checkbox" bind:checked={draft.tolerate.extraComponents} />
        <span>Extra components</span>
      </label>
      <label class="toggle">
        <input type="checkbox" bind:checked={draft.tolerate.unknownSegments} />
        <span>Unknown segments</span>
      </label>
      <label class="toggle">
        <input type="checkbox" bind:checked={draft.tolerate.nonStandardDelimiters} />
        <span>Non-standard delimiters</span>
      </label>
    </div>

    <div class="section-title">Identifier validation</div>
    <div class="id-grid">
      <div class="id-row">
        <div class="id-kind mono">npi</div>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.validation.npi.enabled} />
          <span>enabled</span>
        </label>
        <label class="label small">
          on_invalid
          <select class="select" bind:value={draft.identifiers.validation.npi.onInvalid}>
            <option value="warn">warn</option>
            <option value="error">error</option>
            <option value="pass">pass</option>
          </select>
        </label>
      </div>

      <div class="id-row">
        <div class="id-kind mono">mbi</div>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.validation.mbi.enabled} />
          <span>enabled</span>
        </label>
        <label class="label small">
          on_invalid
          <select class="select" bind:value={draft.identifiers.validation.mbi.onInvalid}>
            <option value="warn">warn</option>
            <option value="error">error</option>
            <option value="pass">pass</option>
          </select>
        </label>
      </div>

      <div class="id-row">
        <div class="id-kind mono">ssn</div>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.validation.ssn.enabled} />
          <span>enabled</span>
        </label>
        <label class="label small">
          on_invalid
          <select class="select" bind:value={draft.identifiers.validation.ssn.onInvalid}>
            <option value="warn">warn</option>
            <option value="error">error</option>
            <option value="pass">pass</option>
          </select>
        </label>
      </div>
    </div>

    <div class="section-title">Normalization</div>
    <div class="norm-grid">
      <div class="norm-block">
        <div class="norm-title mono">ssn</div>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.normalization.ssn.stripDashes} />
          <span>strip_dashes</span>
        </label>
        <label class="label small">
          reject_patterns (comma-separated)
          <input
            class="input mono"
            type="text"
            value={ssnRejectText}
            on:input={(e) => setSSNRejectPatterns((e.currentTarget as HTMLInputElement).value)}
          />
        </label>
      </div>

      <div class="norm-block">
        <div class="norm-title mono">phone</div>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.normalization.phone.stripCountryCode} />
          <span>strip_country_code</span>
        </label>
        <label class="toggle small">
          <input type="checkbox" bind:checked={draft.identifiers.normalization.phone.normalizeToDigits} />
          <span>normalize_to_digits</span>
        </label>
      </div>
    </div>

    <div class="actions">
      <Button variant="secondary" on:click={onReset}>Reset</Button>
      <Button on:click={copyYaml}>{copied ? 'Copied' : 'Copy YAML'}</Button>
    </div>
  </Panel>

  <Panel title="Export (YAML)">
    <TextArea value={yaml} rows={14} readOnly={true} />
  </Panel>
</div>

<style>
  .stack {
    display: grid;
    gap: 12px;
  }

  .empty {
    color: rgba(229, 231, 235, 0.7);
  }

  .fixes {
    display: grid;
    gap: 10px;
  }

  .fix {
    display: grid;
    grid-template-columns: 1fr auto;
    gap: 12px;
    align-items: start;
    padding: 10px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .fix-title {
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .fix-desc {
    margin-top: 4px;
    color: rgba(229, 231, 235, 0.78);
    line-height: 1.4;
  }

  .grid {
    display: grid;
    grid-template-columns: 1fr;
    gap: 10px;
  }

  @media (min-width: 860px) {
    .grid {
      grid-template-columns: 1fr 1fr;
    }
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono', 'Courier New', monospace;
  }

  .input {
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .toggles {
    margin-top: 12px;
    display: grid;
    gap: 10px;
  }

  .section-title {
    margin-top: 14px;
    color: rgba(243, 244, 246, 0.95);
    font-weight: 800;
  }

  .toggle {
    display: flex;
    gap: 10px;
    align-items: center;
    color: rgba(229, 231, 235, 0.86);
    font-weight: 650;
  }

  .toggle.small {
    font-weight: 600;
    color: rgba(229, 231, 235, 0.82);
  }

  .label.small {
    font-size: 0.85rem;
  }

  .id-grid {
    margin-top: 10px;
    display: grid;
    gap: 10px;
  }

  .id-row {
    display: grid;
    grid-template-columns: 70px 110px 1fr;
    gap: 10px;
    align-items: center;
    padding: 10px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .id-kind {
    color: rgba(229, 231, 235, 0.85);
    font-weight: 800;
  }

  .select {
    margin-top: 6px;
    padding: 10px 12px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
  }

  .select:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .norm-grid {
    margin-top: 10px;
    display: grid;
    gap: 10px;
  }

  .norm-block {
    padding: 10px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
    display: grid;
    gap: 10px;
  }

  .norm-title {
    color: rgba(229, 231, 235, 0.8);
    font-weight: 800;
  }

  .actions {
    margin-top: 12px;
    display: flex;
    gap: 10px;
    justify-content: flex-end;
    flex-wrap: wrap;
  }
</style>
