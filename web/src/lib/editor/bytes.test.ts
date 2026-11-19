import { describe, expect, it } from 'vitest';
import { bytesEqual, decodeBytes, encodeText } from './bytes';

const enc = (s: string) => new TextEncoder().encode(s);

describe('bytes round trip', () => {
  it.each([
    ['CRLF', 'GET / HTTP/1.1\r\nHost: a\r\n\r\n'],
    ['lone LF', 'GET / HTTP/1.1\nHost: a\n\nbody'],
    ['lone CR', 'a\rb\r\rc'],
    ['trailing CRLFCRLF', 'POST / HTTP/1.1\r\n\r\n\r\n\r\n'],
    ['empty', ''],
    ['unicode and BOM', '﻿héllo \u{1F600}'],
  ])('%s is byte-identical', (_, s) => {
    const bytes = enc(s);
    expect(bytesEqual(encodeText(decodeBytes(bytes)!), bytes)).toBe(true);
  });

  it('rejects invalid UTF-8', () => {
    expect(decodeBytes(new Uint8Array([0x47, 0xff, 0xfe]))).toBeNull();
  });
});
