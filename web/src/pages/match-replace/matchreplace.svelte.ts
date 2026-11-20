// Match & Replace page state (F-MR-1, docs/02 §3.6): collections with rules, the draft in the
// editor, field errors, drag moves and JSON import/export. The contract has no subscription, so
// the page refetches after its own mutations (T-099).
import type { Client, CombinedError } from '@urql/core';
import {
  CreateMatchReplaceCollectionDocument,
  CreateMatchReplaceRuleDocument,
  DeleteMatchReplaceCollectionDocument,
  DeleteMatchReplaceRuleDocument,
  ExportMatchReplaceDocument,
  ImportMatchReplaceDocument,
  MatchReplaceCollectionsDocument,
  MoveMatchReplaceRuleDocument,
  RenameMatchReplaceCollectionDocument,
  ReorderMatchReplaceCollectionsDocument,
  SetMatchReplaceRuleEnabledDocument,
  UpdateMatchReplaceRuleDocument,
  type MatchReplaceCollectionFieldsFragment,
  type MatchReplaceRuleFieldsFragment,
  type MatchReplaceRuleInput,
} from '$lib/api/generated/graphql';
import { byteToDoc, type QueryError } from '$lib/query/httpql';
import { matchersFor, operationsFor } from './rules';

export type RuleField = 'name' | 'condition' | 'sources' | 'section' | 'matcher' | 'term' | 'form';

/** A rule input with every defaulted field present, so the form can bind them. */
export type RuleInput = Required<Omit<MatchReplaceRuleInput, 'matcher'>> & {
  matcher: { kind: MatchReplaceRuleInput['matcher']['kind']; value: string };
};

export interface RuleDraft {
  /** Null while the rule is not saved yet. */
  id: string | null;
  collectionId: string;
  input: RuleInput;
}

const blankInput = (): RuleInput => ({
  name: '',
  enabled: true,
  condition: '',
  sources: ['PROXY'],
  section: 'REQUEST_HEADER',
  operation: 'ADD',
  matcher: { kind: 'FULL', value: '' },
  term: '',
});

const inputOf = (r: MatchReplaceRuleFieldsFragment): RuleInput => ({
  name: r.name,
  enabled: r.enabled,
  condition: r.condition,
  sources: [...r.sources],
  section: r.section,
  operation: r.operation,
  matcher: { kind: r.matcher.kind, value: r.matcher.value },
  term: r.term,
});

/** Attributes a `BAD_INPUT` message to the field that caused it. */
export function fieldOfMessage(message: string): RuleField {
  const m = message.toLowerCase();
  if (m.includes('name must') || m.startsWith('name')) return 'name';
  if (m.includes('sources')) return 'sources';
  if (m.includes('section') || m.includes('operation')) return 'section';
  if (m.includes('matcher') || m.includes('regex')) return 'matcher';
  if (m.includes('term') || m.includes('header')) return 'term';
  if (m.includes('condition')) return 'condition';
  return 'form';
}

export class MatchReplaceState {
  client: Client;
  collections = $state.raw<MatchReplaceCollectionFieldsFragment[]>([]);
  draft = $state<RuleDraft | null>(null);
  /** Last saved/opened form, to tell unsaved edits from none. */
  baseline = $state<string>('');
  errors = $state<Partial<Record<RuleField, string>>>({});
  /** HTTPQL error for the condition, in editor positions. */
  conditionError = $state<QueryError | null>(null);
  message = $state<string | null>(null);
  notice = $state<string | null>(null);
  busy = $state(false);
  /** Row the context menu was opened on; its commands act on this rule, not the open one. */
  menuRule = $state<string | null>(null);

  constructor(client: Client) {
    this.client = client;
  }

  /** Swaps the backend before the first load (dev mock, T-013). */
  useClient(client: Client): void {
    this.client = client;
  }

  get dirty(): boolean {
    return this.draft !== null && JSON.stringify(this.draft) !== this.baseline;
  }

  /** The draft as the tester's input; null when no rule is open. */
  get testerRule(): MatchReplaceRuleInput | null {
    return this.draft?.input ?? null;
  }

  async start(): Promise<void> {
    await this.load();
  }

  private open(draft: RuleDraft | null): void {
    this.draft = draft;
    this.baseline = JSON.stringify(draft);
    this.errors = {};
    this.conditionError = null;
  }

  /** Opens a saved rule. The caller confirms discarding unsaved edits first (`dirty`). */
  select(id: string | null): void {
    for (const c of this.collections) {
      const rule = c.rules.find((r) => r.id === id);
      if (rule) return this.open({ id, collectionId: c.id, input: inputOf(rule) });
    }
    this.open(null);
  }

