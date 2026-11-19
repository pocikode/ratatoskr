// Replay shell state (F-REP-1): collections tree, open tabs and the active one. The contract has
// no subscription, so every mutation is followed by a refetch (a failed one reverts the UI).
import type { Client } from '@urql/core';
import {
  CreateReplayCollectionDocument,
  CreateReplaySessionDocument,
  DeleteReplayCollectionDocument,
  DeleteReplaySessionDocument,
  DuplicateReplaySessionDocument,
  MoveReplaySessionDocument,
  RenameReplayCollectionDocument,
  RenameReplaySessionDocument,
  ReplayCollectionsDocument,
  SetReplaySessionColorDocument,
  type ReplayCollectionRowFragment,
  type ReplaySessionRowFragment,
  type RequestColor,
} from '$lib/api/generated/graphql';
import { decodeLossy, encodeText } from '$lib/editor/bytes';
import { buildSession, type NewSessionForm } from './url';

export type Collection = ReplayCollectionRowFragment;
export type Session = ReplaySessionRowFragment;
/** `from` tells which surface hosts an inline rename, so only one input appears. */
export type Target = { kind: 'session' | 'collection'; id: string; from?: 'tab' };

const storageKey = (projectId: string) => `replay.tabs.${projectId}`;

/** `index` is the position among the target's sessions once the moved one is taken out. */
export function moveSession(
  collections: Collection[],
  id: string,
  collectionId: string,
  index: number,
): Collection[] {
  const moved = collections.flatMap((c) => c.sessions).find((s) => s.id === id);
  if (!moved) return collections;
  return collections.map((c) => {
    const rest = c.sessions.filter((s) => s.id !== id);
    if (c.id !== collectionId) return { ...c, sessions: rest };
    rest.splice(Math.min(index, rest.length), 0, moved);
    return { ...c, sessions: rest };
  });
}

export class ReplayState {
  client: Client;
  readonly projectId: string;
  collections = $state.raw<Collection[]>([]);
  loaded = $state(false);
  search = $state('');
  openIds = $state.raw<string[]>([]);
  activeId = $state<string | null>(null);
  /** Row in inline rename. */
  renaming = $state<Target | null>(null);
  pendingDelete = $state<Target | null>(null);
  newSessionOpen = $state(false);
  newCollectionOpen = $state(false);
  error = $state<string | null>(null);

  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- T-034: rebuilt by $derived, never mutated
  sessions = $derived(new Map(this.collections.flatMap((c) => c.sessions).map((s) => [s.id, s])));
  openSessions = $derived(this.openIds.flatMap((id) => this.sessions.get(id) ?? []));
  active = $derived(this.activeId ? (this.sessions.get(this.activeId) ?? null) : null);

  constructor(client: Client, projectId: string) {
    this.client = client;
    this.projectId = projectId;
    try {
      const saved = JSON.parse(localStorage.getItem(storageKey(projectId)) ?? 'null');
      if (saved && Array.isArray(saved.open)) {
        this.openIds = saved.open.filter((x: unknown) => typeof x === 'string');
        this.activeId = typeof saved.active === 'string' ? saved.active : null;
      }
    } catch {
      /* unreadable storage: start with no tabs */
    }
  }

  useClient(client: Client): void {
    this.client = client;
  }

  async load(): Promise<void> {
    const r = await this.client
      .query(ReplayCollectionsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (r.error || !r.data) {
      this.error = r.error?.graphQLErrors[0]?.message ?? r.error?.message ?? 'Request failed';
      return;
    }
    this.collections = r.data.replayCollections;
    this.loaded = true;
    // Tabs of deleted sessions go away; an unknown active tab falls back to the last open one.
    this.openIds = this.openIds.filter((id) => this.sessions.has(id));
    if (!this.activeId || !this.sessions.has(this.activeId))
      this.activeId = this.openIds.at(-1) ?? null;
    this.#save();
  }

  #save(): void {
    try {
      localStorage.setItem(
        storageKey(this.projectId),
        JSON.stringify({ open: this.openIds, active: this.activeId }),
      );
    } catch {
      /* private mode: tabs just do not persist */
    }
  }

