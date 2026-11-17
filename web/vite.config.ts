import { svelte } from '@sveltejs/vite-plugin-svelte';
import tailwindcss from '@tailwindcss/vite';
import { writeFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { svelteTesting } from '@testing-library/svelte/vite';
import type { Plugin } from 'vite';
import { defineConfig } from 'vitest/config';

// The Go binary embeds the bundle from internal/api/static/dist (go:embed cannot reach ../web).
const outDir = fileURLToPath(new URL('../internal/api/static/dist', import.meta.url));

// emptyOutDir removes the committed .gitkeep that keeps the go:embed pattern valid before a build.
const keepDist = (): Plugin => ({
  name: 'ratatoskr-keep-dist',
  apply: 'build',
  closeBundle() {
    writeFileSync(`${outDir}/.gitkeep`, '');
  },
});

export default defineConfig({
  plugins: [tailwindcss(), svelte(), svelteTesting(), keepDist()],
  resolve: { alias: { $lib: fileURLToPath(new URL('./src/lib', import.meta.url)) } },
  build: { outDir, emptyOutDir: true, target: 'es2022' },
  server: { proxy: { '/api': 'http://127.0.0.1:8080' } },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test-setup.ts'],
    include: ['src/**/*.test.ts'],
  },
});
