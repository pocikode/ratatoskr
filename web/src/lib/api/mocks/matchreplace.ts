// Fake Match & Replace backend for UI work before the real one lands (T-099).
// Resolvers follow internal/api/graphql/schema/matchreplace.graphqls, including its error codes.
// Seed rules use reserved hosts only.

import { GraphQLError, coerceInputValue, type GraphQLInputType, type GraphQLSchema } from 'graphql';

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
const MAX_COLLECTIONS = 1000;
const MAX_TEXT = 1024 * 1024;
const MAX_CONDITION = 64 * 1024;
const MAX_PROJECT_TEXT = 3 * MAX_TEXT;
const MAX_TEST_RAW = 12 * MAX_TEXT;
const MAX_TEST_OUTPUT = 16 * MAX_TEXT;
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
  if (utf8Len(q) > MAX_CONDITION) throw fail('BAD_INPUT', 'condition must be at most 64 KiB');
  let open = -1;
  for (let i = 0; i < q.length; i++) {
    if (open >= 0 && q[i] === '\\') i++;
    else if (q[i] === '"') open = open < 0 ? i : -1;
  }
  if (open >= 0)
    throw fail('BAD_QUERY', 'unterminated string', {
      start: utf8Len(q.slice(0, open)),
      end: utf8Len(q),
    });
}

const ruleText = (r: MockRuleInput) =>
  utf8Len(r.condition) + utf8Len(r.matcher.value) + utf8Len(r.term);

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
  if (out.matcher.kind === 'FULL' && out.operation !== 'ADD') out.matcher.value = '';
  return out;
}

/**
 * Replaces every non-overlapping `v` (non-empty) with `term` literally, joining in batches so
 * temporary memory stays bounded: one `replaceAll` over 12 million matches peaked at ~600 MiB.
 */
function replaceBytes(text: string, v: string, term: string): string {
  const chunks: string[] = [];
  let parts: string[] = [];
  let last = 0;
  for (let i = text.indexOf(v); i >= 0; i = text.indexOf(v, last)) {
    parts.push(text.slice(last, i), term);
    last = i + v.length;
    if (parts.length >= 8192) {
      chunks.push(parts.join(''));
      parts = [];
    }
  }
  parts.push(text.slice(last));
  chunks.push(parts.join(''));
  return chunks.join('');
}

/**
 * Applies a rule to one message as byte strings.
 * shortcut: only STRING/FULL `UPDATE_RAW` on headers and bodies, plus header `ADD`; every other
 * rule leaves the message unchanged and conditions always match. Body rules skip messages with a
 * `Transfer-Encoding` or `Content-Encoding` header instead of rebuilding chunk framing or codings,
 * and bodyless responses (1xx, 204, 304).
 * The real tester is T-102 (on the T-035 engine).
 */
