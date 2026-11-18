// Project list, selection and the reset signal for project-scoped stores (T-009, F-WSP-1).
import { SvelteSet } from 'svelte/reactivity';
import type { CombinedError } from '@urql/core';
import { getApi } from '$lib/api/client';
import {
  CreateProjectDocument,
  DeleteProjectDocument,
  DuplicateProjectDocument,
  ProjectChangedDocument,
  ProjectsDocument,
  RenameProjectDocument,
  SelectProjectDocument,
  SetDefaultProjectDocument,
  type ProjectsQuery,
} from '$lib/api/generated/graphql';
import type { Command } from '$lib/commands/registry';

export type Project = ProjectsQuery['projects'][number];
export type ProjectDialog =
  { kind: 'create' } | { kind: 'rename' | 'delete'; project: Project } | null;

type Client = ReturnType<typeof getApi>['client'];

export class ProjectStore {
  projects = $state.raw<Project[]>([]);
  loading = $state(true);
  error = $state<string | null>(null);
  /** Dialog requested by a command; rendered by the Workspace page. */
  dialog = $state.raw<ProjectDialog>(null);
  /** Bumped on every SELECTED change; project-scoped pages key their content on it. */
  epoch = $state(0);
  current = $derived(this.projects.find((p) => p.isSelected) ?? null);

  #resets = new SvelteSet<() => void>();

  readonly getClient: () => Client;

  constructor(getClient: () => Client = () => getApi().client) {
    this.getClient = getClient;
  }

  /** Registers a store reset callback, run when another project gets selected. */
  onReset(fn: () => void): () => void {
    this.#resets.add(fn);
    return () => this.#resets.delete(fn);
  }

  /** Loads the list and follows `projectChanged`. Returns the cleanup. */
  start(): () => void {
    void this.refresh();
    const sub = this.getClient()
      .subscription(ProjectChangedDocument, {})
      .subscribe((r) => {
        if (!r.data) return;
        if (r.data.projectChanged.kind === 'SELECTED') {
          this.epoch++;
          for (const fn of this.#resets) fn();
        }
        void this.refresh();
      });
    return () => sub.unsubscribe();
  }

  async refresh(): Promise<void> {
    const r = await this.getClient()
      .query(ProjectsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (r.data) {
      this.projects = r.data.projects;
      this.error = null;
    } else this.error = message(r.error);
    this.loading = false;
  }

  /** Runs a mutation; on failure records the error and returns false. */
  async #run(op: () => Promise<{ error?: CombinedError }>): Promise<boolean> {
    const r = await op();
    if (r.error) {
      this.error = message(r.error);
      return false;
    }
    this.error = null;
    await this.refresh();
    return true;
  }

  create = (name: string, select = false) =>
    this.#run(() => this.getClient().mutation(CreateProjectDocument, { name, select }).toPromise());
  rename = (id: string, name: string) =>
    this.#run(() => this.getClient().mutation(RenameProjectDocument, { id, name }).toPromise());
  duplicate = (id: string, name?: string) =>
    this.#run(() => this.getClient().mutation(DuplicateProjectDocument, { id, name }).toPromise());
  remove = (id: string) =>
    this.#run(() => this.getClient().mutation(DeleteProjectDocument, { id }).toPromise());
  setDefault = (id: string | null) =>
    this.#run(() => this.getClient().mutation(SetDefaultProjectDocument, { id }).toPromise());
  select = (id: string) =>
    this.#run(() => this.getClient().mutation(SelectProjectDocument, { id }).toPromise());
}

function message(e?: CombinedError): string {
  return e?.graphQLErrors[0]?.message ?? e?.message ?? 'Request failed';
}

export const projects = new ProjectStore();

/** Palette entries for projects: one switch command per project plus "New project". */
export function projectCommands(store: ProjectStore, navigate: (id: string) => void): Command[] {
  return [
    {
      id: 'project.create',
      title: 'New project…',
      icon: 'folder',
      group: 'Project',
      run: () => {
        navigate('workspace');
        store.dialog = { kind: 'create' };
      },
    },
    ...store.projects
      .filter((p) => !p.isSelected)
      .map((p) => ({
        id: `project.select.${p.id}`,
        title: `Switch to project: ${p.name}`,
        icon: 'folder' as const,
        group: 'Project',
        run: async () => {
          await store.select(p.id);
        },
      })),
  ];
}
