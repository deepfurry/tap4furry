import { join } from 'node:path';
import { localEnv } from './local-env.mjs';
import { root, run } from './process.mjs';

// Environment input stays private; only the explicit account/role flags go to CLI.
const args = process.argv.slice(2);
if (!['grant-role', 'revoke-role', 'list-roles'].includes(args[0])) throw new Error('Choose grant-role, revoke-role or list-roles');
run('go', ['run', './cmd/adminctl', ...args], { cwd: join(root, 'server'), env: localEnv('migrator') });