function applyRule(rule: MockRuleInput, raw: string): string {
  const cut = raw.indexOf('\r\n\r\n');
  if (cut < 0) return raw;
  const [start, ...headers] = raw.slice(0, cut).split('\r\n');
  let body = raw.slice(cut + 4);
  const term = toBytes(rule.term);
  const { kind, value } = rule.matcher;
  // Converted once: a large matcher re-encoded per header line made the tester quadratic.
  const v = kind === 'STRING' ? toBytes(value) : '';
  // Output cap is checked on the exact result size before any replaced text is built.
  let size = raw.length;
  const grow = (n: number) => {
    size += n;
    if (size > MAX_TEST_OUTPUT) throw fail('BAD_INPUT', 'tester output must be at most 16 MiB');
  };
  // Replaced length without building it; matches are counted, not stored (split() on 12 MiB is ~100 MiB).
  const measure = (text: string): number => {
    if (kind === 'FULL') return term.length;
    if (kind !== 'STRING') return text.length;
    let n = 0;
    for (let i = text.indexOf(v); i >= 0; i = text.indexOf(v, i + v.length)) n++;
    return text.length + n * (term.length - v.length);
  };
  const build = (text: string): string =>
    kind === 'FULL' ? term : kind === 'STRING' ? replaceBytes(text, v, term) : text;
  const isHeader = rule.section.endsWith('_HEADER');
  if (isHeader && rule.operation === 'ADD') {
    grow(term.length + 2);
    headers.push(term);
  } else if (isHeader && rule.operation === 'UPDATE_RAW') {
    // Two passes with no per-header plan objects: count, then replace in place.
    // Deltas are summed before the cap check: a shrinking later header offsets earlier growth.
    grow(headers.reduce((n, h) => n + measure(h) - h.length, 0));
    // Joined in batches so replaced lines do not all stay alive as separate strings.
    const done: string[] = [];
    for (let i = 0; i < headers.length; i += 8192)
      done.push(
        headers
          .slice(i, i + 8192)
          .map(build)
          .join('\r\n'),
      );
    headers.splice(0, headers.length, ...done);
  } else if (rule.section.endsWith('_BODY') && rule.operation === 'UPDATE_RAW') {
    if (headers.some((h) => /^(transfer|content)-encoding:/i.test(h))) return raw;
    // 1xx, 204 and 304 responses have no body on the wire (internal/rawhttp/read.go).
    if (/^HTTP\/\S+ (1\d\d|204|304)(\s|$)/.test(start!)) return raw;
    const isCL = (h: string) => /^content-length:/i.test(h);
    // Empty response declaring a non-zero length is a HEAD reply: it has no body to rewrite.
    if (
      start!.startsWith('HTTP/') &&
      !body &&
      headers.some((h) => isCL(h) && Number(h.slice(15)) !== 0)
    )
      return raw;
    const len = measure(body);
    const i = headers.findIndex(isCL);
    // One correct length replaces every Content-Length; a request gaining a body needs one too.
    const emit = i >= 0 || (!start!.startsWith('HTTP/') && len > 0);
    const cl = `Content-Length: ${len}`;
    // Body and rewritten framing are counted together: a shorter length header offsets body growth.
    let framing = emit ? cl.length + 2 : 0;
    for (const h of headers) if (isCL(h)) framing -= h.length + 2;
    grow(len - body.length + framing);
    const next = build(body);
    if (next === body) return raw;
    body = next;
    if (emit) {
      const kept = headers.filter((h) => !isCL(h));
      kept.splice(i >= 0 ? i : kept.length, 0, cl);
      headers.splice(0, headers.length);
      for (const h of kept) headers.push(h);
    }
  } else return raw;
  const head = headers.length ? `${start}\r\n${headers.join('\r\n')}` : start;
  return `${head}\r\n\r\n${body}`;
}

/**
 * A fake Match & Replace backend: two collections with four rules (one disabled) on reserved
 * hosts. The tester follows {@link applyRule}.
 */
