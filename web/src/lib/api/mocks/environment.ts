// Fake Environments backend for UI work before the real one lands (T-098).
// Resolvers follow internal/api/graphql/schema/environment.graphqls, including its error codes.

import { GraphQLError } from 'graphql';

type Args = Record<string, unknown>;

export interface MockEnvironmentVariable {
  name: string;
  value: string;
  secret: boolean;
}

export interface MockEnvironment {
  id: string;
  name: string;
  isGlobal: boolean;
  selected: boolean;
  variables: MockEnvironmentVariable[];
}

export const GLOBAL_ENV_ID = 'env-global';
const VAR_NAME = /^[A-Za-z_][A-Za-z0-9_.-]*$/;
const MAX_VALUE = 1024 * 1024;

const fail = (code: string, message: string) => new GraphQLError(message, { extensions: { code } });

/**
 * A fake environments backend: Global plus two project environments ("Staging" selected,
 * with one secret variable). `interpolate` applies the F-ENV-1 rules for the Replay mock.
 */
export function createEnvironmentMock() {
  const envs: MockEnvironment[] = [
    {
      id: GLOBAL_ENV_ID,
      name: 'Global',
      isGlobal: true,
      selected: false,
      variables: [{ name: 'userAgent', value: 'Mozilla/5.0 (mock)', secret: false }],
    },
    {
      id: 'env1',
      name: 'Staging',
      isGlobal: false,
      selected: true,
      variables: [
        { name: 'host', value: 'staging.example.test', secret: false },
        { name: 'token', value: 'mock-secret-token', secret: true },
      ],
    },
    {
      id: 'env2',
      name: 'Production',
      isGlobal: false,
      selected: false,
      variables: [{ name: 'host', value: 'www.example.com', secret: false }],
    },
  ];
  let nextId = 3;

  const find = (id: unknown): MockEnvironment => {
    const e = envs.find((x) => x.id === String(id));
    if (!e) throw fail('NOT_FOUND', `environment ${String(id)} not found`);
    return e;
  };
  const project = (id: unknown): MockEnvironment => {
    const e = find(id);
    if (e.isGlobal) throw fail('CONFLICT', 'the Global environment cannot be renamed or deleted');
    return e;
  };
  const checkName = (name: unknown, self?: MockEnvironment): string => {
    const n = String(name).trim();
    // eslint-disable-next-line no-control-regex -- T-098: names must not contain control characters
    if (!n || [...n].length > 200 || /[\u0000-\u001f\u007f]/.test(n))
      throw fail('BAD_INPUT', 'name must be 1-200 characters without control characters');
    if (n === 'Global' || envs.some((e) => e !== self && e.name === n))
      throw fail('CONFLICT', `an environment named ${JSON.stringify(n)} already exists`);
    return n;
  };
  const list = () => [envs[0]!, ...envs.slice(1).sort((a, b) => a.name.localeCompare(b.name))];

  const lookup = (name: string): string | undefined => {
    for (const e of [envs.find((x) => x.selected), envs[0]])
      for (const v of e?.variables ?? []) if (v.name === name) return v.value;
    return undefined;
  };

  return {
    /** `{{name}}` interpolation: selected environment, then Global; unknown names stay verbatim; one pass. */
    interpolate: (text: string): string =>
      text.replace(/\{\{\s*([A-Za-z_][A-Za-z0-9_.-]*)\s*\}\}/g, (m, n: string) => lookup(n) ?? m),
    root: {
      environments: list,
      environment: ({ id }: Args) => envs.find((e) => e.id === String(id)) ?? null,
      createEnvironment({ name }: Args) {
        const e = {
          id: `env${nextId++}`,
          name: checkName(name),
          isGlobal: false,
          selected: false,
          variables: [],
        };
        envs.push(e);
        return e;
      },
      renameEnvironment({ id, name }: Args) {
        const e = project(id);
        e.name = checkName(name, e);
        return e;
      },
      deleteEnvironment({ id }: Args) {
        const e = project(id);
        envs.splice(envs.indexOf(e), 1);
        return e.id;
      },
      setEnvironmentVariables({ id, variables }: Args) {
        const e = find(id);
        const vars = variables as MockEnvironmentVariable[];
        const seen = new Set<string>();
        for (const v of vars) {
          if (!VAR_NAME.test(v.name) || v.name.length > 100)
            throw fail('BAD_INPUT', `invalid variable name ${JSON.stringify(v.name)}`);
          if (seen.has(v.name))
            throw fail('BAD_INPUT', `duplicate variable name ${JSON.stringify(v.name)}`);
          if (new TextEncoder().encode(v.value).length > MAX_VALUE)
            throw fail('BAD_INPUT', `value of ${JSON.stringify(v.name)} is larger than 1 MiB`);
          seen.add(v.name);
        }
        e.variables = vars.map((v) => ({
          name: v.name,
          value: v.value,
          secret: v.secret ?? false,
        }));
        return e;
      },
      selectEnvironment({ id }: Args) {
        if (id == null) {
          for (const e of envs) e.selected = false;
          return null;
        }
        const e = find(id);
        if (e.isGlobal) throw fail('BAD_INPUT', 'Global is always used; pass null to select none');
        for (const x of envs) x.selected = x === e;
        return e;
      },
    },
  };
}
