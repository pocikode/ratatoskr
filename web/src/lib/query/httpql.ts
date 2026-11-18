// HTTPQL editor support for the query bar (docs/04-query-language.md, docs/02 §2):
// syntax highlighting, error spans and UTF-8 byte offsets that match `queryCompletions`.
import { HighlightStyle, StreamLanguage, type StringStream } from '@codemirror/language';
import { tags } from '@lezer/highlight';

/** A query error in editor positions (UTF-16 offsets into the document). */
export interface QueryError {
  message: string;
  from: number;
  /** Exclusive. */
  to: number;
}

interface HttpqlState {
  blockComment: boolean;
}

const NAMESPACES = new Set(['req', 'resp', 'row', 'finding']);
const KEYWORDS = new Set(['preset', 'source', 'and', 'or', 'not']);
const FIELDS = new Set([
  'method',
  'host',
  'port',
  'path',
  'query',
  'ext',
  'raw',
  'body',
  'header',
  'len',
  'tls',
  'created_at',
  'color',
  'note',
  'edited',
  'code',
  'roundtrip',
  'id',
  'title',
  'reporter',
  'name',
  'value',
]);
const OPERATORS = new Set([
  'eq',
  'ne',
  'gt',
  'gte',
  'lt',
  'lte',
  'cont',
  'ncont',
  'like',
  'nlike',
  'regex',
  'nregex',
]);

function token(stream: StringStream, state: HttpqlState): string | null {
  if (state.blockComment) {
    while (!stream.eol()) {
      if (stream.match('*/')) {
        state.blockComment = false;
        return 'comment';
      }
      stream.next();
    }
    return 'comment';
  }
  if (stream.eatSpace()) return null;
  if (stream.match('//')) {
    stream.skipToEnd();
    return 'comment';
  }
  if (stream.match('/*')) {
    state.blockComment = true;
    return 'comment';
  }
  if (stream.match(/^"(?:[^"\\]|\\.)*"/)) return 'string';
  if (stream.peek() === '"') {
    stream.skipToEnd();
    return 'invalid';
  }
  if (stream.match(/^[A-Za-z_][\w-]*/)) {
    const word = stream.current();
    if (NAMESPACES.has(word)) return 'namespace';
    if (KEYWORDS.has(word.toLowerCase())) return 'keyword';
    if (OPERATORS.has(word)) return 'operator';
    if (FIELDS.has(word)) return 'propertyName';
    return 'variableName';
  }
  if (stream.match(/^\d+/)) return 'number';
  if (stream.match(/^[.:[\]()]/)) return 'punctuation';
  stream.next();
  return null;
}

/** The HTTPQL mode: a forgiving per-line tokenizer, not a parser (the backend validates). */
export const httpqlLanguage = StreamLanguage.define<HttpqlState>({
  name: 'httpql',
  token,
  startState: () => ({ blockComment: false }),
  copyState: (state) => ({ ...state }),
  languageData: { commentTokens: { line: '//', block: { open: '/*', close: '*/' } } },
});

/** Colours are classes so the query bar can style them with design tokens (docs/06 §6). */
export const httpqlHighlight = HighlightStyle.define([
  { tag: tags.namespace, class: 'cm-httpql-namespace' },
  { tag: tags.propertyName, class: 'cm-httpql-field' },
  { tag: tags.operator, class: 'cm-httpql-operator' },
  { tag: tags.string, class: 'cm-httpql-string' },
  { tag: tags.number, class: 'cm-httpql-number' },
  { tag: tags.keyword, class: 'cm-httpql-keyword' },
  { tag: tags.comment, class: 'cm-httpql-comment' },
  { tag: tags.invalid, class: 'cm-httpql-invalid' },
  { tag: tags.punctuation, class: 'cm-httpql-punctuation' },
  { tag: tags.variableName, class: 'cm-httpql-value' },
]);

/**
 * Catches the syntax errors that make a query unusable, with the span to underline.
 * The server is authoritative and reports the same errors as `BAD_QUERY`; this keeps the
 * dev mock (which does not parse) honest and saves a round trip for obvious mistakes.
 * Type errors (e.g. `resp.code.cont:"2"`) are the server's to report.
 */
export function syntaxError(text: string): QueryError | null {
  let depth = 0;
  let openParen = 0;
  for (let i = 0; i < text.length; i++) {
    const char = text[i];
    if (char === '/' && text[i + 1] === '/') {
      const newline = text.indexOf('\n', i);
      i = newline < 0 ? text.length : newline;
    } else if (char === '/' && text[i + 1] === '*') {
      const end = text.indexOf('*/', i + 2);
      if (end < 0) return { message: 'Unterminated comment', from: i, to: text.length };
      i = end + 1;
    } else if (char === '"') {
      let j = i + 1;
      for (; j < text.length; j++) {
        if (text[j] === '\\') j++;
        else if (text[j] === '"') break;
      }
      if (j >= text.length) return { message: 'Unterminated string', from: i, to: text.length };
      i = j;
    } else if (char === '(') {
      if (depth++ === 0) openParen = i;
    } else if (char === ')') {
      if (--depth < 0) return { message: 'Unmatched ")"', from: i, to: i + 1 };
    }
  }
  return depth > 0 ? { message: 'Missing ")"', from: openParen, to: text.length } : null;
}

function utf8Length(codePoint: number): number {
  return codePoint <= 0x7f ? 1 : codePoint <= 0x7ff ? 2 : codePoint <= 0xffff ? 3 : 4;
}

/** UTF-16 editor offset → UTF-8 byte offset (docs/04 §7 offsets are bytes). */
export function docToByte(text: string, pos: number): number {
  let bytes = 0;
  for (let i = 0; i < pos && i < text.length; i += text.codePointAt(i)! > 0xffff ? 2 : 1) {
    bytes += utf8Length(text.codePointAt(i)!);
  }
  return bytes;
}

/** UTF-8 byte offset → UTF-16 editor offset, clamped to the text. */
export function byteToDoc(text: string, byte: number): number {
  let bytes = 0;
  let i = 0;
  while (byte > 0 && i < text.length) {
    const size = utf8Length(text.codePointAt(i)!);
    if (bytes + size > byte) break;
    bytes += size;
    i += size === 4 ? 2 : 1;
  }
  return i;
}