export function createMatchReplaceMock(schema: GraphQLSchema) {
  const ruleInput = schema.getType('MatchReplaceRuleInput') as GraphQLInputType;
  // Exact keys at every level, then the schema's own coercion for types, enums and nested fields.
  const sameKeys = (o: unknown, keys: readonly string[]) =>
    !!o &&
    typeof o === 'object' &&
    !Array.isArray(o) &&
    Object.keys(o).length === keys.length &&
    keys.every((k) => Object.hasOwn(o, k));
  const importRule = (r: unknown): MockRuleInput => {
    // Coercion would wrap a lone source in a list and default a missing matcher value; exports never do.
    const exact =
      sameKeys(r, RULE_KEYS) &&
      Array.isArray((r as Args).sources) &&
      sameKeys((r as Args).matcher, ['kind', 'value']);
    const coerced = exact ? coerceInputValue(r, ruleInput) : undefined;
    if (coerced === undefined)
      throw fail('BAD_INPUT', 'unknown, missing or mistyped fields (see MatchReplaceRuleInput)');
    return normalize(coerced as Args);
  };
  const collections: MockCollection[] = [];
  let nextId = 1;

  const ruleCount = () => collections.reduce((n, c) => n + c.rules.length, 0);
  const projectText = () =>
    collections.reduce((n, c) => c.rules.reduce((m, r) => m + ruleText(r), n), 0);
  const checkBudget = (text: number, collectionCount = collections.length) => {
    if (collectionCount > MAX_COLLECTIONS)
      throw fail('BAD_INPUT', `a project holds at most ${MAX_COLLECTIONS} collections`);
    if (text > MAX_PROJECT_TEXT)
      throw fail('BAD_INPUT', 'rule conditions, matcher values and terms exceed 3 MiB per project');
  };
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
      createMatchReplaceCollection({ name }: Args) {
        const n = checkName(name);
        checkBudget(0, collections.length + 1);
        return addCollection(n);
      },
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
        checkBudget(projectText() + ruleText(r));
        return addRule(c, r);
      },
      updateMatchReplaceRule({ id, input }: Args) {
        const r = findRule(id);
        const next = normalize(input as Args);
        checkBudget(projectText() - ruleText(r) + ruleText(next));
        return Object.assign(r, next);
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
        let data: { version: unknown; collections: { name: unknown; rules: unknown[] }[] };
        try {
          data = JSON.parse(String(json));
        } catch (e) {
          throw fail('BAD_INPUT', `invalid JSON: ${(e as Error).message}`);
        }
        if (!sameKeys(data, ['version', 'collections']) || !Array.isArray(data.collections))
          throw fail('BAD_INPUT', 'expected exactly {"version":1,"collections":[…]}');
        if (data.version !== 1) throw fail('BAD_INPUT', 'unsupported export version');
        // Validate everything first: all or nothing.
        const parsed = data.collections.map((c, ci) => {
          if (
            !sameKeys(c, ['name', 'rules']) ||
            typeof c.name !== 'string' ||
            !Array.isArray(c.rules)
          )
            throw fail(
              'BAD_INPUT',
              `collection ${ci + 1}: expected exactly {"name":"…","rules":[…]}`,
            );
          const rules = c.rules.map((r, ri) => {
            try {
              return importRule(r);
            } catch (e) {
              throw fail(
                'BAD_INPUT',
                `collection ${ci + 1}, rule ${ri + 1}: ${(e as Error).message}`,
              );
            }
          });
          try {
            return { name: checkName(c.name), rules };
          } catch (e) {
            throw fail('BAD_INPUT', `collection ${ci + 1}: ${(e as Error).message}`);
          }
        });
        if (ruleCount() + parsed.reduce((n, c) => n + c.rules.length, 0) > MAX_RULES)
          throw fail('BAD_INPUT', `a project holds at most ${MAX_RULES} rules`);
        checkBudget(
          parsed.reduce((n, c) => c.rules.reduce((m, r) => m + ruleText(r), n), projectText()),
          collections.length + parsed.length,
        );
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
        // Match the Go Bytes scalar (StdEncoding): CR/LF ignored, other whitespace and missing padding rejected.
        const b64 = String(raw).replace(/[\r\n]/g, '');
        if (b64.length % 4 !== 0 || !/^[A-Za-z0-9+/]*={0,2}$/.test(b64))
          throw fail('BAD_INPUT', 'raw is not valid base64');
        try {
          text = atob(b64);
        } catch {
          throw fail('BAD_INPUT', 'raw is not valid base64');
        }
        if (text.length > MAX_TEST_RAW) throw fail('BAD_INPUT', 'raw must be at most 12 MiB');
        const line =
          kind === 'REQUEST'
            ? /^[!#$%&'*+.^_`|~0-9A-Za-z-]+ \S+ HTTP\/1\.[01]\r\n/
            : /^HTTP\/1\.[01] \d{3}(?: [^\r\n]*)?\r\n/;
        if (!line.test(text) || !text.includes('\r\n\r\n'))
          throw fail('BAD_INPUT', `raw is not an HTTP/1.x ${String(kind).toLowerCase()}`);
        // Lengths must be decimal and agree (internal/rawhttp/read.go contentLength), always.
        const sep = text.indexOf('\r\n\r\n');
        const fields = text.slice(0, sep).split('\r\n');
        const bodyLen = text.length - sep - 4;
        // Leading zeroes stripped so duplicates compare as exact decimal strings (no float rounding).
        const lens = fields
          .filter((h) => /^content-length:/i.test(h))
          .map((h) =>
            h
              .slice(15)
              // HTTP OWS only (space, tab): Unicode trim() would drop bytes such as 0xA0 that Go rejects.
              .replace(/^[ \t]+|[ \t]+$/g, '')
              .replace(/^0+(?=\d)/, ''),
          );
        if (lens.some((l) => !/^\d+$/.test(l) || l !== lens[0]))
          throw fail('BAD_INPUT', 'raw has an invalid or conflicting Content-Length');
        // A fixed-length body must be complete; a response with no body may keep any length (HEAD, 304).
        if (
          lens.length &&
          !fields.some((h) => /^transfer-encoding:/i.test(h)) &&
          (kind === 'REQUEST' || bodyLen > 0) &&
          Number(lens[0]) !== bodyLen
        )
          throw fail('BAD_INPUT', 'raw body length does not match Content-Length');
        const out = applyRule(draft, text);
        return { output: btoa(out), conditionMatched: true, changed: out !== text };
      },
    },
  };
}
