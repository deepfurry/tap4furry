import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { parseEnv } from 'node:util';
import { localEnv } from './local-env.mjs';
import { root, run } from './process.mjs';

if (process.env.CI) throw new Error('CI must never load private resource smoke inputs');
const env = { ...process.env };
for (const role of ['api', 'admin', 'worker', 'migrator']) {
  env[`RESOURCE_SMOKE_${role.toUpperCase()}_DATABASE_URL`] = localEnv(role).DATABASE_URL;
}
try {
  env.RESOURCE_SMOKE_READONLY_DATABASE_URL = parseEnv(readFileSync(join(root, '.local/readonly.env'), 'utf8')).DATABASE_URL;
} catch { throw new Error('Unable to load private readonly input (values withheld)'); }
if (!env.RESOURCE_SMOKE_READONLY_DATABASE_URL) throw new Error('Missing private readonly database input (values withheld)');
run('go', ['run', './cmd/resource-smoke'], { cwd: join(root, 'server'), env });
