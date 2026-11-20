// Replay session editor state (F-REP-2): the draft with its autosave, sends and the entry
// history. One send per session at a time, so the button and `sending` follow this class.
import type { Client } from '@urql/core';
import {
  CancelReplaySendDocument,
  ReplaySessionDetailDocument,
  SendReplaySessionDocument,
  UpdateReplaySessionDraftDocument,
  type ReplayConnectionInput,
  type ReplayOptionsInput,
  type ReplaySessionDetailQuery,
  type UpdateReplaySessionInput,
} from '$lib/api/generated/graphql';
import { requestTruncated } from '../../http-history/detail';

export type Session = NonNullable<ReplaySessionDetailQuery['replaySession']>;
export type Entry = Session['entries']['edges'][number]['node'];
export type Connection = Session['connection'];
export type Options = Session['options'];

/** Draft autosave: at most one `updateReplaySession` per second while typing. */
export const SAVE_MS = 1000;

const bytesOf = (base64: string): Uint8Array => {
  const bin = atob(base64);
  return Uint8Array.from(bin, (ch) => ch.charCodeAt(0));
};

/** Chunked: spreading a large array into `String.fromCharCode` overflows the argument limit. */
export function toBase64(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000)
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
}

/** The request bytes of an entry as stored: head and body decoded separately, then joined. */
export function requestBytes(entry: Entry): Uint8Array {
  const head = bytesOf(entry.request.head);
  const body = bytesOf(entry.request.body);
  const out = new Uint8Array(head.length + body.length);
  out.set(head);
  out.set(body, head.length);
  return out;
}

/** A message's bytes as text; invalid UTF-8 shows as U+FFFD (Hex view still has the bytes). */
export const messageText = (m: { head: string; body: string }): string =>
  new TextDecoder().decode(bytesOf(m.head)) + new TextDecoder().decode(bytesOf(m.body));

function message(e: unknown): string {
  const err = e as { graphQLErrors?: { message: string }[]; message?: string } | undefined;
  return err?.graphQLErrors?.[0]?.message ?? err?.message ?? 'Request failed';
}

export class SessionEditorState {
  readonly client: Client;
  readonly sessionId: string;
  session = $state.raw<Session | null>(null);
  /** Editor content: the draft, or the request of the entry being viewed. */
  shown = $state.raw<Uint8Array>(new Uint8Array());
  /** Entry whose response is shown; null falls back to the newest entry. */
  activeEntryId = $state<string | null>(null);
  /** Newest first, as the server returns them. */
  entries = $state.raw<Entry[]>([]);
  sending = $state(false);
  loading = $state(true);
  error = $state<string | null>(null);

  /** Bytes waiting for the next save tick. */
  #pending: Uint8Array | null = null;
  #timer: ReturnType<typeof setTimeout> | undefined;

  constructor(client: Client, sessionId: string) {
    this.client = client;
    this.sessionId = sessionId;
  }

  connection = $derived<Connection>(
    this.session?.connection ?? { host: '', port: 443, tls: true, sni: null },
  );
  options = $derived<Options>(
    this.session?.options ?? {
      followRedirects: false,
      updateContentLength: true,
      timeoutMs: 60000,
      applyMatchReplace: false,
    },
  );
  /** The entry whose request and response the panes show; the newest one until one is picked. */
  viewed = $derived(
    this.entries.find((e) => e.id === this.activeEntryId) ?? this.entries[0] ?? null,
  );
  position = $derived(this.entries.findIndex((e) => e.id === this.viewed?.id) + 1);
  /** The stored request is longer than the 4 MiB body cap, so editing it would drop bytes. */
  entryTruncated = $derived(
    !!this.activeEntryId &&
      !!this.viewed &&
      requestTruncated(this.viewed.request.bodyLength, this.viewed.request.body),
  );

