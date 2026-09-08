import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "manual-compaction.json");

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
 const lock = JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json"),"utf8"));
 if (execFileSync("git",["-C",args.pi,"rev-parse","HEAD"],{encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("baseline mismatch");
 if (execFileSync("git",["-C",args.pi,"status","--porcelain=v1","--untracked-files=no"],{encoding:"utf8"}).trim()) throw new Error("dirty Pi source");
 globalThis.fetch = async()=>{throw new Error("unexpected network")};
 const load = p=>import(pathToFileURL(join(args.pi,p)).href);
 const {SessionManager}=await load("packages/coding-agent/src/core/session-manager.ts");
 const {SettingsManager}=await load("packages/coding-agent/src/core/settings-manager.ts");
 const {ModelRuntime}=await load("packages/coding-agent/src/core/model-runtime.ts");
 const {AuthStorage}=await load("packages/coding-agent/src/core/auth-storage.ts");
 const {createAssistantMessageEventStream}=await load("packages/ai/src/utils/event-stream.ts");
 const dir=mkdtempSync(join(tmpdir(),"pi-manual-compaction-"));
 const input={"scenarios": [{"name": "history", "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "split", "keep": 1, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "prefix-only", "keep": 1, "prefixOnly": true, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "small", "keep": 10000, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "files", "files": true, "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "update", "update": true, "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "retry", "errors": ["terminated", "503"], "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "permanent", "errors": ["insufficient_quota"], "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "exhausted", "errors": ["503", "503", "503"], "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "disabled", "errors": ["terminated"], "disabled": true, "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "cancel-retry", "errors": ["terminated"], "cancel": true, "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low"}, {"name": "uncapped", "maxTokens": 0, "keep": 10, "reserve": 100, "thinking": "low"}, {"name": "reasoning-off", "thinking": "off", "keep": 10, "reserve": 100, "maxTokens": 60}, {"name": "tool-result-summarized", "keep": 10, "reserve": 100, "maxTokens": 60, "thinking": "low", "toolResult": true}, {"name": "tool-result-kept", "keep": 1260, "reserve": 100, "maxTokens": 60, "thinking": "low", "toolResult": true}, {"name": "split-second-error", "keep": 1, "reserve": 100, "maxTokens": 60, "thinking": "low", "errors": ["", "insufficient_quota"]}]};
 const usage={input:80,output:20,cacheRead:0,cacheWrite:0,totalTokens:100,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}};
 const project=m=>({role:m.role,text:m.summary??(typeof m.content === "string"?m.content:m.content.filter(b=>b.type==="text").map(b=>b.text).join(""))});
 let runs=[];
 try {
 const runtime=await ModelRuntime.create({credentials:AuthStorage.inMemory(),modelsPath:null,allowModelNetwork:false});
 for (const sdk of ["src/core/sdk.ts","dist/core/sdk.js"]) {
 const {createAgentSession}=await load("packages/coding-agent/"+sdk);
 const observed=[];
 for (const scenario of input.scenarios) {
 const model={id:"fixture",name:"fixture",provider:"compaction-fixture",api:"faux:compaction",baseUrl:"https://invalid.test",reasoning:true,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:scenario.maxTokens};
 const manager=SessionManager.inMemory(dir);
 const assistant=text=>({role:"assistant",content:[{type:"text",text}],api:model.api,provider:model.provider,model:model.id,usage,stopReason:"stop",timestamp:1});
 let seed=[{role:"user",content:"old request",timestamp:1},assistant("old answer"),{role:"user",content:"recent request long enough to keep",timestamp:1},assistant("recent answer")];
 if(scenario.prefixOnly)seed=seed.slice(2);
 if(scenario.toolResult)seed.splice(3,0,{...assistant(""),content:[{type:"toolCall",id:"read-call",name:"read",arguments:{path:"context.go"}}],stopReason:"toolUse"},{role:"toolResult",toolCallId:"read-call",toolName:"read",content:[{type:"text",text:"x".repeat(5000)}],isError:false,timestamp:1});
 if(scenario.files)seed[1].content.push(...["read","edit","read","write"].map((name,i)=>({type:"toolCall",id:"call"+i,name,arguments:{path:["shared.go","shared.go","only-read.go","written.go"][i]}})));
 for (const [i,message] of seed.entries()) {
  manager.appendMessage(message);
  if(scenario.update&&i===1)manager.appendCompaction("previous checkpoint",manager.getEntries()[1].id,100,{readFiles:["prior-read.go"],modifiedFiles:["prior-edit.go"]},false,usage);
 }
 const ids=manager.getEntries().map(e=>e.id);
 const {session}=await createAgentSession({cwd:dir,agentDir:dir,model,modelRuntime:runtime,sessionManager:manager,settingsManager:SettingsManager.inMemory({compaction:{enabled:false,reserveTokens:scenario.reserve,keepRecentTokens:scenario.keep},retry:{enabled:!scenario.disabled,maxRetries:2,baseDelayMs:1}}),thinkingLevel:scenario.thinking,tools:[]});
 const requests=[],events=[];
 session.agent.streamFunction=(m,c,o)=>{
 requests.push({system:c.systemPrompt,messages:c.messages.map(project),maxTokens:o.maxTokens,reasoning:o.reasoning??"",cacheRetention:o.cacheRetention,freshSession:!!o.sessionId && o.sessionId!==manager.getSessionId()});
 const stream=createAssistantMessageEventStream();
 const error=scenario.errors?.[requests.length-1];
 const response={...assistant("checkpoint"),...(error?{stopReason:"error",errorMessage:error}:{})};
 queueMicrotask(()=>stream.push(response.stopReason==="error"?{type:"error",reason:"error",error:response}:{type:"done",reason:"stop",message:response}));
 return stream;
 };
 session.subscribe(e=>{if(scenario.cancel&&e.type==="summarization_retry_scheduled")session.abortCompaction();if(e.type.startsWith("compaction_")||e.type.startsWith("summarization_retry")) events.push({type:e.type,compacting:session.isCompacting,...(e.type==="compaction_end"?{aborted:e.aborted,willRetry:e.willRetry,error:e.errorMessage??""}:{}),...(e.type==="summarization_retry_scheduled"?{attempt:e.attempt,maxAttempts:e.maxAttempts,delayMs:e.delayMs,errorMessage:e.errorMessage}:{}),...(e.type==="summarization_retry_attempt_start"?{source:e.source,reason:e.reason}:{})});});
 let result,error="";
 try {result=await session.compact("preserve constraints")}catch(e){error=scenario.cancel?"cancelled":e.message}
 observed.push({name:scenario.name,requests,events,error,result:result?{summary:result.summary,firstKeptIndex:ids.indexOf(result.firstKeptEntryId),tokensBefore:result.tokensBefore,estimatedTokensAfter:result.estimatedTokensAfter,usage:result.usage,details:result.details}:null,messages:session.messages.map(project),entries:manager.getEntries().map(e=>e.type)});
 session.dispose();
 }
 if(runs.length&&JSON.stringify(canonical(runs))!==JSON.stringify(canonical(observed)))throw new Error("source/dist mismatch");
 runs=observed;
 }
 const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts#compact";
 const observation={outcome:{runs},side_effects:[]};
 const c={schema_version:"1.0.0",id:"go-sdk/codingagent/manual-compaction",catalog_id:"contract:codingagent/compaction",surface:"go-sdk",input,observe:["outcome","side_effects"]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/manual-compaction.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 }finally{rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
