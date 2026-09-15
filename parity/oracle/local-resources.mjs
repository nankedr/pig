import { caseDigest, observationDigest } from './fixture-hash.mjs';
import { execFileSync } from 'node:child_process';
import { realpathSync, symlinkSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { dirname, join, resolve } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';

const root = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const pi = resolve(process.argv.find((v, i) => i > 1 && !v.startsWith('--')) ?? join(root, '.upstream/pi'));
const output = join(root, 'parity/oracle/fixtures/local-resources.json');
const lock = JSON.parse(readFileSync(join(root, 'parity/baseline/upstream.lock.json'), 'utf8'));
if (execFileSync('git', ['-C', pi, 'rev-parse', 'HEAD'], { encoding: 'utf8' }).trim() !== lock.upstream.commit) throw Error('baseline mismatch');
if (execFileSync('git', ['-C', pi, 'status', '--porcelain', '--untracked-files=no'], { encoding: 'utf8' }).trim()) throw Error('dirty baseline');
process.env.COLORTERM = 'truecolor';
const reference = 'packages/coding-agent/src/core/resource-loader.ts';
const { DefaultResourceLoader } = await import(pathToFileURL(join(pi, reference)));
const { SettingsManager } = await import(pathToFileURL(join(pi, 'packages/coding-agent/src/core/settings-manager.ts')));
const { expandPromptTemplate } = await import(pathToFileURL(join(pi, 'packages/coding-agent/src/core/prompt-templates.ts')));
const { formatSkillsForPrompt } = await import(pathToFileURL(join(pi, 'packages/coding-agent/src/core/skills.ts')));
const dark = JSON.parse(readFileSync(join(pi, 'packages/coding-agent/src/modes/interactive/theme/dark.json'), 'utf8'));
const bundle = (base, marker, name = 'same') => ({
  [`${base}/prompts/${name}.md`]: `---\ndescription: "${marker}"\n---\n${marker} $1`,
  [`${base}/skills/${name}/SKILL.md`]: `---\nname: ${name}\ndescription: "${marker}"\n---\n${marker}`,
  [`${base}/themes/${name}.json`]: JSON.stringify({ ...dark, name, colors: { ...dark.colors, accent: marker } }),
});
const all = { ...bundle('agent', '#111111'), ...bundle('repo/.pi', '#222222'), ...bundle('extra', '#333333') };
const paths = { prompts: ['../extra/prompts'], skills: ['../extra/skills'], themes: ['../extra/themes'] };
const settings = (prefix, suffix) => ({ prompts: [prefix + 'prompts', ...suffix.map(s => s.replaceAll('@', 'prompts/a.md'))], skills: [prefix + 'skills', ...suffix.map(s => s.replaceAll('@', 'skills/a'))], themes: [prefix + 'themes', ...suffix.map(s => s.replaceAll('@', 'themes/a.json'))] });
const nested = Object.fromEntries(Object.entries(bundle('agent', '#123456', 'nested')).map(([p,v]) => [p.replace(/(prompts|skills|themes)\//, '$1/deep/'), v]));
const scenarios = [
  { name: 'discovery-recursion', files: { ...nested, ...bundle('agent', '#111111', 'root') } },
  { name: 'settings-recursion', files: nested, global: settings('', []) },
  { name: 'explicit-recursion', files: nested, disabled: true, paths: { prompts: ['../agent/prompts'], skills: ['../agent/skills'], themes: ['../agent/themes'] } },
  { name: 'ignore-explicit-reenable', files: { ...bundle('agent', '#111111', 'skip'), 'agent/prompts/.gitignore': 'skip.md\n', 'agent/skills/.gitignore': 'skip/\n', 'agent/themes/.gitignore': 'skip.json\n' }, paths: { prompts: ['../agent/prompts'], skills: ['../agent/skills/skip'], themes: ['../agent/themes'] } },
  { name: 'global-symlink-untrusted-project', files: { ...bundle('repo/.pi', '#111111'), 'agent/settings.json': '{}' }, links: { 'agent/prompts': '../repo/.pi/prompts', 'agent/skills': '../repo/.pi/skills', 'agent/themes': '../repo/.pi/themes' } },
  { name: 'disabled-exact-explicit', trusted: true, files: all, global: settings('', ['-prompts/same.md', '-skills/same', '-themes/same.json']), paths: { prompts: ['../agent/prompts/same.md'], skills: ['../agent/skills/same/SKILL.md'], themes: ['../agent/themes/same.json'] } },

  { name: 'mixed-priority', trusted: true, files: { ...all, ...bundle('agent/more', '#444444'), ...bundle('repo/.pi/more', '#555555'), ...bundle('repo/.agents', '#666666'), ...bundle('home/.agents', '#777777') }, global: settings('more/', []), project: settings('more/', []), paths },
  { name: 'untrusted-explicit', files: { ...all, ...bundle('repo/.agents', '#666666'), ...bundle('home/.agents', '#777777', 'home') }, paths },
  { name: 'disabled-explicit', trusted: true, disabled: true, files: all, paths },
  { name: 'ancestor-boundary', cwd: 'repo/sub/deep', trusted: true, files: { 'repo/.git': 'gitdir: elsewhere', ...bundle('repo/.agents', '#111111'), ...bundle('repo/sub/.agents', '#222222'), ...bundle('repo/sub/deep/.agents', '#333333'), ...bundle('.agents', '#444444', 'outside'), ...bundle('repo/.pi', '#555555', 'parent'), ...bundle('home/.agents', '#666666', 'home') } },
  { name: 'home-is-ancestor', cwd: 'home/work', trusted: true, files: { ...bundle('home/.agents', '#111111'), ...bundle('agent', '#222222') } },
  { name: 'disabled-alias', files: bundle('agent', '#111111', 'a'), links: { 'agent/prompts/b.md': 'a.md', 'agent/skills/b': 'a', 'agent/themes/b.json': 'a.json' }, global: settings('', ['-@']) },
  { name: 'disabled-alias-explicit', files: bundle('agent', '#111111', 'a'), links: { 'agent/prompts/b.md': 'a.md', 'agent/skills/b': 'a', 'agent/themes/b.json': 'a.json' }, global: settings('', ['-@']), paths: { prompts: ['../agent/prompts/b.md'], skills: ['../agent/skills/b'], themes: ['../agent/themes/b.json'] } },
  { name: 'explicit-overlap', files: all, paths: { prompts: ['../agent/prompts', '../agent/prompts/same.md'], skills: ['../agent/skills', '../agent/skills/same'], themes: ['../agent/themes', '../agent/themes/same.json'] } },
  { name: 'explicit-file-alias', disabled: true, files: bundle('extra', '#111111'), links: { 'extra/prompts/alias.md': 'same.md', 'extra/skills/alias': 'same', 'extra/themes/alias.json': 'same.json' }, paths: { prompts: ['../extra/prompts/same.md', '../extra/prompts/alias.md'], skills: ['../extra/skills/same', '../extra/skills/alias'], themes: ['../extra/themes/same.json', '../extra/themes/alias.json'] } },
  { name: 'explicit-project-trust', files: all, paths: { prompts: ['.pi/prompts'], skills: ['.pi/skills'], themes: ['.pi/themes'] } },
  { name: 'settings-filter', trusted: true, files: { ...bundle('agent/more', '#111111', 'a'), ...bundle('agent/more', '#222222', 'b'), ...bundle('agent/more', '#333333', 'c') }, global: { prompts: ['more/prompts', '*.md', '!**/b.md', '-more/prompts/c.md'], skills: ['more/skills', '!**/b', '-more/skills/c'], themes: ['more/themes', '*.json', '!**/b.json', '-more/themes/c.json'] } },
  { name: 'traversal-order', trusted: true, files: { ...bundle('agent', '#111111', 'zulu'), ...bundle('agent', '#222222', 'alpha'), ...bundle('repo/.pi', '#333333', 'middle'), ...bundle('extra', '#444444', 'beta') }, paths },
  { name: 'disabled-metadata', trusted: true, disabled: true, files: all, paths: { prompts: ['.pi/prompts/same.md'], skills: ['.pi/skills/same/SKILL.md'], themes: ['.pi/themes/same.json'] } },
];
const outcomes = [];
for (const scenario of scenarios) {
  const dir = realpathSync(mkdtempSync(join(tmpdir(), 'pi-local-resources-')));
  try {
    process.env.HOME = join(dir, 'home');
    for (const [p, content] of Object.entries(scenario.files)) { mkdirSync(dirname(join(dir, p)), { recursive: true }); writeFileSync(join(dir, p), content); }
    for (const [p, target] of Object.entries(scenario.links ?? {})) symlinkSync(target, join(dir, p));
    const cwd = join(dir, scenario.cwd ?? 'repo'), agentDir = join(dir, 'agent');
    for (const p of [process.env.HOME, join(cwd, '.pi'), agentDir]) mkdirSync(p, { recursive: true });
    writeFileSync(join(agentDir, 'settings.json'), JSON.stringify(scenario.global ?? {}));
    writeFileSync(join(cwd, '.pi/settings.json'), JSON.stringify(scenario.project ?? {}));
    const manager = SettingsManager.create(cwd, agentDir); manager.setProjectTrusted(scenario.trusted ?? false);
    const loader = new DefaultResourceLoader({ cwd, agentDir, settingsManager: manager, noExtensions: true, noContextFiles: true, noSkills: scenario.disabled, noPromptTemplates: scenario.disabled, noThemes: scenario.disabled, additionalSkillPaths: scenario.paths?.skills ?? [], additionalPromptTemplatePaths: scenario.paths?.prompts ?? [], additionalThemePaths: scenario.paths?.themes ?? [] });
    await loader.reload();
    const prompts = loader.getPrompts(), skills = loader.getSkills(), themes = loader.getThemes();
    const normalize = v => JSON.parse(JSON.stringify(v).replaceAll(dir, '$ROOT'));
    const entry = (name, path, source, effect) => ({ name, path, source, effect });
    outcomes.push(normalize({ name: scenario.name,
      prompts: prompts.prompts.map(p => entry(p.name, p.filePath, p.sourceInfo, expandPromptTemplate('/' + p.name + ' ARG', prompts.prompts))),
      skills: skills.skills.map(s => entry(s.name, s.filePath, s.sourceInfo, s.description)),
      themes: themes.themes.map(t => entry(t.name, t.sourcePath, t.sourceInfo, t.fg('accent', 'X'))),
      skillList: formatSkillsForPrompt(skills.skills),
      diagnostics: [...prompts.diagnostics, ...skills.diagnostics, ...themes.diagnostics],
    }));
  } finally { rmSync(dir, { recursive: true, force: true }); }
}
const c = { schema_version: '1.0.0', id: 'sdk/codingagent/local-resources', catalog_id: 'contract:codingagent/local-resources', surface: 'go-sdk', input: { scenarios }, observe: ['outcome', 'side_effects'] };
const observation = { outcome: outcomes, side_effects: [] };
const fixture = { schema_version: '1.0.0', deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit, upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference }, case: c, observation, input_hash: caseDigest(c), observation_hash: observationDigest(observation), execution_method: 'node --experimental-strip-types parity/oracle/local-resources.mjs <locked-pi-checkout>', platform: 'posix', environment: { node: process.version, oracle_entry: reference } };
if (process.argv.includes('--check')) {
  const committed = JSON.parse(readFileSync(output, 'utf8')); fixture.environment.node = committed.environment.node;
  if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw Error('fixture drift');
  console.log(`verified ${output}`);
} else { writeFileSync(output, JSON.stringify(fixture, null, 2) + '\n'); console.log(`wrote ${output}`); }
