<script lang="ts">
  import {
    defaultKeymap,
    deleteCharBackward,
    deleteCharForward,
    history,
    historyKeymap,
  } from '@codemirror/commands';
  import { syntaxHighlighting, HighlightStyle } from '@codemirror/language';
  import { search, searchKeymap } from '@codemirror/search';
  import { Annotation, Compartment, EditorState } from '@codemirror/state';
  import { EditorView, highlightSpecialChars, keymap } from '@codemirror/view';
  import { tags } from '@lezer/highlight';
  import { untrack } from 'svelte';
  import { bytesEqual, decodeBytes, decodeLossy, encodeText } from './bytes';
  import { httpLanguage, variableDecorations, type EditorVariable } from './http';

  // Marks transactions that push the `value` prop in, so they are not echoed back through onchange.
  const external = Annotation.define<boolean>();

  // The default set flags CR as a control character; here it is just a line ending.
  // eslint-disable-next-line no-control-regex -- T-104: control chars minus CR/LF/tab on purpose
  const SPECIAL = /[\x00-\x08\x0b\x0c\x0e-\x1f\x7f-\x9f]/g;

  interface Props {
    value: Uint8Array;
    onchange?: (bytes: Uint8Array) => void;
    readonly?: boolean;
    variables?: Record<string, EditorVariable>;
    label?: string;
  }

  let {
    value,
    onchange,
    readonly = false,
    variables = {},
    label = 'Raw HTTP message',
  }: Props = $props();

  let host = $state<HTMLDivElement | null>(null);
  let view = $state<EditorView | null>(null);
  let wrap = $state(false);

  // Invalid UTF-8 cannot be edited without changing bytes the user did not touch.
  const text = $derived(decodeBytes(value));
  const invalid = $derived(text === null);
  const locked = $derived(readonly || invalid);

  const lockCompartment = new Compartment();
  const wrapCompartment = new Compartment();
  const historyCompartment = new Compartment();

  const highlight = HighlightStyle.define([
    { tag: tags.keyword, class: 'cm-http-method' },
    { tag: tags.atom, class: 'cm-http-version' },
    { tag: tags.propertyName, class: 'cm-http-header' },
    { tag: tags.string, class: 'cm-http-value' },
    { tag: tags.punctuation, class: 'cm-http-punct' },
  ]);

  const lockExtensions = (on: boolean) => [
    EditorState.readOnly.of(on),
    EditorView.editable.of(!on),
  ];

  // Enter inserts CRLF: HTTP line endings, whatever the document's own "\n" separator is.
  function enter(editor: EditorView): boolean {
    if (editor.state.readOnly) return true;
    editor.dispatch(editor.state.replaceSelection('\r\n'), {
      scrollIntoView: true,
      userEvent: 'input',
    });
    return true;
  }

  // Backspace/Delete treat CRLF as one boundary so no hidden CR is left behind.
  function eraser(forward: boolean) {
    return (editor: EditorView): boolean => {
      const { main } = editor.state.selection;
      if (editor.state.readOnly || !main.empty) return false;
      const from = forward ? main.head : main.head - 2;
      if (from < 0 || editor.state.sliceDoc(from, from + 2) !== '\r\n') {
        return (forward ? deleteCharForward : deleteCharBackward)(editor);
      }
      editor.dispatch({
        changes: { from, to: from + 2 },
        userEvent: 'delete',
        scrollIntoView: true,
      });
      return true;
    };
  }

  $effect(() => {
    const element = host;
    if (!element) return;
    const editor = new EditorView({
      parent: element,
      state: untrack(() =>
        EditorState.create({
          doc: text ?? decodeLossy(value),
          extensions: [
            // CR stays a document character, so no line break is ever normalised.
            EditorState.lineSeparator.of('\n'),
            httpLanguage,
            syntaxHighlighting(highlight),
            highlightSpecialChars({
              specialChars: SPECIAL,
            }),
            historyCompartment.of(history()),
            search({ top: true }),
            variableDecorations(() => variables),
            lockCompartment.of(lockExtensions(locked)),
            wrapCompartment.of(wrap ? EditorView.lineWrapping : []),
            keymap.of([
              { key: 'Enter', run: enter },
              { key: 'Shift-Enter', run: enter },
              { key: 'Backspace', run: eraser(false) },
              { key: 'Delete', run: eraser(true) },
              ...searchKeymap,
              ...historyKeymap,
              ...defaultKeymap,
            ]),
            EditorView.updateListener.of((update) => {
              if (!update.docChanged || update.state.readOnly) return;
              if (update.transactions.some((t) => t.annotation(external))) return;
              onchange?.(encodeText(update.state.doc.toString()));
            }),
            EditorView.contentAttributes.of({
              'aria-label': label,
              'aria-multiline': 'true',
              autocapitalize: 'off',
              spellcheck: 'false',
            }),
          ],
        }),
      ),
    });
    view = editor;
    return () => {
      editor.destroy();
      view = null;
    };
  });

  // CodeMirror is outside Svelte: push external bytes, lock state, wrap and variables in.
  $effect(() => {
    const editor = view;
    const next = text ?? decodeLossy(value);
    if (!editor) return;
    if (text !== null && bytesEqual(encodeText(editor.state.doc.toString()), value)) return;
    if (text === null && editor.state.doc.toString() === next) return;
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: next },
      annotations: external.of(true),
    });
    // A replaced message must not be reachable through undo: drop and re-add the history field.
    editor.dispatch({ effects: historyCompartment.reconfigure([]) });
    editor.dispatch({ effects: historyCompartment.reconfigure(history()) });
  });

  $effect(() => {
    view?.dispatch({ effects: lockCompartment.reconfigure(lockExtensions(locked)) });
  });

  $effect(() => {
    view?.dispatch({ effects: wrapCompartment.reconfigure(wrap ? EditorView.lineWrapping : []) });
  });

  $effect(() => {
    void variables;
    view?.dispatch({}); // an empty transaction makes the decoration plugin re-read the variables
  });
