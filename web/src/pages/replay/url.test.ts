import { describe, expect, it } from 'vitest';
import { buildSession } from './url';

describe('buildSession', () => {
  it('omits the default port from Host', () => {
    expect(buildSession({ tls: true, host: 'a.test', port: '', path: '/x?y=1' })).toEqual({
      connection: { host: 'a.test', port: 443, tls: true },
      raw: 'GET /x?y=1 HTTP/1.1\r\nHost: a.test\r\n\r\n',
    });
    expect(buildSession({ tls: false, host: 'a.test', port: '80', path: '' })).toMatchObject({
      raw: 'GET / HTTP/1.1\r\nHost: a.test\r\n\r\n',
    });
  });

  it('keeps a non-default port and brackets IPv6', () => {
    expect(buildSession({ tls: true, host: '::1', port: '8443', path: 'p' })).toMatchObject({
      raw: 'GET /p HTTP/1.1\r\nHost: [::1]:8443\r\n\r\n',
    });
  });

  it('rejects bad input', () => {
    expect(buildSession({ tls: true, host: '', port: '', path: '/' })).toMatch(/host/);
    expect(buildSession({ tls: true, host: 'a', port: '0', path: '/' })).toMatch(/Port/);
    expect(buildSession({ tls: true, host: 'a', port: '', path: '/a b' })).toMatch(/Path/);
  });
});
