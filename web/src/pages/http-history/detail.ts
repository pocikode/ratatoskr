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
}

const decoder = new TextDecoder();

/** Bytes are base64 (docs/03 §6); non-UTF-8 bytes become U+FFFD, which Hex view notes. */
export function decodeBytes(base64: string): string {
  if (!base64) return '';
  const binary = atob(base64);
  const bytes = Uint8Array.from(binary, (char) => char.charCodeAt(0));
  return decoder.decode(bytes);
}

export function requestTruncated(bodyLength: string): boolean {
  return Number(bodyLength) > MAX_BODY;
}

export function buildDetail(message: HistoryMessageFragment): HistoryDetail {
  const { response } = message;
  return {
    id: message.id,
    request: decodeBytes(message.head) + decodeBytes(message.body),
    response: response ? decodeBytes(response.head) + decodeBytes(response.body) : null,
    requestTruncated: requestTruncated(message.bodyLength),
    responseTruncated: response ? requestTruncated(response.bodyLength) : false,
  };
}
