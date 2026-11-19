import { execFileSync } from 'node:child_process';
import { mkdtempSync, rmSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { fileURLToPath } from 'node:url';

export default function build() {
  const dir = mkdtempSync(join(tmpdir(), 'ratatoskr-e2e-build-'));
  const binary = join(dir, 'ratatoskr');
  const cleanup = () => rmSync(dir, { recursive: true, force: true });

  try {
    execFileSync('make', ['build', `BIN=${binary}`], {
      cwd: fileURLToPath(new URL('../../../', import.meta.url)),
      stdio: 'inherit',
    });
    process.env.RATATOSKR_E2E_BINARY = binary;
    return cleanup;
  } catch (error) {
    cleanup();
    throw error;
  }
}
