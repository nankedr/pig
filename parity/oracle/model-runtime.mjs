import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "model-runtime.json");

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
 const args=parseArgs(process.argv), lock=JSON.parse(readFileSync(join(root,'parity/baseline/upstream.lock.json'),'utf8'));
 if(execFileSync('git',['-C',args.pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit) throw new Error('baseline mismatch');
 for (const key of Object.keys(process.env)) if (/API_KEY|AUTH_TOKEN/.test(key)) delete process.env[key];
 process.env.PI_OFFLINE='1';
 const dir=mkdtempSync(join(tmpdir(),'pi-model-runtime-'));
 try {
 const reference='packages/coding-agent/src/core/model-runtime.ts';
 const {ModelRuntime}=await import(pathToFileURL(join(args.pi,reference)));
 const {resolveCliModel,resolveModelScopeWithDiagnostics}=await import(pathToFileURL(join(args.pi,'packages/coding-agent/src/core/model-resolver.ts')));
 const authPath=join(dir,'auth.json');writeFileSync(authPath,JSON.stringify({deepseek:{type:'api_key',key:'fixture-key'}}));
 const runtime=await ModelRuntime.create({authPath,modelsPath:null,allowModelNetwork:false});
 const queries=[{cliModel:'DEEPSEEK/deepseek-v4-flash:high'},{cliProvider:'deepseek',cliModel:'v4-pro'},{cliProvider:'deepseek',cliModel:'future-model:high'},{cliModel:'does-not-exist'},{cliProvider:'missing',cliModel:'x'},{cliProvider:'deepseek',cliModel:'deepseek-v4-flash:invalid'}];
 const patterns=['deepseek/*:low','deepseek-v4-pro:invalid','absent*'];
 const project=r=>({model:r.model?`${r.model.provider}/${r.model.id}`:null,thinking:r.thinkingLevel??null,warning:r.warning??null,error:r.error??null});
 const scope=await resolveModelScopeWithDiagnostics(patterns,runtime);
 const observation={outcome:{queries:queries.map(q=>project(resolveCliModel({...q,modelRuntime:runtime}))),scope:scope.scopedModels.map(m=>({model:`${m.model.provider}/${m.model.id}`,thinking:m.thinkingLevel??null})),diagnostics:scope.diagnostics,status:runtime.getProviderAuthStatus('deepseek')},side_effects:[]};
 const c={schema_version:'1.0.0',id:'codingagent/model-runtime',catalog_id:'contract:model-runtime/basic',surface:'go-sdk',input:{queries,patterns},observe:['outcome','side_effects']};
 const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node --experimental-strip-types parity/oracle/model-runtime.mjs <locked-pi-checkout>',platform:'any',environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,'utf8'));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error('fixture drift');console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+'\n');console.log(`wrote ${args.out}`)}
 } finally {rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
