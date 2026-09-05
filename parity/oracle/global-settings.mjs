import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { existsSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "global-settings.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
	}
	return value;
}

function canonicalNumber(value) {
	if (!Number.isFinite(value)) throw new Error(`non-finite JSON number: ${value}`);
	if (value === 0) return "0";
	let text = String(value).toLowerCase();
	let sign = "";
	if (text.startsWith("-")) { sign = "-"; text = text.slice(1); }
	const [mantissa, exponentText = "0"] = text.split("e");
	const [integer, fraction = ""] = mantissa.split(".");
	let digits = `${integer}${fraction}`.replace(/^0+/, "");
	if (digits === "") return "0";
	let exponent = Number(exponentText) - fraction.length;
	const trimmed = digits.replace(/0+$/, "");
	exponent += digits.length - trimmed.length;
	digits = trimmed;
	return `${sign}${digits}${exponent === 0 ? "" : `e${exponent}`}`;
}

function canonicalJSONString(value) {
	if (value === null) return "null";
	if (typeof value === "number") return canonicalNumber(value);
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value);
	if (Array.isArray(value)) return `[${value.map(canonicalJSONString).join(",")}]`;
	if (typeof value === "object") return `{${Object.entries(value).map(([key, child]) => `${JSON.stringify(key)}:${canonicalJSONString(child)}`).join(",")}}`;
	throw new Error(`unsupported JSON value: ${typeof value}`);
}


function caseDigest(value) {
	const projected = {
		schema_version: value.schema_version,
		id: value.id,
		catalog_id: value.catalog_id,
		surface: value.surface,
		input: canonical(value.input),
		observe: value.observe,
	};
	return `sha256:${createHash("sha256").update(canonicalJSONString(JSON.parse(JSON.stringify(projected)))).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(canonicalJSONString(JSON.parse(JSON.stringify(projected)))).digest("hex")}`;
}

function parseArgs(argv) {
	const result = { pi: defaultPi, out: defaultOutput, check: false };
	for (let i = 2; i < argv.length; i++) {
		if (argv[i] === "--check") result.check = true;
		else if (argv[i] === "--out") result.out = argv[++i];
		else if (result.pi === defaultPi) result.pi = argv[i];
		else throw new Error(`unexpected argument: ${argv[i]}`);
	}
	return result;
}


async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, 'parity/baseline/upstream.lock.json'), 'utf8'));
 const head = execFileSync('git', ['-C', args.pi, 'rev-parse', 'HEAD'], {encoding:'utf8'}).trim();
 if (head !== lock.upstream.commit) throw new Error('Pi checkout does not match Code Baseline');
 const reference = 'packages/coding-agent/src/core/settings-manager.ts';
 const {SettingsManager} = await import(pathToFileURL(join(args.pi, reference)).href);
 const input = {
  settings: {defaultProvider:'deepseek',defaultModel:'deepseek-v4-flash',defaultThinkingLevel:'high',sessionDir:'./saved-sessions',queueMode:'all',websockets:false,skills:{enableSkillCommands:false,customDirectories:['./skills']},retry:{maxDelayMs:45000,provider:{timeoutMs:30000}},packages:['npm:old'],future:{keep:[1,2]}},
  overrides: {retry:{provider:{maxRetries:2}},packages:[],defaultModel:'temporary'},
 };
 let content = JSON.stringify(input.settings);
 const scopes = [];
 const storage = {withLock(scope, fn) {scopes.push(scope); if(scope !== 'global') throw new Error('project accessed'); const next=fn(content); if(next !== undefined) content=next;}};
 const defaults = SettingsManager.inMemory({}, {projectTrusted:false});
 const manager = SettingsManager.fromStorage(storage, {projectTrusted:false});
 const migrated = manager.getGlobalSettings();
 manager.applyOverrides(input.overrides);
 const overrides = {retry:manager.getProviderRetrySettings(),packages:manager.getPackages(),model:manager.getDefaultModel(),globalModel:manager.getGlobalSettings().defaultModel};
 const external = JSON.parse(content); external.packages=[]; external.retry.provider.maxRetries=7; content=JSON.stringify(external);
 manager.setDefaultThinkingLevel('low'); manager.setRetryEnabled(false); await manager.flush();
 const saved=JSON.parse(content);
 await manager.reload();
 const reloaded={retry:manager.getProviderRetrySettings(),model:manager.getDefaultModel(),thinking:manager.getDefaultThinkingLevel(),packages:manager.getPackages()};
 content='{invalid'; await manager.reload();
 const invalid={retained:manager.getDefaultModel(),errors:manager.drainErrors().map(e=>e.scope),drained:manager.drainErrors().length};
 manager.setTheme('light'); await manager.flush(); invalid.preserved=content==='{invalid';
 content=JSON.stringify({defaultModel:'repaired'}); await manager.reload(); manager.setDefaultProvider('deepseek'); await manager.flush();
 const repaired=JSON.parse(content);
 const memory=SettingsManager.inMemory({defaultModel:'seed'}, {projectTrusted:false}); await memory.reload();
 const failed=SettingsManager.fromStorage({withLock(scope,fn){const next=fn(undefined);if(next!==undefined)throw new Error('write failed');}}, {projectTrusted:false});
 failed.setDefaultModel('local'); await failed.flush();
 const observation={outcome:{
  defaults:{compaction:defaults.getCompactionSettings(),retry:defaults.getRetrySettings(),providerRetry:defaults.getProviderRetrySettings(),transport:defaults.getTransport(),steering:defaults.getSteeringMode(),trust:defaults.getDefaultProjectTrust(),sessionDir:defaults.getSessionDir()??null},
  migrated,overrides,saved,reloaded,invalid,repaired,memory:memory.getDefaultModel(),
  writeFailure:{model:failed.getDefaultModel(),errors:failed.drainErrors().map(e=>e.scope)},
  globalOnly:scopes.every(s=>s==='global')
 },side_effects:[]};
 const caseValue={schema_version:'1.0.0',id:'go-sdk/codingagent/global-settings',catalog_id:'contract:config/settings',surface:'go-sdk',input,observe:['outcome','side_effects']};
 const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:'node --experimental-strip-types parity/oracle/global-settings.mjs <locked-pi-checkout>',platform:'any',environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,'utf8'));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error('committed fixture does not reproduce');console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
}
main().catch(error=>{console.error(error);process.exit(1);});
