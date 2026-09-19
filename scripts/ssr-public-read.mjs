import assert from 'node:assert/strict';
import { createServer } from 'node:http';
import { createServer as createSocketServer } from 'node:net';
import { spawn } from 'node:child_process';
import { once } from 'node:events';
import { join } from 'node:path';
import { root } from './process.mjs';

const cache = 'public, max-age=0, s-maxage=60, stale-while-revalidate=30';
const id = '0198a100-0000-7000-8000-000000000001';
const calls = [];
const core = { id, slug: 'test-resource', name: 'Test Resource', summary: 'A community resource.', category: { id, slug: 'games', name: 'Games' }, lifecycle: 'discontinued', content_rating: 'explicit', published_at: '2026-09-19T00:00:00Z', updated_at: '2026-09-19T00:00:00Z' };
const markdown = `# Markdown heading

## Safe heading

**Strong** and *emphasis* and ~~strike~~.

<script>alert('fixture')</script><img src=x onerror=alert(1)><svg onload=alert(1)></svg>

[unsafe](javascript:alert(1)) [encoded](java&#x73;cript:alert(1)) [data](data:text/html,bad) [file](file:///tmp/x) [vb](vbscript:bad) [mail](mailto:fixture@example.invalid)

![Image disabled](https://example.invalid/picture.png)

[External](https://example.invalid/guide) [Relative](/resources) [Protocol relative](//example.invalid/)

> Quote

- List item

1. Ordered item

| Column | Value |
| --- | --- |
| A | B |

\`inline\`

\`\`\`html
<script>code only</script>
\`\`\`
`;
const detail = { ...core, requested_locale: 'en', default_locale: 'en', available_locales: ['en', 'ja'], description: markdown, tags: [{ id, slug: 'community', name: 'Community' }], sources: [{ id, url: 'https://example.invalid/canonical-source', label: 'Canonical source', source_type: 'official', availability_state: 'unavailable', is_primary: true }], relations: [{ type: 'related_to', direction: 'symmetric', resource: { id, slug: 'another-resource', name: 'Another resource' } }], external_ids: [{ namespace: 'fixture', external_id: 'opaque-id' }] };

const api = createServer((req, res) => {
  const url = new URL(req.url, 'http://localhost');
  calls.push({ url, headers: req.headers });
  res.setHeader('Content-Type', 'application/json');
  const send = (status, value) => { res.statusCode = status; res.end(JSON.stringify(value)); };
  if (url.pathname === '/resources') {
    const page = Number(url.searchParams.get('page'));
    return send(200, { items: page === 3 ? [] : [core], page, page_size: 24, has_next: page < 3 });
  }
  switch (url.pathname) {
    case '/resources/missing': return send(404, { code: 'RESOURCE_NOT_FOUND', message: 'Private upstream diagnostic fixture' });
    case '/resources/invalid': return send(400, { code: 'VALIDATION_ERROR', message: 'Private upstream diagnostic fixture' });
    case '/resources/unavailable': return send(500, { code: 'INTERNAL_ERROR', message: 'Private upstream diagnostic fixture' });
    case '/resources/bad-json': return res.end('not JSON');
    case '/resources/bad-shape': return send(200, { ...detail, category: null });
    case '/resources/redirect': res.writeHead(302, { Location: 'https://example.invalid/never-fetch' }); return res.end();
    case '/resources/timeout': {
      const timer = setTimeout(() => send(200, detail), 6500);
      req.on('close', () => clearTimeout(timer));
      return;
    }
    case '/resources/no-summary': return send(200, { ...detail, slug: 'no-summary', summary: null });
    default: return send(200, { ...detail, requested_locale: url.searchParams.get('locale') });
  }
});
api.listen(0, '127.0.0.1');
await once(api, 'listening');
const apiOrigin = `http://127.0.0.1:${api.address().port}`;
const reserve = createSocketServer();
reserve.listen(0, '127.0.0.1'); await once(reserve, 'listening');
const port = reserve.address().port; await new Promise(resolve => reserve.close(resolve));
const origin = `http://127.0.0.1:${port}`;
const child = spawn(process.execPath, ['dist/server/entry.mjs'], { cwd: join(root, 'apps/web'), windowsHide: true, stdio: ['ignore', 'pipe', 'pipe'], env: { ...process.env, HOST: '127.0.0.1', PORT: String(port), NODE_ENV: 'production', API_INTERNAL_ORIGIN: apiOrigin } });
let output = '';
child.stdout.on('data', x => { output = (output + x).slice(-4000); });
child.stderr.on('data', x => { output = (output + x).slice(-4000); });
const exited = once(child, 'exit');
const wait = ms => new Promise(resolve => setTimeout(resolve, ms));

