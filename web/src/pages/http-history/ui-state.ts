// Column layout of the history table. UI preferences belong in backend `ui_state`
// (docs/06 §4); that contract does not exist yet, so this is the documented mock (T-017 AC).
import type { ColumnConfig } from '$lib/components/TrafficTable/types';

const KEY = 'ratatoskr.http-history.columns';

export function loadColumnConfig(): ColumnConfig | null {
  try {
    const raw = localStorage.getItem(KEY);
    return raw ? (JSON.parse(raw) as ColumnConfig) : null;
  } catch {
    return null; // unreadable or unavailable storage: fall back to the default layout
  }
}

export function saveColumnConfig(config: ColumnConfig): void {
  try {
    localStorage.setItem(KEY, JSON.stringify(config));
  } catch {
    // Storage full or denied; the layout simply does not persist.
  }
}
