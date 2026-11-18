import { describe, expect, it } from 'vitest';
import { MAX_BODY, requestTruncated } from './detail';

describe('requestTruncated', () => {
  it('flags a decoded body that hit the cap even when the stored size is small', () => {
    const body = btoa('a'.repeat(MAX_BODY));
    expect(requestTruncated('1000', body)).toBe(true);
    expect(requestTruncated('1000', btoa('abc'))).toBe(false);
  });
});
