import { readdirSync } from 'node:fs';
import { join } from 'node:path';
import { root, run } from './process.mjs';

function go(args) { run('go', args, { cwd: join(root, 'server') }); }
function goFiles(dir) {
  return readdirSync(dir, { withFileTypes: true }).flatMap(entry => {
    const path = join(dir, entry.name);
    return entry.isDirectory() ? (entry.name === 'bin' ? [] : goFiles(path))
      : entry.name.endsWith('.go') ? [path] : [];
  });
}
const tasks = {
  generate() { go(['generate', './...']); run('pnpm', ['exec', 'orval']); },
  lint() {
    run('pnpm', ['exec', 'eslint', '.']);
    const result = run('gofmt', ['-l', ...goFiles(join(root, 'server'))], { encoding: 'utf8', stdio: 'pipe' });
    if (result.stdout.trim()) throw new Error(`Run gofmt on:\n${result.stdout}`);
    go(['vet', './...']);
  },
  test() { run('node', ['--test', ...readdirSync(join(root, 'scripts/tests')).filter(name => name.endsWith('.test.mjs')).map(name => `scripts/tests/${name}`)]); go(['test', './...']); },
  build() {
    run('pnpm', ['-r', '--if-present', 'build']);
    for (const name of ['api', 'admin', 'worker']) go(['build', '-o', `bin/tap4furry-${name}${process.platform === 'win32' ? '.exe' : ''}`, `./cmd/${name}`]);
  },
  check() {
    run('node', ['scripts/audit-repository.mjs']);
    run('node', ['scripts/check-generated.mjs']);
    tasks.lint(); run('pnpm', ['typecheck']); tasks.test(); tasks.build();
  },
};
const task = tasks[process.argv[2]];
if (!task) throw new Error('Unknown repository task');
task();
