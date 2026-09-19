import { execFileSync } from 'node:child_process';
import { existsSync, readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';
import { root } from './process.mjs';

const git = args => execFileSync('git', args, { cwd: root, encoding: 'utf8', windowsHide: true });
const tracked = git(['ls-files', '-z']).split('\0').filter(Boolean);
const files = [...new Set(git(['ls-files', '-z', '--cached', '--others', '--exclude-standard']).split('\0').filter(Boolean))];
const privatePath = path => path.startsWith('.local/') || /^server\/env\/.*\.local$/.test(path);
if (tracked.some(privatePath)) throw new Error('Private local configuration is tracked; stop and untrack with user guidance');
for (const path of ['server/env/api.local', 'server/env/admin.local', 'server/env/worker.local', 'server/env/migrator.local', '.local/readonly.env', '.local/mail/capture.json']) {
  if (!git(['check-ignore', '--', path]).trim()) throw new Error(`Missing secret ignore rule: ${path}`);
}

// Only local audits read these files; neither their contents nor matched values
// are printed. CI must never read private developer configuration.
const privateValues = new Set();
if (!process.env.CI) {
  for (const path of ['server/env/api.local', 'server/env/admin.local', 'server/env/worker.local', 'server/env/migrator.local', '.local/readonly.env']) {
    if (!existsSync(join(root, path))) continue;
    let env;
    try { env = parseEnv(readFileSync(join(root, path), 'utf8')); }
    catch { throw new Error('Cannot audit private launch input (contents withheld)'); }
    for (const [key, value] of Object.entries(env)) {
      if (/(PASSWORD|SECRET|TOKEN|DSN|DATABASE_URL|REDIS_URL|OAUTH_CLIENT_ID)/i.test(key) && value.length >= 8) privateValues.add(value);
      try {
        const url = new URL(value);
        if (url.password.length >= 8) { privateValues.add(url.password); privateValues.add(decodeURIComponent(url.password)); }
        if (url.hostname && !['localhost', '127.0.0.1', '[::1]'].includes(url.hostname)) privateValues.add(url.hostname);
      } catch { /* Non-URL configuration has no connection hostname. */ }
    }
  }
}
const forbiddenModules = /(?:go\.mongodb\.org\/|github\.com\/nats-io\/|gorm\.io\/|github\.com\/jinzhu\/gorm|github\.com\/pgvector\/)/;
for (const path of files) {
  if (!existsSync(join(root, path))) continue;
  const content = readFileSync(join(root, path), 'utf8');
  if (/\b100\.(?:6[4-9]|[7-9]\d|1[01]\d|12[0-7])\.\d{1,3}\.\d{1,3}\b|\b[\w.-]+\.ts\.net\b/.test(content)
      || /-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----/.test(content)
      || /(?:#token=|["'](?:token|csrf_token)["']\s*:\s*["'])[A-Za-z0-9_-]{43}(?=["'\s]|$)/.test(content)
      || [...privateValues].some(value => content.includes(value))) {
    throw new Error(`Potential private material detected in ${path}; values withheld`);
  }
  if ((path === 'server/go.mod' || path.endsWith('/package.json')) && forbiddenModules.test(content)) throw new Error(`Forbidden dependency in ${path}`);
  if (path.endsWith('.sql') && /CREATE\s+EXTENSION\s+(?:IF\s+NOT\s+EXISTS\s+)?"?vector\b/i.test(content)) throw new Error(`Prohibited vector extension in ${path}`);
  if (path.endsWith('.go') && !path.startsWith('server/internal/jobs/') && /"github\.com\/riverqueue\//.test(content)) throw new Error(`River import outside Jobs: ${path}`);
  if (/^server\/internal\/(auth|identity)\/.*\.go$/.test(path) && /"(?:github\.com\/gofiber\/|github\.com\/google\/uuid|github\.com\/redis\/|github\.com\/deepfurry\/tap4furry\/server\/internal\/transport\/)/.test(content)) throw new Error(`Application boundary violation: ${path}`);
  if (/^apps\//.test(path) && /@tap4furry\/api-client\/.*generated/.test(content)) throw new Error(`Deep generated client import: ${path}`);
  if (/^server\/internal\/oauthprovider\/.*\.go$/.test(path) && /(?:SkipClientIDCheck|SkipIssuerCheck|SkipExpiryCheck|InsecureSkipSignatureCheck)\s*:\s*true/.test(content)) throw new Error(`Unsafe OIDC verifier in ${path}`);
}
if (files.filter(path => path.endsWith('go.mod')).join() !== 'server/go.mod' || files.some(path => path.endsWith('go.work'))) throw new Error('Expected one server/go.mod and no go.work');
const forbiddenDomains = ['resource', 'taxonomy', 'collection', 'contribution', 'discovery', 'exchange', 'discussion', 'poll', 'trust', 'moderation', 'notification', 'analytics'];
if (files.some(path => forbiddenDomains.some(domain => path.startsWith(`server/internal/${domain}/`)))) throw new Error('Future product domain scaffold before its phase');
const runtimeDependencies = execFileSync('go', ['-C', 'server', 'list', '-deps', './cmd/api', './cmd/admin', './cmd/worker'], { cwd: root, encoding: 'utf8', windowsHide: true });
if (/pressly\/goose|river\/rivermigrate|pgx\/v5\/stdlib|go\.mongodb|nats-io|gorm\.io|pgvector|opentelemetry/.test(runtimeDependencies)) throw new Error('Migration or forbidden dependency in a runtime binary');
const stagedDiff = git(['diff', '--cached', '--no-ext-diff', '--unified=0']);
if ([...privateValues].some(value => stagedDiff.includes(value))) throw new Error('Private material detected in staged diff; values withheld');
console.log('Repository secret, namespace, dependency and boundary audit passed (no private values emitted)');
