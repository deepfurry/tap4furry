import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync, readdirSync } from 'node:fs';
import * as pub from '../../packages/api-client/src/generated/public/client.ts';
import * as admin from '../../packages/api-client/src/generated/admin/client.ts';

test('governance clients preserve closed decisions, request keys and explicit CAS failures', async t => {
  const calls = [];
  t.mock.method(globalThis, 'fetch', async (url, options) => {
    calls.push([url, options]);
    return new Response(JSON.stringify({ code: 'GOVERNANCE_CONFLICT', message: 'Reload explicitly.' }), { status: 409 });
  });
  const options = { credentials: 'same-origin', cache: 'no-store', headers: { 'X-CSRF-Token': 'fixture' } };
  const report = { request_id: 'fixture-request', target_kind: 'resource', resource_id: 'fixture-resource', reason: 'other', body: '<script>literal evidence</script>' };
  assert.equal((await pub.submitReport(report, options)).status, 409);
  const decision = { request_id: 'fixture-decision', expected_report_version: 2, expected_resource_version: 7, mode: 'publication', publication_state: 'restricted', reason: 'Policy review', safe_message: 'Temporarily unavailable', internal_note: 'Private review' };
  await admin.resolveReport('fixture-report', decision, options);
  await pub.withdrawReport('fixture-report', { request_id: 'fixture-withdrawal' }, options);
  assert.equal(calls.length, 3); // No generated retry on conflict.
  assert.deepEqual(JSON.parse(calls[0][1].body), report);
  assert.deepEqual(JSON.parse(calls[1][1].body), decision);
  assert.deepEqual(calls.map(([url]) => url), ['/api/reports', '/api/reports/fixture-report/resolve', '/api/me/reports/fixture-report/withdraw']);
  for (const [, request] of calls) {
    assert.equal(request.cache, 'no-store');
    assert.equal(request.credentials, 'same-origin');
    assert.equal(new globalThis.Headers(request.headers).get('X-CSRF-Token'), 'fixture');
  }
});

test('governance text stays inert and private drafts never enter browser persistence', () => {
  for (const dir of ['apps/web/src/components/reports', 'apps/admin/src/pages/governance']) {
    for (const name of readdirSync(dir)) assert.doesNotMatch(readFileSync(`${dir}/${name}`, 'utf8'), /dangerouslySetInnerHTML|set:html|localStorage|sessionStorage|setInterval\s*\(|onMutate\s*:/);
  }
  const shared = readFileSync('apps/admin/src/pages/governance/shared.tsx', 'utf8');
  assert.match(shared, /retry: false/);
  assert.match(shared, /invalidateQueries/);
  assert.match(shared, /Your input is preserved/);
  for (const name of ['ReportsPage', 'UsersPage', 'SourcesPage', 'DistributionPanel']) {
    assert.match(readFileSync(`apps/admin/src/pages/governance/${name}.tsx`, 'utf8'), /useDirtyGuard/);
  }
  for (const name of ['report.astro', 'me/reports.astro', 'reports/[id].astro']) {
    const page = readFileSync(`apps/web/src/pages/${name}`, 'utf8');
    assert.match(page, /AccountLayout/);
  }
  const layout = readFileSync('apps/web/src/layouts/AccountLayout.astro', 'utf8');
  assert.match(layout, /no-store/); assert.match(layout, /noindex/);
});
