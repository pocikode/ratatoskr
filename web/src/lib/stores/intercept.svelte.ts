// Intercept settings and queue (F-INT-1), shared by the top bar, the sidebar badge and the
// Intercept page. Both subscriptions keep it live; a project switch or a reconnect refetches.
import type { CombinedError } from '@urql/core';
import { onEnd, pipe, subscribe, type Source } from 'wonka';
import { getApi } from '$lib/api/client';
import {
  DropAllInterceptItemsDocument,
  DropInterceptItemDocument,
  ForwardAllInterceptItemsDocument,
  ForwardInterceptItemDocument,
  InterceptQueueChangedDocument,
  InterceptQueueDocument,
  InterceptSettingsChangedDocument,
  InterceptSettingsDocument,
  UpdateInterceptSettingsDocument,
  type InterceptItemFieldsFragment,
  type InterceptSettingsFieldsFragment,
  type InterceptSettingsInput,
} from '$lib/api/generated/graphql';
import { badQuerySpan } from '../../pages/http-history/history.svelte';
import { projects, type ProjectStore } from './project.svelte';

export type InterceptItem = InterceptItemFieldsFragment;
export type InterceptSettings = InterceptSettingsFieldsFragment;
export type InterceptKind = InterceptItem['kind'];

type Client = ReturnType<typeof getApi>['client'];

/** Result of a mutation: `error` is the server's message; `span` (UTF-8 bytes) is a `BAD_QUERY` span. */
export interface Outcome {
  error: string | null;
  span?: { start: number; end: number } | null;
}

const outcome = (error?: CombinedError): Outcome =>
  error
    ? { error: error.graphQLErrors[0]?.message ?? error.message, span: badQuerySpan(error) }
    : { error: null };

export function fromBase64(text: string): Uint8Array {
  const bin = atob(text);
  return Uint8Array.from(bin, (c) => c.charCodeAt(0));
}

/** Unwrapped base64, in chunks so a large message does not overflow the call stack. */
export function toBase64(bytes: Uint8Array): string {
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000)
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
}

export class InterceptStore {
  settings = $state.raw<InterceptSettings | null>(null);
  /** Held items, oldest first. */
  queue = $state.raw<InterceptItem[]>([]);
  error = $state<string | null>(null);
  count = $derived(this.queue.length);

  readonly #getClient: () => Client;
  readonly #getWs: () => Pick<ReturnType<typeof getApi>['ws'], 'on'> | undefined;
  readonly #projects: Pick<ProjectStore, 'onReset'>;
  #users = 0;
  #stop: (() => void) | undefined;
  #seq = 0;

  constructor(
    getClient: () => Client = () => getApi().client,
    getWs: () => Pick<ReturnType<typeof getApi>['ws'], 'on'> | undefined = () => getApi().ws,
    projectStore: Pick<ProjectStore, 'onReset'> = projects,
  ) {
    this.#getClient = getClient;
    this.#getWs = getWs;
    this.#projects = projectStore;
  }

  /** Starts following the server; calls nest, the last stop ends it. */
  start(): () => void {
    if (this.#users++ === 0) this.#stop = this.#run();
    let stopped = false;
    return () => {
      if (stopped) return;
      stopped = true;
      if (--this.#users === 0) this.#stop?.();
    };
  }

  #run(): () => void {
    let ended = false;
    const subs: { unsubscribe: () => void }[] = [];
    const retries: ReturnType<typeof setTimeout>[] = [];
    const listen = <T>(source: () => Source<T>, on: (v: T) => void) => {
      const open = () => {
        const sub = pipe(
          source(),
          onEnd(() => {
            // The server ends a lagging stream; resubscribe and refetch, delayed against hammering.
            if (ended) return;
            const t = setTimeout(() => {
              if (ended) return;
              open();
              void this.refresh();
            }, 1000);
            retries.push(t);
          }),
          subscribe(on),
        );
        subs.push(sub);
      };
      open();
    };
    const client = this.#getClient();
    listen(
      () => client.subscription(InterceptSettingsChangedDocument, {}),
      (r) => {
        if (r.data) this.settings = r.data.interceptSettingsChanged;
      },
    );
    listen(
      () => client.subscription(InterceptQueueChangedDocument, {}),
      (r) => {
        const change = r.data?.interceptQueueChanged;
        if (!change) return;
        const { item } = change;
        if (change.kind === 'REMOVED') this.queue = this.queue.filter((i) => i.id !== item.id);
        else if (!this.queue.some((i) => i.id === item.id)) this.queue = [...this.queue, item];
      },
    );
    // A project switch releases the queue without REMOVED events (intercept.graphqls).
    const offReset = this.#projects.onReset(() => {
      this.queue = [];
      void this.refresh();
    });
    let connected = false;
    const offWs = this.#getWs()?.on('connected', () => {
      if (connected) void this.refresh();
      connected = true;
    });
    void this.refresh();
    return () => {
      ended = true;
      retries.forEach(clearTimeout);
      subs.forEach((s) => s.unsubscribe());
      offReset();
      offWs?.();
    };
  }

  async refresh(): Promise<void> {
    const n = ++this.#seq;
    const client = this.#getClient();
    const [s, q] = await Promise.all([
      client.query(InterceptSettingsDocument, {}, { requestPolicy: 'network-only' }).toPromise(),
      client.query(InterceptQueueDocument, {}, { requestPolicy: 'network-only' }).toPromise(),
    ]);
    if (n !== this.#seq) return; // a newer refresh superseded this one
    if (s.data) this.settings = s.data.interceptSettings;
    if (q.data) this.queue = q.data.interceptQueue;
    this.error = s.error?.message ?? q.error?.message ?? null;
  }

  /** Items of one kind, oldest first. */
  of(kind: InterceptKind): InterceptItem[] {
    return this.queue.filter((i) => i.kind === kind);
  }

  async update(input: InterceptSettingsInput): Promise<Outcome> {
    const r = await this.#getClient()
      .mutation(UpdateInterceptSettingsDocument, { input })
      .toPromise();
    if (r.data) this.settings = r.data.updateInterceptSettings;
    return outcome(r.error);
  }

  toggleQueuing = () => this.update({ queuing: !this.settings?.queuing });

  /** Forwards one item; `raw` (bytes) replaces the message, `null` sends it unchanged. */
  async forward(id: string, raw: Uint8Array | null): Promise<Outcome> {
    const r = await this.#getClient()
      .mutation(ForwardInterceptItemDocument, { id, raw: raw ? toBase64(raw) : null })
      .toPromise();
    return outcome(r.error);
  }

  async drop(id: string): Promise<Outcome> {
    const r = await this.#getClient().mutation(DropInterceptItemDocument, { id }).toPromise();
    return outcome(r.error);
  }

  async forwardAll(kind: InterceptKind | null): Promise<Outcome> {
    const r = await this.#getClient()
      .mutation(ForwardAllInterceptItemsDocument, { kind })
      .toPromise();
    return outcome(r.error);
  }

  async dropAll(kind: InterceptKind | null): Promise<Outcome> {
    const r = await this.#getClient().mutation(DropAllInterceptItemsDocument, { kind }).toPromise();
    return outcome(r.error);
  }
}

export const intercept = new InterceptStore();
