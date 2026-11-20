// Replay shell commands: palette, shortcuts and both context menus read these (docs/06 §5).
// Registered while the page is mounted. Menus pass the clicked row as `ctx.replay`; without it
// (palette, shortcut) a command acts on the active tab.
import type { Command, CommandContext } from '$lib/commands/registry';
import { colorLabel, COLORS } from '../http-history/columns';
import type { ReplayState, Target } from './replay.svelte';

export type ReplayContext = CommandContext & { replay?: Target };
export const withTarget = (target: Target): CommandContext =>
  ({ page: 'replay', replay: target }) as ReplayContext;

export const SESSION_MENU = [
  'replay.session.rename',
  'replay.session.duplicate',
  'replay.session.delete',
  ...COLORS.map((c) => `replay.color.${c.toLowerCase()}`),
  'replay.color.none',
];
export const COLLECTION_MENU = ['replay.collection.rename', 'replay.collection.delete'];
export const TAB_MENU = [
  'replay.tab.close',
  'replay.tab.closeOthers',
  'replay.tab.closeRight',
  'replay.session.rename',
  'replay.session.duplicate',
  ...COLORS.map((c) => `replay.color.${c.toLowerCase()}`),
  'replay.color.none',
];

export function replayCommands(state: ReplayState): Command[] {
  const group = 'Replay';
  const target = (ctx: CommandContext, kind: Target['kind']): Target | null => {
    const t = (ctx as ReplayContext).replay;
    if (t) return t.kind === kind ? t : null;
    return kind === 'session' && state.activeId ? { kind, id: state.activeId } : null;
  };
  const sessionId = (ctx: CommandContext) => target(ctx, 'session')?.id;
  const hasSession = (ctx: CommandContext) => sessionId(ctx) !== undefined;
  const collection = (ctx: CommandContext) => {
    const t = target(ctx, 'collection');
    return t ? state.collections.find((c) => c.id === t.id) : undefined;
  };
  const onSession = (fn: (id: string) => void) => (ctx: CommandContext) => {
    const id = sessionId(ctx);
    if (id) fn(id);
  };

  return [
    {
      id: 'replay.session.new',
      title: 'New Replay session',
      group,
      run: () => void (state.newSessionOpen = true),
    },
    {
      id: 'replay.collection.new',
      title: 'New Replay collection',
      group,
      run: () => void (state.newCollectionOpen = true),
    },
    {
      id: 'replay.session.rename',
      title: 'Rename session',
      group,
      when: hasSession,
      run: (ctx) => {
        const t = target(ctx, 'session');
        if (!t) return;
        // A menu target says where it was opened (no `from` = sidebar); palette/shortcut edit the tab.
        const explicit = (ctx as ReplayContext).replay;
        state.renaming = explicit
          ? { kind: 'session', id: t.id, ...(explicit.from && { from: explicit.from }) }
          : { kind: 'session', id: t.id, from: 'tab' };
      },
    },
    {
      id: 'replay.session.duplicate',
      title: 'Duplicate session',
      group,
      when: hasSession,
      run: onSession((id) => void state.duplicate(id)),
    },
    {
      id: 'replay.session.delete',
      title: 'Delete session',
      group,
      when: hasSession,
      run: onSession((id) => void (state.pendingDelete = { kind: 'session', id })),
    },
    {
      id: 'replay.collection.rename',
      title: 'Rename collection',
      group,
      when: (ctx) => collection(ctx) !== undefined,
      run: (ctx) => {
        const c = collection(ctx);
        if (c) state.renaming = { kind: 'collection', id: c.id };
      },
    },
    {
      id: 'replay.collection.delete',
      title: 'Delete collection',
      group,
      when: (ctx) => collection(ctx) !== undefined && !collection(ctx)!.isDefault,
      run: (ctx) => {
        const c = collection(ctx);
        if (c) state.pendingDelete = { kind: 'collection', id: c.id };
      },
    },
    ...COLORS.map((color): Command => ({
      id: `replay.color.${color.toLowerCase()}`,
      title: `Colour: ${colorLabel(color)}`,
      group,
      when: hasSession,
      run: onSession((id) => void state.setColor(id, color)),
    })),
    {
      id: 'replay.color.none',
      title: `Colour: ${colorLabel(null)}`,
      group,
      when: hasSession,
      run: onSession((id) => void state.setColor(id, null)),
    },
    {
      id: 'replay.tab.close',
      title: 'Close tab',
      group,
      when: hasSession,
      run: onSession((id) => state.close(id)),
    },
    {
      id: 'replay.tab.closeOthers',
      title: 'Close other tabs',
      group,
      when: hasSession,
      run: onSession((id) => state.closeOthers(id)),
    },
    {
      id: 'replay.tab.closeRight',
      title: 'Close tabs to the right',
      group,
      when: hasSession,
      run: onSession((id) => state.closeToRight(id)),
    },
    { id: 'tab.next', title: 'Next tab', group, run: () => state.step(1) },
    { id: 'tab.prev', title: 'Previous tab', group, run: () => state.step(-1) },
    {
      id: 'tab.close',
      title: 'Close active tab',
      group,
      when: () => state.activeId !== null,
      run: () => void (state.activeId && state.close(state.activeId)),
    },
  ];
}
