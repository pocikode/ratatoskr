// Page commands: the context menu, the palette and (later) shortcuts read the same registry
// entries (docs/06 §5). Registered while the page is mounted.
import type { Command } from '$lib/commands/registry';
import { colorLabel, COLORS, requestCurl, requestUrl, type HistoryRow } from './columns';
import type { HistoryState } from './history.svelte';

export interface HistoryCommandDeps {
  copy: (text: string) => void;
  /** Opens the note editor for a row. */
  editNote: (row: HistoryRow) => void;
}

/** Escapes a value for an HTTPQL string literal (docs/04 §1). */
function literal(value: string): string {
  return value.replace(/[\\"]/g, '\\$&');
}

export function historyCommands(state: HistoryState, deps: HistoryCommandDeps): Command[] {
  const selected = () => state.rows.filter((row) => state.selection.keys.has(row.id));
  const anySelected = () => state.selection.size > 0;
  const withSelection = (run: (rows: HistoryRow[]) => void) => () => {
    const rows = selected();
    if (rows.length > 0) run(rows);
  };
  const group = 'HTTP History';

  return [
    {
      id: 'history.copy.url',
      title: 'Copy URL',
      group,
      when: anySelected,
      run: withSelection((rows) => deps.copy(rows.map(requestUrl).join('\n'))),
    },
    {
      id: 'history.copy.curl',
      title: 'Copy as cURL',
      group,
      when: anySelected,
      run: withSelection((rows) => deps.copy(rows.map(requestCurl).join('\n'))),
    },
    ...COLORS.map((color): Command => ({
      id: `history.color.${color.toLowerCase()}`,
      title: `Colour: ${colorLabel(color)}`,
      group,
      when: anySelected,
      run: withSelection(
        (rows) =>
          void state.setColor(
            rows.map((row) => row.id),
            color,
          ),
      ),
    })),
    {
      id: 'history.color.none',
      title: `Colour: ${colorLabel(null)}`,
      group,
      when: anySelected,
      run: withSelection(
        (rows) =>
          void state.setColor(
            rows.map((row) => row.id),
            null,
          ),
      ),
    },
    {
      id: 'history.note.edit',
      title: 'Edit note…',
      group,
      when: anySelected,
      run: () => {
        const row = selected()[0];
        if (row) deps.editNote(row);
      },
    },
    {
      id: 'history.filter.host',
      title: 'Filter by this host',
      group,
      when: anySelected,
      run: () => {
        const row = selected()[0];
        if (!row) return;
        const clause = `req.host.eq:"${literal(row.host)}"`;
        // A trailing `//` comment would swallow the closing paren.
        const base = state.applied.replace(/\s*\/\/[^\n"]*$/, '');
        state.query = base ? `(${base}) AND ${clause}` : clause;
        state.apply();
      },
    },
    {
      id: 'history.delete',
      title: 'Delete selected requests',
      group,
      when: anySelected,
      run: withSelection((rows) => void state.remove(rows.map((row) => row.id))),
    },
    {
      id: 'history.query.apply',
      title: 'Apply query',
      group,
      run: () => state.apply(),
    },
    {
      id: 'history.query.clear',
      title: 'Clear query',
      group,
      run: () => state.apply(''),
    },
    {
      id: 'history.layout.toggle',
      title: 'Toggle viewers beside the table',
      group,
      run: () => state.toggleLayout(),
    },
    {
      id: 'history.reload',
      title: 'Reload history',
      group,
      run: () => void state.reload(),
    },
    {
      id: 'history.pending.show',
      title: 'Show new requests',
      group,
      when: () => state.pending.length > 0,
      run: () => state.applyPending(),
    },
  ];
}
