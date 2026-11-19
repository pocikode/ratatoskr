import { syntaxTree } from '@codemirror/language';
import { EditorState } from '@codemirror/state';
import { describe, expect, it } from 'vitest';
import { httpLanguage } from './http';

function headerCount(doc: string): number {
  const state = EditorState.create({
    doc,
    extensions: [EditorState.lineSeparator.of('\n'), httpLanguage],
  });
  let n = 0;
  syntaxTree(state).iterate({ enter: (node) => void (node.name === 'propertyName' && n++) });
  return n;
}

describe('httpLanguage body', () => {
  it('does not highlight a body line as a header, LF or CRLF', () => {
    expect(headerCount('GET / HTTP/1.1\nHost: a\n\nname: body')).toBe(1);
    expect(headerCount('GET / HTTP/1.1\r\nHost: a\r\n\r\nname: body')).toBe(1);
  });
});
