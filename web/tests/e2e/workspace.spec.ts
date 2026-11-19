import { existsSync } from 'node:fs';
import { join } from 'node:path';
import { capture, currentProjectId, expect, test } from './fixtures';

test('F-WSP-1 creates, renames, selects and deletes a project without stale history', async ({
  page,
  request,
  instance,
  target,
}) => {
  await page.goto('/#/http-history');
  await expect(page.getByText('0 matching', { exact: true })).toBeVisible();
  await capture(instance, target, '/original-project');
  const history = page.getByRole('grid', { name: 'HTTP History' });
  const original = history.getByRole('gridcell', { name: '/original-project', exact: true });
  await expect(original).toBeVisible();

  const switcher = page.getByRole('combobox', { name: 'Project', exact: true });
  const initialName = await switcher.inputValue();
  expect(initialName).not.toBe('');
  await page.getByRole('link', { name: 'Workspace', exact: true }).click();
  await page.getByRole('button', { name: 'New project', exact: true }).click();
  let dialog = page.getByRole('dialog', { name: 'New project' });
  await dialog.getByLabel('Project name').fill('Browser project');
  await dialog.getByRole('button', { name: 'Create', exact: true }).click();
  await page.getByRole('button', { name: 'Rename Browser project', exact: true }).click();
  dialog = page.getByRole('dialog', { name: 'Rename project' });
  await dialog.getByLabel('Project name').fill('Renamed browser project');
  await dialog.getByRole('button', { name: 'Rename', exact: true }).click();
  await expect(dialog).toHaveCount(0);

  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  await expect(original).toBeVisible();
  await switcher.click();
  await page.getByRole('option', { name: 'Renamed browser project', exact: true }).click();
  await expect(switcher).toHaveValue('Renamed browser project');
  await expect(page.getByText('0 matching', { exact: true })).toBeVisible();
  await expect(original).toHaveCount(0);
  const directory = join(instance.dataDir, 'projects', await currentProjectId(request));
  expect(existsSync(directory)).toBe(true);

  await page.getByRole('link', { name: 'Workspace', exact: true }).click();
  await page.getByRole('button', { name: `Select ${initialName}`, exact: true }).click();
  await expect(switcher).toHaveValue(initialName);
  await page.getByRole('button', { name: 'Delete Renamed browser project', exact: true }).click();
  dialog = page.getByRole('dialog', { name: 'Delete project' });
  const confirmation = dialog.getByLabel('Confirm project name');
  const remove = dialog.getByRole('button', { name: 'Delete', exact: true });
  await confirmation.fill('Renamed');
  await expect(remove).toBeDisabled();
  await confirmation.fill('Renamed browser project');
  await remove.click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole('button', { name: 'Rename Renamed browser project', exact: true }),
  ).toHaveCount(0);
  expect(existsSync(directory)).toBe(false);
  await page.getByRole('link', { name: 'HTTP History', exact: true }).click();
  await expect(original).toBeVisible();
});
