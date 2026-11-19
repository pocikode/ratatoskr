import { applyQuery, capture, currentProjectId, expect, seed, test } from './fixtures';

test('F-SITE-1 updates live, expands paths and deletes matching history rows', async ({
  page,
  instance,
  target,
}) => {
  await page.goto('/#/sitemap');
  const tree = page.getByRole('tree', { name: 'Sitemap' });
  await expect(tree).toHaveText('No requests match.');
  for (const path of ['/nested/leaf?b=1&a=2', '/nested/keep', '/outside']) {
    await capture(instance, target, path);
  }

  const host = tree.getByRole('treeitem', { name: `${target} · 3 requests`, exact: true });
  await host.click({ modifiers: ['Shift'] });
  await expect(
    tree.getByRole('treeitem', { name: 'nested · 2 requests', exact: true }),
  ).toBeVisible();
  const queryNode = tree.getByRole('treeitem', { name: '?a&b · 1 requests', exact: true });
  await queryNode.click();
  const traffic = page.getByRole('grid', { name: 'Sitemap requests' });
  await traffic.getByRole('gridcell', { name: '/nested/leaf', exact: true }).click();
  const response = page.getByRole('region', { name: 'Response', exact: true });
  await expect(response.getByRole('tabpanel')).toContainText('Local upstream response');

  const leaf = tree.getByRole('treeitem', { name: 'leaf · 1 requests', exact: true });
  await leaf.click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Delete node…' }).click();
  const dialog = page.getByRole('dialog', { name: 'Delete sitemap node?' });
  await expect(dialog).toContainText('Delete 1 matching requests under leaf?');
  await dialog.getByRole('button', { name: 'Delete requests', exact: true }).click();
  await expect(dialog).toHaveCount(0);
  await expect(
    tree.getByRole('treeitem', { name: `${target} · 2 requests`, exact: true }),
  ).toBeVisible();

  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  const history = page.getByRole('grid', { name: 'HTTP History' });
  await expect(page.getByText('2 matching', { exact: true })).toBeVisible();
  await expect(history.getByRole('gridcell', { name: '/nested/leaf', exact: true })).toHaveCount(0);
  await expect(history.getByRole('gridcell', { name: '/nested/keep', exact: true })).toBeVisible();
  await expect(history.getByRole('gridcell', { name: '/outside', exact: true })).toBeVisible();
});

test('F-SITE-1 renders 200 hosts from 10,000 requests under 500 ms', async ({
  page,
  request,
  instance,
}) => {
  seed(instance, await currentProjectId(request), 'sitemap');
  await page.goto('/#/sitemap');
  const tree = page.getByRole('tree', { name: 'Sitemap' });
  const editor = page.getByRole('textbox', { name: 'HTTPQL query' });
  await expect(
    tree.getByRole('treeitem', { name: 'http://h000.test:80 · 50 requests' }),
  ).toBeVisible();
  await applyQuery(editor, 'req.host.eq:"absent.test"');
  await expect(tree).toHaveText('No requests match.');

  await editor.fill('req.method.eq:"GET"');
  const start = await page.evaluate(() => performance.now());
  await editor.press('Enter');
  const result = await page.waitForFunction((start) => {
    const first = document.querySelector('[role="treeitem"][aria-setsize="200"]');
    if (first?.getAttribute('aria-label') === 'http://h000.test:80 · 50 requests') {
      return performance.now() - start;
    }
  }, start);
  expect(await result.jsonValue(), 'sitemap submit to first rendered host (ms)').toBeLessThan(500);
  await tree.press('End');
  await expect(
    tree.getByRole('treeitem', { name: 'http://h199.test:80 · 50 requests' }),
  ).toBeInViewport();
});
