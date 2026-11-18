import { describe, expect, it } from 'vitest';
import { findMatches, hexDump, highlightSegments, indentMarkup, prettyBody } from './format';

describe('format', () => {
  it('pretty-prints JSON bodies only when they parse', () => {
    expect(prettyBody('{"a":1}', 'application/json')).toBe('{\n  "a": 1\n}');
    expect(prettyBody('{"a":', 'application/json')).toBe('{"a":');
  });

  it('re-indents markup and leaves void elements flat', () => {
    expect(indentMarkup('<div><p>hi</p><br><img src="x"></div>')).toBe(
      ['<div>', '  <p>', '    hi', '  </p>', '  <br>', '  <img src="x">', '</div>'].join('\n'),
    );
  });

  it('renders a hex dump with offsets, padded bytes and ascii', () => {
    const dump = hexDump('Hi\n', 4096);
    expect(dump.truncated).toBe(false);
    expect(dump.text.split('\n')[0]).toBe(`00000000  ${'48 69 0a'.padEnd(47)}  Hi.`);
  });

  it('truncates the hex dump at the byte limit', () => {
    const dump = hexDump('A'.repeat(100), 16);
    expect(dump.truncated).toBe(true);
    expect(dump.text.split('\n')).toHaveLength(1);
  });

  it('counts matches case-insensitively and caps the stored offsets', () => {
    const matches = findMatches('Host: a.test\r\nhost: b.test', 'HOST');
    expect(matches.total).toBe(2);
    expect(matches.indices).toEqual([0, 14]);
    expect(findMatches('aaaa', 'a', 2).indices).toHaveLength(2);
    expect(findMatches('abc', '')).toEqual({ total: 0, indices: [] });
  });

  it('marks the current match in the highlighted segments', () => {
    const parts = highlightSegments('one two one', [0, 8], 1, 3);
    expect(parts.map((p) => [p.text, p.match, p.current])).toEqual([
      ['one', true, false],
      [' two ', false, false],
      ['one', true, true],
    ]);
  });

  it('keeps the current match highlighted beyond the cap', () => {
    const text = 'x a x a x a x a';
    const indices = [2, 6, 10, 14];
    const parts = highlightSegments(text, indices, 3, 1, 2);
    expect(parts.filter((p) => p.current).map((p) => p.text)).toEqual(['a']);
    expect(parts.map((p) => p.text).join('')).toBe(text);
  });
});
