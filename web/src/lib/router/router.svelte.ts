import { DEFAULT_ROUTE, findRoute, type PageRoute } from './routes';

export interface Location {
  /** Route id (first segment). Unknown ids resolve to `route = undefined`. */
  page: string;
  /** Remaining segments, e.g. `['session', '42']`. */
  params: string[];
}

function safeDecode(s: string): string {
  try {
    return decodeURIComponent(s);
  } catch {
    return s;
  }
}

/** Parses `#/replay/session/42` → `{ page: 'replay', params: ['session', '42'] }`. */
export function parseHash(hash: string): Location {
  const segments = hash
    .replace(/^#\/?/, '')
    .split('?')[0]!
    .split('/')
    .filter(Boolean)
    .map(safeDecode);
  const [page, ...params] = segments;
  return { page: page ?? DEFAULT_ROUTE, params };
}

export function buildHash(page: string, ...params: (string | number)[]): string {
  return '#/' + [page, ...params].map((p) => encodeURIComponent(String(p))).join('/');
}

export class Router {
  #win: Window | undefined;

  location = $state<Location>(parseHash(''));
  route = $derived<PageRoute | undefined>(findRoute(this.location.page));

  /** Starts listening to hashchange; returns the cleanup function. */
  start(win: Window = window): () => void {
    this.#win = win;
    const sync = () => (this.location = parseHash(win.location.hash));
    sync();
    win.addEventListener('hashchange', sync);
    return () => win.removeEventListener('hashchange', sync);
  }

  navigate(page: string, ...params: (string | number)[]) {
    const hash = buildHash(page, ...params);
    const win = this.#win ?? window;
    if (win.location.hash === hash) return;
    win.location.hash = hash;
  }
}

export const router = new Router();
