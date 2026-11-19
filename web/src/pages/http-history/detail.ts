// Turns a `HistoryMessage` fragment (base64 head + body) into the raw HTTP messages the
// MessageViewer renders (F-HIST-2).
import type { HistoryMessageFragment } from '$lib/api/generated/graphql';

/** Per-call `body(limit:)` cap from the schema; longer bodies show their first 4 MiB. */
export const MAX_BODY = 4_194_304;

export interface HistoryDetail {
  id: string;
  request: string;
  /** Null when the request has no response (dropped, pending or failed). */
  response: string | null;
  requestTruncated: boolean;
  responseTruncated: boolean;
  /** The original of this record (`null` for an original). F-HIST-4. */
  editedFromId: string | null;
  /** The edited copy of this record (`null` for an edited record). F-HIST-4. */
  editedById: string | null;
}

const decoder = new TextDecoder();

/** Bytes are base64 (docs/03 §6); non-UTF-8 bytes become U+FFFD, which Hex view notes. */
export function decodeBytes(base64: string): string {
  if (!base64) return '';
  const binary = atob(base64);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return decoder.decode(bytes);
}

/** `bodyLength` is the stored (encoded) size, so the decoded body that came back decides. */
export function requestTruncated(bodyLength: string, body = ''): boolean {
  return Number(bodyLength) > MAX_BODY || decodedLength(body) >= MAX_BODY;
}

function decodedLength(base64: string): number {
  if (!base64) return 0;
  return (
    Math.floor((base64.length * 3) / 4) - (base64.endsWith('==') ? 2 : base64.endsWith('=') ? 1 : 0)
  );
}

export function buildDetail(message: HistoryMessageFragment): HistoryDetail {
  const { response } = message;
  return {
    id: message.id,
    request: decodeBytes(message.head) + decodeBytes(message.body),
    response: response ? decodeBytes(response.head) + decodeBytes(response.body) : null,
    requestTruncated: requestTruncated(message.bodyLength, message.body),
    responseTruncated: response ? requestTruncated(response.bodyLength, response.body) : false,
    editedFromId: message.editedFrom?.id ?? null,
    editedById: message.editedBy?.id ?? null,
  };
}
