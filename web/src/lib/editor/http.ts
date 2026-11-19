// HTTP message language and {{var}} decorations for RawEditor (docs/01 F-REP-2).
import { StreamLanguage, type StringStream } from '@codemirror/language';
import { RangeSetBuilder } from '@codemirror/state';
import {
  Decoration,
  type DecorationSet,
  EditorView,
  ViewPlugin,
  type ViewUpdate,
} from '@codemirror/view';

export interface EditorVariable {
  value: string;
  secret: boolean;
}

type Phase = 'start' | 'headers' | 'body';
interface HttpState {
  phase: Phase;
}

function token(stream: StringStream, state: HttpState): string | null {
  if (state.phase === 'body') {
    stream.skipToEnd();
    return null;
  }
  if (state.phase === 'headers' && stream.match(/^\r?$/)) {
    state.phase = 'body';
    return null;
  }
  if (stream.sol() && state.phase === 'start') {
    state.phase = 'headers';
    // Request line, or "HTTP/1.1 200 OK" for a response.
    if (stream.match(/^HTTP\/[\d.]+/)) return 'keyword';
    if (stream.match(/^[A-Za-z]+/)) return 'keyword';
    stream.skipToEnd();
    return null;
  }
  if (stream.sol() && stream.match(/^[^:\s][^:]*(?=:)/)) return 'propertyName';
  if (stream.match(/^:/)) return 'punctuation';
  if (stream.match(/^HTTP\/[\d.]+/)) return 'atom';
  stream.skipToEnd();
  return 'string';
}

export const httpLanguage = StreamLanguage.define<HttpState>({
  name: 'http-message',
  token,
  blankLine: (state) => {
    if (state.phase === 'headers') state.phase = 'body';
  },
  startState: () => ({ phase: 'start' }),
  copyState: (state) => ({ ...state }),
});

const VAR = /\{\{\s*([^{}\s]+)\s*\}\}/g;
export const MASK = '••••';

/** Hover text for a variable: its value, the mask when secret, or a note when unknown. */
export function variableTitle(name: string, vars: Record<string, EditorVariable>): string {
  const known = Object.hasOwn(vars, name) ? vars[name] : undefined;
  if (!known) return `${name}: not defined`;
  return `${name} = ${known.secret ? MASK : known.value}`;
}

export function variableDecorations(getVars: () => Record<string, EditorVariable>) {
  const build = (view: EditorView): DecorationSet => {
    const vars = getVars();
    const builder = new RangeSetBuilder<Decoration>();
    for (const { from, to } of view.visibleRanges) {
      const text = view.state.sliceDoc(from, to);
      for (const m of text.matchAll(VAR)) {
        const name = m[1]!;
        const known = Object.hasOwn(vars, name);
        builder.add(
          from + m.index,
          from + m.index + m[0].length,
          Decoration.mark({
            class: known ? 'cm-var cm-var-known' : 'cm-var cm-var-unknown',
            attributes: { title: variableTitle(name, vars) },
          }),
        );
      }
    }
    return builder.finish();
  };
  return ViewPlugin.fromClass(
    class {
      decorations: DecorationSet;
      constructor(view: EditorView) {
        this.decorations = build(view);
      }
      update(update: ViewUpdate) {
        // Variables live outside the document, so every update (incl. a config change) rebuilds.
        this.decorations = build(update.view);
      }
    },
    { decorations: (plugin) => plugin.decorations },
  );
}
