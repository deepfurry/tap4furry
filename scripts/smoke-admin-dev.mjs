import { join } from 'node:path';
import { localEnv } from './local-env.mjs';
import { root, run } from './process.mjs';

const api = localEnv('api');
const env = { ...localEnv('admin'), ADMIN_SMOKE_API_DATABASE_URL: api.DATABASE_URL,
  ADMIN_SMOKE_API_REDIS_URL: api.REDIS_URL, ADMIN_SMOKE_OWNER_DATABASE_URL: localEnv('migrator').DATABASE_URL };
run('go', ['run', './cmd/admin-smoke'], { cwd: join(root, 'server'), env });
