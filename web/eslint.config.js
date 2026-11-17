import js from '@eslint/js';
import svelte from 'eslint-plugin-svelte';
import globals from 'globals';
import ts from 'typescript-eslint';

export default ts.config(
  { ignores: ['node_modules/', '../internal/api/static/dist/', 'src/lib/api/generated/'] },
  js.configs.recommended,
  ts.configs.recommended,
  svelte.configs.recommended,
  { languageOptions: { globals: { ...globals.browser, ...globals.node } } },
  {
    files: ['**/*.svelte', '**/*.svelte.ts'],
    languageOptions: { parserOptions: { parser: ts.parser } },
  },
  {
    // Navigation uses hash routes built from the typed routes table, not SvelteKit's resolve().
    rules: { 'svelte/no-navigation-without-resolve': 'off' },
  },
);