try {
  let ready = false;
  for (let i = 0; i < 80; i++) {
    if (child.exitCode !== null) throw new Error('Astro acceptance server exited: ' + output);
    try { if ((await fetch(origin + '/resources')).status === 200) { ready = true; break; } } catch { /* wait for listener */ }
    await wait(250);
  }
  assert.ok(ready, 'Astro acceptance listener unavailable');
  async function page(path, status) {
    const response = await fetch(origin + path, { headers: { Cookie: 'tap4furry_session=browser-fixture', Authorization: 'Bearer browser-fixture', 'X-CSRF-Token': 'browser-fixture' } });
    assert.equal(response.status, status, path);
    assert.equal(response.headers.get('cache-control'), status === 200 ? cache : 'no-store');
    assert.equal(response.headers.get('set-cookie'), null);
    assert.ok(!response.headers.get('vary')?.toLowerCase().includes('accept-language'));
    const html = await response.text();
    assert.doesNotMatch(html, /<astro-island\b|<img\b|Private upstream diagnostic fixture|127\.0\.0\.1|API_INTERNAL_ORIGIN/i);
    if (status !== 200) assert.match(html, /name="robots" content="noindex, nofollow"/);
    return html;
  }
  let html = await page('/resources', 200);
  assert.match(html, /Test Resource/); assert.match(html, /href="\/resources\/test-resource"/);
  assert.match(html, /rel="canonical" href="https:\/\/tap4furry.com\/resources"/);
  assert.match(html, /<html lang="en"/);
  assert.equal(calls.at(-1).url.searchParams.get('locale'), 'en');
  html = await page('/resources?page=2&locale=JA', 200);
  assert.match(html, /<html lang="ja"/);
  assert.match(html, /rel="canonical" href="https:\/\/tap4furry.com\/resources\?page=2"/);
  assert.match(html, /rel="prev" href="\/resources\?locale=ja"/);
  assert.match(html, /rel="next" href="\/resources\?page=3(?:&amp;|&)locale=ja"/);
  assert.match(html, /href="\/resources\/test-resource\?locale=ja"/);
  html = await page('/resources?page=3', 200); assert.match(html, /No published resources yet/); assert.doesNotMatch(html, /rel="next"/);
  html = await page('/resources/test-resource?locale=zh-hans&page=9', 200);
  assert.match(html, /<html lang="zh-Hans"/);
  assert.match(html, /<title>Test Resource · Tap4Furry<\/title>/);
  assert.match(html, /name="description" content="A community resource\."/);
  assert.match(html, /rel="canonical" href="https:\/\/tap4furry.com\/resources\/test-resource"/);
  assert.match(html, /property="og:title"/); assert.doesNotMatch(html, /og:image/);
  assert.match(html, /href="\/resources\/another-resource\?locale=zh-Hans"/);
  assert.doesNotMatch(html, /href="[^"\s]*page=9/);
  assert.equal((html.match(/<h1\b/g) ?? []).length, 1);
  assert.match(html, /<h2>Markdown heading<\/h2>/);
  assert.match(html, /<strong>Strong<\/strong>/); assert.match(html, /<table>/);
  const renderedMarkdown = html.split('<div class="resource-markdown">')[1]?.split('</div>')[0];
  assert.ok(renderedMarkdown, 'Markdown SSR content missing');
  assert.doesNotMatch(renderedMarkdown, /<(?:script|iframe|svg|style|object|embed|video|audio)\b/i);
  assert.doesNotMatch(html, /<script\b/i);
  assert.doesNotMatch(html, /href="(?:javascript|data|file|vbscript|mailto):|target="_blank"/i);
  assert.match(html, /href="https:\/\/example.invalid\/guide" rel="ugc nofollow noreferrer"/);
  assert.match(html, /<a href="https:\/\/example.invalid\/canonical-source">/);
  assert.equal(calls.at(-1).url.searchParams.get('locale'), 'zh-Hans');
  html = await page('/resources/no-summary', 200); assert.match(html, /Discover Test Resource on Tap4Furry\./);
  for (const path of ['/resources?locale=en_US', '/resources?locale=', '/resources?page=0', '/resources?page=bad']) await page(path, 400);
  for (const [slug, status] of [['missing', 404], ['UPPER', 404], ['invalid', 400], ['unavailable', 503], ['bad-json', 503], ['bad-shape', 503], ['redirect', 503]]) await page('/resources/' + slug, status);
  const start = Date.now(); await page('/resources/timeout', 503); assert.ok(Date.now() - start >= 4500 && Date.now() - start < 6500, 'five-second upstream timeout');
  for (const call of calls) {
    assert.ok(call.url.pathname.startsWith('/resources'));
    assert.equal(call.headers.accept, 'application/json');
    for (const header of ['cookie', 'authorization', 'x-csrf-token']) assert.equal(call.headers[header], undefined);
  }
  console.log('Astro anonymous SSR, generated URL routing, Markdown security, zero islands, locale, SEO, cache, status and timeout acceptance PASS');
} finally {
  child.kill(); await exited;
  api.closeAllConnections(); await new Promise(resolve => api.close(resolve));
}
