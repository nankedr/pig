import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "coding-tools.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a.localeCompare(b)).map(([key, child]) => [key, canonical(child)]));
	}
	return value;
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
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
}

function observationDigest(value) {
	const projected = {
		outcome: canonical(value.outcome),
		side_effects: value.side_effects.map((effect) => ({ kind: effect.kind, target: effect.target, detail: canonical(effect.detail) })),
	};
	return `sha256:${createHash("sha256").update(JSON.stringify(projected)).digest("hex")}`;
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
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 const reference = "packages/coding-agent/src/core/sdk.ts";
 const {createAgentSession} = await import(pathToFileURL(join(args.pi, reference)).href);
 const {SessionManager} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/session-manager.ts")).href);
 const {SettingsManager} = await import(pathToFileURL(join(args.pi, "packages/coding-agent/src/core/settings-manager.ts")).href);
 const dir = mkdtempSync(join(tmpdir(), "pi-coding-tools-oracle-"));
 try {
  const input = {selections:[{}, {tools:["write","read"]}, {excludeTools:["bash","edit"]}, {noTools:"all"}, {noTools:"builtin"}, {tools:[]}, {noTools:"all",tools:["read","bash"],excludeTools:["read"]}]};
  const selections=[];
  const model={id:"fixture",provider:"fixture",api:"openai-completions",input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:2048};
  for(const selection of input.selections){
   const {session}=await createAgentSession({cwd:dir,agentDir:dir,model,sessionManager:SessionManager.inMemory(dir),settingsManager:SettingsManager.inMemory({}),...selection});
   try {selections.push({tools:session.getActiveToolNames(),promptTools:["read","bash","edit","write"].filter(name=>session.systemPrompt.includes(`- ${name}:`))});}
   finally {session.dispose();}
  }
  const observation={outcome:{selections},side_effects:[]};
  const caseValue={schema_version:"1.0.0",id:"go-sdk/codingagent/default-coding-tools",catalog_id:"contract:codingagent/default-coding-tools",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:caseValue,observation,input_hash:caseDigest(caseValue),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/coding-tools.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("committed fixture does not reproduce");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(error=>{console.error(error);process.exit(1);});
