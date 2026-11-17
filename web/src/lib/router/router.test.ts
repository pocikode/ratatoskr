import { describe, expect, it } from 'vitest';
import { buildHash, parseHash } from './router.svelte';
import { routes } from './routes';

describe('hash router', () => {
  it('defaults to sitemap on an empty hash', () => {
    expect(parseHash('')).toEqual({ page: 'sitemap', params: [] });
    expect(parseHash('#/')).toEqual({ page: 'sitemap', params: [] });
  });

  it('parses page and params', () => {
    expect(parseHash('#/replay/session/42')).toEqual({ page: 'replay', params: ['session', '42'] });
  });

  it('ignores a query string and decodes segments', () => {
    expect(parseHash('#/search/a%20b?x=1')).toEqual({ page: 'search', params: ['a b'] });
  });

  it('round-trips through buildHash', () => {
    expect(buildHash('replay', 'session', 42)).toBe('#/replay/session/42');
    expect(parseHash(buildHash('search', 'a b/c'))).toEqual({ page: 'search', params: ['a b/c'] });
  });

  it('has unique route ids', () => {
    expect(new Set(routes.map((r) => r.id)).size).toBe(routes.length);
  });
});
