import { createHash } from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import { existsSync, mkdirSync, readdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { basename, dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "settings-startup.json");

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


import { createServer } from "node:http";

async function main(){
 const args=parseArgs(process.argv),lock=JSON.parse(readFileSync(join(root,'parity/baseline/upstream.lock.json'),'utf8'));
 if(execFileSync('git',['-C',args.pi,'rev-parse','HEAD'],{encoding:'utf8'}).trim()!==lock.upstream.commit)throw new Error('baseline mismatch');
 const dir=mkdtempSync(join(tmpdir(),'pi-settings-startup-')),agentDir=join(dir,'agent'),cwd=join(dir,'project');mkdirSync(agentDir);mkdirSync(cwd);
 const requests=[];
 const server=createServer(async(req,res)=>{let raw='';for await(const part of req)raw+=part;const body=JSON.parse(raw);requests.push(body);res.writeHead(200,{'Content-Type':'text/event-stream'});res.end('data: {"id":"fixture","choices":[{"delta":{"content":"reply"},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n');});
 await new Promise(resolve=>server.listen(0,'127.0.0.1',resolve));
 try{
  const baseURL=`http://127.0.0.1:${server.address().port}`;
  writeFileSync(join(agentDir,'models.json'),JSON.stringify({providers:{deepseek:{baseUrl:baseURL}}}));
  const reference='packages/coding-agent/src/cli.ts';
  const run=async(extra,env={})=>{const child=spawn(process.execPath,[join(args.pi,reference),'--offline','--no-extensions','--no-skills','--no-prompt-templates','--no-themes','--no-tools','-p','hello',...extra],{cwd,env:{PATH:process.env.PATH,HOME:dir,PI_CODING_AGENT_DIR:agentDir,DEEPSEEK_API_KEY:'fixture',PI_SKIP_VERSION_CHECK:'1',NO_COLOR:'1',...env},stdio:['ignore','pipe','pipe'],timeout:20000});let out='',err='';child.stdout.on('data',x=>out+=x);child.stderr.on('data',x=>err+=x);const code=await new Promise((resolve,reject)=>{child.on('error',reject);child.on('close',resolve)});if(code!==0)throw new Error(`Pi failed ${code}: ${err}`);return out;};
  const configured=join(dir,'configured'),environment=join(dir,'environment'),explicit=join(dir,'explicit');
  const settings={defaultProvider:'deepseek',defaultModel:'deepseek-v4-flash',defaultThinkingLevel:'high',sessionDir:configured};
  writeFileSync(join(agentDir,'settings.json'),JSON.stringify(settings));
  const cases=[];
  const capture=async(name,extra,storage,env={})=>{await run(extra,env);const files=readdirSync(storage).filter(x=>x.endsWith('.jsonl'));const file=join(storage,files[files.length-1]);const records=readFileSync(file,'utf8').trim().split('\n').map(JSON.parse);cases.push({name,model:requests.at(-1).model,thinking:records.find(x=>x.type==='thinking_level_change').thinkingLevel,sessionDir:true});return file;};
  const first=await capture('settings',[],configured);
  settings.defaultModel='deepseek-v4-pro';settings.defaultThinkingLevel='low';writeFileSync(join(agentDir,'settings.json'),JSON.stringify(settings));
  await capture('restart',[],configured);
  await run(['--session',first]);cases.push({name:'reopen',model:requests.at(-1).model,thinking:requests.at(-1).reasoning_effort,history:requests.at(-1).messages.map(m=>m.role)});
  await capture('explicit',['--model','deepseek/deepseek-v4-flash:low','--thinking','off','--session-dir',explicit],explicit,{PI_CODING_AGENT_SESSION_DIR:environment});
  await capture('environment',[],environment,{PI_CODING_AGENT_SESSION_DIR:environment});
  const input={projection:'Real locked Pi CLI: saved defaults, settings changed before restart, session restoration, explicit model/thinking/path override, environment path override. Only random paths/IDs/time are projected.'};
  const observation={outcome:{cases},side_effects:[]};
  const c={schema_version:'1.0.0',id:'cli/pig/settings-startup',catalog_id:'contract:config/settings',surface:'cli',input,observe:['outcome','side_effects']};
  const fixture={schema_version:'1.0.0',deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:'node --experimental-strip-types parity/oracle/settings-startup.mjs <locked-pi-checkout>',platform:'any',environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,'utf8'));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error('fixture drift');console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+'\n');console.log(`wrote ${args.out}`)}
 }finally{await new Promise(resolve=>server.close(resolve));rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