  async load(): Promise<void> {
    const r = await this.client
      .query(ReplaySessionDetailDocument, { id: this.sessionId }, { requestPolicy: 'network-only' })
      .toPromise();
    this.loading = false;
    if (r.error || !r.data?.replaySession) {
      this.error = r.error ? message(r.error) : `Session ${this.sessionId} not found`;
      return;
    }
    const session = r.data.replaySession;
    this.session = session;
    this.entries = session.entries.edges.map((edge) => edge.node);
    this.shown = bytesOf(session.draft);
    this.sending = session.sending;
  }

  /** A keystroke in the editor: the edit becomes the draft, saved on the next tick. */
  edit(bytes: Uint8Array): void {
    this.shown = bytes;
    this.#pending = bytes;
    this.#timer ??= setTimeout(() => {
      this.#timer = undefined;
      void this.save();
    }, SAVE_MS);
  }

  /** Saves the pending draft, if any. */
  async save(): Promise<void> {
    const bytes = this.#pending;
    if (!bytes) return;
    this.#pending = null;
    await this.#update({ draft: toBase64(bytes) });
  }

  /** Saves now, for a send, an entry switch or the tab closing. */
  async flush(): Promise<void> {
    if (this.#timer) clearTimeout(this.#timer);
    this.#timer = undefined;
    await this.save();
  }

  saveConnection(connection: ReplayConnectionInput): Promise<void> {
    return this.#update({ connection });
  }

  saveOptions(options: ReplayOptionsInput): Promise<void> {
    return this.#update({ options });
  }

  /** The server's saved values, so the bar shows what it accepted. */
  async #update(input: UpdateReplaySessionInput): Promise<void> {
    const s = this.session;
    const r = await this.client
      .mutation(UpdateReplaySessionDraftDocument, { id: this.sessionId, input })
      .toPromise();
    if (r.error) {
      this.error = message(r.error);
      return;
    }
    this.error = null;
    if (!s || !this.session) return;
    const options = input.options;
    this.session = {
      ...this.session,
      ...(input.connection && { connection: { sni: null, ...input.connection } }),
      ...(options && {
        options: {
          followRedirects: options.followRedirects ?? s.options.followRedirects,
          updateContentLength: options.updateContentLength ?? s.options.updateContentLength,
          timeoutMs: options.timeoutMs ?? s.options.timeoutMs,
          applyMatchReplace: options.applyMatchReplace ?? s.options.applyMatchReplace,
        },
      }),
      ...(input.draft && { draft: input.draft }),
    };
  }

  async send(): Promise<void> {
    if (this.sending) return;
    this.sending = true;
    this.error = null;
    try {
      await this.flush(); // nothing typed is lost even when the send fails
      const raw = toBase64(this.shown);
      const r = await this.client
        .mutation(SendReplaySessionDocument, {
          id: this.sessionId,
          input: { raw, connection: this.connection, options: this.options },
        })
        .toPromise();
      if (r.error || !r.data) {
        this.error = r.error ? message(r.error) : 'Request failed';
        return;
      }
      const entry = r.data.sendReplaySession;
      this.entries = [entry, ...this.entries];
      this.activeEntryId = entry.id;
      if (this.session) this.session = { ...this.session, draft: raw };
    } finally {
      this.sending = false;
    }
  }

  async cancel(): Promise<void> {
    const r = await this.client
      .mutation(CancelReplaySendDocument, { id: this.sessionId })
      .toPromise();
    if (r.error) this.error = message(r.error);
  }

  /** Shows an entry's request in the editor; the pending draft is saved first. */
  async selectEntry(id: string | null): Promise<void> {
    const entry = id ? this.entries.find((e) => e.id === id) : undefined;
    if (!entry) return;
    if (id !== this.activeEntryId) await this.flush();
    this.activeEntryId = entry.id;
    if (!requestTruncated(entry.request.bodyLength, entry.request.body))
      this.shown = requestBytes(entry);
  }

  /** `delta` -1 is the newer entry, +1 the older one; both stop at the ends. */
  step(delta: number): void {
    const i = this.entries.findIndex((e) => e.id === this.viewed?.id);
    const next = this.entries[i + delta];
    if (next) void this.selectEntry(next.id);
  }

  /** The tab is going away: save what is pending. */
  dispose(): void {
    void this.flush();
  }
}
