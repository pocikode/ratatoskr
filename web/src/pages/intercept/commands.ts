// Intercept page commands: palette, shortcuts and the page's buttons all run these (docs/06 §5).
// Registered while the page is mounted; they act on the item shown.
import type { Command } from '$lib/commands/registry';

export interface InterceptActions {
  hasItem: () => boolean;
  forward: () => Promise<void>;
  drop: () => Promise<void>;
  forwardAll: () => Promise<void>;
  dropAll: () => Promise<void>;
  undo: () => void;
  replay: () => Promise<void>;
  copyCurl: () => Promise<void>;
}

export function interceptCommands(a: InterceptActions): Command[] {
  const group = 'Intercept';
  const when = a.hasItem;
  return [
    { id: 'intercept.forward', title: 'Forward held item', group, when, run: a.forward },
    { id: 'intercept.drop', title: 'Drop held item', group, when, run: a.drop },
    { id: 'intercept.forwardAll', title: 'Forward all held items', group, run: a.forwardAll },
    { id: 'intercept.dropAll', title: 'Drop all held items', group, run: a.dropAll },
    { id: 'intercept.undo', title: 'Undo edits of held item', group, when, run: a.undo },
    { id: 'intercept.replay', title: 'Send held item to Replay', group, when, run: a.replay },
    { id: 'intercept.copy.curl', title: 'Copy held item as cURL', group, when, run: a.copyCurl },
  ];
}
