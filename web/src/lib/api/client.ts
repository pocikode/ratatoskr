// The one GraphQL client of the SPA (docs/06-frontend-conventions.md §4).
// Queries and mutations go over HTTP POST with the CSRF header, subscriptions over
// graphql-transport-ws (docs/03-architecture.md §6, §8).
import { Client, cacheExchange, fetchExchange, subscriptionExchange } from '@urql/core';
import { createClient as createWsClient, type Client as WsClient } from 'graphql-ws';

export const GRAPHQL_PATH = '/api/graphql';
export const GRAPHQL_WS_PATH = '/api/graphql/ws';
/** Header the server requires on every GraphQL HTTP request (CSRF defence). */
export const API_HEADER = 'X-Ratatoskr';
/** WebSocket close code the server uses for a missing or wrong token. */
export const CLOSE_UNAUTHENTICATED = 4401;
/** How often the client pings the server to notice a dead connection. */
const KEEP_ALIVE_MS = 10_000;

export interface Api {
  client: Client;
  ws: WsClient;
}

export function wsUrl(loc: Pick<Location, 'protocol' | 'host'> = window.location): string {
  return `${loc.protocol === 'https:' ? 'wss:' : 'ws:'}//${loc.host}${GRAPHQL_WS_PATH}`;
}

export function isUnauthenticatedClose(e: unknown): boolean {
  return typeof e === 'object' && e !== null && 'code' in e && e.code === CLOSE_UNAUTHENTICATED;
}

export interface ApiOptions {
  /** WebSocket URL; defaults to {@link wsUrl} of the current page. */
  wsUrl?: string;
  /** fetch implementation, for tests. */
  fetch?: typeof fetch;
}

/** Builds a client pair. The WebSocket opens lazily, on the first subscription. */
export function createApi(opts: ApiOptions = {}): Api {
  const ws = createWsClient({
    url: opts.wsUrl ?? wsUrl(),
    keepAlive: KEEP_ALIVE_MS,
    retryAttempts: Infinity,
    // Retrying with the same (missing) credentials cannot succeed.
    shouldRetry: (e) => !isUnauthenticatedClose(e),
  });

  const client = new Client({
    url: GRAPHQL_PATH,
    // The server only serves POST; GET queries are not CSRF-safe.
    preferGetMethod: false,
    fetch: opts.fetch,
    fetchOptions: { headers: { [API_HEADER]: '1' } },
    exchanges: [
      cacheExchange,
      fetchExchange,
      subscriptionExchange({
        forwardSubscription: (request) => ({
          subscribe: (sink) => ({
            unsubscribe: ws.subscribe({ ...request, query: request.query ?? '' }, sink),
          }),
        }),
      }),
    ],
  });

  return { client, ws };
}

let api: Api | undefined;

/** The shared client pair, created on first use. */
export function getApi(): Api {
  api ??= createApi();
  return api;
}
