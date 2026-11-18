import { describe, expect, it } from 'vitest';
import { formParams, parseMessage, queryParams } from './parse';

const request =
  'POST /v2/login?next=%2Fhome&retry=2 HTTP/1.1\r\n' +
  'Host: api.acme.test\r\n' +
  'Content-Type: application/x-www-form-urlencoded\r\n' +
  'X-Long: first\r\n' +
  '  second\r\n' +
  '\r\n' +
  'user=admin&pass=s%26cret';

describe('parseMessage', () => {
  it('splits start line, headers and body', () => {
    const parsed = parseMessage(request);
    expect(parsed.startLine).toBe('POST /v2/login?next=%2Fhome&retry=2 HTTP/1.1');
    expect(parsed.body).toBe('user=admin&pass=s%26cret');
    expect(parsed.contentType).toBe('application/x-www-form-urlencoded');
    expect(parsed.headers).toHaveLength(3);
  });

  it('unfolds folded header values', () => {
    expect(parseMessage(request).headers.find((h) => h.name === 'X-Long')?.value).toBe(
      'first second',
    );
  });

  it('accepts bare LF separators and headerless messages', () => {
    const parsed = parseMessage('HTTP/1.1 200 OK\nContent-Length: 0\n\n');
    expect(parsed.startLine).toBe('HTTP/1.1 200 OK');
    expect(parsed.body).toBe('');
    expect(parseMessage('just a body').body).toBe('');
  });

  it('reads query and form parameters', () => {
    expect(queryParams('GET /a?b=1&c=two HTTP/1.1')).toEqual([
      { name: 'b', value: '1' },
      { name: 'c', value: 'two' },
    ]);
    expect(queryParams('GET /a HTTP/1.1')).toEqual([]);
    expect(formParams(parseMessage(request))).toEqual([
      { name: 'user', value: 'admin' },
      { name: 'pass', value: 's&cret' },
    ]);
  });
});
