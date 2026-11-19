// Fake Match & Replace backend for UI work before the real one lands (T-099).
// Resolvers follow internal/api/graphql/schema/matchreplace.graphqls, including its error codes.
// Seed rules use reserved hosts only.

import { GraphQLError } from 'graphql';

type Args = Record<string, unknown>;

export interface MockMatcher {
  kind: 'FULL' | 'STRING' | 'REGEX' | 'NAME';
  value: string;
}

export interface MockRuleInput {
  name: string;
  enabled: boolean;
  condition: string;
  sources: string[];
  section: string;
  operation: string;
  matcher: MockMatcher;
  term: string;
}

export interface MockRule extends MockRuleInput {
  id: string;
  collection: () => MockCollection;
}

export interface MockCollection {
  id: string;
  name: string;
  rules: MockRule[];
}

const MAX_RULES = 1000;
const MAX_TEXT = 1024 * 1024;
const OPS: Record<string, string[]> = {
  REQUEST_FIRST_LINE: ['UPDATE_RAW'],
  REQUEST_METHOD: ['UPDATE'],
  REQUEST_PATH: ['UPDATE_RAW'],
  REQUEST_QUERY: ['UPDATE_RAW', 'UPDATE_VALUE', 'ADD', 'REMOVE'],
  REQUEST_HEADER: ['UPDATE_RAW', 'UPDATE_VALUE', 'ADD', 'REMOVE'],
  REQUEST_BODY: ['UPDATE_RAW'],
  RESPONSE_FIRST_LINE: ['UPDATE_RAW'],
  RESPONSE_STATUS_CODE: ['UPDATE'],
  RESPONSE_HEADER: ['UPDATE_RAW', 'UPDATE_VALUE', 'ADD', 'REMOVE'],
  RESPONSE_BODY: ['UPDATE_RAW'],
};
const RULE_KEYS = [
  'name',
  'enabled',
  'condition',
  'sources',
  'section',
  'operation',
  'matcher',
  'term',
] as const;

const fail = (code: string, message: string, span?: { start: number; end: number }) =>
  new GraphQLError(message, { extensions: span ? { code, span } : { code } });
const utf8Len = (s: string) => new TextEncoder().encode(s).length;
const toBytes = (s: string) =>
  Array.from(new TextEncoder().encode(s), (b) => String.fromCharCode(b)).join('');

function checkName(name: unknown): string {
  const n = String(name).trim();
  // eslint-disable-next-line no-control-regex -- T-099: names must not contain control characters
  if (!n || [...n].length > 200 || /[\u0000-\u001f\u007f]/.test(n))
    throw fail('BAD_INPUT', 'name must be 1-200 characters without control characters');
  return n;
}

