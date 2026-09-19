import assert from 'node:assert/strict';
import { spawnSync } from 'node:child_process';
import test from 'node:test';
import { root } from '../process.mjs';

test('real mail smoke refuses CI before reading any private input or sending', () => {
  const result = spawnSync(process.execPath, ['scripts/smoke-mail-resend-dev.mjs'], {
    cwd: root, env: { ...process.env, CI: 'true' }, encoding: 'utf8', windowsHide: true,
  });
  assert.equal(result.status, 1);
  assert.equal(result.stdout, '');
  assert.equal(result.stderr.trim(), 'Resend mail smoke FAIL (check private input and provider connectivity; values withheld)');
});
