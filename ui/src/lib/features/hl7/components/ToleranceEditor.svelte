<script lang="ts">
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';

  // Common missing segment options
  const commonSegments = ['PD1', 'PV2', 'NK1', 'GT1', 'IN1', 'IN2', 'DG1', 'PR1', 'AL1', 'ZPD'];

  // Timezone options
  const timezones = [
    'America/New_York',
    'America/Chicago',
    'America/Denver',
    'America/Los_Angeles',
    'America/Phoenix',
    'UTC'
  ];

  // HL7 version options
  const hl7Versions = ['2.3', '2.3.1', '2.4', '2.5', '2.5.1', '2.6', '2.7', '2.7.1', '2.8'];

  $: hl7v2 = $selectedProfile?.hl7v2;
  $: tolerance = hl7v2?.tolerance || {
    missingSegments: [],
    nteAnywhere: false,
    extraComponents: false,
    unknownSegments: false,
    nonStandardDelimiters: false
  };

  // Update default version
  function updateDefaultVersion(version: string) {
    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: version,
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: tolerance,
        eventClassifications: hl7v2?.eventClassifications || []
      }
    });
  }

  // Update timezone
  function updateTimezone(tz: string) {
    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: tz,
        tolerance: tolerance,
        eventClassifications: hl7v2?.eventClassifications || []
      }
    });
  }

  // Toggle missing segment
  function toggleMissingSegment(segment: string) {
    const current = tolerance.missingSegments || [];
    const newSegments = current.includes(segment)
      ? current.filter((s) => s !== segment)
      : [...current, segment].sort();

    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: {
          ...tolerance,
          missingSegments: newSegments
        },
        eventClassifications: hl7v2?.eventClassifications || []
      }
    });
  }

  // Add custom segment
  let customSegment = '';
  function addCustomSegment() {
    const segment = customSegment.trim().toUpperCase();
    if (segment && segment.length >= 2 && segment.length <= 4) {
      toggleMissingSegment(segment);
      customSegment = '';
    }
  }

  // Toggle boolean options
  function toggleOption(key: keyof typeof tolerance) {
    if (key === 'missingSegments') return;

    profileStore.updateLocal({
      hl7v2: {
        defaultVersion: hl7v2?.defaultVersion || '2.5.1',
        timezone: hl7v2?.timezone || 'UTC',
        tolerance: {
          ...tolerance,
          [key]: !tolerance[key]
        },
        eventClassifications: hl7v2?.eventClassifications || []
      }
    });
  }
</script>

