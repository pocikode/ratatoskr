import { describe, expect, it } from 'vitest';
import { byteToDoc, docToByte, syntaxError } from './httpql';

describe('syntaxError', () => {
  it('accepts complete queries and the empty query', () => {
    expect(syntaxError('')).toBeNull();
    expect(
      syntaxError('req.host.cont:"api" AND (resp.code.gte:400 OR resp.code.eq:500)'),
    ).toBeNull();
    expect(syntaxError('// comment\npreset:auth')).toBeNull();
  });

  it('reports an unterminated string with its span', () => {
    expect(syntaxError('req.host.eq:"api')).toEqual({
      message: 'Unterminated string',
      from: 12,
      to: 16,
    });
  });

  it('reports an unmatched closing paren', () => {
    expect(syntaxError('req.host.eq:"a")')).toEqual({
      message: 'Unmatched ")"',
      from: 15,
      to: 16,
    });
  });

  it('points at the opening paren when a group is not closed', () => {
    expect(syntaxError('(req.host.eq:"a"')).toEqual({
      message: 'Missing ")"',
      from: 0,
      to: 16,
    });
  });

  it('ignores brackets and quotes inside strings and comments', () => {
    expect(syntaxError('req.raw.cont:"((" // )')).toBeNull();
    expect(syntaxError('/* ( " */ req.host.eq:"a"')).toBeNull();
    expect(syntaxError('req.raw.cont:"a\\"b"')).toBeNull();
  });

  it('reports an unterminated block comment', () => {
    expect(syntaxError('req.host.eq:"a" /* todo')).toEqual({
      message: 'Unterminated comment',
      from: 16,
      to: 23,
    });
  });
});

describe('UTF-8 byte offsets', () => {
  it('counts bytes, not UTF-16 units', () => {
    expect(docToByte('', 0)).toBe(0);
    expect(docToByte('abc', 3)).toBe(3);
    expect(docToByte('é', 1)).toBe(2);
    expect(docToByte('a😀b', 3)).toBe(5);
  });

  it('round-trips every code-point boundary', () => {
    const text = 'a😀bé"';
    for (const pos of [0, 1, 3, 4, 5, 6]) {
      expect(byteToDoc(text, docToByte(text, pos))).toBe(pos);
    }
  });

  it('clamps out-of-range offsets', () => {
    expect(byteToDoc('abc', -5)).toBe(0);
    expect(byteToDoc('abc', 99)).toBe(3);
    expect(docToByte('abc', 99)).toBe(3);
  });

  it('maps a server span after multi-byte text', () => {
    // `"é"` is four bytes but three editor positions, so byte 6 is the start of `host`.
    expect(byteToDoc('"é" host', 6)).toBe(5);
    expect(byteToDoc('"é" host', 5)).toBe(4);
    expect(byteToDoc('"é"', 2)).toBe(1); // inside a multi-byte character: stop at its start
  });
});
