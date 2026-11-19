import { defineConfig } from '@playwright/test';

export default defineConfig({
  testDir: './tests/e2e',
  outputDir: '../.dev-data/e2e-results',
  globalSetup: './tests/e2e/build.ts',
  workers: 1,
  use: { trace: 'retain-on-failure' },
});
