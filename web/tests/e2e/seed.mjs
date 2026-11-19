import { DatabaseSync } from 'node:sqlite';

const [, , path, dataset] = process.argv;
const db = new DatabaseSync(path);
const history = dataset === 'history';
const count = history ? 100_000 : 10_000;
const host = history
  ? "CASE WHEN i = ? THEN 'selected.test' ELSE 'history.test' END"
  : "printf('h%03d.test', (i - 1) % 200)";

try {
  db.exec('PRAGMA busy_timeout = 5000');
  db.prepare(
    `
    WITH RECURSIVE seq(i) AS (SELECT 1 UNION ALL SELECT i + 1 FROM seq WHERE i < ?),
    traffic AS (
      SELECT i, ${host} AS host, '/item-' || i AS path FROM seq
    ),
    wire AS (
      SELECT i, host, path, 'GET ' || path || ' HTTP/1.1\r\nHost: ' || host || '\r\n\r\n' AS raw FROM traffic
    )
    INSERT INTO requests (id, source, host, port, tls, method, path, raw, body_offset, len, created_at)
    SELECT i, 'proxy', host, 80, 0, 'GET', path, CAST(raw AS BLOB), length(raw), length(raw), ? FROM wire
  `,
  ).run(...(history ? [count, count, Date.now()] : [count, Date.now()]));
} finally {
  db.close();
}
