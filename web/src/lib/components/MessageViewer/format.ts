import { parseMessage } from './parse';

const VOID_TAGS = /^<(area|base|br|col|embed|hr|img|input|link|meta|param|source|track|wbr)\b/i;

/** Best-effort re-indent for markup. Script/style/pre contents are not special-cased. */
export function indentMarkup(text: string): string {
  const tokens = text
    .replace(/>\s+</g, '><')
    .replace(/(<[^>]+>)/g, '\n$1\n')
    .split('\n')
    .map((t) => t.trim())
    .filter(Boolean);

  const lines: string[] = [];
  let depth = 0;
  for (const token of tokens) {
    if (/^<\//.test(token)) depth = Math.max(0, depth - 1);
    lines.push('  '.repeat(depth) + token);
    const opens = /^<[^!?/][^>]*[^/]?>$/.test(token) && !VOID_TAGS.test(token);
    if (opens && !/^<!--/.test(token)) depth += 1;
  }
  return lines.join('\n');
}

export function prettyBody(body: string, contentType: string): string {
  const trimmed = body.trim();
  if (!trimmed) return body;
  if (contentType.toLowerCase().includes('json') || /^[[{]/.test(trimmed)) {
    try {
      return JSON.stringify(JSON.parse(trimmed), null, 2);
    } catch {
      // fall through: not JSON after all
    }
  }
  if (
    contentType.toLowerCase().includes('html') ||
    contentType.toLowerCase().includes('xml') ||
    trimmed.startsWith('<')
  ) {
    return indentMarkup(trimmed);
  }
  return body;
}

/** Readable form of a message: one header per line, formatted body when we have a formatter. */
export function prettyMessage(raw: string): string {
  const parsed = parseMessage(raw);
  const head =
    parsed.headers.length === 0
      ? parsed.startLine
      : [parsed.startLine, ...parsed.headers.map((h) => `${h.name}: ${h.value}`)].join('\n');
  if (!parsed.body.trim()) return head;
  return `${head}\n\n${prettyBody(parsed.body, parsed.contentType)}`;
}

export interface HexDump {
  text: string;
  /** Body bytes skipped because of the limit. */
  truncated: boolean;
}

export function hexDump(body: string, maxBytes: number): HexDump {
  // Encode the shown prefix only: a 5 MB body re-encoded per "Show more" would freeze the tab.
  const bytes = new TextEncoder().encode(body.slice(0, maxBytes + 1));
  const shown = bytes.subarray(0, maxBytes);
  const lines: string[] = [];
  for (let offset = 0; offset < shown.length; offset += 16) {
    const chunk = shown.subarray(offset, offset + 16);
    const hex = [...chunk].map((b) => b.toString(16).padStart(2, '0')).join(' ');
    const ascii = [...chunk]
      .map((b) => (b >= 0x20 && b < 0x7f ? String.fromCharCode(b) : '.'))
      .join('');
    lines.push(`${offset.toString(16).padStart(8, '0')}  ${hex.padEnd(47)}  ${ascii}`);
  }
  return { text: lines.join('\n'), truncated: bytes.length > maxBytes };
}

const NO_MATCHES = { total: 0, indices: [] as number[] };

/**
 * Case-insensitive count of matches, keeping the first `cap` offsets for highlighting; large
 * bodies would otherwise hold millions of indices.
 */
export function findMatches(
  text: string,
  query: string,
  cap = 1000,
): { total: number; indices: number[] } {
  if (!query) return NO_MATCHES;
  const haystack = text.toLowerCase();
  const needle = query.toLowerCase();
  const indices: number[] = [];
  let total = 0;
  let at = haystack.indexOf(needle);
  while (at >= 0) {
    total += 1;
    if (indices.length < cap) indices.push(at);
    at = haystack.indexOf(needle, at + needle.length);
  }
  return { total, indices };
}

export interface Segment {
  text: string;
  match: boolean;
  current: boolean;
}

/** Splits `text` into plain and `<mark>` segments; capped so huge bodies stay cheap to render. */
export function highlightSegments(
  text: string,
  indices: readonly number[],
  current: number,
  queryLength: number,
  cap = 200,
): Segment[] {
  const usable = indices
    .map((at, i) => ({ at, i }))
    .filter(({ at }) => at < text.length && at + queryLength <= text.length);
  // Window around the current match, so jumping past the cap still marks and scrolls to it.
  const center = usable.findIndex((m) => m.i === current);
  const start =
    center < 0 ? 0 : Math.max(0, Math.min(center - Math.floor(cap / 2), usable.length - cap));
  const inText = usable.slice(start, start + cap);
  if (inText.length === 0) return [{ text, match: false, current: false }];

  const segments: Segment[] = [];
  let cursor = 0;
  for (const { at, i } of inText) {
    if (at > cursor) segments.push({ text: text.slice(cursor, at), match: false, current: false });
    const end = at + queryLength;
    segments.push({ text: text.slice(at, end), match: true, current: i === current });
    cursor = end;
  }
  if (cursor < text.length)
    segments.push({ text: text.slice(cursor), match: false, current: false });
  return segments;
}