  newRule(collectionId?: string): void {
    const target = collectionId ?? this.draft?.collectionId ?? this.collections[0]?.id;
    if (!target) {
      this.message = 'Create a collection first';
      return;
    }
    this.message = null;
    this.open({ id: null, collectionId: target, input: blankInput() });
  }

  cancel(): void {
    this.select(this.draft?.id ?? null);
  }

  // Section → first valid operation → first valid matcher, so the form never holds an invalid combination.
  setSection(section: RuleDraft['input']['section']): void {
    const d = this.draft;
    if (!d) return;
    d.input.section = section;
    if (!operationsFor(section).includes(d.input.operation))
      this.setOperation(operationsFor(section)[0]!);
  }

  setOperation(operation: RuleDraft['input']['operation']): void {
    const d = this.draft;
    if (!d) return;
    d.input.operation = operation;
    const allowed = matchersFor(operation);
    if (allowed.length > 0 && !allowed.includes(d.input.matcher.kind))
      d.input.matcher = { kind: 'FULL', value: '' };
  }

  toggleSource(source: RuleDraft['input']['sources'][number]): void {
    const d = this.draft;
    if (!d) return;
    const has = d.input.sources.includes(source);
    d.input.sources = has
      ? d.input.sources.filter((s) => s !== source)
      : [...d.input.sources, source];
  }

  private fail(error: CombinedError): void {
    const gql = error.graphQLErrors[0];
    const code = gql?.extensions?.code;
    const message = gql?.message ?? error.message;
    if (code === 'BAD_QUERY') {
      const span = gql?.extensions?.span as { start: number; end: number } | undefined;
      const text = this.draft?.input.condition ?? '';
      const from = span ? byteToDoc(text, span.start) : 0;
      this.conditionError = { message, from, to: span ? byteToDoc(text, span.end) : text.length };
      this.errors = { condition: message };
    } else if (code === 'BAD_INPUT') {
      this.errors = { [fieldOfMessage(message)]: message };
    } else {
      this.errors = { form: message };
    }
  }

  /** Creates or updates the draft; on error the message lands on its field and the draft stays. */
  async save(): Promise<boolean> {
    const d = this.draft;
    if (!d) return false;
    this.busy = true;
    this.errors = {};
    this.conditionError = null;
    const sent = JSON.stringify(d.input);
    // ADD ignores the matcher, so none is sent.
    const input: RuleInput =
      d.input.operation === 'ADD' ? { ...d.input, matcher: { kind: 'FULL', value: '' } } : d.input;
    let saved: MatchReplaceRuleFieldsFragment | undefined;
    let error: CombinedError | undefined;
    if (d.id) {
      const r = await this.client
        .mutation(UpdateMatchReplaceRuleDocument, { id: d.id, input })
        .toPromise();
      saved = r.data?.updateMatchReplaceRule;
      error = r.error;
    } else {
      const r = await this.client
        .mutation(CreateMatchReplaceRuleDocument, { collectionId: d.collectionId, input })
        .toPromise();
      saved = r.data?.createMatchReplaceRule;
      error = r.error;
    }
    this.busy = false;
    // The user may have opened another rule while the request ran; only the originating draft
    // gets the outcome.
    const current = this.draft === d;
    if (error || !saved) {
      if (current) {
        if (error) this.fail(error);
        else this.errors = { form: 'Saving the rule failed' };
      }
      return false;
    }
    await this.load();
    if (this.draft !== d) return true;
    if (JSON.stringify(d.input) === sent) this.select(saved.id);
    else {
      // Edited while saving: keep the edits, but the rule exists now, so later saves update it.
      d.id = saved.id;
      this.baseline = JSON.stringify({
        id: saved.id,
        collectionId: d.collectionId,
        input: JSON.parse(sent),
      });
    }
    return true;
  }

  async removeRule(id: string): Promise<void> {
    const r = await this.client.mutation(DeleteMatchReplaceRuleDocument, { id }).toPromise();
    if (r.error) return void (this.message = r.error.message);
    this.message = null;
    const wasOpen = this.draft?.id === id;
    await this.load();
    if (wasOpen) this.select(null);
  }

