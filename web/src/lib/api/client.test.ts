import { describe, expect, it, vi } from 'vitest';
import { API_HEADER, createApi, isUnauthenticatedClose, wsUrl } from './client';
import { ServerInfoDocument } from './generated/graphql';

describe('wsUrl', () => {
  it('follows the page protocol and host', () => {
    expect(wsUrl({ protocol: 'http:', host: '127.0.0.1:8080' })).toBe(
      'ws://127.0.0.1:8080/api/graphql/ws',
    );
    expect(wsUrl({ protocol: 'https:', host: 'proxy.lan' })).toBe('wss://proxy.lan/api/graphql/ws');
  });
});

describe('isUnauthenticatedClose', () => {
  it('matches only close code 4401', () => {
    expect(isUnauthenticatedClose({ code: 4401 })).toBe(true);
    expect(isUnauthenticatedClose({ code: 1006 })).toBe(false);
    expect(isUnauthenticatedClose(new Error('x'))).toBe(false);
    expect(isUnauthenticatedClose(null)).toBe(false);
  });
});

describe('createApi', () => {
  it('sends queries as JSON POST with the CSRF header', async () => {
    const fetch = vi.fn(
      async () =>
        new Response(
          JSON.stringify({
            data: {
              serverInfo: {
                version: 'dev',
                commit: 'unknown',
                goVersion: 'go1.27.4',
                startedAt: '2026-11-18T00:00:00Z',
                authRequired: false,
              },
            },
          }),
          { headers: { 'Content-Type': 'application/json' } },
        ),
    );
    const { client, ws } = createApi({ wsUrl: 'ws://127.0.0.1:1/api/graphql/ws', fetch });

    const result = await client.query(ServerInfoDocument, {}).toPromise();
    await ws.dispose();

    expect(result.data?.serverInfo.version).toBe('dev');
    expect(fetch).toHaveBeenCalledOnce();
    const [url, init] = fetch.mock.calls[0] as unknown as [string, RequestInit];
    expect(url).toBe('/api/graphql');
    expect(init.method).toBe('POST');
    const headers = new Headers(init.headers);
    expect(headers.get(API_HEADER)).toBe('1');
    expect(headers.get('Content-Type')).toBe('application/json');
  });
});