  /** Runs a mutation, then refetches so the UI shows what the server holds. */
  async #run(
    op: () => Promise<{ error?: { graphQLErrors: { message: string }[]; message: string } }>,
  ) {
    const r = await op();
    this.error = r.error ? (r.error.graphQLErrors[0]?.message ?? r.error.message) : null;
    await this.load();
    return !r.error;
  }

  // --- tabs ---

  async open(id: string): Promise<void> {
    if (!this.sessions.has(id)) await this.load(); // e.g. just created by openInReplay
    if (!this.sessions.has(id)) return;
    if (!this.openIds.includes(id)) this.openIds = [...this.openIds, id];
    this.activeId = id;
    this.#save();
  }

  close(id: string): void {
    const i = this.openIds.indexOf(id);
    if (i < 0) return;
    this.openIds = this.openIds.filter((x) => x !== id);
    if (this.activeId === id)
      this.activeId = this.openIds[Math.min(i, this.openIds.length - 1)] ?? null;
    this.#save();
  }

  closeOthers(id: string): void {
    this.openIds = this.openIds.filter((x) => x === id);
    this.activeId = this.openIds[0] ?? null;
    this.#save();
  }

  closeToRight(id: string): void {
    const i = this.openIds.indexOf(id);
    if (i < 0) return;
    this.openIds = this.openIds.slice(0, i + 1);
    if (this.activeId && !this.openIds.includes(this.activeId)) this.activeId = id;
    this.#save();
  }

  /** Next (+1) or previous (-1) tab, wrapping. */
  step(delta: number): void {
    const n = this.openIds.length;
    if (n === 0) return;
    const i = this.activeId ? this.openIds.indexOf(this.activeId) : -1;
    this.activeId = this.openIds[(i + delta + n) % n]!;
    this.#save();
  }

  // --- tree ---

  /** Collections with only matching sessions while searching (name, host or path in the draft). */
  filtered = $derived.by<Collection[]>(() => {
    const q = this.search.trim().toLowerCase();
    if (!q) return this.collections;
    return this.collections
      .map((c) => ({ ...c, sessions: c.sessions.filter((s) => matches(s, q)) }))
      .filter((c) => c.sessions.length > 0);
  });

  async createFromUrl(form: NewSessionForm, collectionId?: string): Promise<string | null> {
    const built = buildSession(form);
    if (typeof built === 'string') return built;
    const r = await this.client
      .mutation(CreateReplaySessionDocument, {
        input: { collectionId, connection: built.connection, raw: toBase64(encodeText(built.raw)) },
      })
      .toPromise();
    if (r.error || !r.data) return r.error?.graphQLErrors[0]?.message ?? 'Request failed';
    await this.load();
    await this.open(r.data.createReplaySession.id);
    return null;
  }

  createCollection = (name: string) =>
    this.#run(() => this.client.mutation(CreateReplayCollectionDocument, { name }).toPromise());

  rename(t: Target, name: string): Promise<boolean> {
    this.renaming = null;
    return this.#run(() =>
      t.kind === 'session'
        ? this.client.mutation(RenameReplaySessionDocument, { id: t.id, name }).toPromise()
        : this.client.mutation(RenameReplayCollectionDocument, { id: t.id, name }).toPromise(),
    );
  }

  setColor(id: string, color: RequestColor | null) {
    return this.#run(() =>
      this.client.mutation(SetReplaySessionColorDocument, { id, color }).toPromise(),
    );
  }

  async duplicate(id: string): Promise<void> {
    await this.#run(() => this.client.mutation(DuplicateReplaySessionDocument, { id }).toPromise());
  }

  async remove(t: Target): Promise<void> {
    this.pendingDelete = null;
    await this.#run(() =>
      t.kind === 'session'
        ? this.client.mutation(DeleteReplaySessionDocument, { id: t.id }).toPromise()
        : this.client.mutation(DeleteReplayCollectionDocument, { id: t.id }).toPromise(),
    );
  }

  /**
   * Optimistic; a failed mutation restores the previous tree without needing a refetch.
   * `index` counts the *visible* (search-filtered) sessions of the target without the moved one; it is
   * translated to a position among all of them so hidden sessions keep their order.
   */
  #moving = false;

  async move(id: string, collectionId: string, visibleIndex: number): Promise<void> {
    if (this.#moving) return; // one move at a time so a rollback never restores another failed move
    this.#moving = true;
    try {
      await this.#move(id, collectionId, visibleIndex);
    } finally {
      this.#moving = false;
    }
  }

  async #move(id: string, collectionId: string, visibleIndex: number): Promise<void> {
    const visible = this.filtered.find((c) => c.id === collectionId)?.sessions ?? [];
    const before = visible.filter((s) => s.id !== id)[visibleIndex];
    const all = (this.collections.find((c) => c.id === collectionId)?.sessions ?? []).filter(
      (s) => s.id !== id,
    );
    const index = before ? all.findIndex((s) => s.id === before.id) : all.length;
    const prev = this.collections;
    const next = moveSession(prev, id, collectionId, index);
    this.collections = next;
    const r = await this.client
      .mutation(MoveReplaySessionDocument, { id, collectionId, index })
      .toPromise();
    if (r.error) {
      if (this.collections === next) this.collections = prev; // a newer tree (e.g. after a delete) wins
      this.error = r.error.graphQLErrors[0]?.message ?? r.error.message;
      return;
    }
    this.error = null;
    await this.load();
  }
}

// Keyed by the (immutable, $state.raw) session object, so a refetch naturally drops stale entries.
const targetCache = new WeakMap<Session, string>();

/** Request target of a draft: decodes only as much base64 as needed to reach the end of the request line. */
function requestPath(s: Session): string {
  const hit = targetCache.get(s);
  if (hit !== undefined) return hit;
  let len = 1024; // multiple of 4 base64 chars
  let first: string;
  for (;;) {
    const head = atob(s.draft.slice(0, len));
    const text = decodeLossy(Uint8Array.from(head, (c) => c.charCodeAt(0)));
    const nl = text.search(/\r?\n/);
    if (nl >= 0 || len >= s.draft.length) {
      first = nl >= 0 ? text.slice(0, nl) : text;
      break;
    }
    len *= 2;
  }
  const path = first.split(' ')[1] ?? '';
  targetCache.set(s, path);
  return path;
}

function matches(s: Session, q: string): boolean {
  return [s.name, s.connection.host, requestPath(s)].some((v) => v.toLowerCase().includes(q));
}

// Chunked: spreading a large array into String.fromCharCode overflows the argument limit.
const toBase64 = (bytes: Uint8Array): string => {
  let bin = '';
  for (let i = 0; i < bytes.length; i += 0x8000)
    bin += String.fromCharCode(...bytes.subarray(i, i + 0x8000));
  return btoa(bin);
};
