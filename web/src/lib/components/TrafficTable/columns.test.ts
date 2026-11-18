import { describe, expect, it } from 'vitest';
import {
  MAX_COLUMN_WIDTH,
  MIN_COLUMN_WIDTH,
  clampWidth,
  moveColumn,
  normalizeConfig,
  setColumnWidth,
  toggleColumn,
} from './columns';
import type { TrafficColumn } from './types';

interface Row {
  a: string;
  b: string;
}

const columns: TrafficColumn<Row>[] = [
  { id: 'id', title: 'ID', width: 60, value: (r) => r.a },
  { id: 'host', title: 'Host', width: 180, value: (r) => r.b },
];

describe('normalizeConfig', () => {
  it('uses column defaults when nothing is stored', () => {
    expect(normalizeConfig(columns, null)).toEqual({
      order: ['id', 'host'],
      hidden: [],
      widths: { id: 60, host: 180 },
    });
  });

  it('keeps stored order and widths, appending columns added since', () => {
    const config = normalizeConfig(columns, {
      order: ['host', 'gone', 'id'],
      hidden: ['gone', 'host'],
      widths: { host: 240, gone: 100 },
    });
    expect(config.order).toEqual(['host', 'id']);
    expect(config.hidden).toEqual(['host']);
    expect(config.widths).toEqual({ id: 60, host: 240 });
  });

  it('clamps out-of-range widths', () => {
    expect(clampWidth(1)).toBe(MIN_COLUMN_WIDTH);
    expect(clampWidth(10_000)).toBe(MAX_COLUMN_WIDTH);
  });

  it('tolerates a stored config without widths and with repeated ids', () => {
    const config = normalizeConfig(columns, {
      order: ['host', 'id'],
      hidden: ['host', 'host'],
      widths: undefined as unknown as Record<string, number>,
    });
    expect(config.widths).toEqual({ id: 60, host: 180 });
    expect(config.hidden).toEqual(['host']);
    // The menu must show a usable state, not a wrong "last visible" lock.
    expect(toggleColumn(config, 'host').hidden).toEqual([]);
  });
});

describe('column config edits', () => {
  const base = normalizeConfig(columns);

  it('hides and shows a column', () => {
    const hidden = toggleColumn(base, 'host');
    expect(hidden.hidden).toEqual(['host']);
    expect(toggleColumn(hidden, 'host').hidden).toEqual([]);
  });

  it('refuses to hide the last visible column', () => {
    const single = toggleColumn(base, 'host'); // only "id" stays visible
    expect(toggleColumn(single, 'id')).toBe(single);
    expect(toggleColumn(single, 'host').hidden).toEqual([]);
  });

  it('reorders within bounds only', () => {
    expect(moveColumn(base, 'host', -1).order).toEqual(['host', 'id']);
    expect(moveColumn(base, 'id', -1)).toBe(base);
    expect(moveColumn(base, 'host', 1)).toBe(base);
  });

  it('sets widths clamped to the allowed range', () => {
    expect(setColumnWidth(base, 'id', 120).widths.id).toBe(120);
    expect(setColumnWidth(base, 'id', 5).widths.id).toBe(MIN_COLUMN_WIDTH);
  });
});
