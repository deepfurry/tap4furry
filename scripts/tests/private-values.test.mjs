import assert from 'node:assert/strict';
import test from 'node:test';
import { privateEnvValues } from '../private-values.mjs';

test('private audit recognizes API keys, recipients and existing credentials', () => {
  const env = Object.fromEntries(['RESEND_API_KEY', 'OTHER_API_KEY', 'API_KEY', 'RESEND_TEST_RECIPIENT', 'TEST_EMAIL', 'TEST_PASSWORD', 'CSRF_SECRET', 'GOOGLE_OAUTH_CLIENT_ID', 'GOOGLE_OAUTH_CLIENT_SECRET'].map((key, i) => [key, `synthetic-private-fixture-${i}`]));
  const values = privateEnvValues(env);
  for (const value of Object.values(env)) assert.ok(values.has(value), 'private input was omitted');
  assert.equal(values.size, Object.keys(env).length);
});

test('private audit preserves encoded password and remote host detection', () => {
  const values = privateEnvValues({ DATABASE_URL: 'postgres://fixture:synthetic%2Dpassword@db.example.invalid/gfp_ci' });
  assert.ok(values.has('synthetic%2Dpassword'));
  assert.ok(values.has('synthetic-password'));
  assert.ok(values.has('db.example.invalid'));
  const publicValues = privateEnvValues({ MAIL_MODE: 'resend', MAIL_FROM: 'Tap4Furry <no-reply@tap4furry.com>', MAIL_REPLY_TO: 'support@tap4furry.com', PUBLIC_ORIGIN: 'http://localhost:4321' });
  assert.equal(publicValues.size, 0);
});