</script>

<div class="rawed flex h-full min-h-0 flex-col">
  <div class="flex items-center gap-2 border-b border-line px-2 py-1 text-xs text-muted">
    {#if invalid}
      <span role="status" class="text-warn"
        >Not valid UTF-8: shown read-only, bytes are unchanged.</span
      >
    {/if}
    <button
      type="button"
      class="ml-auto rounded-token px-1.5 py-0.5 hover:bg-hover"
      aria-pressed={wrap}
      onclick={() => (wrap = !wrap)}>Wrap</button
    >
  </div>
  <div bind:this={host} class="min-h-0 flex-1 overflow-hidden"></div>
</div>

<style>
  .rawed :global(.cm-editor) {
    height: 100%;
    background: var(--bg);
    color: var(--text);
    font-family: var(--font-code);
    font-size: var(--text-base);
  }
  .rawed :global(.cm-editor.cm-focused) {
    outline: 1px solid var(--focus-ring);
  }
  .rawed :global(.cm-scroller) {
    font-family: inherit;
    line-height: var(--leading-dense);
  }
  .rawed :global(.cm-http-method) {
    color: var(--accent);
  }
  .rawed :global(.cm-http-header) {
    color: var(--accent-hover);
  }
  .rawed :global(.cm-http-version),
  .rawed :global(.cm-http-punct) {
    color: var(--text-muted);
  }
  .rawed :global(.cm-var-known) {
    background: var(--accent-soft);
    border-radius: 3px;
  }
  .rawed :global(.cm-var-unknown) {
    background: color-mix(in srgb, var(--warn) 20%, transparent);
    border-radius: 3px;
  }
  .rawed :global(.cm-panels) {
    background: var(--bg-elevated);
    color: var(--text);
    border-color: var(--border);
  }
</style>
