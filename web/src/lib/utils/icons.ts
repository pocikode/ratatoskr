import type { Component } from 'svelte';
import BellIcon from '@lucide/svelte/icons/bell';
import ChevronDownIcon from '@lucide/svelte/icons/chevron-down';
import ChevronsLeftIcon from '@lucide/svelte/icons/chevrons-left';
import ChevronsRightIcon from '@lucide/svelte/icons/chevrons-right';
import CircleIcon from '@lucide/svelte/icons/circle';
import CommandIcon from '@lucide/svelte/icons/command';
import CrosshairIcon from '@lucide/svelte/icons/crosshair';
import DownloadIcon from '@lucide/svelte/icons/download';
import FunnelIcon from '@lucide/svelte/icons/funnel';
import FlagIcon from '@lucide/svelte/icons/flag';
import FolderIcon from '@lucide/svelte/icons/folder';
import ClockIcon from '@lucide/svelte/icons/clock';
import KeyboardIcon from '@lucide/svelte/icons/keyboard';
import LayoutDashboardIcon from '@lucide/svelte/icons/layout-dashboard';
import LoaderIcon from '@lucide/svelte/icons/loader';
import MoonIcon from '@lucide/svelte/icons/moon';
import NetworkIcon from '@lucide/svelte/icons/network';
import PaletteIcon from '@lucide/svelte/icons/palette';
import PanelLeftIcon from '@lucide/svelte/icons/panel-left';
import CirclePauseIcon from '@lucide/svelte/icons/circle-pause';
import PuzzleIcon from '@lucide/svelte/icons/puzzle';
import RadioIcon from '@lucide/svelte/icons/radio';
import RepeatIcon from '@lucide/svelte/icons/repeat';
import ReplaceIcon from '@lucide/svelte/icons/replace';
import SearchIcon from '@lucide/svelte/icons/search';
import SettingsIcon from '@lucide/svelte/icons/settings';
import SparklesIcon from '@lucide/svelte/icons/sparkles';
import SunIcon from '@lucide/svelte/icons/sun';
import UserIcon from '@lucide/svelte/icons/user';
import VariableIcon from '@lucide/svelte/icons/variable';
import WorkflowIcon from '@lucide/svelte/icons/workflow';
import ZapIcon from '@lucide/svelte/icons/zap';

/** Explicit icon table keeps the bundle to the icons actually used. */
const icons = {
  bell: BellIcon,
  'chevron-down': ChevronDownIcon,
  'chevrons-left': ChevronsLeftIcon,
  'chevrons-right': ChevronsRightIcon,
  circle: CircleIcon,
  command: CommandIcon,
  crosshair: CrosshairIcon,
  download: DownloadIcon,
  funnel: FunnelIcon,
  flag: FlagIcon,
  folder: FolderIcon,
  clock: ClockIcon,
  keyboard: KeyboardIcon,
  'layout-dashboard': LayoutDashboardIcon,
  loader: LoaderIcon,
  moon: MoonIcon,
  network: NetworkIcon,
  palette: PaletteIcon,
  'panel-left': PanelLeftIcon,
  'circle-pause': CirclePauseIcon,
  puzzle: PuzzleIcon,
  radio: RadioIcon,
  repeat: RepeatIcon,
  replace: ReplaceIcon,
  search: SearchIcon,
  settings: SettingsIcon,
  sparkles: SparklesIcon,
  sun: SunIcon,
  user: UserIcon,
  variable: VariableIcon,
  workflow: WorkflowIcon,
  zap: ZapIcon,
} satisfies Record<string, Component>;

export type IconName = keyof typeof icons;

export function iconFor(name: IconName | undefined): Component | undefined {
  return name ? (icons[name] as Component) : undefined;
}
