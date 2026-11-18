// Scopes page state (F-SCOPE-1, docs/02 §3.3): the preset list, the draft in the editor
// and its "matches N requests" count. The contract has no subscription, so the page
// refetches the list after its own mutations (T-091).
import type { Client } from '@urql/core';
import {
  CreateScopePresetDocument,
  DeleteScopePresetDocument,
  DuplicateScopePresetDocument,
  ReorderScopePresetsDocument,
  ScopeMatchCountDocument,
  ScopePresetsDocument,
  UpdateScopePresetDocument,
  type ScopePresetFieldsFragment,
} from '$lib/api/generated/graphql';

export type RuleKind = 'allow' | 'deny';

export interface ScopeDraft {
  /** Null while the editor holds a preset that is not saved yet. */
  id: string | null;
  name: string;
  allow: string[];
  deny: string[];
}

const blank = (): ScopeDraft => ({ id: null, name: '', allow: [], deny: [] });

export class ScopesState {
  client: Client;
  presets = $state.raw<ScopePresetFieldsFragment[]>([]);
  draft = $state<ScopeDraft>(blank());
  /** Matches of the saved preset, or null while unknown (a draft has no count). */
  count = $state<number | null>(null);
  message = $state<string | null>(null);
  busy = $state(false);

  constructor(client: Client) {
    this.client = client;
  }

  /** Swaps the backend before the first load (dev mock, T-013). */
  useClient(client: Client): void {
    this.client = client;
  }

  /** Loads the list and opens the first preset. */
  async start(): Promise<void> {
    await this.load();
    this.select(this.presets[0]?.id ?? null);
  }

  select(id: string | null): void {
    const preset = this.presets.find((p) => p.id === id);
    this.draft = preset
      ? { id: preset.id, name: preset.name, allow: [...preset.allow], deny: [...preset.deny] }
      : blank();
    this.count = null;
    void this.refreshCount();
  }

  /** Starts a preset that does not exist yet. */
  newPreset(): void {
    this.select(null);
  }

  addRule(kind: RuleKind, rule: string): void {
    const value = rule.trim();
    if (!value || this.draft[kind].includes(value)) return;
    this.draft = { ...this.draft, [kind]: [...this.draft[kind], value] };
  }

  removeRule(kind: RuleKind, index: number): void {
    this.draft = { ...this.draft, [kind]: this.draft[kind].filter((_, i) => i !== index) };
  }

  /** Creates or updates the draft, then reopens the saved preset. */
  async save(): Promise<void> {
    const { id, name, allow, deny } = this.draft;
    this.busy = true;
    let saved: ScopePresetFieldsFragment | null | undefined;
    let error: string | null;
    if (id) {
      const result = await this.client
        .mutation(UpdateScopePresetDocument, { id, name, allow, deny })
        .toPromise();
      saved = result.data?.updateScopePreset;
      error = result.error?.message ?? null;
    } else {
      const result = await this.client
        .mutation(CreateScopePresetDocument, { name, allow, deny })
        .toPromise();
      saved = result.data?.createScopePreset;
      error = result.error?.message ?? null;
    }
    this.busy = false;
    if (error || !saved) {
      this.message = error ?? 'Saving the scope failed';
      return;
    }
    this.message = null;
    await this.load();
    this.select(saved.id);
  }

  async duplicate(): Promise<void> {
    const id = this.draft.id;
    if (!id) return;
    const result = await this.client.mutation(DuplicateScopePresetDocument, { id }).toPromise();
    const copy = result.data?.duplicateScopePreset;
    if (result.error || !copy) {
      this.message = result.error?.message ?? 'Duplicating the scope failed';
      return;
    }
    this.message = null;
    await this.load();
    this.select(copy.id);
  }

  async remove(): Promise<void> {
    const id = this.draft.id;
    if (!id) return;
    const index = this.presets.findIndex((p) => p.id === id);
    const result = await this.client.mutation(DeleteScopePresetDocument, { id }).toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.message = null;
    await this.load();
    // The page falls back to no scope when the last preset went away (scope.graphqls).
    this.select(this.presets[Math.min(index, this.presets.length - 1)]?.id ?? null);
  }

  /** Moves a preset one place up (-1) or down (1) and stores the new order. */
  async move(id: string, delta: -1 | 1): Promise<void> {
    const ids = this.presets.map((p) => p.id);
    const from = ids.indexOf(id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= ids.length) return;
    [ids[from], ids[to]] = [ids[to]!, ids[from]!];
    const result = await this.client.mutation(ReorderScopePresetsDocument, { ids }).toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.presets = result.data?.reorderScopePresets ?? this.presets;
  }

  private async load(): Promise<void> {
    const result = await this.client
      .query(ScopePresetsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.presets = result.data?.scopePresets ?? [];
  }

  /** Counts the traffic of the saved preset; a newer selection drops this result. */
  private async refreshCount(): Promise<void> {
    const id = this.draft.id;
    if (!id) return;
    const result = await this.client
      .query(ScopeMatchCountDocument, { scopeId: id }, { requestPolicy: 'network-only' })
      .toPromise();
    if (this.draft.id !== id) return;
    this.count = result.data?.requests.totalCount ?? null;
  }
}
