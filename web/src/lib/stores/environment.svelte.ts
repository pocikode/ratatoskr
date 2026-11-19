// Environments (F-ENV-1, T-103): the list, the one selected per project, and the `{{name}}`
// lookup Replay reads. No subscription: refetch after own mutations and on project switches.
import type { Client } from '@urql/core';
import { getApi } from '$lib/api/client';
import {
  CreateEnvironmentDocument,
  DeleteEnvironmentDocument,
  EnvironmentsDocument,
  RenameEnvironmentDocument,
  SelectEnvironmentDocument,
  SetEnvironmentVariablesDocument,
  type EnvironmentFieldsFragment,
  type EnvironmentVariableInput,
} from '$lib/api/generated/graphql';
import { projects } from './project.svelte';

export type Environment = EnvironmentFieldsFragment;
export type Variable = Environment['variables'][number];

export class EnvironmentStore {
  environments = $state.raw<Environment[]>([]);
  loading = $state(true);
  /** Last mutation error, or null. The page reads it to point at the offending row. */
  error = $state<string | null>(null);
  /** The selected project environment, or null when only Global is used. */
  selected = $derived(this.environments.find((e) => e.selected) ?? null);
  /** `{{name}}` lookup: the selected environment over Global (F-ENV-1). Global is listed first. */
  variables = $derived.by(() => {
    // eslint-disable-next-line svelte/prefer-svelte-reactivity -- T-103: rebuilt by $derived, never mutated by readers
    const map = new Map<string, string>();
    for (const env of this.environments)
      if (env.isGlobal || env.selected) for (const v of env.variables) map.set(v.name, v.value);
    return map;
  });

  readonly getClient: () => Client;

  constructor(getClient: () => Client = () => getApi().client) {
    this.getClient = getClient;
  }

  /** Loads the list and refetches on project switches. Returns the cleanup. */
  start(): () => void {
    void this.refresh();
    return projects.onReset(() => void this.refresh());
  }

  async refresh(): Promise<void> {
    const r = await this.getClient()
      .query(EnvironmentsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (r.data) {
      this.environments = r.data.environments;
      this.error = null;
    } else this.error = message(r.error);
    this.loading = false;
  }

  /** Selects a project environment, or none (Global only) with null. */
  select = (id: string | null) =>
    this.#run(() => this.getClient().mutation(SelectEnvironmentDocument, { id }).toPromise());

  create = (name: string) =>
    this.#run(() => this.getClient().mutation(CreateEnvironmentDocument, { name }).toPromise());

  rename = (id: string, name: string) =>
    this.#run(() => this.getClient().mutation(RenameEnvironmentDocument, { id, name }).toPromise());

  remove = (id: string) =>
    this.#run(() => this.getClient().mutation(DeleteEnvironmentDocument, { id }).toPromise());

  saveVariables = (id: string, variables: EnvironmentVariableInput[]) =>
    this.#run(() =>
      this.getClient().mutation(SetEnvironmentVariablesDocument, { id, variables }).toPromise(),
    );

  /** Runs a mutation, then refetches; on failure records the error and returns false. */
  async #run(op: () => Promise<{ error?: unknown }>): Promise<boolean> {
    const r = await op();
    if (r.error) {
      this.error = message(r.error);
      return false;
    }
    await this.refresh();
    return true;
  }
}

function message(e: unknown): string {
  const err = e as { graphQLErrors?: { message: string }[]; message?: string } | undefined;
  return err?.graphQLErrors?.[0]?.message ?? err?.message ?? 'Request failed';
}

export const environment = new EnvironmentStore();
