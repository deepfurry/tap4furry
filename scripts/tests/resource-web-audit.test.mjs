import assert from 'node:assert/strict';
import { test } from 'node:test';
import { readFileSync } from 'node:fs';
import { auditResourceWeb } from '../resource-web-audit.mjs';

const sink = ['apps/web/src/components/Markdown.astro', readFileSync(new URL('../../apps/web/src/components/Markdown.astro', import.meta.url), 'utf8')];
test('Resource server-only helpers cannot cross the browser boundary or add HTML sinks', () => {
  assert.doesNotThrow(() => auditResourceWeb([sink, ['apps/web/src/pages/resources/index.astro', "---\nimport { read } from '../../lib/public-api.server';\n---\n<p>Safe</p>"]]));
  for (const entry of [
    ['apps/web/src/components/Unsafe.tsx', "import { read } from '../lib/public-api.server';"],
    ['apps/web/src/lib/indirect.ts', "export * from './markdown.server.ts';"],
    ['apps/web/src/components/Unsafe.tsx', "const reader = import('../lib/public-api.server');"],
    ['apps/web/src/pages/test.astro', "---\nconst x = 1;\n---\n<script>import './public-api.server';</script>"],
    ['apps/web/src/pages/test.astro', '<div set:html={arbitrary} />'],
    ['apps/web/src/pages/resources/index.astro', '<Interactive client:load />'],
  ]) assert.throws(() => auditResourceWeb([sink, entry]));
  assert.throws(() => auditResourceWeb([]));
  assert.throws(() => auditResourceWeb([[sink[0], sink[1].replace('renderMarkdown(Astro.props.source)', 'Astro.props.source')]]));
});