<div class="editor">
  <div class="section">
    <h4 class="section-title">HL7 Settings</h4>
    <div class="form-row">
      <label class="label">
        Default Version
        <select
          class="select"
          value={hl7v2?.defaultVersion || '2.5.1'}
          on:change={(e) => updateDefaultVersion((e.target as HTMLSelectElement).value)}
        >
          {#each hl7Versions as version (version)}
            <option value={version}>{version}</option>
          {/each}
        </select>
      </label>
      <label class="label">
        Timezone
        <select
          class="select"
          value={hl7v2?.timezone || 'UTC'}
          on:change={(e) => updateTimezone((e.target as HTMLSelectElement).value)}
        >
          {#each timezones as tz (tz)}
            <option value={tz}>{tz}</option>
          {/each}
        </select>
      </label>
    </div>
  </div>

  <div class="section">
    <h4 class="section-title">Tolerate Missing Segments</h4>
    <p class="section-desc">
      Select segments that can be missing from messages without generating warnings.
    </p>
    <div class="segment-grid">
      {#each commonSegments as segment (segment)}
        <label class="segment-toggle">
          <input
            type="checkbox"
            checked={tolerance.missingSegments.includes(segment)}
            on:change={() => toggleMissingSegment(segment)}
          />
          <span class="mono">{segment}</span>
        </label>
      {/each}
    </div>

    {#if tolerance.missingSegments.length > 0}
      <div class="selected-segments">
        <span class="selected-label">Selected:</span>
        {#each tolerance.missingSegments as segment (segment)}
          <button class="segment-chip" on:click={() => toggleMissingSegment(segment)}>
            {segment} <span class="remove">x</span>
          </button>
        {/each}
      </div>
    {/if}

    <div class="custom-segment-row">
      <input
        class="input mono"
        type="text"
        bind:value={customSegment}
        placeholder="Add custom segment (e.g., ZPD)"
        maxlength="4"
        on:keydown={(e) => e.key === 'Enter' && addCustomSegment()}
      />
      <button class="add-btn" on:click={addCustomSegment} disabled={!customSegment.trim()}>
        Add
      </button>
    </div>
  </div>

  <div class="section">
    <h4 class="section-title">Parsing Options</h4>
    <p class="section-desc">
      Configure how the parser handles non-standard HL7 messages.
    </p>
    <div class="option-list">
      <label class="option-toggle">
        <input
          type="checkbox"
          checked={tolerance.nteAnywhere}
          on:change={() => toggleOption('nteAnywhere')}
        />
        <div class="option-content">
          <span class="option-label">Allow NTE anywhere</span>
          <span class="option-desc">NTE segments can appear after any segment, not just the standard positions</span>
        </div>
      </label>

      <label class="option-toggle">
        <input
          type="checkbox"
          checked={tolerance.extraComponents}
          on:change={() => toggleOption('extraComponents')}
        />
        <div class="option-content">
          <span class="option-label">Allow extra components</span>
          <span class="option-desc">Fields can have more components than defined in the spec</span>
        </div>
      </label>

      <label class="option-toggle">
        <input
          type="checkbox"
          checked={tolerance.unknownSegments}
          on:change={() => toggleOption('unknownSegments')}
        />
        <div class="option-content">
          <span class="option-label">Allow unknown segments</span>
          <span class="option-desc">Pass through segments not defined in HL7 standard (like Z-segments)</span>
        </div>
      </label>

      <label class="option-toggle">
        <input
          type="checkbox"
          checked={tolerance.nonStandardDelimiters}
          on:change={() => toggleOption('nonStandardDelimiters')}
        />
        <div class="option-content">
          <span class="option-label">Allow non-standard delimiters</span>
          <span class="option-desc">Accept messages with different field/component separators</span>
        </div>
      </label>
    </div>
  </div>
</div>

<style>
  .editor {
    display: grid;
    gap: 20px;
  }

  .section {
    padding: 16px;
    border-radius: 12px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.02);
  }

  .section-title {
    margin: 0 0 8px;
    font-size: 0.95rem;
    font-weight: 800;
    color: rgba(243, 244, 246, 0.95);
  }

  .section-desc {
    margin: 0 0 14px;
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.65);
    line-height: 1.4;
  }

  .form-row {
    display: grid;
    grid-template-columns: 1fr 1fr;
    gap: 14px;
  }

  @media (max-width: 600px) {
    .form-row {
      grid-template-columns: 1fr;
    }
  }

  .label {
    display: grid;
    gap: 6px;
    color: rgba(229, 231, 235, 0.8);
    font-size: 0.9rem;
  }

  .select {
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

  .segment-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(80px, 1fr));
    gap: 8px;
  }

  .segment-toggle {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 8px 10px;
    border-radius: 8px;
    border: 1px solid rgba(255, 255, 255, 0.1);
    background: rgba(255, 255, 255, 0.02);
    cursor: pointer;
  }

  .segment-toggle:hover {
    background: rgba(255, 255, 255, 0.05);
  }

  .segment-toggle input:checked + span {
    color: rgba(59, 130, 246, 0.95);
  }

  .mono {
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
    color: rgba(229, 231, 235, 0.85);
    font-weight: 600;
  }

  .selected-segments {
    margin-top: 12px;
    display: flex;
    flex-wrap: wrap;
    gap: 8px;
    align-items: center;
  }

  .selected-label {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.6);
  }

  .segment-chip {
    display: flex;
    align-items: center;
    gap: 6px;
    padding: 4px 10px;
    border-radius: 6px;
    background: rgba(59, 130, 246, 0.15);
    border: 1px solid rgba(59, 130, 246, 0.3);
    color: rgba(59, 130, 246, 0.95);
    font-size: 0.85rem;
    font-weight: 600;
    font-family: ui-monospace, SFMono-Regular, Menlo, Monaco, Consolas, 'Liberation Mono',
      'Courier New', monospace;
    cursor: pointer;
  }

  .segment-chip:hover {
    background: rgba(59, 130, 246, 0.25);
  }

  .segment-chip .remove {
    opacity: 0.7;
  }

  .custom-segment-row {
    margin-top: 12px;
    display: flex;
    gap: 8px;
  }

  .input {
    flex: 1;
    max-width: 240px;
    padding: 8px 12px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.12);
    background: rgba(255, 255, 255, 0.03);
    color: rgba(229, 231, 235, 0.92);
    outline: none;
    text-transform: uppercase;
  }

  .input:focus {
    border-color: rgba(59, 130, 246, 0.45);
    box-shadow: 0 0 0 3px rgba(59, 130, 246, 0.15);
  }

  .add-btn {
    padding: 8px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.14);
    background: rgba(255, 255, 255, 0.06);
    color: #f3f4f6;
    font-weight: 600;
    cursor: pointer;
  }

  .add-btn:hover:not(:disabled) {
    background: rgba(255, 255, 255, 0.1);
  }

  .add-btn:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .option-list {
    display: grid;
    gap: 10px;
  }

  .option-toggle {
    display: flex;
    align-items: flex-start;
    gap: 12px;
    padding: 12px 14px;
    border-radius: 10px;
    border: 1px solid rgba(255, 255, 255, 0.08);
    background: rgba(255, 255, 255, 0.015);
    cursor: pointer;
  }

  .option-toggle:hover {
    background: rgba(255, 255, 255, 0.04);
  }

  .option-toggle input {
    margin-top: 2px;
  }

  .option-content {
    display: grid;
    gap: 2px;
  }

  .option-label {
    color: rgba(229, 231, 235, 0.9);
    font-weight: 650;
  }

  .option-desc {
    font-size: 0.85rem;
    color: rgba(229, 231, 235, 0.55);
    line-height: 1.35;
  }
</style>
