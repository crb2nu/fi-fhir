<script lang="ts">
  import { onMount, onDestroy, createEventDispatcher } from 'svelte';
  import { EditorView, keymap, placeholder as cmPlaceholder, lineNumbers as cmLineNumbers } from '@codemirror/view';
  import { EditorState, Compartment } from '@codemirror/state';
  import { defaultKeymap, history, historyKeymap } from '@codemirror/commands';
  import { bracketMatching, indentOnInput } from '@codemirror/language';
  import { linter, type Diagnostic } from '@codemirror/lint';
  import { closeBrackets, closeBracketsKeymap } from '@codemirror/autocomplete';
  import { json } from '@codemirror/lang-json';
  import { yaml } from '@codemirror/lang-yaml';
  import { cel } from './lang-cel';
  import { hl7v2 } from './lang-hl7v2';
  import { toCM6Diagnostics } from './diagnostics';
  import { createThemeExtension } from './cmTheme';
  import type { EditorLanguage, EditorDiagnostic } from './types';

  export let language: EditorLanguage = 'text';
  export let value: string = '';
  export let readOnly: boolean = false;
  export let lineNumbers: boolean = true;
  export let diagnostics: EditorDiagnostic[] = [];
  export let placeholder: string = '';
  export let height: string = '100%';

  const dispatch = createEventDispatcher<{ change: string }>();

  let container: HTMLDivElement;
  let view: EditorView | undefined;

  const languageCompartment = new Compartment();
  const readOnlyCompartment = new Compartment();
  const lineNumbersCompartment = new Compartment();
  const lintCompartment = new Compartment();

  let updatingFromProp = false;

  function getLanguageExtension(lang: EditorLanguage) {
    switch (lang) {
      case 'cel':
        return cel();
      case 'hl7v2':
        return hl7v2();
      case 'json':
        return json();
      case 'yaml':
        return yaml();
      case 'text':
      default:
        return [];
    }
  }

  function createLintExtension(diags: EditorDiagnostic[]) {
    if (diags.length === 0) return [];
    const converted: Diagnostic[] = toCM6Diagnostics(diags);
    return linter(() => converted);
  }

  onMount(() => {
    const themeExt = createThemeExtension();

    const startState = EditorState.create({
      doc: value,
      extensions: [
        themeExt.extension,
        languageCompartment.of(getLanguageExtension(language)),
        readOnlyCompartment.of(EditorState.readOnly.of(readOnly)),
        lineNumbersCompartment.of(lineNumbers ? cmLineNumbers() : []),
        lintCompartment.of(createLintExtension(diagnostics)),
        history(),
        bracketMatching(),
        closeBrackets(),
        indentOnInput(),
        placeholder ? cmPlaceholder(placeholder) : [],
        keymap.of([
          ...closeBracketsKeymap,
          ...defaultKeymap,
          ...historyKeymap
        ]),
        EditorView.updateListener.of((update) => {
          if (update.docChanged && !updatingFromProp) {
            const newValue = update.state.doc.toString();
            dispatch('change', newValue);
          }
        })
      ]
    });

    view = new EditorView({
      state: startState,
      parent: container
    });

    // Start theme observer
    const ext = themeExt as unknown as { startObserving?: (v: EditorView) => void };
    if (ext.startObserving) {
      ext.startObserving(view);
    }

    return () => {
      themeExt.cleanup();
    };
  });

  onDestroy(() => {
    view?.destroy();
    view = undefined;
  });

  // React to prop changes
  $: if (view) {
    const currentDoc = view.state.doc.toString();
    if (value !== currentDoc) {
      updatingFromProp = true;
      view.dispatch({
        changes: { from: 0, to: view.state.doc.length, insert: value }
      });
      updatingFromProp = false;
    }
  }

  $: if (view) {
    view.dispatch({
      effects: languageCompartment.reconfigure(getLanguageExtension(language))
    });
  }

  $: if (view) {
    view.dispatch({
      effects: readOnlyCompartment.reconfigure(EditorState.readOnly.of(readOnly))
    });
  }

  $: if (view) {
    view.dispatch({
      effects: lineNumbersCompartment.reconfigure(lineNumbers ? cmLineNumbers() : [])
    });
  }

  $: if (view) {
    view.dispatch({
      effects: lintCompartment.reconfigure(createLintExtension(diagnostics))
    });
  }
</script>

<div
  class="code-editor"
  bind:this={container}
  style:height={height}
  data-testid="code-editor"
></div>

<style>
  .code-editor {
    width: 100%;
    overflow: hidden;
  }

  .code-editor :global(.cm-editor) {
    height: 100%;
  }

  .code-editor :global(.cm-scroller) {
    font-family: var(--font-mono);
    line-height: 1.5;
  }
</style>
