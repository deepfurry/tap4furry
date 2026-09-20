import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync, readdirSync } from 'node:fs';
import { auditAdminCuration } from '../admin-curation-audit.mjs';
import { canAdminAccess, canEditorial, canModerate, canAdministrate } from '../../apps/admin/src/lib/capabilities.ts';
import { externalIDsError } from '../../apps/admin/src/lib/external-ids.ts';
import * as client from '../../packages/api-client/src/generated/admin/client.ts';

test('static capability UX matches Moderator, Editor and Admin boundaries', () => {
  for (const [roles, expected] of [[[], [false, false, false, false]], [['moderator'], [true, false, true, false]], [['editor'], [true, true, false, false]], [['admin'], [true, true, true, true]]]) {
    assert.deepEqual([canAdminAccess(roles), canEditorial(roles), canModerate(roles), canAdministrate(roles)], expected);
  }
});
test('external ID whole-set helper rejects blank and normalized duplicate rows', () => {
  assert.equal(externalIDsError([]), null);
  assert.ok(externalIDsError([{ namespace: '', external_id: '123' }]));
  assert.ok(externalIDsError([{ namespace: 'steam_app', external_id: '123' }, { namespace: 'steam_app', external_id: ' 123 ' }]));
  assert.equal(externalIDsError([{ namespace: 'steam_app', external_id: '123' }, { namespace: 'steam_app', external_id: '456' }]), null);
});
test('Admin source boundaries reject optimistic writes, retries and new Source deletion', () => {
  const base = 'apps/admin/src';
  const entries = readdirSync(base, { recursive: true, withFileTypes: true }).filter(entry => entry.isFile()).map(entry => {
    const path = `${entry.parentPath}/${entry.name}`.replaceAll('\\', '/');
    return [path, readFileSync(path, 'utf8')];
  });
  assert.doesNotThrow(() => auditAdminCuration(entries));
  for (const source of ['useMutation({ retry: true })', 'onMutate: optimistic', 'client.setQueryData(key, data)', 'setInterval(save, 1000)', 'localStorage.setItem(key, draft)']) assert.throws(() => auditAdminCuration([...entries, [`${base}/unsafe.tsx`, source]]));
  assert.throws(() => auditAdminCuration(entries.filter(([path]) => !path.endsWith('router.tsx'))));
});
test('generated curation clients send expected version and preserve conflict without retry', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => { calls.push([url, options]); return new Response(JSON.stringify({ code: 'RESOURCE_VERSION_CONFLICT', message: 'Reload latest.' }), { status: 409 }); });
  const response = await client.patchResource('fixture-id', { lifecycle: 'active' }, { expected_version: 3 }, { credentials: 'same-origin', headers: { 'X-CSRF-Token': 'fixture' } });
  assert.equal(response.status, 409); assert.equal(response.data.code, 'RESOURCE_VERSION_CONFLICT'); assert.equal(calls.length, 1);
  assert.equal(calls[0][0], '/api/resources/fixture-id?expected_version=3');
  assert.equal(calls[0][1].credentials, 'same-origin');
  assert.equal(new globalThis.Headers(calls[0][1].headers).get('X-CSRF-Token'), 'fixture');
  assert.equal(client.getListResourcesUrl({ slug: 'exact-slug' }), '/api/resources?slug=exact-slug');
});
