// New-session URL dialog → connection and request bytes (F-REP-1: `GET <path> HTTP/1.1` + `Host`).
export interface NewSessionForm {
  tls: boolean;
  host: string;
  /** Empty means the scheme default. */
  port: string;
  path: string;
}

export interface NewSession {
  connection: { host: string; port: number; tls: boolean };
  raw: string;
}

/** Null with a message when the form is invalid; the server validates again. */
export function buildSession(form: NewSessionForm): NewSession | string {
  const host = form.host.trim();
  // eslint-disable-next-line no-control-regex -- T-034: mirrors the server's host rule
  if (!host || /[\s\u0000-\u001f\u007f]/.test(host)) return 'Enter a host without spaces.';
  const def = form.tls ? 443 : 80;
  const port = form.port.trim() === '' ? def : Number(form.port);
  if (!Number.isInteger(port) || port < 1 || port > 65535) return 'Port must be 1-65535.';
  let path = form.path.trim() || '/';
  if (!path.startsWith('/')) path = `/${path}`;
  // eslint-disable-next-line no-control-regex -- T-034: no CR/LF injection into the request line
  if (/[\s\u0000-\u001f\u007f]/.test(path)) return 'Path must not contain spaces.';
  const hostHeader = host.includes(':') && !host.startsWith('[') ? `[${host}]` : host;
  const authority = port === def ? hostHeader : `${hostHeader}:${port}`;
  return {
    connection: { host, port, tls: form.tls },
    raw: `GET ${path} HTTP/1.1\r\nHost: ${authority}\r\n\r\n`,
  };
}
