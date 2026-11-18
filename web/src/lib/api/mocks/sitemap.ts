// Fake sitemap backend (T-094): derives the tree of internal/api/graphql/schema/sitemap.graphqls
// from the history mock's rows, so the Sitemap page works with no server. Node `query`
// texts are registered here so the history mock can filter by them (see nodeFilter).

import { GraphQLError } from 'graphql';
import type { MockRequest } from './history';

type Args = Record<string, unknown>;
type Pred = (r: MockRequest) => boolean;

export interface MockSitemapNode {
  id: string;
  parentId: string | null;
  kind: 'HOST' | 'DIRECTORY' | 'FILE' | 'QUERY';
  label: string;
  url: string;
  host: string;
  port: number;
  tls: boolean;
  requestCount: number;
  hasChildren: boolean;
  query: string;
}

const MAX_RECURSIVE = 5000;

// Node query text → predicate. Pure function of the text, so sharing it between mock instances is safe.
const registry = new Map<string, Pred>();

// shortcut: JSON.stringify escapes control characters as \uXXXX, which HTTPQL lacks; fine for mock rows.
const lit = (s: string) => JSON.stringify(s);
const reEsc = (s: string) => s.replace(/[.*+?^${}()|[\]\\]/g, '\\$&');
const segmentsOf = (path: string) => (path.startsWith('/') ? path.split('/').filter(Boolean) : []);
const namesOf = (query: string) =>
  [
    ...new Set(
      query
        .split('&')
        .filter(Boolean)
        .map((p) => p.split('=')[0]!),
    ),
  ].sort();

interface Desc {
  id: string;
  kind: 'HOST' | 'PATH' | 'QUERY';
  label: string;
  url: string;
  query: string;
}

/** The chain of nodes from the host down to the row's own node. */
function chain(r: MockRequest): Desc[] {
  const hostLabel = `${r.tls ? 'https' : 'http'}://${r.host.includes(':') ? `[${r.host}]` : r.host}:${r.port}`;
  const base = `req.tls.eq:${r.tls} and req.host.eq:${lit(r.host)} and req.port.eq:${r.port}`;
  const key = [r.tls, r.host, r.port];
  const out: Desc[] = [
    { id: JSON.stringify(key), kind: 'HOST', label: hostLabel, url: hostLabel, query: base },
  ];
  const segs = segmentsOf(r.path);
  segs.forEach((seg, i) => {
    const prefix = segs.slice(0, i + 1);
    out.push({
      id: JSON.stringify([...key, prefix]),
      kind: 'PATH',
      label: seg,
      url: `${hostLabel}/${prefix.join('/')}`,
      query: `${base} and req.path.regex:${lit(`^/+${prefix.map(reEsc).join('/+')}(/|$)`)}`,
    });
  });
  const names = namesOf(r.query);
  if (names.length) {
    const label = `?${names.join('&')}`;
    const exact = `^/*${segs.map(reEsc).join('/+')}/*$`;
    const all = `^&*(?:(?:${names.map(reEsc).join('|')})(?:=[^&]*)?(?:&+|$))+$`;
    out.push({
      id: JSON.stringify([...key, segs, names]),
      kind: 'QUERY',
      label,
      url: `${out.at(-1)!.url}${label}`,
      query:
        `${base} and req.path.regex:${lit(exact)}` +
        names.map((n) => ` and req.query.regex:${lit(`(^|&)${reEsc(n)}(=|&|$)`)}`).join('') +
        ` and req.query.regex:${lit(all)}`,
    });
  }
  for (const d of out) {
    if (!registry.has(d.query)) registry.set(d.query, (x) => chain(x).some((c) => c.id === d.id));
  }
  return out;
}

/**
 * Splits `(<node query>) and (<rest>)` as the Sitemap page builds it. Returns null when the
 * text does not start with a node query this mock produced.
 */
export function nodeFilter(text: string): { match: Pred; rest: string } | null {
  if (!text.startsWith('(')) return null;
  let depth = 0;
  let inStr = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (inStr) {
      if (c === '\\') i++;
      else if (c === '"') inStr = false;
    } else if (c === '"') inStr = true;
    else if (c === '(') depth++;
    else if (c === ')' && --depth === 0) {
      const match = registry.get(text.slice(1, i));
      if (!match) return null;
      const rest = text
        .slice(i + 1)
        .trim()
        .replace(/^and\s+/i, '')
        .replace(/^\(([\s\S]*)\)$/, '$1');
      return { match, rest };
    }
  }
  return null;
}

/** Sitemap root resolver over `rows`, filtered with the history mock's `matcher`. */
export function createSitemapMock(rows: MockRequest[], matcher: (query: unknown) => Pred) {
  const children = (parentId: string | null, list: MockRequest[]): MockSitemapNode[] => {
    const nodes = new Map<string, MockSitemapNode>();
    const order = new Map<MockSitemapNode, unknown[]>();
    for (const r of list) {
      const c = chain(r);
      const d = parentId === null ? -1 : c.findIndex((x) => x.id === parentId);
      const child = c[d + 1];
      if ((parentId !== null && d < 0) || !child) continue;
      const next = c[d + 2];
      let n = nodes.get(child.id);
      if (!n) {
        n = {
          id: child.id,
          parentId,
          kind: child.kind === 'PATH' ? 'FILE' : child.kind,
          label: child.label,
          url: child.url,
          host: r.host,
          port: r.port,
          tls: r.tls,
          requestCount: 0,
          hasChildren: false,
          query: child.query,
        };
        nodes.set(child.id, n);
        order.set(
          n,
          child.kind === 'HOST'
            ? [r.host, r.port, r.tls ? 1 : 0]
            : [child.kind === 'QUERY' ? 1 : 0, child.label],
        );
      }
      n.requestCount++;
      if (next) {
        n.hasChildren = true;
        if (n.kind === 'FILE' && next.kind === 'PATH') n.kind = 'DIRECTORY';
      }
    }
    const cmp = (a: unknown[], b: unknown[]) => {
      for (let i = 0; i < a.length; i++)
        if (a[i] !== b[i]) return (a[i] as never) < (b[i] as never) ? -1 : 1;
      return 0;
    };
    return [...nodes.values()].sort((a, b) => cmp(order.get(a)!, order.get(b)!));
  };

  return {
    root: {
      sitemapNodes({ parentId, query, recursive }: Args): MockSitemapNode[] {
        if (parentId != null) {
          try {
            if (!Array.isArray(JSON.parse(String(parentId)))) throw new Error();
          } catch {
            throw new GraphQLError(`malformed sitemap node id ${String(parentId)}`, {
              extensions: { code: 'BAD_INPUT' },
            });
          }
        }
        const list = rows.filter(matcher(query));
        const top = children((parentId as string | null) ?? null, list);
        if (!recursive) return top;
        const out: MockSitemapNode[] = [];
        const walk = (ns: MockSitemapNode[]) => {
          for (const n of ns) {
            out.push(n);
            if (out.length > MAX_RECURSIVE)
              throw new GraphQLError(`subtree has more than ${MAX_RECURSIVE} nodes`, {
                extensions: { code: 'BAD_INPUT' },
              });
            if (n.hasChildren) walk(children(n.id, list));
          }
        };
        walk(top);
        return out;
      },
    },
  };
}
