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
 * Theme preference. Stored in localStorage until the backend settings API exists (T-007+);
 * it must be readable before first paint, which index.html also does.
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
