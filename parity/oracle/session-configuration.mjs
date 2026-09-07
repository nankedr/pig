import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-configuration.json");

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
 const args=parseArgs(process.argv);
 const lock=JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json"),"utf8"));
 if(execFileSync("git",["-C",args.pi,"rev-parse","HEAD"],{encoding:"utf8"}).trim()!==lock.upstream.commit)throw new Error("baseline mismatch");
 if(execFileSync("git",["-C",args.pi,"status","--porcelain=v1","--untracked-files=no"],{encoding:"utf8"}).trim())throw new Error("dirty Pi sources");
 globalThis.fetch=async()=>{throw new Error("unexpected network")};
 const load=p=>import(pathToFileURL(join(args.pi,p)).href);
 const {ModelRuntime}=await load("packages/coding-agent/src/core/model-runtime.ts");
 const {AuthStorage}=await load("packages/coding-agent/src/core/auth-storage.ts");
 const {SessionManager}=await load("packages/coding-agent/src/core/session-manager.ts");
 const {SettingsManager}=await load("packages/coding-agent/src/core/settings-manager.ts");
 const {createFauxCore,fauxAssistantMessage}=await load("packages/ai/src/providers/faux.ts");
 const dir=mkdtempSync(join(tmpdir(),"pi-config-"));
 const input={models:[{id:"deepseek-v4-flash",provider:"deepseek",reasoning:true},{id:"deepseek-v4-pro",provider:"deepseek",reasoning:false},{id:"deepseek-v4-flash",provider:"deepseek",reasoning:true,thinkingLevelMap:{xhigh:"xhigh",max:"max"}}]};
 let runs;
 try {
 for(const path of ["src/core/sdk.ts","dist/core/sdk.js"]){
  const {createAgentSession}=await load("packages/coding-agent/"+path);
  const runtime=await ModelRuntime.create({credentials:AuthStorage.inMemory(),modelsPath:null,allowModelNetwork:false});
  for(const provider of ["deepseek"])runtime.registerProvider(provider,{api:"openai-completions",apiKey:"fixture",baseUrl:"https://invalid.test",models:input.models.slice(0,2).filter(m=>m.provider===provider).map(m=>({...m,name:m.id,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:2048}))});
  const models=input.models.map(m=>({...runtime.getModel(m.provider,m.id),...m}));
  const manager=SessionManager.inMemory(dir),settings=SettingsManager.inMemory({compaction:{enabled:false},retry:{enabled:false}});
  const {session}=await createAgentSession({cwd:dir,agentDir:dir,model:models[0],modelRuntime:runtime,sessionManager:manager,settingsManager:settings,thinkingLevel:"off",tools:["read","bash","edit","write"]});
  const core=createFauxCore({});session.agent.streamFunction=core.streamSimple;
  const events=[],requests=[],states=[];
  session.subscribe(e=>{if(e.type==="thinking_level_changed")events.push(e.level)});
  const capture=label=>states.push({label,model:session.model.id,thinking:session.thinkingLevel,levels:session.getAvailableThinkingLevels(),supports:session.supportsThinking(),tools:session.getActiveToolNames()});
  const request=async()=>{core.setResponses([(c,o,_s,m)=>{requests.push({model:m.id,thinking:o?.reasoning??"off",tools:(c.tools??[]).map(t=>t.name),promptTools:["read","bash","edit","write"].filter(n=>c.systemPrompt.includes(`- ${n}:`))});return fauxAssistantMessage("ok")}]);await session.prompt("go")};
  session.setThinkingLevel("high");capture("high");await request();
  session.setScopedModels([{model:models[0]},{model:{...models[0],id:"missing"}},{model:models[1]}]);
  await session.cycleModel();capture("nonreasoning");session.setScopedModels([{model:models[1]},{model:models[2],thinkingLevel:"max"}]);
  await session.cycleModel();capture("scoped-max");
  session.cycleThinkingLevel();capture("cycle-off");
  session.setThinkingLevel("high");session.cycleThinkingLevel();capture("cycle-xhigh");
  await session.cycleModel("backward");capture("backward");
  await session.setModel(models[0]);capture("restore-preference");
  session.setActiveToolsByName(["write","read"]);capture("tools");await request();
  session.setActiveToolsByName([]);capture("empty-tools");await request();
  session.setScopedModels([{model:models[0]},{model:{...models[0],id:"missing"}}]);
  const single=(await session.cycleModel())??null;
  const entries=manager.getEntries().filter(e=>["model_change","thinking_level_change"].includes(e.type)).map(e=>e.type==="model_change"?{type:e.type,model:e.modelId}:{type:e.type,thinking:e.thinkingLevel});
  const captured={states,requests,events,entries,single,defaultThinking:settings.getDefaultThinkingLevel(),defaultModel:settings.getDefaultModel()};
  session.dispose();
  if(runs&&JSON.stringify(canonical(runs))!==JSON.stringify(canonical(captured)))throw new Error("source/dist mismatch");runs=captured;
 }
 const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts";
 const observation={outcome:runs,side_effects:[]};
 const c={schema_version:"1.0.0",id:"go-sdk/codingagent/session-configuration",catalog_id:"contract:codingagent/session-configuration",surface:"go-sdk",input,observe:["outcome","side_effects"]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/session-configuration.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 }finally{rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
