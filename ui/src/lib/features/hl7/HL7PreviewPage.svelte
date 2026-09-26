<script lang="ts">
  import { browser } from '$app/environment';
  import { createHL7PreviewStore } from '$lib/features/hl7/hl7PreviewStore';
  import { parseHL7Preview } from '$lib/features/hl7/hl7Preview';
  import CodeEditor from '$lib/ui/editor/CodeEditor.svelte';
  import SplitPane from '$lib/ui/ide/SplitPane.svelte';
  import WarningList from '$lib/ui/WarningList.svelte';
  import HL7Inspector from '$lib/features/hl7/components/HL7Inspector.svelte';
  import PipelineChips, {
    type PipelineStage,
    type PipelineStep
  } from '$lib/features/hl7/components/PipelineChips.svelte';
  import { parseHL7Path } from '$lib/domain/hl7Path';
  import type { HL7PathLocation } from '$lib/domain/hl7Path';
  import { getHL7Value, normalizeHL7Newlines } from '$lib/domain/hl7Access';
  import SampleInbox from '$lib/features/hl7/components/SampleInbox.svelte';
  import { createHL7SampleStore } from '$lib/features/hl7/samples/sampleStore';
  import { rememberRecentSource } from '$lib/features/hl7/samples/recentSources';
  import type { HL7Sample } from '$lib/features/hl7/samples/types';
  import { onMount } from 'svelte';
  import ProfileDraftPanel from '$lib/features/hl7/components/ProfileDraftPanel.svelte';
  import { suggestFixes } from '$lib/features/hl7/profile/fixes';
  import { profileStore, selectedProfile } from '$lib/features/hl7/profile/profileStore';
  import type { ProfileFix } from '$lib/features/hl7/profile/types';
  import type { NewHL7Sample } from '$lib/features/hl7/samples/types';
  import { redactHL7, type HL7RedactionMode } from '$lib/domain/hl7Redact';
  import EventLineagePanel from '$lib/features/hl7/components/EventLineagePanel.svelte';
  import EventStreamPanel from '$lib/features/events/EventStreamPanel.svelte';
  import ExtractionPanel from '$lib/ui/ExtractionPanel.svelte';
  import QualityBadge from '$lib/ui/QualityBadge.svelte';
  import CommandPalette, { type PaletteCommand } from '$lib/ui/CommandPalette.svelte';
  import {
    Badge,
    Button,
    EmptyState,
    Icon,
    IconButton,
    Input,
    KeyValue,
    Select,
    Table,
    Tabs,
    Td,
    Th,
    Toolbar,
    Tr,
    type BadgeTone,
    type SelectOption,
    type TabItem
  } from '$lib/ui/primitives';
  import FolderOpen from '@lucide/svelte/icons/folder-open';
  import Play from '@lucide/svelte/icons/play';
  import Send from '@lucide/svelte/icons/send';
  import WrapText from '@lucide/svelte/icons/wrap-text';
  import Eraser from '@lucide/svelte/icons/eraser';
  import RotateCcw from '@lucide/svelte/icons/rotate-ccw';
  import FileText from '@lucide/svelte/icons/file-text';
  import { graphqlFetch, isErrorToasted } from '$lib/graphql/client';
  import { ExplainWarningsDocument, type ParseWarningInput, type SourceFormat, type EventType } from '$lib/gen/graphql';
  import type { WarningLike } from '$lib/domain/warnings';
  import { submitHL7Message } from '$lib/features/hl7/hl7Submit';
  import { resolveMapping } from '$lib/features/terminology/terminologyApi';
  import { toasts } from '$lib/ui/toastStore';
  import { SvelteSet } from 'svelte/reactivity';
  import { integrationSessionEngineEnabled } from '$lib/features/integration-session';
  import SessionRunProgress from '$lib/features/integration-session/SessionRunProgress.svelte';
  import SessionStreamNotice from '$lib/features/integration-session/SessionStreamNotice.svelte';
  import {
    problemNavigation,
    setSessionDiagnostics
  } from '$lib/ui/ide/panels/workflowProblemsStore';

  const store = createHL7PreviewStore();
  // Build flag AND the API's integrationSessions capability (see
  // resolveIntegrationSessionEngine); otherwise Run uses the stateless preview.
  $: sessionEngineEnabled = $integrationSessionEngineEnabled;

  let fileInputEl: HTMLInputElement | null = null;
  let dragDepth = 0;
  let isDragging = false;

  let editorRedactionMode: HL7RedactionMode = 'none';
  let useRedactionForPreview = false;
  let lastRunRedactionMode: HL7RedactionMode = 'none';
  let useRedactionForProcess = false;
  let lastProcessRedactionMode: HL7RedactionMode = 'none';

  // Track the profile ID and version used for the last parse
  let lastUsedProfileId: string | null = null;
  let lastUsedProfileVersion: string | null = null;

  const MAX_RECENT_SOURCES = 8;
  let recentSources: string[] = [];

  // Detect if profile has changed since last parse
  $: profileChanged =
    $state.result &&
    $selectedProfile &&
    (lastUsedProfileId !== $selectedProfile.id ||
      lastUsedProfileVersion !== $selectedProfile.version);
  const { state, warningsByPhase, events, sessionDiagnostics, hl7, updateWarningExplanation } = store;
  const samplesStore = createHL7SampleStore();

  // LLM explanation state - tracks which warning codes are currently loading
  let explainLoadingCodes = new SvelteSet<string>();

  /**
   * Creates a unique key for a warning (code is unique within a parse result).
   */
  function warningKey(w: WarningLike): string {
    return w.code;
  }

  /**
   * Handles the explain event from WarningList.
   * Calls the GraphQL API to get LLM-powered explanation for a warning.
   */
  async function onExplainWarning(e: CustomEvent<WarningLike>) {
    const warning = e.detail;
    const key = warningKey(warning);
    explainLoadingCodes.add(key);

    try {
      const input: ParseWarningInput[] = [
        {
          phase: warning.phase,
          code: warning.code,
          message: warning.message,
          path: warning.path ?? null,
          severity: warning.severity ?? null
        }
      ];

      const result = await graphqlFetch(ExplainWarningsDocument, {
        warnings: input,
        format: 'HL7V2' as SourceFormat
      });

      // Update the store with the explanation
      const firstResult = result.explainWarnings[0];
      if (firstResult) {
        updateWarningExplanation(warning.code, firstResult);
      }
    } catch (err) {
      console.error('Failed to get explanation:', err);
    } finally {
      explainLoadingCodes.delete(key);
    }
  }

  /**
   * Explains all warnings that don't have explanations yet.
   * Calls the GraphQL API in a single batch request.
   */
  async function onExplainAll() {
    const warnings = $state.result?.parsePreview.warnings ?? [];
    const unexplained = warnings.filter((w) => !w.explanation);

    if (unexplained.length === 0) return;

    // Mark all as loading
    for (const w of unexplained) {
      explainLoadingCodes.add(warningKey(w));
    }

    try {
      const input: ParseWarningInput[] = unexplained.map((w) => ({
        phase: w.phase,
        code: w.code,
        message: w.message,
        path: w.path ?? null,
        severity: w.severity ?? null
      }));

      const result = await graphqlFetch(ExplainWarningsDocument, {
        warnings: input,
        format: 'HL7V2' as SourceFormat
      });

      // Update each warning with its explanation
      for (const explained of result.explainWarnings) {
        updateWarningExplanation(explained.code, explained);
      }
    } catch (err) {
      console.error('Failed to get explanations:', err);
    } finally {
      // Clear all loading states
      for (const w of unexplained) {
        explainLoadingCodes.delete(warningKey(w));
      }
    }
  }
  const { samples, activeId, activeSample } = samplesStore;

  type ResultsTab =
    | 'samples'
    | 'warnings'
    | 'events'
    | 'extraction'
    | 'inspector'
    | 'profile'
    | 'process'
    | 'live';
  let activeTab: ResultsTab = 'warnings';
  let selectedPath: string | null = null;
  let selectedLocation: HL7PathLocation | null = null;
  let warningCount = 0;
  let eventCount = 0;

  $: activeSampleModified = Boolean($activeSample && $activeSample.raw !== $state.data);
  $: selectedValue = selectedLocation ? getHL7Value($hl7, selectedLocation) : null;

  let paletteOpen = false;

  function isEditableTarget(t: EventTarget | null): boolean {
    const el = t as HTMLElement | null;
    if (!el) return false;
    const tag = el.tagName?.toLowerCase?.() ?? '';
    if (tag === 'input' || tag === 'textarea' || tag === 'select') return true;
    return Boolean(el.isContentEditable);
  }

  async function copyText(text: string): Promise<void> {
    if (!browser) return;
    if (!text) return;
    if (navigator.clipboard?.writeText) {
      await navigator.clipboard.writeText(text);
      return;
    }
    const ta = document.createElement('textarea');
    ta.value = text;
    ta.style.position = 'fixed';
    ta.style.opacity = '0';
    document.body.appendChild(ta);
    ta.select();
    document.execCommand('copy');
    document.body.removeChild(ta);
  }

  function focusWarningFilter(): void {
    const el = document.getElementById('warning-filter') as HTMLInputElement | null;
    el?.focus();
  }

  function focusInspectorFilter(): void {
    const el = document.getElementById('hl7-inspector-filter') as HTMLInputElement | null;
    el?.focus();
  }

  function clearSelection(): void {
    selectedPath = null;
    selectedLocation = null;
    activeTab = 'warnings';
  }

  async function previewFromPalette(): Promise<void> {
    if ($state.loading) return;
    if (!$state.data.trim()) return;
    await run();
  }

  async function processFromPalette(): Promise<void> {
    if ($state.loading) return;
    if (!$state.data.trim()) return;
    await processMessage();
  }

  function loadFileFromPalette(): void {
    if ($state.loading) return;
    fileInputEl?.click();
  }

  async function copyRawFromPalette(): Promise<void> {
    const data = normalizeHL7Newlines(getSnapshot().data);
    if (!data.trim()) return;
    await copyText(data);
  }

  function selectWarningRelative(delta: number): void {
    const warnings = $state.result?.parsePreview.warnings ?? [];
    const withPath = warnings.filter((w) => Boolean(w.path));
    if (!withPath.length) return;
    const current = selectedPath ? withPath.findIndex((w) => w.path === selectedPath) : -1;
    const start = current >= 0 ? current : 0;
    const next = ((start + delta) % withPath.length + withPath.length) % withPath.length;
    const w = withPath[next];
    if (!w) return;
    selectedPath = w.path ?? null;
    selectedLocation = parseHL7Path(selectedPath);
    activeTab = 'warnings';
  }

  const redactionOptions: SelectOption[] = [
    { value: 'none', label: 'No redaction' },
    { value: 'mask_basic', label: 'Mask basic (PID/NK1/PV1)' },
    { value: 'segment_sanitize', label: 'Sanitize segments (PID/NK1/IN*)' },
    { value: 'pattern_replace', label: 'Pattern replacement (SSN/phone/email)' }
  ];

  type ProcessState =
    | { state: 'idle' }
    | { state: 'running'; correlationId: string }
    | { state: 'error'; correlationId: string; message: string }
    | { state: 'done'; correlationId: string; result: Awaited<ReturnType<typeof submitHL7Message>> };

  let processState: ProcessState = { state: 'idle' };
  let lastProcessedSource: string | null = null;

  function makeCorrelationId(): string {
    const fromMsg = (msh10 ?? '').trim();
    if (fromMsg) return fromMsg;
    if (browser && typeof crypto !== 'undefined' && 'randomUUID' in crypto) {
      return (crypto as Crypto).randomUUID();
    }
    return `ui-${Date.now()}-${Math.random().toString(16).slice(2)}`;
  }

  async function processMessage(): Promise<void> {
    if ($state.loading) return;
    if (!($state.data ?? '').trim()) return;

    const snapshot = getSnapshot();
    const correlationId = makeCorrelationId();
    const data =
      useRedactionForProcess && editorRedactionMode !== 'none'
        ? redactHL7(snapshot.data, editorRedactionMode)
        : snapshot.data;
    lastProcessRedactionMode =
      useRedactionForProcess && editorRedactionMode !== 'none' ? editorRedactionMode : 'none';

    processState = { state: 'running', correlationId };
    lastProcessedSource = snapshot.source;

    try {
      const result = await submitHL7Message({
        source: snapshot.source,
        data,
        correlationId
      });
      processState = { state: 'done', correlationId, result };
      activeTab = 'process';
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      processState = { state: 'error', correlationId, message: msg };
      activeTab = 'process';
    }
  }

  async function run() {
    // A new run starts from no session state, so a run that fails before its
    // first session update never leaves the previous run's "Preview complete"
    // (or its diagnostics) on screen. The server session itself is reused.
    const previousSessionId = $state.session?.mode === 'session' ? $state.session.id : null;
    state.update((s) => ({ ...s, loading: true, error: null, result: null, session: null }));
    setSessionDiagnostics(null);
    selectedPath = null;
    selectedLocation = null;
    const snapshot = getSnapshot();
    rememberSource(snapshot.source);
    const profileId = $selectedProfile?.id ?? null;
    const data =
      useRedactionForPreview && editorRedactionMode !== 'none'
        ? redactHL7(snapshot.data, editorRedactionMode)
        : snapshot.data;

    try {
      const result = await parseHL7Preview({
        source: snapshot.source,
        data,
        profileId,
        profile: $selectedProfile,
        sessionId: previousSessionId,
        onSessionUpdate: (session) => {
          state.update((current) => ({ ...current, session }));
          setSessionDiagnostics(session);
        }
      });
      lastUsedProfileId = profileId;
      lastUsedProfileVersion = $selectedProfile?.version ?? null;
      lastRunRedactionMode =
        useRedactionForPreview && editorRedactionMode !== 'none' ? editorRedactionMode : 'none';
      state.update((s) => ({ ...s, loading: false, result, session: result.session ?? null }));
    } catch (e) {
      const msg = e instanceof Error ? e.message : String(e);
      state.update((s) => ({ ...s, loading: false, error: msg }));
    }
  }

  function onSelectWarning(
    e: CustomEvent<{ phase: string; code: string; message: string; path?: string | null }>
  ) {
    selectedPath = e.detail.path ?? null;
    selectedLocation = parseHL7Path(selectedPath);
    activeTab = 'warnings';
  }

  function onInspectWarning(
    e: CustomEvent<{ phase: string; code: string; message: string; path?: string | null }>
  ) {
    selectedPath = e.detail.path ?? null;
    selectedLocation = parseHL7Path(selectedPath);
    activeTab = selectedLocation ? 'inspector' : 'warnings';
  }

  function getSnapshot() {
    let snapshot: { source: string; data: string } | null = null;
    state.subscribe((s) => (snapshot = { source: s.source, data: s.data }))();
    if (!snapshot) {
      return { source: 'ui', data: '' };
    }
    return snapshot;
  }

  function rememberSource(source: string): void {
    recentSources = rememberRecentSource(recentSources, source, MAX_RECENT_SOURCES);
  }

  function loadSample(sample: HL7Sample) {
    state.update((s) => ({ ...s, source: sample.source, data: sample.raw }));
    rememberSource(sample.source);
  }

  function baseName(filename: string): string {
    return filename.replace(/\.[^.]+$/, '');
  }

  async function filesToInputs(files: File[]): Promise<NewHL7Sample[]> {
    const inputs: NewHL7Sample[] = [];
    for (const f of files) {
      const raw = await f.text();
      const inferred = baseName(f.name);
      inputs.push({
        name: inferred,
        source: inferred || ($state.source || 'ui_preview'),
        raw
      });
    }
    return inputs;
  }

  async function importFiles(
    files: File[],
    activate: 'first' | 'last' = 'first',
    opts?: { source?: string; feed?: string; tags?: string[]; redactionMode?: HL7RedactionMode }
  ) {
    if (!files.length) return;
    const inputs = await filesToInputs(files);
    const sourceOverride = opts?.source?.trim() || '';
    const feed = opts?.feed?.trim() || undefined;
    const tags = opts?.tags;
    const redactionMode = opts?.redactionMode ?? 'none';
    const next = inputs.map((i) => {
      const raw = redactionMode !== 'none' ? redactHL7(i.raw, redactionMode) : i.raw;
      return {
        ...i,
        source: sourceOverride || i.source,
        ...(feed ? { feed } : {}),
        ...(tags?.length ? { tags } : {}),
        ...(redactionMode !== 'none' ? { redactionMode } : {}),
        raw
      };
    });
    samplesStore.addMany(next, activate);
  }

  async function loadFromFile(e: Event) {
    const input = e.currentTarget as HTMLInputElement;
    const files = input.files ? Array.from(input.files) : [];
    if (!files.length) return;

    if (files.length === 1) {
      const file = files[0]!;
      const text = await file.text();
      const inferredSource = baseName(file.name);
      state.update((s) => ({
        ...s,
        data: text,
        source: s.source && s.source !== 'ui_preview' ? s.source : inferredSource
      }));
      rememberSource(inferredSource);
    } else {
      await importFiles(files, 'first');
    }

    input.value = '';
  }

  function onDragEnter(e: DragEvent) {
    if ($state.loading) return;
    if (!e.dataTransfer?.types?.includes('Files')) return;
    dragDepth += 1;
    isDragging = true;
  }

  function onDragLeave() {
    dragDepth = Math.max(0, dragDepth - 1);
    if (dragDepth === 0) isDragging = false;
  }

  async function onDropFiles(e: DragEvent) {
    if ($state.loading) return;
    const files = e.dataTransfer?.files ? Array.from(e.dataTransfer.files) : [];
    if (!files.length) return;
    e.preventDefault();
    dragDepth = 0;
    isDragging = false;
    await importFiles(files, 'first');
  }

  function applyRedactionToEditor(): void {
    if ($state.loading) return;
    if (editorRedactionMode === 'none') return;
    if (!$state.data.trim()) return;
    const redacted = redactHL7($state.data, editorRedactionMode);
    state.update((s) => ({ ...s, data: redacted }));
  }

  function normalizeEditorNewlines(): void {
    if ($state.loading) return;
    if (!$state.data.trim()) return;
    state.update((s) => ({ ...s, data: normalizeHL7Newlines(s.data) }));
  }

  function inspectPath(path: string): void {
    selectedPath = path;
    selectedLocation = parseHL7Path(path);
    activeTab = selectedLocation ? 'inspector' : 'warnings';
  }

  $: msh9 = getHL7Value($hl7, parseHL7Path('MSH-9'));
  $: msh10 = getHL7Value($hl7, parseHL7Path('MSH-10'));
  $: msh12 = getHL7Value($hl7, parseHL7Path('MSH-12'));

  // Infer event type from MSH-9 for quality analysis
  function inferEventType(msh9Val: string | null): EventType {
    if (!msh9Val) return 'LAB_RESULT';
    const normalized = msh9Val.toUpperCase();
    if (normalized.startsWith('ADT^A01')) return 'PATIENT_ADMIT';
    if (normalized.startsWith('ADT^A02')) return 'PATIENT_TRANSFER';
    if (normalized.startsWith('ADT^A03')) return 'PATIENT_DISCHARGE';
    if (normalized.startsWith('ADT^A04')) return 'PATIENT_ADMIT'; // Registration -> admit
    if (normalized.startsWith('ADT^A08')) return 'PATIENT_UPDATE';
    if (normalized.startsWith('ORU')) return 'LAB_RESULT';
    if (normalized.startsWith('ORM')) return 'LAB_ORDERED';
    if (normalized.startsWith('MDM')) return 'DOCUMENT';
    if (normalized.startsWith('SIU')) return 'APPOINTMENT_SCHEDULED';
    if (normalized.startsWith('VXU')) return 'IMMUNIZATION';
    return 'LAB_RESULT';
  }
  $: inferredEventType = inferEventType(msh9);
  $: warningCount = $state.result?.parsePreview.warnings.length ?? 0;
  $: eventCount = $events.length;

  // Generate fixes based on warnings and current profile
  $: fixes = suggestFixes($state.result?.parsePreview.warnings ?? [], $selectedProfile);

  // ── Pipeline chips: what the last preview/process actually used ──────────
  // Values come from the stateless preview's artifact revisions and planned
  // deliveries, or from the process result; nothing is filled in by default.
  function unique(values: readonly string[]): string[] {
    return Array.from(new Set(values.filter(Boolean)));
  }

  $: preview = $state.result?.preview ?? null;
  $: parseOk = $state.result ? $state.result.parsePreview.success : null;
  $: matchedRoutes = preview?.routes.filter((route) => route.matched) ?? [];
  $: plannedDestinations = unique(
    preview?.deliveries.map((delivery) => delivery.destination.artifactId) ?? []
  );
  $: processedWorkflows =
    processState.state === 'done'
      ? unique(processState.result.workflowResults.map((workflow) => workflow.workflowName))
      : [];
  $: workflowState = ((): PipelineStep['state'] => {
    if (preview) return matchedRoutes.length > 0 ? 'ok' : 'warning';
    if (processState.state !== 'done') return 'idle';
    return processState.result.workflowResults.some((workflow) => workflow.errors.length > 0)
      ? 'error'
      : 'ok';
  })();
  $: pipelineSteps = [
    {
      id: 'source',
      label: 'Source',
      value: $state.source || 'ui_preview',
      state: $state.error ? 'error' : parseOk === null ? 'idle' : parseOk ? 'ok' : 'error',
      title: 'Source name sent with the message. Opens Samples.'
    },
    {
      id: 'profile',
      label: 'Profile',
      value: preview?.artifactRevisions.profile.artifactId ?? $selectedProfile?.id ?? '',
      state: parseOk === null ? 'idle' : !parseOk ? 'error' : warningCount > 0 ? 'warning' : 'ok',
      title: preview
        ? 'Profile revision the preview ran with. Opens the profile draft.'
        : 'Selected source profile. Opens the profile draft.'
    },
    {
      id: 'workflow',
      label: 'Workflow',
      value: preview?.artifactRevisions.workflow.artifactId ?? processedWorkflows.join(', '),
      state: workflowState,
      title: preview
        ? `${matchedRoutes.length} of ${preview.routes.length} routes matched. Opens Events.`
        : 'Workflow the message was routed through. Opens Events.'
    },
    {
      id: 'destination',
      label: 'Destination',
      value: plannedDestinations.join(', '),
      state: plannedDestinations.length > 0 ? 'ok' : 'idle',
      title: 'Destinations the preview planned deliveries to. Opens Process.'
    }
  ] satisfies PipelineStep[];

  const PIPELINE_TAB: Record<PipelineStage, ResultsTab> = {
    source: 'samples',
    profile: 'profile',
    workflow: 'events',
    destination: 'process'
  };

  $: tabItems = [
    { id: 'samples', label: 'Samples', count: $samples.length || undefined },
    { id: 'warnings', label: 'Warnings', count: $state.result ? warningCount : undefined },
    { id: 'events', label: 'Events', count: $state.result ? eventCount : undefined },
    { id: 'extraction', label: 'Extraction' },
    { id: 'inspector', label: 'Inspector' },
    { id: 'profile', label: 'Profile draft' },
    { id: 'process', label: 'Process' },
    { id: 'live', label: 'Live events' }
  ] satisfies TabItem[];

  // The stateless status line (the session engine renders SessionRunProgress).
  let runTone: BadgeTone = 'neutral';
  $: runTone = $state.loading
    ? 'info'
    : $state.error
      ? 'danger'
      : !$state.result
        ? 'neutral'
        : $state.result.parsePreview.success
          ? 'success'
          : 'danger';
  $: runLabel = $state.loading
    ? 'Previewing'
    : $state.error
      ? 'Preview failed'
      : !$state.result
        ? 'Not previewed'
        : $state.result.parsePreview.success
          ? 'Parsed'
          : 'Parse failed';

  // The context row under the status line; hidden when it would be empty.
  $: hasContext =
    processState.state !== 'idle' ||
    lastProcessRedactionMode !== 'none' ||
    (Boolean($state.result) &&
      (Boolean(lastUsedProfileId) ||
        lastRunRedactionMode !== 'none' ||
        (sessionEngineEnabled && Boolean($state.session))));

  let processTone: BadgeTone = 'info';
  $: processTone =
    processState.state === 'done'
      ? processState.result.success
        ? 'success'
        : 'danger'
      : processState.state === 'error'
        ? 'danger'
        : 'info';

  // Apply a suggested fix to the current profile
  function applyFix(fix: ProfileFix) {
    if (!$selectedProfile) {
      console.warn('Cannot apply fix: no profile selected');
      return;
    }
    // Apply the changes to the local state
    profileStore.updateLocal(fix.changes);
    // Switch to the profile tab to show the changes
    // activeTab = 'profile'; // Commented out to stay on the warnings tab while editing
    if ($activeSample) {
      void run();
    }
  }

  async function handleResolveWarning(w: WarningLike) {
    if (!w.path) return;
    
    // Parse the path to get the value from HL7
    const loc = parseHL7Path(w.path);
    if (!loc) return;
    
    const value = getHL7Value($hl7, loc);
    if (!value) {
      toasts.error('Could not find code value at path ' + w.path);
      return;
    }

    // Best effort to find the system (usually field 3 or 4 in OBX/DG1)
    // For now we'll let the backend suggest it or use a default.
    try {
      await resolveMapping({
        sourceSystem: 'FIXME_SYSTEM', // Backend should ideally infer this
        sourceCode: value,
        targetSystem: 'http://loinc.org', // Default target
        sourceDisplay: null,
        profileId: $selectedProfile?.id ?? null,
        minConfidence: 0,
        allowAutoroute: true
      });
      toasts.success(`Resolved mapping for ${value}`);
      // Re-run to clear the warning
      void run();
    } catch (e) {
      // Global graphqlFetch net already toasts graphql failures (B4 dedupe).
      if (!isErrorToasted(e)) {
        toasts.error('Failed to resolve mapping: ' + (e instanceof Error ? e.message : String(e)));
      }
    }
  }

  $: paletteCommands = (() => {
    const cmds: PaletteCommand[] = [
      {
        id: 'preview',
        label: 'Preview (parse)',
        hint: 'Cmd/Ctrl+Enter',
        keywords: ['run', 'parse', 'preview'],
        run: previewFromPalette
      },
      {
        id: 'process',
        label: 'Process message',
        hint: 'Submit to pipeline',
        keywords: ['submit', 'process', 'workflow'],
        run: processFromPalette
      },
      {
        id: 'load-file',
        label: 'Load HL7 file…',
        hint: 'Cmd/Ctrl+O',
        keywords: ['open', 'file', 'upload'],
        run: loadFileFromPalette
      },
      {
        id: 'open-samples',
        label: 'Open samples',
        hint: 'Browse inbox',
        keywords: ['samples', 'inbox'],
        run: () => {
          activeTab = 'samples';
        }
      },
      {
        id: 'go-warnings',
        label: 'Go to warnings',
        hint: 'Tab',
        keywords: ['warnings', 'phase'],
        run: () => {
          activeTab = 'warnings';
        }
      },
      {
        id: 'go-events',
        label: 'Go to events',
        hint: 'Tab',
        keywords: ['events', 'canonical'],
        run: () => {
          activeTab = 'events';
        }
      },
      {
        id: 'go-extraction',
        label: 'Go to extraction',
        hint: 'Tab',
        keywords: ['extraction', 'fields'],
        run: () => {
          activeTab = 'extraction';
        }
      },
      {
        id: 'go-inspector',
        label: 'Go to inspector',
        hint: 'Tab',
        keywords: ['inspector', 'hl7', 'segments'],
        run: () => {
          activeTab = 'inspector';
        }
      },
      {
        id: 'go-profile',
        label: 'Go to profile draft',
        hint: 'Tab',
        keywords: ['profile', 'draft', 'fix'],
        run: () => {
          activeTab = 'profile';
        }
      },
      {
        id: 'go-process',
        label: 'Go to process',
        hint: 'Tab',
        keywords: ['process', 'submit'],
        run: () => {
          activeTab = 'process';
        }
      },
      {
        id: 'focus-warnings-filter',
        label: 'Focus warnings filter',
        hint: 'Jump to warnings search',
        keywords: ['warnings', 'search', 'filter'],
        run: () => {
          activeTab = 'warnings';
          focusWarningFilter();
        }
      },
      {
        id: 'focus-inspector-filter',
        label: 'Focus inspector filter',
        hint: 'Jump to segment filter',
        keywords: ['inspector', 'segments', 'search'],
        run: () => {
          activeTab = 'inspector';
          focusInspectorFilter();
        }
      },
      {
        id: 'next-warning',
        label: 'Next warning (with path)',
        hint: 'Alt+ArrowDown',
        keywords: ['warnings', 'next'],
        run: () => selectWarningRelative(1)
      },
      {
        id: 'prev-warning',
        label: 'Previous warning (with path)',
        hint: 'Alt+ArrowUp',
        keywords: ['warnings', 'previous'],
        run: () => selectWarningRelative(-1)
      },
      {
        id: 'clear-selection',
        label: 'Clear selection',
        hint: 'Esc',
        keywords: ['clear', 'selection', 'reset'],
        run: clearSelection
      }
    ];

    const raw = ($state.data ?? '').trim();
    if (raw) {
      cmds.unshift({
        id: 'copy-raw',
        label: 'Copy raw HL7',
        hint: 'Editor contents',
        keywords: ['copy', 'raw', 'message'],
        run: copyRawFromPalette
      });
    }

    if (msh9) {
      cmds.unshift({
        id: 'copy-msh-9',
        label: 'Copy MSH-9 (message type)',
        hint: 'ADT^A01',
        keywords: ['copy', 'msh', 'type', 'event'],
        run: () => copyText(msh9)
      });
    }
    if (msh10) {
      cmds.unshift({
        id: 'copy-msh-10',
        label: 'Copy MSH-10 (control ID)',
        hint: 'Correlation ID',
        keywords: ['copy', 'msh', 'id', 'control'],
        run: () => copyText(msh10)
      });
    }
    if (msh12) {
      cmds.unshift({
        id: 'copy-msh-12',
        label: 'Copy MSH-12 (version)',
        hint: '2.5.1',
        keywords: ['copy', 'msh', 'version'],
        run: () => copyText(msh12)
      });
    }

    const path = selectedPath;
    if (path) {
      cmds.unshift({
        id: 'copy-path',
        label: 'Copy selected path',
        hint: 'Cmd/Ctrl+Shift+C',
        keywords: ['copy', 'path'],
        run: () => copyText(path)
      });
    }
    const value = selectedValue;
    if (value) {
      cmds.unshift({
        id: 'copy-value',
        label: 'Copy selected value',
        hint: 'Cmd/Ctrl+Shift+X',
        keywords: ['copy', 'value'],
        run: () => copyText(value)
      });
    }

    return cmds;
  })();

  onMount(() => {
    // Load a sample into the editor when the active sample *changes*. The
    // store re-emits on every samples update (a rename, tags, a removal of
    // another sample), and reloading then would overwrite the editor without
    // asking. Picking a row still loads it through the inbox's select event.
    let loadedSampleId: string | null = null;
    const unsub = activeSample.subscribe((s) => {
      const id = s?.id ?? null;
      if (id === loadedSampleId) return;
      loadedSampleId = id;
      if (s) loadSample(s);
    });
    let lastNavigationSequence = 0;
    const unsubProblemNavigation = problemNavigation.subscribe((navigation) => {
      if (!navigation || navigation.sequence === lastNavigationSequence) return;
      lastNavigationSequence = navigation.sequence;
      inspectPath(navigation.path);
    });

    const onKeyDown = (e: KeyboardEvent) => {
      if (e.defaultPrevented) return;
      if (paletteOpen) return;
      if (isEditableTarget(e.target)) return;

      const mod = e.metaKey || e.ctrlKey;
      const shift = e.shiftKey;
      if (mod && e.key === 'Enter') {
        if ($state.loading) return;
        if (!$state.data.trim()) return;
        e.preventDefault();
        void run();
        return;
      }

      if (mod && (e.key === 'o' || e.key === 'O')) {
        if ($state.loading) return;
        e.preventDefault();
        fileInputEl?.click();
        return;
      }

      if (mod && (e.key === 'k' || e.key === 'K')) {
        e.preventDefault();
        paletteOpen = true;
        return;
      }

      if (e.altKey && e.key === 'ArrowDown') {
        e.preventDefault();
        selectWarningRelative(1);
        return;
      }
      if (e.altKey && e.key === 'ArrowUp') {
        e.preventDefault();
        selectWarningRelative(-1);
        return;
      }

      if (mod && shift && (e.key === 'c' || e.key === 'C')) {
        if (!selectedPath) return;
        e.preventDefault();
        void copyText(selectedPath);
        return;
      }

      if (mod && shift && (e.key === 'x' || e.key === 'X')) {
        if (!selectedValue) return;
        e.preventDefault();
        void copyText(selectedValue);
        return;
      }

      if (e.key === 'Escape') {
        if (selectedPath || selectedLocation) {
          e.preventDefault();
          selectedPath = null;
          selectedLocation = null;
          activeTab = 'warnings';
          return;
        }
        if (activeTab !== 'warnings') {
          e.preventDefault();
          activeTab = 'warnings';
        }
      }
    };

    window.addEventListener('keydown', onKeyDown);

    return () => {
      unsub();
      unsubProblemNavigation();
      setSessionDiagnostics(null);
      window.removeEventListener('keydown', onKeyDown);
    };
  });