// shortcut: only flags an unterminated string literal; the real backend parses HTTPQL.
function checkCondition(q: string): void {
  const quotes = [...q.matchAll(/"/g)];
  if (quotes.length % 2)
    throw fail('BAD_QUERY', 'unterminated string', { start: quotes.at(-1)!.index, end: q.length });
}

/** Validates everything but name, enabled and sources (the tester skips those). */
function checkRuleBody(r: MockRuleInput): void {
  const ops = OPS[r.section];
  if (!ops?.includes(r.operation))
    throw fail('BAD_INPUT', `operation ${r.operation} is not valid for section ${r.section}`);
  const { kind, value } = r.matcher;
  if (r.operation !== 'ADD') {
    if (kind === 'NAME' && r.operation !== 'UPDATE_VALUE' && r.operation !== 'REMOVE')
      throw fail('BAD_INPUT', `matcher NAME is not allowed with ${r.operation}`);
    if (kind !== 'FULL' && !value) throw fail('BAD_INPUT', `matcher ${kind} needs a value`);
    if (kind === 'REGEX') {
      // JS regex stands in for RE2; look-around and backreferences are rejected as RE2 would.
      if (/\(\?<?[=!]|\\[1-9]/.test(value))
        throw fail('BAD_INPUT', `invalid regex: invalid or unsupported Perl syntax`);
      try {
        new RegExp(value);
      } catch (e) {
        throw fail('BAD_INPUT', `invalid regex: ${(e as Error).message}`);
      }
    }
  }
  if (utf8Len(r.term) > MAX_TEXT || utf8Len(value) > MAX_TEXT)
    throw fail('BAD_INPUT', 'term and matcher value must be at most 1 MiB');
  if (
    r.operation === 'ADD' &&
    r.section.endsWith('_HEADER') &&
    !/^[!#$%&'*+.^_`|~0-9A-Za-z-]+:/.test(r.term)
  )
    throw fail('BAD_INPUT', 'an added header must be written as "Name: value"');
  checkCondition(r.condition);
}

function normalize(input: Args): MockRuleInput {
  const r = input as unknown as MockRuleInput;
  const name = checkName(r.name);
  if (!Array.isArray(r.sources) || r.sources.length === 0)
    throw fail('BAD_INPUT', 'sources must not be empty');
  const out: MockRuleInput = {
    name,
    enabled: r.enabled ?? true,
    condition: r.condition ?? '',
    sources: [...new Set(r.sources)],
    section: r.section,
    operation: r.operation,
    matcher: { kind: r.matcher.kind, value: r.matcher.value ?? '' },
    term: r.term ?? '',
  };
  checkRuleBody(out);
  return out;
}

/**
 * Applies a rule to one message as byte strings.
 * shortcut: only STRING/FULL `UPDATE_RAW` on headers and bodies, plus header `ADD`; every other
 * rule leaves the message unchanged and conditions always match. The real tester is T-102.
 */
function applyRule(rule: MockRuleInput, raw: string): string {
  const cut = raw.indexOf('\r\n\r\n');
  if (cut < 0) return raw;
  const [start, ...headers] = raw.slice(0, cut).split('\r\n');
  let body = raw.slice(cut + 4);
  const term = toBytes(rule.term);
  const { kind, value } = rule.matcher;
  const replace = (text: string) =>
    kind === 'FULL' ? term : kind === 'STRING' ? text.split(toBytes(value)).join(term) : text;
  const isHeader = rule.section.endsWith('_HEADER');
  if (isHeader && rule.operation === 'ADD') headers.push(term);
  else if (isHeader && rule.operation === 'UPDATE_RAW')
    headers.splice(0, Infinity, ...headers.map(replace));
  else if (rule.section.endsWith('_BODY') && rule.operation === 'UPDATE_RAW') {
    const next = replace(body);
    if (next === body) return raw;
    body = next;
    const i = headers.findIndex((h) => /^content-length:/i.test(h));
    if (i >= 0) headers[i] = `Content-Length: ${body.length}`;
  } else return raw;
  return [start, ...headers].join('\r\n') + '\r\n\r\n' + body;
}

/**
 * A fake Match & Replace backend: two collections with four rules (one disabled) on reserved
 * hosts. The tester follows {@link applyRule}.
 */
export function createMatchReplaceMock() {
  const collections: MockCollection[] = [];
  let nextId = 1;

  const ruleCount = () => collections.reduce((n, c) => n + c.rules.length, 0);
  const addCollection = (name: string): MockCollection => {
    const c: MockCollection = { id: `mrc${nextId++}`, name, rules: [] };
    collections.push(c);
    return c;
  };
  const addRule = (c: MockCollection, input: MockRuleInput): MockRule => {
    const r: MockRule = {
      ...input,
      id: `mrr${nextId++}`,
      collection: () => collections.find((x) => x.rules.includes(r))!,
    };
    c.rules.push(r);
    return r;
  };
  const findCollection = (id: unknown): MockCollection => {
    const c = collections.find((x) => x.id === String(id));
    if (!c) throw fail('NOT_FOUND', `collection ${String(id)} not found`);
    return c;
  };
  const findRule = (id: unknown): MockRule => {
    for (const c of collections) for (const r of c.rules) if (r.id === String(id)) return r;
    throw fail('NOT_FOUND', `rule ${String(id)} not found`);
  };
  const seed = (section: string, operation: string, rest: Partial<MockRuleInput>) =>
    ({
      enabled: true,
      condition: '',
      sources: ['PROXY', 'REPLAY'],
      section,
      operation,
      matcher: { kind: 'FULL', value: '' },
      term: '',
      ...rest,
    }) as MockRuleInput;

  const headers = addCollection('Headers');
  addRule(headers, seed('REQUEST_HEADER', 'ADD', { name: 'Add debug header', term: 'X-Debug: 1' }));
  addRule(
    headers,
    seed('REQUEST_HEADER', 'UPDATE_VALUE', {
      name: 'Mock user agent',
      condition: 'req.host.eq:"www.example.com"',
      matcher: { kind: 'NAME', value: 'User-Agent' },
      term: 'Mozilla/5.0 (mock)',
    }),
  );
  const bodies = addCollection('Bodies');
  addRule(
    bodies,
    seed('RESPONSE_BODY', 'UPDATE_RAW', {
      name: 'Rewrite staging links',
      sources: ['PROXY'],
      matcher: { kind: 'STRING', value: 'staging.example.test' },
      term: 'www.example.com',
    }),
  );
  addRule(
    bodies,
    seed('RESPONSE_STATUS_CODE', 'UPDATE', {
      name: 'Force 200',
      enabled: false,
      matcher: { kind: 'REGEX', value: '^4\\d\\d$' },
      term: '200',
    }),
  );

  const exportJSON = () =>
    JSON.stringify({
      version: 1,
      collections: collections.map((c) => ({
        name: c.name,
        rules: c.rules.map((r) => Object.fromEntries(RULE_KEYS.map((k) => [k, r[k]]))),
      })),
    });

  return {
    root: {
      matchReplaceCollections: () => collections,
      matchReplaceRule({ id }: Args) {
        try {
          return findRule(id);
        } catch {
          return null;
        }
      },
      exportMatchReplace: exportJSON,
      createMatchReplaceCollection: ({ name }: Args) => addCollection(checkName(name)),
      renameMatchReplaceCollection({ id, name }: Args) {
        const c = findCollection(id);
        c.name = checkName(name);
        return c;
      },
      deleteMatchReplaceCollection({ id }: Args) {
        const c = findCollection(id);
        collections.splice(collections.indexOf(c), 1);
        return c.id;
      },
      reorderMatchReplaceCollections({ ids }: Args) {
        const list = (ids as string[]).map(String);
        const byId = new Map(collections.map((c) => [c.id, c]));
        if (
          list.length !== collections.length ||
          new Set(list).size !== list.length ||
          !list.every((id) => byId.has(id))
        )
          throw fail('BAD_INPUT', 'ids must list every collection exactly once');
        collections.splice(0, Infinity, ...list.map((id) => byId.get(id)!));
        return collections;
      },
      createMatchReplaceRule({ collectionId, input }: Args) {
        const c = findCollection(collectionId);
        const r = normalize(input as Args);
        if (ruleCount() >= MAX_RULES)
          throw fail('BAD_INPUT', `a project holds at most ${MAX_RULES} rules`);
        return addRule(c, r);
      },
      updateMatchReplaceRule({ id, input }: Args) {
        const r = findRule(id);
        return Object.assign(r, normalize(input as Args));
      },
      setMatchReplaceRuleEnabled({ id, enabled }: Args) {
        const r = findRule(id);
        r.enabled = Boolean(enabled);
        return r;
      },
      moveMatchReplaceRule({ id, collectionId, index }: Args) {
        const r = findRule(id);
        const to = findCollection(collectionId);
        if (Number(index) < 0) throw fail('BAD_INPUT', 'index must not be negative');
        const from = r.collection();
        from.rules.splice(from.rules.indexOf(r), 1);
        to.rules.splice(Number(index), 0, r);
        return r;
      },
      deleteMatchReplaceRule({ id }: Args) {
        const r = findRule(id);
        const c = r.collection();
        c.rules.splice(c.rules.indexOf(r), 1);
        return r.id;
      },
      importMatchReplace({ json }: Args) {
        let data: { version?: unknown; collections?: { name: unknown; rules: Args[] }[] };
        try {
          data = JSON.parse(String(json));
        } catch (e) {
          throw fail('BAD_INPUT', `invalid JSON: ${(e as Error).message}`);
        }
        if (data?.version !== 1) throw fail('BAD_INPUT', 'unsupported export version');
        if (!Array.isArray(data.collections)) throw fail('BAD_INPUT', 'collections is missing');
        // Validate everything first: all or nothing.
        const parsed = data.collections.map((c, ci) => {
          try {
            return { name: checkName(c.name), rules: (c.rules ?? []).map((r) => normalize(r)) };
          } catch (e) {
            throw fail('BAD_INPUT', `collection ${ci + 1}: ${(e as Error).message}`);
          }
        });
        if (ruleCount() + parsed.reduce((n, c) => n + c.rules.length, 0) > MAX_RULES)
          throw fail('BAD_INPUT', `a project holds at most ${MAX_RULES} rules`);
        for (const c of parsed) {
          const added = addCollection(c.name);
          for (const r of c.rules) addRule(added, r);
        }
        return parsed.length;
      },
      testMatchReplace({ rule, kind, raw }: Args) {
        const r = rule as unknown as MockRuleInput;
        const draft = { ...r, condition: r.condition ?? '', term: r.term ?? '' };
        draft.matcher = { kind: r.matcher.kind, value: r.matcher.value ?? '' };
        checkRuleBody(draft);
        if (!draft.section.startsWith(`${String(kind)}_`))
          throw fail(
            'BAD_INPUT',
            `section ${draft.section} cannot be tested on a ${String(kind).toLowerCase()}`,
          );
        let text: string;
        try {
          text = atob(String(raw));
        } catch {
          throw fail('BAD_INPUT', 'raw is not valid base64');
        }
        const line =
          kind === 'REQUEST' ? /^[A-Za-z]+ \S+ HTTP\/1\.[01]\r\n/ : /^HTTP\/1\.[01] \d{3}[ \r]/;
        if (!line.test(text) || !text.includes('\r\n\r\n'))
          throw fail('BAD_INPUT', `raw is not an HTTP/1.x ${String(kind).toLowerCase()}`);
        const out = applyRule(draft, text);
        return { output: btoa(out), conditionMatched: true, changed: out !== text };
      },
    },
  };
}
