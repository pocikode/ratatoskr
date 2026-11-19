// Lossless line endings for RawEditor (docs/01 F-REP-2, lead escalation on T-104).
// The document uses CodeMirror's default splitting: every CRLF, lone CR or lone LF is one "\n", so stock
// commands are correct by construction. The original terminator of a non-CRLF break lives out of band in a
// RangeSet; a break without a marker is CRLF, which is what Enter, paste and line commands produce.
import { invertedEffects } from '@codemirror/commands';
import {
  type EditorState,
  RangeSet,
  RangeValue,
  StateEffect,
  StateField,
  type Extension,
} from '@codemirror/state';

class Ending extends RangeValue {
  // Stay on the "\n" character when text is inserted right before it.
  startSide = 1;
  constructor(readonly sep: '\n' | '\r') {
    super();
  }
  eq(other: RangeValue): boolean {
    return other instanceof Ending && other.sep === this.sep;
  }
}
const LF = new Ending('\n');
const CR = new Ending('\r');

type Endings = RangeSet<Ending>;
interface Mark {
  pos: number;
  sep: '\n' | '\r';
}

/** Replace every marker (used when a whole message is loaded). Positions are in the new document. */
const load = StateEffect.define<Endings>();
/** Re-add markers (undo of a deletion). Positions are in the document after the transaction. */
const restore = StateEffect.define<Mark[]>({
  map: (marks, mapping) => marks.map((m) => ({ ...m, pos: mapping.mapPos(m.pos, 1) })),
});

const field = StateField.define<Endings>({
  create: () => RangeSet.empty,
  update(set, tr) {
    for (const e of tr.effects) if (e.is(load)) return e.value;
    if (tr.docChanged) {
      // Drop markers inside changed ranges (old coordinates), touching only those lines, then map the rest.
      tr.changes.iterChangedRanges((from, to) => {
        set = set.update({
          filter: (f) => f < from || f >= to,
          filterFrom: from,
          filterTo: to + 1,
        });
      });
      set = set.map(tr.changes);
    }
    const added: { from: number; to: number; value: Ending }[] = [];
    for (const e of tr.effects)
      if (e.is(restore))
        for (const m of e.value)
          added.push({ from: m.pos, to: m.pos + 1, value: m.sep === '\r' ? CR : LF });
    return added.length ? set.update({ add: added, sort: true }) : set;
  },
});

// Undo of an edit that removed markers must bring them back.
const undoRestore = invertedEffects.of((tr) => {
  if (!tr.docChanged || tr.effects.some((e) => e.is(load))) return [];
  const old = tr.startState.field(field);
  const marks: Mark[] = [];
  tr.changes.iterChangedRanges((from, to) =>
    old.between(from, to, (f, _t, v) => {
      if (f >= from && f < to) marks.push({ pos: f, sep: v.sep });
    }),
  );
  return marks.length ? [restore.of(marks)] : [];
});

export const lineEndings: Extension = [field, undoRestore];

/** Document text (breaks as "\n") plus the markers for non-CRLF breaks. */
export function parseLineEndings(text: string): { doc: string; effect: StateEffect<unknown> } {
  const marks: { from: number; to: number; value: Ending }[] = [];
  let doc = '';
  let last = 0;
  for (const m of text.matchAll(/\r\n?|\n/g)) {
    doc += text.slice(last, m.index);
    if (m[0] !== '\r\n')
      marks.push({ from: doc.length, to: doc.length + 1, value: m[0] === '\r' ? CR : LF });
    doc += '\n';
    last = m.index + m[0].length;
  }
  doc += text.slice(last);
  return { doc, effect: load.of(RangeSet.of(marks)) };
}

/** The message bytes' text: every "\n" in the document becomes its recorded terminator, CRLF by default. */
export function encodeDoc(state: EditorState): string {
  const seps = new Map<number, string>();
  state.field(field).between(0, state.doc.length, (from, _to, v) => void seps.set(from, v.sep));
  const text = state.doc.toString();
  let out = '';
  let last = 0;
  for (let i = text.indexOf('\n'); i >= 0; i = text.indexOf('\n', i + 1)) {
    out += text.slice(last, i) + (seps.get(i) ?? '\r\n');
    last = i + 1;
  }
  return out + text.slice(last);
}
