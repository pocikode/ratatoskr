import { history, undo, redo } from '@codemirror/commands';
import { EditorState, type ChangeSpec } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { encodeDoc, lineEndings, parseLineEndings } from './lineEndings';

function make(text: string) {
  const { doc, effect } = parseLineEndings(text);
  return EditorState.create({ doc, extensions: [lineEndings, history()] }).update({
    effects: effect,
  }).state;
}
const edit = (s: EditorState, changes: ChangeSpec) =>
  s.update({ changes, userEvent: 'input' }).state;
const run = (cmd: typeof undo, s: EditorState) => {
  let next = s;
  cmd({ state: s, dispatch: (tr) => (next = tr.state) });
  return next;
};

describe('lineEndings', () => {
  it.each([
    'GET / HTTP/1.1\r\nHost: a\r\n\r\n',
    'a\nb\n\nc',
    'a\rb\r\rc',
    'a\r\nb\nc\rd',
    'x\r\n\r\n\r\n\r\n',
    '',
    '\r\n',
    '\r',
  ])('round-trips %j', (text) => {
    expect(encodeDoc(make(text))).toBe(text);
  });

  it('a new break is CRLF, untouched ones keep their bytes', () => {
    const s = edit(make('a\nb\rc'), { from: 3, insert: '\nX' });
    expect(encodeDoc(s)).toBe('a\nb\r\nX\rc');
  });

  it('insert before and after a lone-LF break keeps it', () => {
    expect(encodeDoc(edit(make('a\nb'), { from: 1, insert: 'x' }))).toBe('ax\nb');
    expect(encodeDoc(edit(make('a\nb'), { from: 2, insert: 'x' }))).toBe('a\nxb');
    expect(encodeDoc(edit(make('a\nb'), { from: 1, insert: '\n' }))).toBe('a\r\n\nb');
  });

  it('replacing a break with a new one makes it CRLF', () => {
    expect(encodeDoc(edit(make('a\nb'), { from: 1, to: 2, insert: '\n' }))).toBe('a\r\nb');
    expect(encodeDoc(edit(make('a\rb'), { from: 1, to: 2 }))).toBe('ab');
  });

  it.each(['\n', '\r'])('undo/redo of deleting a lone %j break restores the bytes', (sep) => {
    const text = `a${sep}b`;
    const deleted = edit(make(text), { from: 1, to: 2 });
    expect(encodeDoc(deleted)).toBe('ab');
    const undone = run(undo, deleted);
    expect(encodeDoc(undone)).toBe(text);
    expect(encodeDoc(run(redo, undone))).toBe('ab');
    expect(encodeDoc(run(undo, run(redo, undone)))).toBe(text);
  });

  it('undo of a delete spanning several breaks restores all of them', () => {
    const text = 'a\nb\rc\r\nd';
    const s = edit(make(text), { from: 0, to: text.length - 2 });
    expect(encodeDoc(run(undo, s))).toBe(text);
  });
});
