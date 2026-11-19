<script lang="ts">
  import { defaultKeymap, history, historyKeymap, insertNewline } from '@codemirror/commands';
  import { syntaxHighlighting, HighlightStyle } from '@codemirror/language';
  import { search, searchKeymap } from '@codemirror/search';
  import { Annotation, Compartment, EditorState } from '@codemirror/state';
  import { EditorView, highlightSpecialChars, keymap } from '@codemirror/view';
  import { tags } from '@lezer/highlight';
  import { untrack } from 'svelte';
  import { bytesEqual, decodeBytes, decodeLossy, encodeText } from './bytes';
  import { encodeDoc, lineEndings, parseLineEndings } from './lineEndings';
  import { httpLanguage, variableDecorations, type EditorVariable } from './http';

  // Marks transactions that push the `value` prop in, so they are not echoed back through onchange.
  const external = Annotation.define<boolean>();

  interface Props {
    value: Uint8Array;
    onchange?: (bytes: Uint8Array) => void;
    readonly?: boolean;
    variables?: Record<string, EditorVariable>;
    /** Message identity: changing it resets the editor (and its undo history) even if the bytes are equal. */
    identity?: unknown;
    label?: string;
  }

  let {
    value,
    onchange,
    readonly = false,
    variables = {},
    identity,
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

  let loadedIdentity: unknown;

  $effect(() => {
    const element = host;
    if (!element) return;
    loadedIdentity = untrack(() => identity);
    const initial = untrack(() => parseLineEndings(text ?? decodeLossy(value)));
    // untrack: plugin constructors read `variables`, which must not rebuild the editor.
    const editor = untrack(
      () =>
        new EditorView({
          parent: element,
          state: EditorState.create({
            doc: initial.doc,
            extensions: [
              lineEndings,
              httpLanguage,
              syntaxHighlighting(highlight),
              highlightSpecialChars(),
              historyCompartment.of(history()),
              search({ top: true }),
              variableDecorations(() => variables),
              lockCompartment.of(lockExtensions(locked)),
              wrapCompartment.of(wrap ? EditorView.lineWrapping : []),
              keymap.of([
                // Plain newline: no auto-indent, which would add or strip bytes. New breaks are CRLF.
                { key: 'Enter', run: insertNewline, shift: insertNewline },
                ...searchKeymap,
                ...historyKeymap,
                // Mod-Enter is the app's Send/Forward shortcut; the editor must not consume it.
                ...defaultKeymap.filter((b) => b.key !== 'Mod-Enter'),
              ]),
              EditorView.updateListener.of((update) => {
                if (!update.docChanged || update.state.readOnly) return;
                if (update.transactions.some((t) => t.annotation(external))) return;
                onchange?.(encodeText(encodeDoc(update.state)));
              }),
              EditorView.contentAttributes.of({
                'aria-label': label,
                'aria-multiline': 'true',
                autocapitalize: 'off',
                spellcheck: 'false',
              }),
            ],
          }).update({ effects: initial.effect }).state,
        }),
    );
    view = editor;
    return () => {
      editor.destroy();
      view = null;
    };
  });

  // CodeMirror is outside Svelte: push external bytes, lock state, wrap and variables in.
  $effect(() => {
    const editor = view;
    if (!editor) return;
    const next = parseLineEndings(text ?? decodeLossy(value));
    const swapped = identity !== loadedIdentity;
    loadedIdentity = identity;
    // Same identity and same bytes: the parent only echoed our own edit.
    const same =
      text !== null
        ? bytesEqual(encodeText(encodeDoc(editor.state)), value)
        : editor.state.doc.toString() === next.doc;
    if (same && !swapped) return;
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: next.doc },
      effects: next.effect,
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
