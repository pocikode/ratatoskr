// Project list, selection and the reset signal for project-scoped stores (T-009, F-WSP-1).
import type { CombinedError } from '@urql/core';
import { onEnd, pipe, subscribe } from 'wonka';
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

  // eslint-disable-next-line svelte/prefer-svelte-reactivity -- T-009: never read reactively
  #resets = new Set<() => void>();
  #seq = 0;

  readonly getClient: () => Client;
  /** Socket lifecycle source; refetches after a reconnect, when events may have been missed. */
  readonly getWs: () => Pick<ReturnType<typeof getApi>['ws'], 'on'> | undefined;

  constructor(
    getClient: () => Client = () => getApi().client,
    getWs: ProjectStore['getWs'] = () => getApi().ws,
  ) {
    this.getClient = getClient;
    this.getWs = getWs;
  }

  /** Opens a dialog and clears any stale error so it doesn't leak into it. */
  openDialog(d: NonNullable<ProjectDialog>) {
    this.error = null;
    this.dialog = d;
  }

  /** Registers a store reset callback, run when another project gets selected. */
  onReset(fn: () => void): () => void {
    this.#resets.add(fn);
    return () => this.#resets.delete(fn);
  }

  /** Loads the list and follows `projectChanged`. Returns the cleanup. */
  start(): () => void {
    void this.refresh();
    let sub: { unsubscribe: () => void } | undefined;
    let stopped = false;
    let retry: ReturnType<typeof setTimeout> | undefined;
    // The server ends the stream when the client lags; resubscribe and refetch then.
    const listen = () => {
      sub = pipe(
        this.getClient().subscription(ProjectChangedDocument, {}),
        onEnd(() => {
          // Delay so a server that keeps rejecting the stream is not hammered.
          if (!stopped) retry = setTimeout(relisten, 1000);
        }),
        subscribe((r) => {
          if (!r.data) return;
          if (r.data.projectChanged.kind === 'SELECTED') {
            this.epoch++;
            for (const fn of this.#resets) fn();
          }
          void this.refresh();
        }),
      );
    };
    const relisten = () => {
      if (stopped) return;
      sub?.unsubscribe();
      listen();
      void this.refresh();
    };
    listen();
    let connected = false;
    const off = this.getWs()?.on('connected', () => {
      if (connected) void this.refresh();
      connected = true;
    });
    return () => {
      stopped = true;
      clearTimeout(retry);
      sub?.unsubscribe();
      off?.();
    };
  }

  async refresh(): Promise<void> {
    const n = ++this.#seq;
    const r = await this.getClient()
      .query(ProjectsDocument, {}, { requestPolicy: 'network-only' })
      .toPromise();
    if (n !== this.#seq) return; // a newer refresh superseded this one
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
        store.openDialog({ kind: 'create' });
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

/** Per-row actions; take the target project from the command context. */
export function projectRowCommands(store: ProjectStore): Command[] {
  const row = (
    id: string,
    title: string,
    run: (p: Project) => void | Promise<unknown>,
    when: (p: Project) => boolean = () => true,
  ): Command => ({
    id: `project.${id}`,
    title,
    group: 'Project',
    when: (ctx) => !!ctx.project && when(ctx.project),
    run: async (ctx) => {
      await run(ctx.project!);
    },
  });
  return [
    row(
      'select',
      'Select project',
      (p) => store.select(p.id),
      (p) => !p.isSelected,
    ),
    row('rename', 'Rename project…', (p) => store.openDialog({ kind: 'rename', project: p })),
    row('duplicate', 'Duplicate project', (p) => store.duplicate(p.id)),
    row(
      'setDefault',
      'Set project as default',
      (p) => store.setDefault(p.id),
      (p) => !p.isTemporary && !p.isDefault,
    ),
    row(
      'clearDefault',
      'Clear default project',
      () => store.setDefault(null),
      (p) => p.isDefault,
    ),
    row('delete', 'Delete project…', (p) => store.openDialog({ kind: 'delete', project: p })),
  ];
}
