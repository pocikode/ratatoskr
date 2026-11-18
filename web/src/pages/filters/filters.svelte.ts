// Filters page state (F-FILTER-1, docs/02 §3.3): filter presets, the draft in the editor,
// the live "matches N requests" count of a draft query and JSON import/export. The
// contract has no subscription, so the page refetches after its own mutations (T-091).
import type { Client } from '@urql/core';
import {
  CreateFilterPresetDocument,
  DeleteFilterPresetDocument,
  ExportFilterPresetsDocument,
  FilterMatchCountDocument,
  FilterPresetsDocument,
  ImportFilterPresetsDocument,
  ReorderFilterPresetsDocument,
  UpdateFilterPresetDocument,
  type FilterPresetFieldsFragment,
} from '$lib/api/generated/graphql';

export interface FilterDraft {
  /** Null while the editor holds a preset that is not saved yet. */
  id: string | null;
  name: string;
  alias: string;
  query: string;
}

const blank = (): FilterDraft => ({ id: null, name: '', alias: '', query: '' });

export class FiltersState {
  client: Client;
  presets = $state.raw<FilterPresetFieldsFragment[]>([]);
  draft = $state<FilterDraft>(blank());
  /** Matches of `draft.query`, or null while unknown or unparsable. */
  count = $state<number | null>(null);
  message = $state<string | null>(null);
  notice = $state<string | null>(null);
  busy = $state(false);

  private countToken = 0;

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
      ? { id: preset.id, name: preset.name, alias: preset.alias ?? '', query: preset.query }
      : blank();
    void this.countMatches(this.draft.query);
  }

  /** Starts a preset that does not exist yet. */
  newPreset(): void {
    this.select(null);
  }

  /** Counts a draft query; a newer call drops this result. Invalid queries count as unknown. */
  async countMatches(query: string): Promise<void> {
    const token = ++this.countToken;
    const text = query.trim();
    if (!text) {
      this.count = null;
      return;
    }
    const result = await this.client
      .query(FilterMatchCountDocument, { query: text }, { requestPolicy: 'network-only' })
      .toPromise();
    if (token !== this.countToken) return;
    this.count = result.error ? null : (result.data?.requests.totalCount ?? null);
  }

  /** Creates or updates the draft, then reopens the saved preset. */
  async save(): Promise<void> {
    const { id, name, query } = this.draft;
    const alias = this.draft.alias.trim() || null;
    this.busy = true;
    let saved: FilterPresetFieldsFragment | null | undefined;
    let error: string | null;
    if (id) {
      const result = await this.client
        .mutation(UpdateFilterPresetDocument, { id, name, alias, query })
        .toPromise();
      saved = result.data?.updateFilterPreset;
      error = result.error?.message ?? null;
    } else {
      const result = await this.client
        .mutation(CreateFilterPresetDocument, { name, alias, query })
        .toPromise();
      saved = result.data?.createFilterPreset;
      error = result.error?.message ?? null;
    }
    this.busy = false;
    if (error || !saved) {
      this.message = error ?? 'Saving the preset failed';
      return;
    }
    this.message = null;
    await this.load();
    this.select(saved.id);
  }

  async remove(): Promise<void> {
    const id = this.draft.id;
    if (!id) return;
    const index = this.presets.findIndex((p) => p.id === id);
    const result = await this.client.mutation(DeleteFilterPresetDocument, { id }).toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.message = null;
    await this.load();
    this.select(this.presets[Math.min(index, this.presets.length - 1)]?.id ?? null);
  }

  /** Moves a preset one place up (-1) or down (1) and stores the new order. */
  async move(id: string, delta: -1 | 1): Promise<void> {
    const ids = this.presets.map((p) => p.id);
    const from = ids.indexOf(id);
    const to = from + delta;
    if (from < 0 || to < 0 || to >= ids.length) return;
    [ids[from], ids[to]] = [ids[to]!, ids[from]!];
    const result = await this.client.mutation(ReorderFilterPresetsDocument, { ids }).toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.presets = result.data?.reorderFilterPresets ?? this.presets;
  }

  /** The `exportFilterPresets` JSON document, or null (with `message`) on failure. */
  async exportJson(): Promise<string | null> {
    const result = await this.client
      .query(ExportFilterPresetsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (result.error || !result.data) {
      this.message = result.error?.message ?? 'Exporting the presets failed';
      return null;
    }
    return result.data.exportFilterPresets;
  }

  /** Adds the presets of an export document; returns how many were added. */
  async importJson(json: string): Promise<number | null> {
    const result = await this.client.mutation(ImportFilterPresetsDocument, { json }).toPromise();
    const added = result.data?.importFilterPresets;
    if (result.error || added == null) {
      this.message = result.error?.message ?? 'Importing the presets failed';
      return null;
    }
    this.message = null;
    this.notice = `Imported ${added} preset${added === 1 ? '' : 's'}`;
    await this.load();
    return added;
  }

  private async load(): Promise<void> {
    const result = await this.client
      .query(FilterPresetsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (result.error) {
      this.message = result.error.message;
      return;
    }
    this.presets = result.data?.filterPresets ?? [];
  }
}
