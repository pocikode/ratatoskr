import { applyQuery, capture, expect, test } from './fixtures';

test('F-SCOPE-1 manages presets and applies the same allow and deny rules to history and sitemap', async ({
  page,
  instance,
  target,
}) => {
  for (const path of ['/included/public', '/included/private', '/outside']) {
    await capture(instance, target, path);
  }
  await page.goto('/#/scopes');
  const editor = page.getByRole('form', { name: 'Scope editor' });
  const list = page.getByRole('region', { name: 'Scope presets' });
  await list.getByRole('button', { name: 'New', exact: true }).click();
  await editor.getByLabel('Name', { exact: true }).fill('Local routes');
  const authority = new URL(target).host;
  const allow = editor.getByLabel('Add to the allow list');
  const deny = editor.getByLabel('Add to the deny list');
  await allow.fill(`${authority}/included`);
  await allow.press('Enter');
  await deny.fill(`${authority}/included/private`);
  await deny.press('Enter');
  await editor.getByRole('button', { name: 'Save', exact: true }).click();
  await expect(editor.getByText('Matches 1 request', { exact: false })).toBeVisible();

  await editor.getByRole('button', { name: 'Duplicate' }).click();
  const copy = list.getByRole('button', { name: 'Local routes (copy)', exact: true });
  await expect(copy).toBeVisible();
  await list.getByRole('button', { name: 'Move Local routes (copy) up', exact: true }).click();
  await expect(list.getByRole('listitem').first()).toContainText('Local routes (copy)');
  await editor.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(copy).toHaveCount(0);

  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  await expect(page.getByText('3 matching', { exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Scope' }).selectOption({ label: 'Local routes' });
  const history = page.getByRole('grid', { name: 'HTTP History' });
  const included = history.getByRole('gridcell', { name: '/included/public', exact: true });
  await expect(page.getByText('1 matching', { exact: true })).toBeVisible();
  await expect(included).toBeVisible();
  await expect(
    history.getByRole('gridcell', { name: '/included/private', exact: true }),
  ).toHaveCount(0);

  await page.getByRole('link', { name: 'Sitemap', exact: true }).click();
  await expect(
    page.getByRole('treeitem', { name: `${target} · 3 requests`, exact: true }),
  ).toBeVisible();
  await page.getByRole('combobox', { name: 'Scope' }).selectOption({ label: 'Local routes' });
  await expect(
    page.getByRole('treeitem', { name: `${target} · 1 requests`, exact: true }),
  ).toBeVisible();

  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  await expect(page.getByText('3 matching', { exact: true })).toBeVisible();
  await page.getByRole('combobox', { name: 'Scope' }).selectOption({ label: 'Local routes' });
  await expect(page.getByText('1 matching', { exact: true })).toBeVisible();
  await included.click({ button: 'right' });
  await page.getByRole('menuitem', { name: 'Exclude host from scope', exact: true }).click();
  await expect(page.getByText('0 matching', { exact: true })).toBeVisible();
});

test('F-FILTER-1 saves, applies, validates and round-trips filter presets', async ({
  page,
  instance,
  target,
}) => {
  await capture(instance, target, '/selected');
  await capture(instance, target, '/other');
  await page.goto('/#/filters');
  const editor = page.getByRole('form', { name: 'Filter editor' });
  const list = page.getByRole('region', { name: 'Filter presets' });
  const query = editor.getByRole('textbox', { name: 'Preset query' });
  const save = editor.getByRole('button', { name: 'Save', exact: true });
  const preset = list.getByRole('button', { name: 'Selected path · selected-path', exact: true });
  const pathQuery = 'req.path.eq:"/selected"';
  await list.getByRole('button', { name: 'New', exact: true }).click();
  await editor.getByLabel('Name', { exact: true }).fill('Selected path');
  await editor.getByLabel('Alias', { exact: true }).fill('selected-path');
  await query.fill(pathQuery);
  await save.click();
  await expect(preset).toBeVisible();

  await query.fill('preset:selected-path');
  await save.click();
  await expect(editor.getByRole('alert')).toContainText(/cycl/i);

  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  const matching = page.getByText('1 matching', { exact: true });
  const bar = page.getByRole('textbox', { name: 'HTTPQL query' });
  await page.getByRole('button', { name: 'Selected path', exact: true }).click();
  await expect(bar).toHaveText('preset:"Selected path"');
  await expect(matching).toBeVisible();
  await applyQuery(bar, '');
  await expect(page.getByText('2 matching', { exact: true })).toBeVisible();
  await applyQuery(bar, 'preset:selected-path');
  await expect(matching).toBeVisible();
  await expect(page.getByRole('gridcell', { name: '/selected', exact: true })).toBeVisible();

  await page.getByRole('link', { name: 'Filters', exact: true }).click();
  await preset.click();
  const downloaded = page.waitForEvent('download');
  await editor.getByRole('button', { name: 'Export', exact: true }).click();
  const download = await downloaded;
  expect(download.suggestedFilename()).toBe('filter-presets.json');
  await editor.getByRole('button', { name: 'Delete', exact: true }).click();
  await expect(preset).toHaveCount(0);
  await editor.getByLabel('Import filter presets').setInputFiles(await download.path());
  await expect(editor.getByRole('status')).toHaveText('Imported 1 preset');
  await preset.click();
  await expect(query).toHaveText(pathQuery);
});
