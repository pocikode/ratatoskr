// Bytes <-> text. Plain UTF-8 decode/encode is the identity for any valid input; the editor keeps its
// line breaks lossless through lineEndings.ts.
const decoder = new TextDecoder('utf-8', { fatal: true, ignoreBOM: true });
const lossy = new TextDecoder('utf-8', { ignoreBOM: true });
const encoder = new TextEncoder();

/** The text of `bytes`, or null when they are not valid UTF-8. */
export function decodeBytes(bytes: Uint8Array): string | null {
  try {
    return decoder.decode(bytes);
  } catch {
    return null;
  }
}

/** Display-only text for input that is not valid UTF-8 (U+FFFD replaces bad bytes). */
export const decodeLossy = (bytes: Uint8Array): string => lossy.decode(bytes);

export const encodeText = (text: string): Uint8Array => encoder.encode(text);

export function bytesEqual(a: Uint8Array, b: Uint8Array): boolean {
  return a.length === b.length && a.every((byte, i) => byte === b[i]);
}
