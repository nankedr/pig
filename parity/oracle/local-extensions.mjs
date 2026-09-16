import { caseDigest, observationDigest } from './fixture-hash.mjs';
import { execFileSync } from 'node:child_process';
import { realpathSync, symlinkSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { pathToFileURL } from 'node:url';

const pi = resolve(process.argv.find((v, i) => i > 1 && !v.startsWith('--')) ?? '.upstream/pi');
const output = 'parity/oracle/fixtures/local-extensions.json';
const lock = JSON.parse(readFileSync('parity/baseline/upstream.lock.json', 'utf8'));
if (execFileSync('git', ['-C', pi, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git', ['-C', pi, 'status', '--porcelain', '--untracked-files=no'], { encoding: 'utf8' }).trim()) throw Error('dirty baseline');
const reference = 'packages/coding-agent/src/core/package-manager.ts';
const { DefaultPackageManager } = await import(pathToFileURL(join(pi, reference)));
const { SettingsManager } = await import(pathToFileURL(join(pi, 'packages/coding-agent/src/core/settings-manager.ts')));
const { loadExtensions } = await import(pathToFileURL(join(pi, 'packages/coding-agent/src/core/extensions/loader.ts')));
const files = {
  'agent/extensions/a.ts': 'export default 42;',
  'agent/extensions/b/index.ts': 'export default 42;',
  'agent/extensions/b/index.js': 'export default 42;',
  'agent/extensions/ignored.ts': 'export default 42;',
  'agent/extensions/.gitignore': 'ignored.ts\n',
  'agent/extensions/deep/nested/no.ts': 'export default 42;',
  'agent/extensions/.hidden.ts': 'export default 42;',
  'agent/extensions/node_modules/no.js': 'export default 42;',
  'repo/.pi/extensions/project.js': 'export default 42;',
  'agent/more/local.js': 'export default 42;',
  'extra/explicit.js': 'export default 42;',
};
const scenarios = [
  { name: 'untrusted', files, trusted: false },
  { name: 'trusted', files, trusted: true },
  { name: 'global-path-before-project-auto', files, trusted: true, global: { extensions: ['../repo/.pi/extensions/project.js'] } },
  { name: 'global-disabled-project-auto', files, trusted: true, global: { extensions: ['../repo/.pi/extensions/project.js', '-../repo/.pi/extensions/project.js'] } },
  { name: 'settings-filter', files, trusted: true, global: { extensions: ['more', '-extensions/a.ts'] } },
  { name: 'explicit-disabled', files, trusted: false, disabled: true, paths: ['../extra/explicit.js'] },
  { name: 'canonical-project-before-global', files, trusted: true, global: { extensions: ['extensions/alias.js'] }, links: { 'agent/extensions/alias.js': '../../repo/.pi/extensions/project.js' } },
  { name: 'root-index', files: { ...files, 'agent/extensions/index.js': 'export default 42;' }, trusted: true },
];
const outcomes = [];
for (const s of scenarios) {
  const dir = realpathSync(mkdtempSync(join(tmpdir(), 'pi-local-extensions-')));
  try {
    const cwd = join(dir, 'repo'), agentDir = join(dir, 'agent');
    for (const [p, content] of Object.entries(s.files)) { mkdirSync(dirname(join(dir, p)), { recursive: true }); writeFileSync(join(dir, p), content); }
    for (const [p, target] of Object.entries(s.links ?? {})) symlinkSync(target, join(dir, p));
    writeFileSync(join(agentDir, 'settings.json'), JSON.stringify(s.global ?? {}));
    const settingsManager = SettingsManager.create(cwd, agentDir); settingsManager.setProjectTrusted(s.trusted);
    const manager = new DefaultPackageManager({ cwd, agentDir, settingsManager });
    const auto = (await manager.resolve()).extensions;
    const explicit = (await manager.resolveExtensionSources(s.paths ?? [], { temporary: true })).extensions;
    const entries = [...explicit, ...(s.disabled ? [] : auto)];
    const result = await loadExtensions(entries.filter(e => e.enabled).map(e => e.path), cwd);
    if (result.extensions.length || result.errors.length !== entries.filter(e => e.enabled).length) throw Error('expected failed extensions');
    const normalize = v => JSON.parse(JSON.stringify(v).replaceAll(dir, '$ROOT'));
    outcomes.push(normalize({ name: s.name, entries, errors: result.errors }));
  } finally { rmSync(dir, { recursive: true, force: true }); }
}
const c = { schema_version: '1.0.0', id: 'sdk/codingagent/local-extensions', catalog_id: 'contract:codingagent/local-extensions', surface: 'go-sdk', input: { scenarios }, observe: ['outcome', 'side_effects'] };
const observation = { outcome: outcomes, side_effects: [] };
const fixture = { schema_version: '1.0.0', deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit, upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference }, case: c, observation, input_hash: caseDigest(c), observation_hash: observationDigest(observation), execution_method: 'node --experimental-strip-types parity/oracle/local-extensions.mjs <locked-pi-checkout>', platform: 'posix', environment: { node: process.version, oracle_entry: reference } };
if (process.argv.includes('--check')) {
  const committed = JSON.parse(readFileSync(output, 'utf8')); fixture.environment.node = committed.environment.node;
  if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw Error('fixture drift');
  console.log(`verified ${output}`);
} else { writeFileSync(output, JSON.stringify(fixture, null, 2) + '\n'); console.log(`wrote ${output}`); }
