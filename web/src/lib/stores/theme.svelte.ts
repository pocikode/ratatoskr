export type Theme = 'dark' | 'light';

const KEY = 'ratatoskr.theme';

function readStored(): Theme {
  try {
    const v = localStorage.getItem(KEY);
    if (v === 'light' || v === 'dark') return v;
  } catch {
    // storage unavailable: fall back to the default
  }
  return 'dark';
}

/**
 * Theme preference. TODO(T-007): backend settings become the source of truth (docs/06 §4);
 * keep a localStorage mirror only for the pre-paint read in index.html.
 */
export class ThemeState {
  current = $state<Theme>('dark');

  constructor(private doc: Document = document) {
    this.current = readStored();
    this.apply();
  }

  set(theme: Theme) {
    this.current = theme;
    this.apply();
    try {
      localStorage.setItem(KEY, theme);
    } catch {
      // ignore: preference simply won't persist
    }
  }

  toggle() {
    this.set(this.current === 'dark' ? 'light' : 'dark');
  }

  private apply() {
    this.doc.documentElement.dataset.theme = this.current;
  }
}

export const theme = new ThemeState();
