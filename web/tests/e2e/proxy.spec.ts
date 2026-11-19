import { execFileSync } from 'node:child_process';
import { createHash, X509Certificate } from 'node:crypto';
import { once } from 'node:events';
import { readFileSync } from 'node:fs';
import { request } from 'node:http';
import { createServer } from 'node:https';
import { join } from 'node:path';
import { connect } from 'node:tls';
import { capture, close, expect, listen, test } from './fixtures';

async function leafPin(proxy: URL, target: string, ca: string) {
  const tunnel = request(proxy, { method: 'CONNECT', path: target });
  tunnel.setTimeout(5_000, () => tunnel.destroy(new Error('CONNECT timed out')));
  tunnel.end();
  const [response, socket] = await once(tunnel, 'connect');
  try {
    expect(response.statusCode).toBe(200);
    // Validate the leaf against this instance's downloaded CA before pinning it in Chromium.
    const tls = connect({ socket, host: '127.0.0.1', ca, rejectUnauthorized: true });
    tls.setTimeout(5_000, () => tls.destroy(new Error('TLS handshake timed out')));
    await once(tls, 'secureConnect');
    const cert = new X509Certificate(tls.getPeerCertificate().raw);
    return createHash('sha256')
      .update(cert.publicKey.export({ type: 'spki', format: 'der' }))
      .digest('base64');
  } finally {
    socket.destroy();
  }
}

test('F-NET-1/2 serves the UI and captures HTTP and HTTPS through the proxy', async ({
  instance,
  target,
  page,
  playwright,
}) => {
  expect((await page.goto('/#/http-history'))?.status()).toBe(200);
  await expect(page.getByText('0 matching', { exact: true })).toBeVisible();
  expect(await capture(instance, target, '/curl-http')).toBe('Local upstream response');

  const key = join(instance.dataDir, 'target.key');
  const cert = join(instance.dataDir, 'target.crt');
  execFileSync(
    'openssl',
    [
      'req',
      '-x509',
      '-newkey',
      'rsa:2048',
      '-nodes',
      '-subj',
      '/CN=127.0.0.1',
      '-days',
      '1',
      '-keyout',
      key,
      '-out',
      cert,
    ],
    { stdio: 'ignore' },
  );
  const server = createServer(
    { key: readFileSync(key), cert: readFileSync(cert) },
    (_, response) => {
      response.writeHead(200, { 'Content-Type': 'text/plain' }).end('Local TLS response');
    },
  );
  const address = await listen(server);

  try {
    const proxy = new URL(instance.url);
    const ca = await page.request.get('/ca.crt');
    expect(ca.ok()).toBe(true);
    const pin = await leafPin(proxy, address, await ca.text());
    const chromium = await playwright.chromium.launch({
      args: [
        `--proxy-server=${instance.url}`,
        '--proxy-bypass-list=<-loopback>',
        `--ignore-certificate-errors-spki-list=${pin}`,
      ],
    });
    try {
      const traffic = await chromium.newPage();
      expect((await traffic.goto(`${target}/chromium-http`))?.status()).toBe(200);
      const response = await traffic.goto(`https://${address}/chromium-https`);
      expect(response?.status(), await traffic.locator('body').innerText()).toBe(200);
      await expect(traffic.locator('body')).toHaveText('Local TLS response');
    } finally {
      await chromium.close();
    }

    const history = page.getByRole('grid', { name: 'HTTP History' });
    for (const path of ['/curl-http', '/chromium-http', '/chromium-https']) {
      await expect(history.getByRole('gridcell', { name: path, exact: true })).toBeVisible();
    }
    const row = history.getByRole('row').filter({ hasText: '/chromium-https' });
    await expect(row.getByRole('gridcell', { name: '✓' })).toBeVisible();
  } finally {
    await close(server);
  }
});
