import { applyQuery, capture, currentProjectId, encodings, expect, seed, test } from './fixtures';

test('F-HIST-2 displays captured requests and decoded responses in the history viewers', async ({
  instance,
  target,
  page,
}) => {
  await page.goto('/#/http-history');
  await expect(page.getByText('0 matching', { exact: true })).toBeVisible();
  const grid = page.getByRole('grid', { name: 'HTTP History' });
  const request = page.getByRole('region', { name: 'Request', exact: true });
  const response = page.getByRole('region', { name: 'Response', exact: true });

  for (const encoding of encodings) {
    await capture(instance, target, `/viewer/${encoding}?mode=fixture`);
  }
  await expect(page.getByText('4 matching', { exact: true })).toBeVisible();
  for (const encoding of encodings) {
    await grid.getByRole('gridcell', { name: `/viewer/${encoding}`, exact: true }).click();
    await expect(response.getByRole('tabpanel')).toContainText(
      `{"captured":"decoded ${encoding}"}`,
    );
  }

  await grid.getByRole('gridcell', { name: '/viewer/gzip', exact: true }).click();
  await expect(request.getByRole('tabpanel')).toContainText(
    `GET ${target}/viewer/gzip?mode=fixture HTTP/1.1`,
  );
  await expect(response.getByRole('tabpanel')).toContainText('HTTP/1.1 200');
  await expect(response.locator('[contenteditable="true"]')).toHaveCount(0);
  await response.getByRole('tab', { name: 'Pretty' }).click();
  await expect(response.getByRole('tabpanel')).toContainText('"captured": "decoded gzip"');
  await response.getByRole('tab', { name: 'Hex' }).click();
  await expect(response.getByRole('tabpanel')).toContainText('7b 22 63 61 70 74 75 72');
  await response.getByRole('tab', { name: 'Table' }).click();
  await expect(response.getByRole('row', { name: /content-encoding gzip/i })).toBeVisible();
  await request.getByRole('tab', { name: 'Table' }).click();
  await expect(request.getByRole('table', { name: 'Params' })).toContainText('mode');
  await expect(request.getByRole('table', { name: 'Params' })).toContainText('fixture');
});

test('F-HIST-1 filters 100,000 rows under 300 ms and scrolls to the oldest row', async ({
  instance,
  request,
  page,
}) => {
  test.setTimeout(120_000);
  seed(instance, await currentProjectId(request), 'history');
  await page.goto('/#/http-history');
  const grid = page.getByRole('grid', { name: 'HTTP History' });
  const newest = grid.getByRole('gridcell', { name: '/item-100000', exact: true });
  const editor = page.getByRole('textbox', { name: 'HTTPQL query' });
  await expect(newest).toBeVisible();
  await expect(page.getByText('100000 matching', { exact: true })).toBeVisible();

  await editor.fill('req.host.eq:"selected.test"');
  const start = await page.evaluate(() => performance.now());
  await editor.press('Enter');
  const result = await page.waitForFunction((start) => {
    const count = document.querySelector('p[aria-live="polite"]');
    if (count?.textContent?.trim() === '1 matching') return performance.now() - start;
  }, start);
  expect(await result.jsonValue(), 'host filter submit to rendered result (ms)').toBeLessThan(300);
  await expect(newest).toBeVisible();
  await expect(grid).toHaveAttribute('aria-rowcount', '2');

  await applyQuery(editor, '');
  await expect(page.getByText('100000 matching', { exact: true })).toBeVisible();
  const viewport = grid.locator('.overflow-auto');
  for (let rows = 1_000; rows <= 100_000; rows += 1_000) {
    await page.waitForFunction(
      (count) => {
        return (
          document
            .querySelector('[role="grid"][aria-label="HTTP History"]')
            ?.getAttribute('aria-rowcount') === String(count + 1)
        );
      },
      rows,
      { timeout: 5_000 },
    );
    await viewport.evaluate((el) => el.scrollTo(0, el.scrollHeight));
    expect(await grid.getByRole('row').count()).toBeLessThan(100);
  }
  await expect(grid.getByRole('gridcell', { name: '/item-1', exact: true })).toBeInViewport();
});
