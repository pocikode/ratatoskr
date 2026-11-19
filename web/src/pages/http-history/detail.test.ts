import { describe, expect, it } from 'vitest';
import type { HistoryMessageFragment } from '$lib/api/generated/graphql';
import { MAX_BODY, buildDetail, requestTruncated } from './detail';

describe('requestTruncated', () => {
  it('flags a decoded body that hit the cap even when the stored size is small', () => {
    const body = btoa('a'.repeat(MAX_BODY));
    expect(requestTruncated('1000', body)).toBe(true);
    expect(requestTruncated('1000', btoa('abc'))).toBe(false);
  });
});

describe('buildDetail', () => {
  const message = (overrides: Partial<HistoryMessageFragment> = {}): HistoryMessageFragment => ({
    id: '1',
    head: btoa('GET / HTTP/1.1\r\nHost: a.test\r\n\r\n'),
    bodyLength: '0',
    body: '',
    editedFrom: null,
    editedBy: null,
    response: null,
    ...overrides,
  });

  it('carries the pair ids so the viewer can switch Original / Modified', () => {
    expect(buildDetail(message())).toMatchObject({ editedFromId: null, editedById: null });
    expect(buildDetail(message({ editedBy: { id: '2' } })).editedById).toBe('2');
    expect(buildDetail(message({ editedFrom: { id: '1' } })).editedFromId).toBe('1');
  });
});
