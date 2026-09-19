import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';
import { root, run } from './process.mjs';

// Never load application credentials, print child output or retry a delivery.
try {
  if (process.env.CI) throw new Error('Local smoke only');
  const input = parseEnv(readFileSync(join(root, '.local/resend-smoke.env'), 'utf8'));
  if (!input.RESEND_API_KEY?.trim() || !input.RESEND_TEST_RECIPIENT?.trim()) throw new Error('Missing private input');
  const env = {
    ...process.env,
    RESEND_SMOKE_OPT_IN: '1',
    RESEND_API_KEY: input.RESEND_API_KEY,
    RESEND_TEST_RECIPIENT: input.RESEND_TEST_RECIPIENT,
    MAIL_FROM: input.MAIL_FROM ?? 'Tap4Furry <no-reply@tap4furry.com>',
    MAIL_REPLY_TO: input.MAIL_REPLY_TO ?? 'support@tap4furry.com',
    PUBLIC_ORIGIN: input.PUBLIC_ORIGIN ?? 'http://localhost:4321',
  };
  run('go', ['run', './cmd/mail-smoke'], { cwd: join(root, 'server'), env, stdio: 'pipe', timeout: 120_000 });
  console.log('Resend mail smoke PASS (one synthetic verification email accepted)');
} catch {
  console.error('Resend mail smoke FAIL (check private input and provider connectivity; values withheld)');
  process.exitCode = 1;
}
