<script lang="ts">
  import { autocompletion, completionKeymap, completionStatus } from '@codemirror/autocomplete';
  import { defaultKeymap, history as cmHistory, historyKeymap } from '@codemirror/commands';
  import { syntaxHighlighting } from '@codemirror/language';
  import { EditorState, StateEffect, StateField } from '@codemirror/state';
  import { Decoration, EditorView, keymap, placeholder as cmPlaceholder } from '@codemirror/view';
  import type { DecorationSet } from '@codemirror/view';
  import { untrack, type Snippet } from 'svelte';
  import { completionSource, type CompletionFetcher } from '$lib/query/completions';
  import { httpqlHighlight, httpqlLanguage, type QueryError } from '$lib/query/httpql';

  interface Props {
    /** Query text; bindable, so commands can set a filter (e.g. "filter by this host"). */
    value: string;
    label: string;
    placeholder?: string;
    /** Squiggle and message for a query the server or the local check rejected. */
    error?: QueryError | null;
    /** Applied queries, oldest first: ↑/↓ walk them (docs/02 §2). */
    history?: readonly string[];
    onsubmit?: (value: string) => void;
    oncomplete?: CompletionFetcher;
    leading?: Snippet;
    trailing?: Snippet;
  }

  let {
    value = $bindable(''),
    label,
    placeholder = 'req.host.cont:"api" AND resp.code.gte:400',
    error = null,
    history = [],
    onsubmit,
    oncomplete,
    leading,
    trailing,
  }: Props = $props();

  const setError = StateEffect.define<QueryError | null>();
  const errorField = StateField.define<DecorationSet>({
    create: () => Decoration.none,
    update(decorations, tr) {
      let next = decorations.map(tr.changes);
      for (const effect of tr.effects) {
        if (!effect.is(setError)) continue;
        const { doc } = tr.state;
        const span = effect.value;
        next =
          !span || doc.length === 0
            ? Decoration.none
            : Decoration.set([
                Decoration.mark({
                  class: 'cm-httpql-error',
                  attributes: { title: span.message },
                }).range(
                  Math.min(span.from, doc.length - 1),
                  Math.min(Math.max(span.to, 1), doc.length),
                ),
              ]);
      }
      return next;
    },
    provide: (field) => EditorView.decorations.from(field),
  });

  // Single line: a pasted newline becomes a space instead of growing the bar.
  const singleLine = EditorState.transactionFilter.of((tr) => {
    if (!tr.docChanged || tr.newDoc.lines === 1) return tr;
    const text = tr.newDoc.toString().replace(/\s*\n\s*/g, ' ');
    return {
      changes: { from: 0, to: tr.newDoc.length, insert: text },
      selection: { anchor: Math.min(tr.newSelection.main.head, text.length) },
    };
  });

  let host = $state<HTMLDivElement | null>(null);
  let view = $state<EditorView | null>(null);
  let historyStep = -1;
  let draft = '';

  function replaceDoc(editor: EditorView, text: string) {
    editor.dispatch({
      changes: { from: 0, to: editor.state.doc.length, insert: text },
      selection: { anchor: text.length },
    });
  }

  function submit(editor: EditorView): boolean {
    if (completionStatus(editor.state) !== null) return false; // the popup owns Enter
    onsubmit?.(editor.state.doc.toString());
    return true;
  }

  function step(delta: -1 | 1, editor: EditorView): boolean {
    if (completionStatus(editor.state) !== null || history.length === 0) return false;
    if (delta < 0) {
      if (historyStep < 0) {
        draft = editor.state.doc.toString();
        historyStep = history.length - 1;
      } else {
        historyStep = Math.max(0, historyStep - 1);
      }
      replaceDoc(editor, history[historyStep] ?? '');
      return true;
    }
    if (historyStep < 0) return false;
    historyStep += 1;
    if (historyStep >= history.length) {
      historyStep = -1;
      replaceDoc(editor, draft);
    } else {
      replaceDoc(editor, history[historyStep] ?? '');
    }
    return true;
  }

  $effect(() => {
    const element = host;
    if (!element) return;
    const source = completionSource(
      (text, cursor) => oncomplete?.(text, cursor) ?? Promise.resolve(null),
    );
    // Created once: untracked, so a prop change never rebuilds the editor (the sync
    // effects below push text and errors in).
    const editor = new EditorView({
      parent: element,
      state: untrack(() =>
        EditorState.create({
          doc: value,
          extensions: [
            httpqlLanguage,
            syntaxHighlighting(httpqlHighlight),
            cmPlaceholder(placeholder),
            cmHistory(),
            autocompletion({ override: [source], activateOnTyping: true, icons: false }),
            errorField,
            singleLine,
            keymap.of([
              { key: 'Enter', run: submit },
              { key: 'ArrowUp', run: (v) => step(-1, v) },
              { key: 'ArrowDown', run: (v) => step(1, v) },
              ...completionKeymap,
              ...historyKeymap,
              ...defaultKeymap,
            ]),
            // Page state is the source of truth; the editor only reports document changes.
            EditorView.updateListener.of((update) => {
              if (update.docChanged) value = update.state.doc.toString();
            }),
            EditorView.contentAttributes.of({
              'aria-label': label,
              'aria-autocomplete': 'list',
              autocomplete: 'off',
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

  // CodeMirror is outside Svelte: push external text and errors into the document.
  $effect(() => {
    const editor = view;
    const text = value;
    if (!editor) return;
    if (editor.state.doc.toString() !== text) replaceDoc(editor, text);
  });

  $effect(() => {
    const editor = view;
    const span = error;
    if (!editor) return;
    editor.dispatch({ effects: setError.of(span) });
    editor.contentDOM.setAttribute('aria-invalid', span ? 'true' : 'false');
  });
</script>

<div class="flex min-w-0 flex-col gap-1">
  <div
    class="flex min-w-0 items-center gap-2 rounded-token border border-line bg-elevated px-2
      focus-within:border-accent"
  >
    {#if leading}
      {@render leading()}
    {/if}
    <div bind:this={host} class="querybar min-w-0 flex-1"></div>
    {#if trailing}
      {@render trailing()}
    {/if}
  </div>
  {#if error}
    <p role="alert" class="text-xs text-danger">{error.message}</p>
  {/if}
</div>

<style>
  .querybar :global(.cm-editor) {
    background: transparent;
    font-family: var(--font-code);
    font-size: var(--text-base);
    line-height: 1.6;
  }
  .querybar :global(.cm-editor.cm-focused) {
    outline: none;
  }
  .querybar :global(.cm-editor.cm-focused .cm-cursor) {
    border-left-color: var(--accent);
  }
  .querybar :global(.cm-scroller) {
    overflow-x: auto;
    overflow-y: hidden;
  }
  .querybar :global(.cm-content) {
    padding: 2px 0;
    caret-color: var(--accent);
  }
  .querybar :global(.cm-placeholder) {
    color: var(--text-muted);
  }
  .querybar :global(.cm-tooltip) {
    border: 1px solid var(--border);
    border-radius: var(--radius);
    background: var(--bg-elevated);
    box-shadow: var(--shadow-overlay);
  }
  .querybar :global(.cm-tooltip-autocomplete ul li) {
    font-family: var(--font-code);
    font-size: var(--text-base);
    padding: 2px 6px;
  }
  .querybar :global(.cm-tooltip-autocomplete ul li[aria-selected]) {
    background: var(--accent-soft);
    color: var(--text);
  }
  .querybar :global(.cm-completionDetail) {
    color: var(--text-muted);
    font-style: normal;
  }
  .querybar :global(.cm-httpql-namespace) {
    color: var(--accent-hover);
    font-weight: 600;
  }
  .querybar :global(.cm-httpql-field) {
    color: var(--text);
  }
  .querybar :global(.cm-httpql-operator),
  .querybar :global(.cm-httpql-keyword) {
    color: var(--warn);
  }
  .querybar :global(.cm-httpql-string) {
    color: var(--success);
  }
  .querybar :global(.cm-httpql-number) {
    color: var(--accent);
  }
  .querybar :global(.cm-httpql-comment) {
    color: var(--text-muted);
    font-style: italic;
  }
  .querybar :global(.cm-httpql-punctuation) {
    color: var(--text-muted);
  }
  .querybar :global(.cm-httpql-value) {
    color: var(--text);
  }
  .querybar :global(.cm-httpql-invalid),
  .querybar :global(.cm-httpql-error) {
    text-decoration: underline wavy var(--danger);
    text-underline-offset: 2px;
  }
</style>
