// Autocomplete client for the query bar (docs/04-query-language.md §7): maps the server's
// UTF-8 byte offsets onto editor positions and tags each item with its kind.
import type {
  CompletionContext,
  CompletionResult,
  CompletionSource,
} from '@codemirror/autocomplete';
import { byteToDoc, docToByte } from './httpql';

/** One `Completion` of the schema. */
export interface QueryCompletion {
  label: string;
  insertText: string;
  /** `CompletionKind` enum value: NAMESPACE, FIELD, OPERATOR, PRESET or VALUE. */
  kind: string;
  detail: string;
  isExtension: boolean;
}

/** `queryCompletions` result; `from`/`to` are UTF-8 byte offsets into the text. */
export interface CompletionResponse {
  from: number;
  to: number;
  items: readonly QueryCompletion[];
}

export type CompletionFetcher = (
  text: string,
  cursorByte: number,
) => Promise<CompletionResponse | null>;

/** Icon by completion kind; CodeMirror's built-in completion types. */
const TYPES: Record<string, string> = {
  NAMESPACE: 'namespace',
  FIELD: 'property',
  OPERATOR: 'keyword',
  PRESET: 'constant',
  VALUE: 'text',
};

export function completionSource(fetch: CompletionFetcher): CompletionSource {
  return async (context: CompletionContext): Promise<CompletionResult | null> => {
    const text = context.state.doc.toString();
    const response = await fetch(text, docToByte(text, context.pos));
    if (!response || response.items.length === 0) return null;
    return {
      from: byteToDoc(text, response.from),
      to: byteToDoc(text, response.to),
      // The server already ranked and filtered the list for this cursor.
      filter: false,
      options: response.items.map((item) => ({
        label: item.label,
        apply: item.insertText,
        type: TYPES[item.kind] ?? 'text',
        detail: item.isExtension ? `${item.detail || 'extension'} (extension)` : item.detail,
      })),
    };
  };
}