</script>

<CommandPalette bind:open={paletteOpen} title="HL7 commands" commands={paletteCommands} />

<div class="intake">
  <Toolbar title="HL7 intake">
    {#snippet actions()}
      <span class="redaction-select">
        <Select
          aria-label="Redaction"
          title="Redaction mode for the editor, preview and process. Best-effort: free-text fields may still contain PHI."
          value={editorRedactionMode}
          options={redactionOptions}
          disabled={$state.loading}
          onchange={(e: Event) => {
            editorRedactionMode = (e.currentTarget as HTMLSelectElement).value as HL7RedactionMode;
          }}
        />
      </span>
      <input
        class="file-input"
        type="file"
        multiple
        accept=".hl7,.txt,.msg,.dat,text/plain"
        bind:this={fileInputEl}
        on:change={loadFromFile}
        disabled={$state.loading}
      />
      <Button
        variant="ghost"
        icon={FolderOpen}
        title="Load an HL7 file into the editor (⌘/Ctrl+O). Several files go to Samples."
        disabled={$state.loading}
        onclick={() => fileInputEl?.click()}
      >
        Load file
      </Button>
      <Button
        variant="primary"
        icon={Play}
        title="Parse the editor contents (⌘/Ctrl+Enter). Esc returns to Warnings; ⌘/Ctrl+K opens HL7 commands."
        loading={$state.loading}
        disabled={!$state.data.trim()}
        onclick={run}
      >
        Preview
      </Button>
      <Button
        icon={Send}
        title="Submit the message to the pipeline"
        loading={processState.state === 'running'}
        disabled={$state.loading || !$state.data.trim()}
        onclick={processMessage}
      >
        Process
      </Button>
    {/snippet}
  </Toolbar>

  <div class="pipeline-row">
    <PipelineChips steps={pipelineSteps} onselect={(stage) => (activeTab = PIPELINE_TAB[stage])} />
  </div>

  <div class="workspace">
    <SplitPane
      orientation="horizontal"
      initialSize={600}
      minSize={360}
      maxSize={1200}
      storageKey="fi-fhir-hl7-intake-split"
    >
      <div
        class="editor-pane"
        class:dragging={isDragging}
        on:dragenter={onDragEnter}
        on:dragleave={onDragLeave}
        on:dragover|preventDefault
        on:drop={onDropFiles}
        role="region"
        aria-label="HL7 input. Drag and drop HL7 files to import."
        aria-describedby="hl7-drop-hint"
      >
        <p id="hl7-drop-hint" class="sr-only">
          Drag and drop HL7 files to import into Samples. Use the Load file button to open the file picker.
        </p>

        <div class="editor-bar">
          <label class="inline-field">
            <span class="inline-label">Source</span>
            <span class="source-input">
              <Input
                mono
                value={$state.source}
                placeholder="epic_adt_hosp_a"
                list="hl7-recent-sources"
                disabled={$state.loading}
                oninput={(e: Event) => {
                  const value = (e.currentTarget as HTMLInputElement).value;
                  state.update((s) => ({ ...s, source: value }));
                }}
              />
            </span>
          </label>
          <datalist id="hl7-recent-sources">
            {#each recentSources as src (src)}
              <option value={src}></option>
            {/each}
          </datalist>

          {#if $activeSample}
            <button
              class="sample-link"
              type="button"
              title="Active sample — open Samples"
              disabled={$state.loading}
              on:click={() => (activeTab = 'samples')}
            >
              <Icon icon={FileText} size={14} />
              <span class="sample-name">{$activeSample.name}</span>
            </button>
            {#if activeSampleModified}
              <Badge tone="warning">Modified</Badge>
            {/if}
          {/if}

          <div class="editor-bar-end">
            <label class="check" title="Apply the redaction mode to the preview payload">
              <input type="checkbox" bind:checked={useRedactionForPreview} disabled={$state.loading} />
              Redact preview
            </label>
            <label class="check" title="Apply the redaction mode to the process payload">
              <input type="checkbox" bind:checked={useRedactionForProcess} disabled={$state.loading} />
              Redact process
            </label>
            <IconButton
              icon={Eraser}
              label="Apply redaction to editor"
              disabled={$state.loading || editorRedactionMode === 'none' || !$state.data.trim()}
              onclick={applyRedactionToEditor}
            />
            <IconButton
              icon={WrapText}
              label="Normalize newlines"
              title="Normalize newlines (CR segment terminators)"
              disabled={$state.loading || !$state.data.trim()}
              onclick={normalizeEditorNewlines}
            />
          </div>
        </div>

        <div class="editor-body">
          <CodeEditor
            language="hl7v2"
            value={$state.data}
            on:change={(e) => { $state.data = e.detail; }}
            readOnly={$state.loading}
          />
          {#if isDragging}
            <div class="drop-hint">Drop files to import into Samples</div>
          {/if}
        </div>

        <div class="editor-foot">
          <span>{$hl7.segments.length} segment{$hl7.segments.length === 1 ? '' : 's'}</span>
          {#if msh9}<span>MSH-9 <span class="mono">{msh9}</span></span>{/if}
          {#if msh10}<span>MSH-10 <span class="mono">{msh10}</span></span>{/if}
          {#if msh12}<span>MSH-12 <span class="mono">{msh12}</span></span>{/if}
        </div>
      </div>

      <div slot="secondary" class="results-pane" aria-label="Results" role="region">
        <div class="status-line">
          {#if sessionEngineEnabled && $state.session}
            <div class="session-status">
              <SessionRunProgress session={$state.session} />
              {#if $state.error}
                <p class="run-error" role="alert" title={$state.error}>{$state.error}</p>
              {/if}
            </div>
          {:else}
            <span class="run-state">
              <Badge tone={runTone} dot>{runLabel}</Badge>
              <span class="status-text" aria-live="polite">
                {#if $state.loading}
                  Parsing the message…
                {:else if $state.error}
                  {$state.error}
                {:else if !$state.result}
                  {#if !$state.data.trim()}
                    Paste an HL7v2 message to enable preview.
                  {:else}
                    Press Preview (⌘/Ctrl+Enter) to see warnings and extracted events.
                  {/if}
                {:else}
                  {eventCount} event{eventCount === 1 ? '' : 's'} · {warningCount} warning{warningCount === 1 ? '' : 's'}
                {/if}
              </span>
            </span>
          {/if}
          {#if profileChanged}
            <Button variant="ghost" icon={RotateCcw} disabled={$state.loading} onclick={run}>
              Profile changed — re-run
            </Button>
          {/if}
        </div>

        <SessionStreamNotice />

        {#if hasContext}
          <div class="context-row">
            {#if $state.result}
              {#if lastUsedProfileId}
                <span class="ctx" title="Source profile selected when this run started">
                  <span class="ctx-key">Draft profile</span>
                  <span class="mono">{lastUsedProfileId}</span>
                </span>
              {/if}
              {#if sessionEngineEnabled && $state.session}
                {#if $state.session.mode === 'session'}
                  <span class="ctx">
                    <span class="ctx-key">Session</span>
                    <span class="mono ctx-id" title={$state.session.id ?? ''}>{$state.session.id}</span>
                  </span>
                  {#if $state.session.runId}
                    <span class="ctx">
                      <span class="ctx-key">Run</span>
                      <span class="mono ctx-id" title={$state.session.runId}>{$state.session.runId}</span>
                    </span>
                  {/if}
                {:else}
                  <Badge tone="warning">Session fallback</Badge>
                {/if}
                {#if $sessionDiagnostics.length}
                  <Badge tone="warning" mono>{$sessionDiagnostics.length} diagnostics</Badge>
                {/if}
              {/if}
              {#if lastRunRedactionMode !== 'none'}
                <Badge tone="warning">Preview redacted: {lastRunRedactionMode}</Badge>
              {/if}
            {/if}
            {#if processState.state !== 'idle'}
              <button class="ctx ctx-link" type="button" on:click={() => (activeTab = 'process')}>
                <span class="ctx-key">Process</span>
                <Badge tone={processTone} dot>{processState.state}</Badge>
              </button>
            {/if}
            {#if lastProcessRedactionMode !== 'none'}
              <Badge tone="warning">Process redacted: {lastProcessRedactionMode}</Badge>
            {/if}
          </div>
        {/if}

        {#if $state.result?.parsePreview.errors.length}
          <ul class="issue-list" aria-label="Parse errors">
            {#each $state.result.parsePreview.errors as err (err)}
              <li>{err}</li>
            {/each}
          </ul>
        {/if}

        {#if sessionEngineEnabled && $sessionDiagnostics.length}
          <div class="diagnostics" aria-label="Session diagnostics" role="group">
            {#each $sessionDiagnostics as diagnostic (diagnostic.id)}
              <div class="diagnostic">
                <span class="mono">{diagnostic.code}</span>
                <span class="diagnostic-message">{diagnostic.message}</span>
                {#if diagnostic.path}
                  <button
                    class="path-link mono"
                    type="button"
                    on:click={() => inspectPath(diagnostic.path ?? '')}
                    disabled={$state.loading}
                  >
                    {diagnostic.path}
                  </button>
                {/if}
              </div>
            {/each}
          </div>
        {/if}

        <div class="results-tabs">
          <Tabs
            label="Results"
            items={tabItems}
            value={activeTab}
            onchange={(id) => (activeTab = id as ResultsTab)}
          />
        </div>

        <div class="results-body">
          {#if activeTab === 'samples'}
            <SampleInbox
              samples={$samples}
              activeId={$activeId}
              disabled={$state.loading}
              currentRaw={$state.data}
              on:importFiles={async (e) => importFiles(e.detail.files, 'first', e.detail)}
              on:saveCurrent={(e) => {
                const n = e.detail.name;
                const source = e.detail.source?.trim() || $state.source;
                const redactionMode = e.detail.redactionMode ?? 'none';
                const raw = redactionMode !== 'none' ? redactHL7($state.data, redactionMode) : $state.data;
                const input: NewHL7Sample = {
                  ...(n ? { name: n } : {}),
                  source,
                  ...(e.detail.feed?.trim() ? { feed: e.detail.feed.trim() } : {}),
                  ...(e.detail.tags?.length ? { tags: e.detail.tags } : {}),
                  ...(redactionMode !== 'none' ? { redactionMode } : {}),
                  raw
                };
                samplesStore.add(input);
              }}
              on:updateMeta={(e) => {
                const before = $activeSample;
                const changes = {
                  name: e.detail.name,
                  source: e.detail.source,
                  feed: e.detail.feed,
                  tags: e.detail.tags,
                  ...(e.detail.redactionMode !== undefined ? { redactionMode: e.detail.redactionMode } : {})
                };
                samplesStore.updateMeta(e.detail.id, changes);
                if (before && before.id === e.detail.id && !activeSampleModified) {
                  state.update((s) => ({
                    ...s,
                    source: s.source === before.source ? e.detail.source : s.source
                  }));
                }
              }}
              on:select={(e) => {
                samplesStore.setActive(e.detail.id);
                const s = $samples.find((x) => x.id === e.detail.id);
                if (s) loadSample(s);
              }}
              on:remove={(e) => samplesStore.remove(e.detail.id)}
              on:bulkRemove={(e) => {
                for (const id of e.detail.ids) samplesStore.remove(id);
              }}
              on:bulkUpdateMeta={(e) => {
                for (const id of e.detail.ids) samplesStore.updateMeta(id, e.detail.changes);
              }}
              on:clear={() => samplesStore.clear()}
              on:loadExamples={() => samplesStore.loadDemoSamples()}
            />
          {:else if activeTab === 'warnings'}
            {#if !$state.result}
              <EmptyState align="start" message="Preview the message to list parse warnings by phase." />
            {:else}
              <WarningList
                groups={$warningsByPhase}
                {selectedPath}
                {explainLoadingCodes}
                on:select={onSelectWarning}
                on:inspect={onInspectWarning}
                on:explain={onExplainWarning}
                on:explainAll={onExplainAll}
                on:resolve={(e) => handleResolveWarning(e.detail)}
              />
            {/if}
          {:else if activeTab === 'events'}
            {#if !$state.result}
              <EmptyState align="start" message="Preview the message to see the extracted semantic events." />
            {:else}
              <EventLineagePanel
                events={$events}
                message={$hl7}
                lineage={$state.session?.lineage ?? []}
                on:inspectPath={(e) => inspectPath(e.detail.path)}
              />
              <div class="quality">
                <QualityBadge
                  event={$events[0] ?? { raw: $state.data, source: $state.source }}
                  eventType={inferredEventType}
                />
              </div>
            {/if}
          {:else if activeTab === 'extraction'}
            <ExtractionPanel text={$state.data} />
          {:else if activeTab === 'inspector'}
            <HL7Inspector
              message={$hl7}
              selected={selectedLocation}
              on:selectPath={(e) => inspectPath(e.detail.path)}
            />
          {:else if activeTab === 'profile'}
            <ProfileDraftPanel {fixes} onApplyFix={applyFix} />
          {:else if activeTab === 'process'}
            {#if processState.state === 'idle'}
              <EmptyState align="start" icon={Send} message="Process submits this message to the backend pipeline." />
            {:else if processState.state === 'running'}
              <p class="muted-line">
                Submitting… correlation id <span class="mono">{processState.correlationId}</span>
              </p>
            {:else if processState.state === 'error'}
              <div class="process-error" role="alert">
                <KeyValue items={[{ key: 'Correlation id', value: processState.correlationId, mono: true }]} />
                <p>{processState.message}</p>
              </div>
            {:else if processState.state === 'done'}
              <div class="process-result">
                <div class="process-head">
                  <Badge tone={processState.result.success ? 'success' : 'danger'} dot>
                    {processState.result.success ? 'Submitted' : 'Submission failed'}
                  </Badge>
                  <Button variant="ghost" disabled={$state.loading} onclick={() => (activeTab = 'live')}>
                    View live events
                  </Button>
                </div>
                <KeyValue
                  columns={2}
                  items={[
                    { key: 'Event id', value: processState.result.eventId, mono: true, truncate: true },
                    { key: 'Correlation id', value: processState.correlationId, mono: true, truncate: true },
                    { key: 'Source', value: lastProcessedSource, mono: true },
                    { key: 'Workflows', value: processState.result.workflowResults.length, mono: true },
                    { key: 'Warnings', value: processState.result.warnings.length, mono: true },
                    { key: 'Errors', value: processState.result.errors.length, mono: true }
                  ]}
                />

                {#if processState.result.errors.length}
                  <ul class="issue-list">
                    {#each processState.result.errors as err (err)}
                      <li>{err}</li>
                    {/each}
                  </ul>
                {/if}

                {#if processState.result.workflowResults.length}
                  <Table label="Workflow results" layout="fixed">
                    {#snippet head()}
                      <tr>
                        <Th>Workflow</Th>
                        <Th width="80px" numeric>Routes</Th>
                        <Th width="80px" numeric>Actions</Th>
                        <Th width="80px" numeric>Errors</Th>
                        <Th width="80px" numeric>ms</Th>
                      </tr>
                    {/snippet}
                    {#each processState.result.workflowResults as wf, idx (wf.workflowName + ':' + idx)}
                      <Tr>
                        <Td mono truncate value={wf.workflowName} />
                        <Td numeric value={wf.routesMatched} />
                        <Td numeric value={wf.actionsExecuted} />
                        <Td numeric value={wf.errors.length} />
                        <Td numeric value={wf.duration} />
                      </Tr>
                    {/each}
                  </Table>
                {/if}
              </div>
            {/if}
          {:else if activeTab === 'live'}
            <EventStreamPanel
              initialSource={lastProcessedSource ?? $state.source}
              initialCorrelationId={
                processState.state === 'running' || processState.state === 'error' || processState.state === 'done'
                  ? processState.correlationId
                  : ''
              }
            />
          {/if}
        </div>
      </div>
    </SplitPane>
  </div>
</div>

<style>
  .intake {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-height: 0;
  }

  .redaction-select {
    display: inline-flex;
    width: 200px;
  }

  .file-input {
    display: none;
  }

  .pipeline-row {
    display: flex;
    align-items: center;
    flex: 0 0 auto;
    min-width: 0;
    height: 36px;
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .workspace {
    flex: 1 1 auto;
    min-height: 0;
  }

  /* ── Editor pane ───────────────────────────────────────────────────── */
  .editor-pane {
    position: relative;
    display: flex;
    flex-direction: column;
    height: 100%;
    min-width: 0;
    min-height: 0;
  }

  .editor-pane.dragging {
    outline: 2px dashed var(--color-primary);
    outline-offset: -2px;
  }

  .editor-bar {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-3);
    flex: 0 0 auto;
    min-height: 36px;
    padding: var(--space-1) var(--space-2) var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .inline-field {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
  }

  .inline-label {
    font-size: var(--text-label);
    font-weight: var(--font-medium);
    letter-spacing: var(--tracking-label);
    text-transform: uppercase;
    color: var(--color-text-tertiary);
  }

  .source-input {
    display: inline-flex;
    width: 160px;
  }

  .sample-link {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    max-width: 180px;
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-text-secondary);
    font: inherit;
    font-size: var(--text-xs);
    cursor: pointer;
  }

  .sample-link:hover:not(:disabled) {
    color: var(--color-text-primary);
  }

  .sample-link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 2px;
    border-radius: var(--radius-sm);
  }

  .sample-name {
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }

  .editor-bar-end {
    display: inline-flex;
    align-items: center;
    gap: var(--space-2);
    margin-left: auto;
  }

  .check {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
    white-space: nowrap;
    user-select: none;
  }

  .check input {
    margin: 0;
    accent-color: var(--color-primary);
  }

  .editor-body {
    position: relative;
    flex: 1 1 auto;
    min-height: 0;
    background: var(--editor-bg, var(--color-bg-base));
  }

  /* The pane owns the edges; the editor theme's own frame would double them. */
  .editor-body :global(.cm-editor) {
    border-width: 0;
    border-radius: 0;
  }

  .drop-hint {
    position: absolute;
    inset: var(--space-3);
    display: grid;
    place-items: center;
    border: 1px dashed var(--color-primary-border);
    border-radius: var(--radius-sm);
    background: var(--color-bg-overlay);
    color: var(--color-text-primary);
    font-size: var(--text-ui);
    pointer-events: none;
  }

  .editor-foot {
    display: flex;
    align-items: center;
    gap: var(--space-4);
    flex: 0 0 auto;
    height: 24px;
    padding: 0 var(--space-3);
    border-top: 1px solid var(--color-border-subtle);
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
    white-space: nowrap;
    overflow: hidden;
  }

  /* ── Results pane ──────────────────────────────────────────────────── */
  .results-pane {
    display: flex;
    flex-direction: column;
    height: 100%;
    min-width: 0;
    min-height: 0;
    background: var(--color-bg-elevated);
  }

  .status-line {
    display: flex;
    align-items: center;
    gap: var(--space-3);
    flex: 0 0 auto;
    min-height: 36px;
    padding: var(--space-1) var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .session-status {
    display: flex;
    flex-direction: column;
    gap: 2px;
    flex: 1 1 auto;
    min-width: 0;
  }

  .run-error {
    margin: 0;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
    color: var(--color-danger-text);
  }

  .run-state {
    display: flex;
    align-items: center;
    gap: var(--space-2);
    flex: 1 1 auto;
    min-width: 0;
  }

  .status-text {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .results-pane > :global(.streaming-unavailable) {
    flex: 0 0 auto;
    margin: var(--space-2) var(--space-3) 0;
  }

  .context-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    gap: var(--space-1) var(--space-4);
    flex: 0 0 auto;
    padding: var(--space-2) var(--space-3) 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .ctx {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    min-width: 0;
  }

  .ctx-key {
    color: var(--color-text-tertiary);
  }

  .ctx-id {
    max-width: 200px;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .ctx-link {
    padding: 0;
    border: 0;
    background: none;
    font: inherit;
    color: inherit;
    cursor: pointer;
  }

  .ctx-link:focus-visible {
    outline: 2px solid var(--color-focus-ring);
    outline-offset: 2px;
    border-radius: var(--radius-sm);
  }

  .issue-list {
    display: grid;
    gap: 2px;
    flex: 0 0 auto;
    margin: var(--space-2) var(--space-3) 0;
    padding: var(--space-2) var(--space-2) var(--space-2) var(--space-6);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .diagnostics {
    display: grid;
    gap: 2px;
    flex: 0 0 auto;
    margin: var(--space-2) var(--space-3) 0;
    font-size: var(--text-xs);
    color: var(--color-text-secondary);
  }

  .diagnostic {
    display: flex;
    align-items: baseline;
    gap: var(--space-2);
    min-width: 0;
  }

  .diagnostic-message {
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .path-link {
    padding: 0;
    border: 0;
    background: none;
    color: var(--color-accent-text);
    cursor: pointer;
  }

  .path-link:hover:not(:disabled) {
    text-decoration: underline;
  }

  .path-link:disabled {
    opacity: 0.5;
    cursor: not-allowed;
  }

  .results-tabs {
    display: flex;
    flex: 0 0 auto;
    height: 36px;
    margin-top: var(--space-2);
    padding: 0 var(--space-3);
    border-bottom: 1px solid var(--color-border-subtle);
  }

  .results-body {
    flex: 1 1 auto;
    min-height: 0;
    overflow: auto;
    padding: var(--space-3);
  }

  .quality {
    margin-top: var(--space-3);
  }

  .muted-line {
    margin: 0;
    font-size: var(--text-xs);
    color: var(--color-text-tertiary);
  }

  .process-error {
    display: grid;
    gap: var(--space-2);
    padding: var(--space-3);
    border: 1px solid var(--color-danger-border);
    border-radius: var(--radius-sm);
    background: var(--color-danger-bg);
  }

  .process-error p {
    margin: 0;
    color: var(--color-danger-text);
    font-size: var(--text-xs);
  }

  .process-result {
    display: grid;
    gap: var(--space-3);
  }

  .process-head {
    display: flex;
    align-items: center;
    justify-content: space-between;
    gap: var(--space-2);
  }

  .mono {
    font-family: var(--font-mono);
    font-size: var(--text-mono);
  }
</style>
