import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync, readdirSync } from 'node:fs';
import * as pub from '../../packages/api-client/src/generated/public/client.ts';
import * as admin from '../../packages/api-client/src/generated/admin/client.ts';

test('contribution clients preserve null, idempotency, CSRF and explicit conflict without retry', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => { calls.push([url, options]); return new Response(JSON.stringify({ code: 'RESOURCE_VERSION_CONFLICT', message: 'Reload current content.' }), { status: 409 }); });
  const options = { credentials: 'same-origin', cache: 'no-store', headers: { 'X-CSRF-Token': 'fixture' } };
  const body = { kind: 'update_resource', request_id: 'request-fixture', target_resource_id: 'resource-fixture', base_revision: 'opaque-fixture', reason: 'Correction', content: { summary: null } };
  const response = await pub.submitContribution(body, options);
  assert.equal(response.status, 409); assert.equal(calls.length, 1);
  assert.deepEqual(JSON.parse(calls[0][1].body), body);
  await pub.withdrawContribution('proposal-fixture', options);
  await admin.rejectContribution('proposal-fixture', { message: 'Reason', internal_note: 'Private' }, options);
  assert.deepEqual(calls.map(([url]) => url), ['/api/contributions', '/api/me/contributions/proposal-fixture/withdraw', '/api/contributions/proposal-fixture/reject']);
  for (const [, request] of calls) { assert.equal(request.credentials, 'same-origin'); assert.equal(request.cache, 'no-store'); assert.equal(new globalThis.Headers(request.headers).get('X-CSRF-Token'), 'fixture'); }
  assert.equal(pub.getListMyContributionsUrl({ status: 'pending', page: 2 }), '/api/me/contributions?status=pending&page=2');
});

test('private proposal UI retains inputs and does not introduce HTML execution or persisted drafts', () => {
  for (const dir of ['apps/web/src/components/contributions', 'apps/admin/src/pages/contributions']) {
    for (const name of readdirSync(dir)) assert.doesNotMatch(readFileSync(`${dir}/${name}`, 'utf8'), /dangerouslySetInnerHTML|set:html|localStorage|sessionStorage|setInterval\s*\(|onMutate\s*:/);
  }
  const review = readFileSync('apps/admin/src/pages/contributions/ContributionReviewPage.tsx', 'utf8');
  assert.match(review, /useDirtyGuard/); assert.match(review, /retry: false/); assert.match(review, /invalidateQueries/); assert.match(review, /Reload canonical review/);
  const form = readFileSync('apps/web/src/components/contributions/SubmitForm.tsx', 'utf8');
  assert.match(form, /useContributionDirty/); assert.match(form, /request\?\.body === serialized \? request.id/);
});
