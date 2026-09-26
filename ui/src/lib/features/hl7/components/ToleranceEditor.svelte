<script lang="ts">
  import Plus from '@lucide/svelte/icons/plus';
  import { Badge, Button, Field, Input, Panel, Select, type SelectOption } from '$lib/ui/primitives';
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

  const versionOptions: SelectOption[] = hl7Versions.map((v) => ({ value: v, label: v }));
  const timezoneOptions: SelectOption[] = timezones.map((tz) => ({ value: tz, label: tz }));

  type ParsingOptionKey = 'nteAnywhere' | 'extraComponents' | 'unknownSegments' | 'nonStandardDelimiters';

  const parsingOptions: { key: ParsingOptionKey; label: string; desc: string }[] = [
    {
      key: 'nteAnywhere',
      label: 'Allow NTE anywhere',
      desc: 'NTE segments can appear after any segment, not just the standard positions.'
    },
    {
      key: 'extraComponents',
      label: 'Allow extra components',
      desc: 'Fields can have more components than the spec defines.'
    },
    {
      key: 'unknownSegments',
      label: 'Allow unknown segments',
      desc: 'Pass through segments the HL7 standard does not define, such as Z-segments.'
    },
    {
      key: 'nonStandardDelimiters',
      label: 'Allow non-standard delimiters',
      desc: 'Accept messages with different field or component separators.'
    }
  ];

  $: hl7v2 = $selectedProfile?.hl7v2;
  $: tolerance = hl7v2?.tolerance || {
    missingSegments: [],
    nteAnywhere: false,
    extraComponents: false,
    unknownSegments: false,
    nonStandardDelimiters: false
  };

  // Common segments first, then any custom segments already tolerated, so
  // every selected segment has a checkbox that removes it.
  $: segmentOptions = [
    ...commonSegments,
    ...tolerance.missingSegments.filter((s) => !commonSegments.includes(s))
  ];

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
  <Panel title="HL7 settings" titleTag="h3">
    <div class="form-grid">
      <Field label="Default version">
        <Select
          mono
          value={hl7v2?.defaultVersion || '2.5.1'}
          options={versionOptions}
          onchange={(e) => updateDefaultVersion(e.currentTarget.value)}
        />
      </Field>
      <Field label="Timezone">
        <Select
          value={hl7v2?.timezone || 'UTC'}
          options={timezoneOptions}
          onchange={(e) => updateTimezone(e.currentTarget.value)}
        />
      </Field>
    </div>
  </Panel>

  <Panel title="Parsing options" titleTag="h3" flush class="span-rows">
    <ul class="options">
      {#each parsingOptions as option (option.key)}
        <li>
          <label class="option">
            <input
              type="checkbox"
              checked={tolerance[option.key]}
              on:change={() => toggleOption(option.key)}
            />
            <span class="option-text">
              <span class="option-label">{option.label}</span>
              <span class="option-desc">{option.desc}</span>
            </span>
          </label>
        </li>
      {/each}
    </ul>
  </Panel>

  <Panel title="Tolerated missing segments" titleTag="h3">
    {#snippet actions()}
      <Badge mono>{tolerance.missingSegments.length} selected</Badge>
    {/snippet}
    <p class="hint">Segments that can be missing from a message without a warning.</p>
    <div class="segment-grid">
      {#each segmentOptions as segment (segment)}
        <label class="check">
          <input
            type="checkbox"
            checked={tolerance.missingSegments.includes(segment)}
            on:change={() => toggleMissingSegment(segment)}
          />
          <span class="text-mono">{segment}</span>
        </label>
      {/each}
    </div>

    <div class="add-row">
      <div class="add-input">
        <Input
          mono
          bind:value={customSegment}
          aria-label="Custom segment"
          placeholder="Custom segment, e.g. ZPD"
          maxlength={4}
          onkeydown={(e) => e.key === 'Enter' && addCustomSegment()}
        />
      </div>
      <Button icon={Plus} onclick={addCustomSegment} disabled={!customSegment.trim()}>Add</Button>
    </div>
  </Panel>
</div>

<style>
  .editor {
    display: grid;
    grid-template-columns: repeat(auto-fit, minmax(320px, 1fr));
    gap: var(--space-3);
    align-items: start;
  }

  /* Two columns: settings and segments on the left, parsing options beside
     them; one column when the pane is narrow. */
  .editor :global(.span-rows) {
    grid-row: span 2;
  }

  .form-grid {
    display: grid;
    grid-template-columns: repeat(2, minmax(0, 1fr));
    gap: var(--space-3);
  }

  .hint {
    margin: 0 0 var(--space-3);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .options {
    margin: 0;
    padding: 0;
    list-style: none;
  }

  .options li + li {
    border-top: 1px solid var(--color-border-subtle);
  }

  .option {
    display: flex;
    align-items: flex-start;
    gap: var(--space-2);
    padding: var(--space-2) var(--space-3);
    cursor: pointer;
  }

  .option:hover {
    background: var(--color-bg-hover);
  }

  .option input,
  .check input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  .option input {
    margin-top: 2px;
  }

  .option-text {
    display: grid;
    gap: 2px;
    min-width: 0;
  }

  .option-label {
    font-size: var(--text-ui);
    color: var(--color-text-primary);
  }

  .option-desc {
    font-size: var(--text-xs);
    line-height: var(--leading-snug);
    color: var(--color-text-tertiary);
  }

  .segment-grid {
    display: grid;
    grid-template-columns: repeat(auto-fill, minmax(72px, 1fr));
    gap: var(--space-2) var(--space-3);
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    height: 24px;
    font-size: var(--text-ui);
    color: var(--color-text-secondary);
    cursor: pointer;
    user-select: none;
  }

  .add-row {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    margin-top: var(--space-3);
  }

  .add-input {
    width: 220px;
  }

  .add-input :global(.ui-input) {
    text-transform: uppercase;
  }

  .add-input :global(.ui-input::placeholder) {
    text-transform: none;
  }
</style>
