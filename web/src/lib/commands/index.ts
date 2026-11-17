import { router } from '../router/router.svelte';
import { theme } from '../stores/theme.svelte';
import { ui } from '../stores/ui.svelte';
import { registerBuiltins } from './builtin';
import { CommandRegistry } from './registry';

export const registry = new CommandRegistry();
registerBuiltins(registry, { router, ui, theme });
