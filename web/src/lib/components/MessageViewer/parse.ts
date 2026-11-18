export interface Header {
  name: string;
  value: string;
}

export interface ParsedMessage {
  /** Request line or status line. */
  startLine: string;
  headers: Header[];
  body: string;
  contentType: string;
}

function headerValue(headers: readonly Header[], name: string): string | undefined {
  return headers.find((h) => h.name.toLowerCase() === name)?.value;
}

/** Splits a raw HTTP message on the first blank line; header folding is unfolded. */
export function parseMessage(raw: string): ParsedMessage {
  const separator = raw.includes('\r\n\r\n') ? '\r\n\r\n' : '\n\n';
  const cut = raw.indexOf(separator);
  const head = cut < 0 ? raw : raw.slice(0, cut);
  const body = cut < 0 ? '' : raw.slice(cut + separator.length);

  const [startLine = '', ...lines] = head.replace(/\r\n/g, '\n').split('\n');
  const headers: Header[] = [];
  for (const line of lines) {
    const last = headers.at(-1);
    if (/^[ \t]/.test(line) && last) {
      last.value += ` ${line.trim()}`;
      continue;
    }
    const colon = line.indexOf(':');
    if (colon > 0)
      headers.push({ name: line.slice(0, colon), value: line.slice(colon + 1).trim() });
  }
  return { startLine, headers, body, contentType: headerValue(headers, 'content-type') ?? '' };
}

export interface Param {
  name: string;
  value: string;
}

function searchParams(encoded: string): Param[] {
  const params = new URLSearchParams(encoded);
  return [...params].map(([name, value]) => ({ name, value }));
}

/** Query-string parameters of a request line (`GET /a?b=c HTTP/1.1`). */
export function queryParams(startLine: string): Param[] {
  const target = /^\S+\s+(\S+)/.exec(startLine)?.[1];
  const query = target?.split('?')[1];
  return query ? searchParams(query) : [];
}

/** `application/x-www-form-urlencoded` body parameters. */
export function formParams(parsed: ParsedMessage): Param[] {
  return parsed.contentType.toLowerCase().includes('x-www-form-urlencoded')
    ? searchParams(parsed.body)
    : [];
}
