import assert from 'node:assert/strict';
import { test } from 'node:test';
import * as publicClient from '../../packages/api-client/src/generated/public/client.ts';
import * as adminClient from '../../packages/api-client/src/generated/admin/client.ts';

test('admin: generated session clients preserve CSRF, cookie options, 204 and rate errors', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push([url, options]);
    if (url === '/api/auth/login') return new Response(JSON.stringify({ code: 'AUTH_RATE_LIMITED', message: 'Try again later.' }), { status: 429 });
    return new Response(null, { status: 204 });
  });
  const options = { credentials: 'same-origin', headers: new globalThis.Headers({ 'X-CSRF-Token': 'admin-csrf-fixture' }) };
  const login = await adminClient.login({ email: 'fixture@example.invalid', password: 'a fixture password only' }, options);
  assert.equal(login.status, 429);
  assert.equal(login.data.code, 'AUTH_RATE_LIMITED');
  const reauth = await adminClient.reauthenticate({ password: 'a fixture password only' }, options);
  assert.equal(reauth.status, 204);
  assert.equal(reauth.data, undefined);
  await adminClient.revokeSession('session-fixture', options);
  await adminClient.revokeOtherSessions(options);
  const logout = await adminClient.logout(options);
  assert.equal(logout.status, 204);
  assert.deepEqual(calls.map(([url, request]) => [url, request.method]), [
    ['/api/auth/login', 'POST'], ['/api/auth/reauthenticate', 'POST'],
    ['/api/me/sessions/session-fixture', 'DELETE'], ['/api/me/sessions/revoke-others', 'POST'], ['/api/auth/logout', 'POST'],
  ]);
  for (const [, request] of calls) {
    assert.equal(request.credentials, 'same-origin');
    assert.equal(new globalThis.Headers(request.headers).get('X-CSRF-Token'), 'admin-csrf-fixture');
  }
});

test('public: OAuth owner clients preserve Origin-bound CSRF options and safe errors', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push([url, options]);
    return new Response(JSON.stringify({ code: 'AUTH_REAUTH_REQUIRED', message: 'Reauthenticate to continue.' }), { status: 403 });
  });
  const options = { credentials: 'same-origin', headers: { 'X-CSRF-Token': 'csrf-test-fixture' } };
  await publicClient.linkOAuthProvider('google', options);
  await publicClient.reauthenticateOAuthProvider('github', options);
  const result = await publicClient.unlinkOAuthProvider('google', options);
  assert.equal(result.status, 403);
  assert.equal(result.data.code, 'AUTH_REAUTH_REQUIRED');
  assert.deepEqual(calls.map(([url, request]) => [url, request.method]), [
    ['/api/me/auth-methods/google/link', 'POST'],
    ['/api/me/auth-methods/github/reauthenticate', 'POST'],
    ['/api/me/auth-methods/google', 'DELETE'],
  ]);
  for (const [, request] of calls) {
    assert.equal(request.credentials, 'same-origin');
    assert.equal(new globalThis.Headers(request.headers).get('X-CSRF-Token'), 'csrf-test-fixture');
  }
});

for (const [name, client] of Object.entries({ public: publicClient, admin: adminClient })) {
  test(`${name}: generated fetch preserves degraded/503 responses and AbortSignal`, async t => {
    const controller = new AbortController();
    const calls = [];
    t.mock.method(globalThis, 'fetch', async (url, options) => {
      calls.push([url, options]);
      return new Response(JSON.stringify({ status: 'unavailable', postgres: 'down', redis: 'up' }), {
        status: 503, headers: { 'Content-Type': 'application/json' },
      });
    });
    const response = await client.getReady({ signal: controller.signal });
    assert.equal(response.status, 503);
    assert.equal(response.data.status, 'unavailable');
    assert.equal(calls[0][0], '/api/health/ready');
    assert.equal(calls[0][1].signal, controller.signal);
    assert.equal(calls[0][1].method, 'GET');
  });
  test(`${name}: generated liveness URL uses the same-origin API prefix`, () => {
    assert.equal(client.getGetLiveUrl(), '/api/health/live');
  });
}

test('public: auth clients preserve same-origin options, nullable PATCH fields and empty logout', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push([url, options]);
    if (url === '/api/auth/logout') return new Response(null, { status: 204 });
    return new Response(JSON.stringify({ code: 'AUTH_UNAUTHENTICATED', message: 'Please log in.' }), { status: 401 });
  });
  const controller = new AbortController();
  const options = { credentials: 'same-origin', signal: controller.signal, headers: { 'X-CSRF-Token': 'test-only-csrf' } };
  const login = await publicClient.login({ email: 'test@example.invalid', password: 'test password only' }, options);
  assert.equal(login.status, 401);
  assert.equal(login.data.code, 'AUTH_UNAUTHENTICATED');
  await publicClient.updateProfile({ bio: null }, options);
  const logout = await publicClient.logout(options);
  assert.equal(logout.status, 204);
  assert.equal(logout.data, undefined);
  assert.equal(calls[0][0], '/api/auth/login');
  assert.equal(calls[1][0], '/api/me/profile');
  assert.deepEqual(JSON.parse(calls[1][1].body), { bio: null });
  for (const [, request] of calls) {
    assert.equal(request.credentials, 'same-origin');
    assert.equal(request.signal, controller.signal);
    assert.equal(request.headers['X-CSRF-Token'], 'test-only-csrf');
  }
});

test('public: recovery and session clients preserve CSRF headers, empty 204 and safe error codes', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push([url, options]);
    if (url === '/api/auth/password/reset') return new Response(JSON.stringify({ code: 'AUTH_CHALLENGE_INVALID', message: 'Invalid link.' }), { status: 400 });
    return new Response(null, { status: 204 });
  });
  const options = { credentials: 'same-origin', headers: new globalThis.Headers({ 'X-CSRF-Token': 'csrf-test-fixture' }) };
  const reauth = await publicClient.reauthenticate({ password: 'a fixture password only' }, options);
  assert.equal(reauth.status, 204);
  assert.equal(reauth.data, undefined);
  await publicClient.revokeSession('session-fixture', options);
  await publicClient.revokeOtherSessions(options);
  const reset = await publicClient.resetPassword({ token: 'invalid-fixture', new_password: 'a fixture password only' }, { credentials: 'same-origin' });
  assert.equal(reset.status, 400);
  assert.equal(reset.data.code, 'AUTH_CHALLENGE_INVALID');
  assert.equal(new globalThis.Headers(calls[0][1].headers).get('X-CSRF-Token'), 'csrf-test-fixture');
  assert.equal(calls[1][0], '/api/me/sessions/session-fixture');
  assert.equal(calls[1][1].method, 'DELETE');
  assert.equal(calls[1][1].headers.get('X-CSRF-Token'), 'csrf-test-fixture');
  assert.equal(calls[2][0], '/api/me/sessions/revoke-others');
});
