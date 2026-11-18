import { describe, expect, it } from 'vitest';
import {
  colorLabel,
  insertRow,
  requestCurl,
  requestUrl,
  rowColor,
  type HistoryRow,
} from './columns';

function row(id: number, overrides: Partial<HistoryRow> = {}): HistoryRow {
  return {
    id: String(id),
    source: 'PROXY',
    host: 'api.example.com',
    port: 443,
    tls: true,
    method: 'GET',
    path: '/v1/users',
    query: '',
    ext: '',
    length: '120',
    createdAt: '2026-11-18T09:00:00Z',
    color: null,
    note: null,
    isDropped: false,
    isTruncated: false,
    editedFrom: null,
    response: { code: 200, length: '80', roundtripMs: 12, createdAt: '2026-11-18T09:00:00Z' },
    ...overrides,
  };
}

describe('insertRow', () => {
  const rows = [row(3), row(2), row(1)]; // ID descending, like the default sort

  it('places a live row by the current sort', () => {
    expect(insertRow(rows, row(4), { columnId: 'id', direction: 'desc' }).map((r) => r.id)).toEqual(
      ['4', '3', '2', '1'],
    );
    expect(insertRow(rows, row(0), { columnId: 'id', direction: 'desc' }).map((r) => r.id)).toEqual(
      ['3', '2', '1', '0'],
    );
  });

  it('falls back to ID ascending without a sort state (the server default)', () => {
    expect(insertRow(rows, row(0), null).map((r) => r.id)).toEqual(['0', '3', '2', '1']);
  });

  it('sorts null statuses lowest and breaks ties by ID', () => {
    const live = row(4, { host: 'z.example.com' });
    expect(
      insertRow(rows, live, { columnId: 'host', direction: 'asc' }).map((r) => r.host),
    ).toEqual(['api.example.com', 'api.example.com', 'api.example.com', 'z.example.com']);

    const dropped = row(4, { response: null });
    expect(insertRow(rows, dropped, { columnId: 'status', direction: 'asc' })[0]?.id).toBe('4');
  });
});

describe('row presenters', () => {
  it('maps schema colours to row stripes', () => {
    expect(rowColor(row(1, { color: 'CYAN' }))).toBe('teal');
    expect(rowColor(row(1))).toBeNull();
    expect(colorLabel(null)).toBe('None');
    expect(colorLabel('RED')).toBe('Red');
  });

  it('builds URLs with the port only when it is not the scheme default', () => {
    expect(requestUrl(row(1, { query: 'a=1' }))).toBe('https://api.example.com/v1/users?a=1');
    expect(requestUrl(row(1, { tls: false, port: 8080 }))).toBe(
      'http://api.example.com:8080/v1/users',
    );
    expect(requestCurl(row(1, { method: 'POST' }))).toBe(
      "curl -X 'POST' 'https://api.example.com/v1/users'",
    );
  });

  it('quotes shell metacharacters in method and path', () => {
    const curl = requestCurl(row(1, { method: 'A|B', path: "/x'$(id)'" }));
    expect(curl).toBe("curl -X 'A|B' 'https://api.example.com/x'\\''$(id)'\\'''");
  });
});
