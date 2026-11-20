import { describe, expect, it } from 'vitest';
import { SECTIONS, matchersFor, operationsFor } from './rules';

describe('rule dependency table', () => {
  it('limits operations per section', () => {
    expect(operationsFor('REQUEST_HEADER')).toEqual([
      'UPDATE_RAW',
      'UPDATE_VALUE',
      'ADD',
      'REMOVE',
    ]);
    expect(operationsFor('REQUEST_METHOD')).toEqual(['UPDATE']);
    expect(operationsFor('RESPONSE_STATUS_CODE')).toEqual(['UPDATE']);
    for (const s of [
      'REQUEST_FIRST_LINE',
      'REQUEST_PATH',
      'REQUEST_BODY',
      'RESPONSE_BODY',
    ] as const)
      expect(operationsFor(s)).toEqual(['UPDATE_RAW']);
    expect(SECTIONS).toHaveLength(10);
  });

  it('limits matchers per operation and hides them for ADD', () => {
    expect(matchersFor('UPDATE_RAW')).toEqual(['FULL', 'STRING', 'REGEX']);
    expect(matchersFor('UPDATE')).toEqual(['FULL', 'STRING', 'REGEX']);
    expect(matchersFor('UPDATE_VALUE')).toContain('NAME');
    expect(matchersFor('REMOVE')).toContain('NAME');
    expect(matchersFor('ADD')).toEqual([]);
  });
});
