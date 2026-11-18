// Fake scope- and filter-preset backend for UI work before the real one lands (T-091).
// Resolvers follow internal/api/graphql/schema/{scope,filter}.graphqls, including their
// error codes. Rules use reserved names only (example.*, *.test): never real targets.

import { GraphQLError } from 'graphql';

type Args = Record<string, unknown>;

export interface MockScopePreset {
  id: string;
  name: string;
  allow: string[];
  deny: string[];
}

export interface MockFilterPreset {
  id: string;
  name: string;
  alias: string | null;
  query: string;
}

const MAX = 1000;

const fail = (code: string, message: string, span?: { start: number; end: number }) =>
  new GraphQLError(message, { extensions: span ? { code, span } : { code } });

function checkName(name: unknown): string {
  const n = String(name).trim();
  if (!n || [...n].length > 200) throw fail('BAD_INPUT', 'name must be 1-200 characters');
  return n;
}

// shortcut: checks only host presence and port range of `host[:port][/path]`; the real
// parser (internal/scope.ParseRule) also validates IPv6 brackets and IDN hosts.
function checkRules(list: unknown): string[] {
  const rules = (list as string[] | undefined) ?? [];
  if (rules.length > MAX) throw fail('BAD_INPUT', `at most ${MAX} entries per list`);
  for (const r of rules) {
    const m = /^\s*(\[[^\]]*\]|[^:/\s]+)(?::(\d+))?(\/\S*)?\s*$/.exec(r);
    const port = m?.[2] === undefined ? 1 : Number(m[2]);
    if (!m || port < 1 || port > 65535)
      throw fail('BAD_INPUT', `scope entry ${JSON.stringify(r)}: invalid`);
  }
  return [...rules];
}

/** Reorders `items` to `ids`, which must name every item exactly once. */
function reorder<T extends { id: string }>(items: T[], ids: unknown): T[] {
  const want = (ids as string[]).map(String);
  const byId = new Map(items.map((p) => [p.id, p]));
  if (
    want.length !== items.length ||
    new Set(want).size !== want.length ||
    !want.every((id) => byId.has(id))
  )
    throw fail('BAD_INPUT', 'ids must list every preset exactly once');
  return want.map((id) => byId.get(id)!);
}

const PRESET_REF = /preset:(?:"((?:[^"\\]|\\.)*)"|([a-z0-9][a-z0-9_-]*))/g;

/**
 * Throws BAD_QUERY (span into `p.query`) when `p` references a missing preset, a cycle,
 * or nests deeper than 8. shortcut: does not parse HTTPQL itself, only `preset:` refs.
 */
function checkRefs(p: MockFilterPreset, all: MockFilterPreset[]): void {
  const find = (ref: string) => all.find((o) => o.name === ref) ?? all.find((o) => o.alias === ref);
  const walk = (
    q: MockFilterPreset,
    path: string[],
    top: { start: number; end: number } | null,
  ) => {
    for (const m of q.query.matchAll(PRESET_REF)) {
      const span = top ?? { start: m.index, end: m.index + m[0].length };
      const ref = m[1] !== undefined ? m[1].replace(/\\(.)/g, '$1') : m[2]!;
      const next = find(ref);
      if (!next) throw fail('BAD_QUERY', `unknown preset ${JSON.stringify(ref)}`, span);
      if (path.includes(next.id))
        throw fail('BAD_QUERY', `preset ${JSON.stringify(ref)} refers to itself (cycle)`, span);
      if (path.length >= 8) throw fail('BAD_QUERY', 'presets nested deeper than 8 levels', span);
      walk(next, [...path, next.id], span);
    }
  };
  walk(p, [p.id], null);
}

const SEED_SCOPES: Omit<MockScopePreset, 'id'>[] = [
  { name: 'Example app', allow: ['*.example.com', 'example.com'], deny: ['cdn.example.com'] },
  { name: 'API only', allow: ['api.example.com/api/v1'], deny: [] },
  { name: 'Shop (TLS)', allow: ['shop.example.org:443'], deny: ['shop.example.org/static'] },
];

const SEED_FILTERS: Omit<MockFilterPreset, 'id'>[] = [
  { name: 'No assets', alias: 'no-assets', query: 'NOT req.ext.regex:"^(js|css|png|svg|woff2?)$"' },
  { name: 'Errors', alias: 'errors', query: 'resp.code.gte:400' },
  { name: 'API errors', alias: null, query: 'req.path.cont:"/api/" AND preset:errors' },
];

/**
 * A fake preset backend: root resolvers for graphql-js. Each instance has its own
 * state, so tests do not share it.
 */
