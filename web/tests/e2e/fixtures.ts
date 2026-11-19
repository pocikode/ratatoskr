import { execFile, execFileSync, spawn } from 'node:child_process';
import { once } from 'node:events';
import { mkdtemp, rm } from 'node:fs/promises';
import { createServer, type Server } from 'node:http';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';
import { promisify } from 'node:util';
import { brotliCompressSync, deflateSync, gzipSync, zstdCompressSync } from 'node:zlib';
import { expect, test as base, type APIRequestContext, type Locator } from '@playwright/test';

type Instance = { url: string; dataDir: string };
const run = promisify(execFile);
const encoders = new Map<string, (text: string) => Buffer>([
  ['gzip', gzipSync],
  ['deflate', deflateSync],
  ['br', brotliCompressSync],
  ['zstd', zstdCompressSync],
]);
export const encodings = [...encoders.keys()];

export async function listen(server: Server) {
  server.listen(0, '127.0.0.1');
  await once(server, 'listening');
  const address = server.address();
  if (!address || typeof address === 'string') throw new Error('Expected a TCP listener');
  return `127.0.0.1:${address.port}`;
}

export async function close(server: Server) {
  await new Promise<void>((resolve, reject) => {
    server.close((error) => (error ? reject(error) : resolve()));
    server.closeAllConnections();
  });
}

export const test = base.extend<{ instance: Instance; target: string }>({
  // eslint-disable-next-line no-empty-pattern -- T-021: Playwright requires destructured fixture dependencies.
  instance: async ({}, use, testInfo) => {
    const binary = process.env.RATATOSKR_E2E_BINARY;
    if (!binary) throw new Error('The E2E binary was not built');
    const dataDir = await mkdtemp(join(tmpdir(), 'ratatoskr-e2e-'));
    const env = Object.fromEntries(
      Object.entries(process.env).filter(([key]) => !key.startsWith('RATATOSKR_')),
    );
    const child = spawn(binary, ['--data-dir', dataDir, '--listen', '127.0.0.1:0', '--no-open'], {
      env,
      stdio: ['ignore', 'ignore', 'pipe'],
    });
    let logs = '';
    const exited = new Promise<void>((resolve) => child.once('close', () => resolve()));
    try {
      const address = await new Promise<string>((resolve, reject) => {
        const timer = setTimeout(
          () => reject(new Error(`Ratatoskr did not start\n${logs}`)),
          10_000,
        );
        child.stderr.setEncoding('utf8').on('data', (chunk: string) => {
          logs += chunk;
          const address = /msg="ratatoskr listening" addr=(127\.0\.0\.1:\d+) /.exec(logs)?.[1];
          if (address) {
            clearTimeout(timer);
            resolve(address);
          }
        });
        child.once('error', (error) => {
          clearTimeout(timer);
          reject(error);
        });
        child.once('close', (code) => {
          clearTimeout(timer);
          reject(new Error(`Ratatoskr stopped (${code})\n${logs}`));
        });
      });
      await use({ dataDir, url: `http://${address}` });
    } finally {
      child.kill();
      const kill = setTimeout(() => child.kill('SIGKILL'), 5_000);
      await exited;
      clearTimeout(kill);
      await rm(dataDir, { recursive: true, force: true });
      if (testInfo.status !== testInfo.expectedStatus) {
        await testInfo.attach('ratatoskr.log', { body: logs, contentType: 'text/plain' });
      }
    }
  },

  baseURL: async ({ instance }, use) => use(instance.url),

  // eslint-disable-next-line no-empty-pattern -- T-021: Playwright requires destructured fixture dependencies.
  target: async ({}, use) => {
    const server = createServer((request, response) => {
      const path = new URL(request.url ?? '/', 'http://127.0.0.1').pathname;
      const encoding = path.slice('/viewer/'.length);
      const encode = path.startsWith('/viewer/') && encoders.get(encoding);
      if (path === '/favicon.ico') {
        response.writeHead(204).end();
      } else if (encode) {
        response.writeHead(200, {
          'Content-Type': 'application/json',
          'Content-Encoding': encoding,
          'Transfer-Encoding': 'chunked',
        });
        response.end(encode(JSON.stringify({ captured: `decoded ${encoding}` })));
      } else {
        response.writeHead(200, { 'Content-Type': 'text/plain' }).end('Local upstream response');
      }
    });
    const address = await listen(server);
    try {
      await use(`http://${address}`);
    } finally {
      await close(server);
    }
  },
});

export { expect };

export async function capture(instance: Instance, target: string, path: string) {
  const { stdout } = await run('curl', [
    '--disable',
    '--fail',
    '--silent',
    '--show-error',
    '--max-time',
    '5',
    '--noproxy',
    '',
    '--proxy',
    instance.url,
    `${target}${path}`,
  ]);
  return stdout;
}

export async function applyQuery(editor: Locator, query: string) {
  await editor.fill(query);
  await editor.press('Enter');
}

export async function currentProjectId(request: APIRequestContext) {
  const response = await request.post('/api/graphql', {
    headers: { 'X-Ratatoskr': '1' },
    data: { query: '{ currentProject { id } }' },
  });
  expect(response.ok()).toBe(true);
  const { data, errors } = await response.json();
  expect(errors).toBeUndefined();
  return String(data.currentProject.id);
}

export function seed(instance: Instance, projectId: string, dataset: 'history' | 'sitemap') {
  execFileSync('bun', [
    fileURLToPath(new URL('./seed.mjs', import.meta.url)),
    join(instance.dataDir, 'projects', projectId, 'project.db'),
    dataset,
  ]);
}
