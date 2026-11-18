import type { ColumnConfig, TrafficColumn } from './types';

export const MIN_COLUMN_WIDTH = 40;
export const MAX_COLUMN_WIDTH = 900;

export function clampWidth(px: number): number {
  return Math.min(MAX_COLUMN_WIDTH, Math.max(MIN_COLUMN_WIDTH, Math.round(px)));
}

/**
 * Completes a stored config against the current column list: new columns go last (visible, default
 * width), ids of columns that no longer exist are dropped.
 */
export function normalizeConfig<Row>(
  columns: readonly TrafficColumn<Row>[],
  config?: ColumnConfig | null,
): ColumnConfig {
  const known = new Set(columns.map((c) => c.id));
  const order = [...new Set(config?.order ?? [])].filter((id) => known.has(id));
  for (const c of columns) if (!order.includes(c.id)) order.push(c.id);

  const widths: Record<string, number> = {};
  for (const c of columns) widths[c.id] = clampWidth(config?.widths?.[c.id] ?? c.width);

  return {
    order,
    // A stored config can repeat ids; that would break the visible count and the menu.
    hidden: [...new Set(config?.hidden ?? [])].filter((id) => known.has(id)),
    widths,
  };
}

export function setColumnWidth(config: ColumnConfig, id: string, width: number): ColumnConfig {
  return { ...config, widths: { ...config.widths, [id]: clampWidth(width) } };
}

/** Hides/shows a column; refuses to hide the last visible one. */
export function toggleColumn(config: ColumnConfig, id: string): ColumnConfig {
  const hidden = config.hidden.includes(id);
  if (!hidden && config.order.length - config.hidden.length <= 1) return config;
  return {
    ...config,
    hidden: hidden ? config.hidden.filter((h) => h !== id) : [...config.hidden, id],
  };
}

/** Moves a column by `delta` positions; returns the config unchanged at the edges. */
export function moveColumn(config: ColumnConfig, id: string, delta: number): ColumnConfig {
  const from = config.order.indexOf(id);
  const to = from + delta;
  if (from < 0 || to < 0 || to >= config.order.length) return config;
  const order = [...config.order];
  order.splice(to, 0, ...order.splice(from, 1));
  return { ...config, order };
}
