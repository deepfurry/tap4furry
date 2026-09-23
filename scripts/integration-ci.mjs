import { join } from 'node:path';
import { root, run } from './process.mjs';

if (process.env.CI !== 'true' || process.env.GFP_DISPOSABLE_INFRA !== '1') throw new Error('CI integration requires explicitly disposable infrastructure');
// Endpoints are fixed loopback CI services, never inherited developer URLs.
const env = { ...process.env, APP_ENV: 'test', CI_POSTGRES_URL: 'postgres://postgres:postgres@127.0.0.1:5432/postgres?sslmode=disable',
  MAIL_MODE: 'disabled', RESEND_API_KEY: '', RESEND_TEST_RECIPIENT: '',
  REDIS_URL: 'redis://gfp_runtime:gfp_ci_only@127.0.0.1:6379/0', REDIS_KEY_PREFIX: 'gfp:', RIVER_SCHEMA: 'river', HTTP_ADDR: '127.0.0.1:8080', PUBLIC_ORIGIN: 'http://localhost:4321', ADMIN_ORIGIN: 'http://localhost:5173' };
const cwd = join(root, 'server');
run('go', ['run', './cmd/ci-setup'], { cwd, env });
function serviceEnv(service) { return { ...env, DATABASE_URL: `postgres://gfp_${service}:gfp_ci_only@127.0.0.1:5432/gfp_ci?sslmode=disable` }; }
run('go', ['run', './cmd/migrate', '-database', 'gfp_ci'], { cwd, env: serviceEnv('migrator') });
// Repeat up to verify version tracking and the idempotent official River path.
run('go', ['run', './cmd/migrate', '-database', 'gfp_ci'], { cwd, env: serviceEnv('migrator') });
const resourceEnv = { ...env, GFP_RESOURCE_INTEGRATION: '1' };
run('go', ['test', '-count=1', '-timeout=2m', '-run', 'TestIntegrationResourceMigrationRoundTrip', './internal/database/migrate'], { cwd, env: resourceEnv });
for (const service of ['api', 'admin', 'migrator', 'worker']) run('go', ['run', './cmd/smoke', '-service', service, '-database', 'gfp_ci'], { cwd, env: serviceEnv(service) });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegration', './internal/transport/public', './internal/redisstore', './internal/oauthprovider'], { cwd, env: { ...env, GFP_AUTH_INTEGRATION: '1' } });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegrationPublic', './internal/transport/public'], { cwd, env: { ...env, GFP_PUBLIC_READ_INTEGRATION: '1' } });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegrationCuration', './internal/transport/public'], { cwd, env: { ...env, GFP_CURATION_INTEGRATION: '1' } });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegrationContribution', './internal/transport/public'], { cwd, env: { ...env, GFP_CONTRIBUTION_INTEGRATION: '1' } });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegrationGovernance', './internal/transport/public'], { cwd, env: { ...env, GFP_GOVERNANCE_INTEGRATION: '1' } });
run('go', ['test', '-count=1', '-timeout=3m', '-run', 'TestIntegration', './internal/database/resourcecheck'], { cwd, env: resourceEnv });
for (const role of ['api', 'admin', 'worker', 'migrator', 'readonly']) resourceEnv[`RESOURCE_SMOKE_${role.toUpperCase()}_DATABASE_URL`] = serviceEnv(role).DATABASE_URL;
run('go', ['run', './cmd/resource-smoke', '-database', 'gfp_ci'], { cwd, env: resourceEnv });
run('pnpm', ['--filter', '@tap4furry/web', 'build']);
run('node', ['scripts/ssr-public-read.mjs']);