  /** Flips `enabled` in place; a failure puts it back. */
  async setEnabled(id: string, enabled: boolean): Promise<void> {
    this.patchRule(id, { enabled });
    const r = await this.client
      .mutation(SetMatchReplaceRuleEnabledDocument, { id, enabled })
      .toPromise();
    if (r.error) {
      this.patchRule(id, { enabled: !enabled }); // only this rule: other toggles may have succeeded since
      this.message = r.error.message;
      return;
    }
    if (this.draft?.id === id) {
      this.draft.input.enabled = enabled;
      const base = JSON.parse(this.baseline) as RuleDraft;
      base.input.enabled = enabled;
      this.baseline = JSON.stringify(base);
    }
  }

  private ruleById(id: string): MatchReplaceRuleFieldsFragment | undefined {
    for (const c of this.collections) {
      const r = c.rules.find((x) => x.id === id);
      if (r) return r;
    }
  }

  private patchRule(id: string, patch: Partial<MatchReplaceRuleFieldsFragment>): void {
    this.collections = this.collections.map((c) => ({
      ...c,
      rules: c.rules.map((r) => (r.id === id ? { ...r, ...patch } : r)),
    }));
  }

  /**
   * Moves a rule to `index` of `collectionId` (the server's semantics: the final position, own
   * collection allowed). Shown at once; a failure reloads the server's order.
   */
  async moveRule(id: string, collectionId: string, index: number): Promise<void> {
    const rule = this.ruleById(id);
    if (!rule || index < 0) return;
    const optimistic = this.collections.map((c) => ({
      ...c,
      rules: c.rules.filter((r) => r.id !== id),
    }));
    const target = optimistic.find((c) => c.id === collectionId);
    if (!target) return;
    target.rules.splice(Math.min(index, target.rules.length), 0, rule);
    const before = this.collections;
    this.collections = optimistic;
    const r = await this.client
      .mutation(MoveMatchReplaceRuleDocument, { id, collectionId, index })
      .toPromise();
    if (r.error) {
      this.collections = before;
      this.message = r.error.message;
      await this.load();
      return;
    }
    this.message = null;
    if (this.draft?.id === id) this.draft.collectionId = collectionId;
    await this.load();
  }

  async createCollection(name: string): Promise<void> {
    const r = await this.client
      .mutation(CreateMatchReplaceCollectionDocument, { name })
      .toPromise();
    if (r.error) return void (this.message = r.error.message);
    this.message = null;
    await this.load();
  }

  async renameCollection(id: string, name: string): Promise<void> {
    const r = await this.client
      .mutation(RenameMatchReplaceCollectionDocument, { id, name })
      .toPromise();
    if (r.error) return void (this.message = r.error.message);
    this.message = null;
    await this.load();
  }

  async removeCollection(id: string): Promise<void> {
    const r = await this.client.mutation(DeleteMatchReplaceCollectionDocument, { id }).toPromise();
    if (r.error) return void (this.message = r.error.message);
    this.message = null;
    const open = this.draft?.collectionId === id;
    await this.load();
    if (open) this.select(null);
  }

  /** Moves a collection to `index` of the list. */
  async moveCollection(id: string, index: number): Promise<void> {
    const ids = this.collections.map((c) => c.id);
    const from = ids.indexOf(id);
    if (from < 0 || index < 0 || index >= ids.length || from === index) return;
    ids.splice(index, 0, ...ids.splice(from, 1));
    const before = this.collections;
    this.collections = ids.map((i) => before.find((c) => c.id === i)!);
    const r = await this.client
      .mutation(ReorderMatchReplaceCollectionsDocument, { ids })
      .toPromise();
    if (r.error) {
      this.collections = before;
      this.message = r.error.message;
      return;
    }
    this.message = null;
    this.collections = r.data?.reorderMatchReplaceCollections ?? this.collections;
  }

  async exportJson(): Promise<string | null> {
    const r = await this.client
      .query(ExportMatchReplaceDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (r.error || r.data == null) {
      this.message = r.error?.message ?? 'Exporting the rules failed';
      return null;
    }
    return r.data.exportMatchReplace;
  }

  /** Appends the collections of an export; returns how many were added. */
  async importJson(json: string): Promise<number | null> {
    const r = await this.client.mutation(ImportMatchReplaceDocument, { json }).toPromise();
    const added = r.data?.importMatchReplace;
    if (r.error || added == null) {
      this.notice = null;
      this.message = r.error?.graphQLErrors[0]?.message ?? r.error?.message ?? 'Importing failed';
      return null;
    }
    this.message = null;
    this.notice = `Imported ${added} collection${added === 1 ? '' : 's'}`;
    await this.load();
    return added;
  }

  private async load(): Promise<void> {
    const r = await this.client
      .query(MatchReplaceCollectionsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (r.error) return void (this.message = r.error.message);
    this.collections = r.data?.matchReplaceCollections ?? [];
  }
}
