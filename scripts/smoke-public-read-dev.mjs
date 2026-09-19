import { join } from 'node:path';
import { localEnv } from './local-env.mjs';
import { root, run } from './process.mjs';

if (process.env.CI) throw new Error('CI must never load private Public read smoke inputs');
const env = { ...process.env,
  PUBLIC_READ_API_DATABASE_URL: localEnv('api').DATABASE_URL,
  PUBLIC_READ_MIGRATOR_DATABASE_URL: localEnv('migrator').DATABASE_URL,
};
run('go', ['run', './cmd/public-read-smoke'], { cwd: join(root, 'server'), env });