export function createPresetsMock() {
  let nextId = 1;
  let scopes: MockScopePreset[] = SEED_SCOPES.map((s) => ({ ...s, id: String(nextId++) }));
  let filters: MockFilterPreset[] = SEED_FILTERS.map((f) => ({ ...f, id: String(nextId++) }));

  const findIn = <T extends { id: string }>(list: T[], id: unknown, kind: string): T => {
    const p = list.find((x) => x.id === String(id));
    if (!p) throw fail('NOT_FOUND', `${kind} ${String(id)} not found`);
    return p;
  };

  const scopeFrom = (a: Args, id: string): MockScopePreset => {
    const p = { id, name: checkName(a.name), allow: checkRules(a.allow), deny: checkRules(a.deny) };
    if (scopes.some((o) => o.id !== id && o.name === p.name))
      throw fail('CONFLICT', `a scope named ${JSON.stringify(p.name)} already exists`);
    return p;
  };

  const filterFrom = (a: Args, id: string, others: MockFilterPreset[]): MockFilterPreset => {
    const alias = typeof a.alias === 'string' && a.alias.trim() ? a.alias.trim() : null;
    if (alias !== null && !/^[a-z0-9][a-z0-9_-]*$/.test(alias))
      throw fail(
        'BAD_INPUT',
        `alias ${JSON.stringify(alias)} must be lowercase letters, digits, - or _`,
      );
    const query = String(a.query).trim();
    if (!query) throw fail('BAD_INPUT', 'preset query must not be empty');
    const p = { id, name: checkName(a.name), alias, query };
    for (const o of others) {
      if (o.id === id) continue;
      if (o.name === p.name)
        throw fail('CONFLICT', `a preset named ${JSON.stringify(p.name)} already exists`);
      if (alias && o.alias === alias)
        throw fail('CONFLICT', `alias ${JSON.stringify(alias)} is already used`);
    }
    return p;
  };

  const replaceFilter = (p: MockFilterPreset) => {
    const all = filters.some((o) => o.id === p.id)
      ? filters.map((o) => (o.id === p.id ? p : o))
      : [...filters, p];
    checkRefs(p, all);
    filters = all;
    return p;
  };

  return {
    root: {
      scopePresets: () => scopes,
      scopePreset: ({ id }: Args) => scopes.find((p) => p.id === String(id)) ?? null,
      createScopePreset(a: Args) {
        const p = scopeFrom(a, String(nextId));
        nextId++;
        scopes = [...scopes, p];
        return p;
      },
      updateScopePreset(a: Args) {
        findIn(scopes, a.id, 'scope');
        const p = scopeFrom(a, String(a.id));
        scopes = scopes.map((o) => (o.id === p.id ? p : o));
        return p;
      },
      duplicateScopePreset({ id }: Args) {
        const src = findIn(scopes, id, 'scope');
        const base = [...src.name].slice(0, 150).join('');
        let name = `${base} (copy)`;
        for (let i = 2; scopes.some((o) => o.name === name); i++) name = `${base} (copy ${i})`;
        const p = { id: String(nextId++), name, allow: [...src.allow], deny: [...src.deny] };
        scopes = [...scopes, p];
        return p;
      },
      deleteScopePreset({ id }: Args) {
        findIn(scopes, id, 'scope');
        scopes = scopes.filter((o) => o.id !== String(id));
        return String(id);
      },
      reorderScopePresets({ ids }: Args) {
        return (scopes = reorder(scopes, ids));
      },

      filterPresets: () => filters,
      filterPreset: ({ id }: Args) => filters.find((p) => p.id === String(id)) ?? null,
      exportFilterPresets: () =>
        JSON.stringify({
          version: 1,
          presets: filters.map(({ name, alias, query }) =>
            alias ? { name, alias, query } : { name, query },
          ),
        }),
      createFilterPreset(a: Args) {
        if (filters.length >= MAX) throw fail('BAD_INPUT', `at most ${MAX} presets`);
        const p = replaceFilter(filterFrom(a, String(nextId), filters));
        nextId++;
        return p;
      },
      updateFilterPreset(a: Args) {
        findIn(filters, a.id, 'filter preset');
        return replaceFilter(filterFrom(a, String(a.id), filters));
      },
      deleteFilterPreset({ id }: Args) {
        findIn(filters, id, 'filter preset');
        filters = filters.filter((o) => o.id !== String(id));
        return String(id);
      },
      reorderFilterPresets({ ids }: Args) {
        return (filters = reorder(filters, ids));
      },
      importFilterPresets({ json }: Args) {
        let file: { version?: unknown; presets?: unknown };
        try {
          file = JSON.parse(String(json)) as typeof file;
        } catch (e) {
          throw fail('BAD_INPUT', `invalid preset file: ${String(e)}`);
        }
        if (file?.version !== 1)
          throw fail('BAD_INPUT', `unsupported preset file version ${String(file?.version)}`);
        if (!Array.isArray(file.presets))
          throw fail('BAD_INPUT', 'invalid preset file: presets must be a list');
        if (file.presets.length > MAX) throw fail('BAD_INPUT', `at most ${MAX} presets per file`);
        // All or nothing: build the new list, validate, then commit.
        const all = [...filters];
        const added: MockFilterPreset[] = [];
        let id = nextId;
        for (const raw of file.presets as Args[]) {
          const name = checkName(raw.name);
          if (all.some((o) => o.name === name)) continue;
          if (all.length >= MAX) throw fail('BAD_INPUT', `at most ${MAX} presets per project`);
          const p = filterFrom(raw, String(id++), all);
          all.push(p);
          added.push(p);
        }
        for (const p of added) checkRefs(p, all);
        filters = all;
        nextId = id;
        return added.length;
      },
    },
  };
}
