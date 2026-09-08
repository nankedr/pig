import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "auto-compaction.json");

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
 const dir=mkdtempSync(join(tmpdir(),"pi-auto-compaction-"));
 const input={"scenarios": [{"name": "threshold", "responses": [{"input": 950}]}, {"name": "at-threshold", "responses": [{"input": 900}]}, {"name": "disabled", "disabled": true, "responses": [{"input": 950}]}, {"name": "overflow", "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "overflow-twice", "responses": [{"error": "prompt is too long", "input": 0}, {"error": "prompt is too long", "input": 0}]}, {"name": "length", "responses": [{"reason": "length", "output": 20}]}, {"name": "length-limit", "responses": [{"reason": "length", "output": 100}]}, {"name": "silent-overflow", "responses": [{"input": 1001}]}, {"name": "other-model", "responses": [{"error": "prompt is too long", "input": 0, "otherModel": true}]}, {"name": "retry-overflow-retry", "responses": [{"error": "503", "input": 0}, {"error": "prompt is too long", "input": 0}, {"error": "503", "input": 0}]}, {"name": "summary-retry", "summaryErrors": ["503"], "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "summary-failed", "summaryErrors": ["insufficient_quota"], "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "summary-exhausted", "summaryErrors": ["503", "503", "503"], "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "threshold-queue", "queue": true, "responses": [{"input": 950}]}, {"name": "overflow-queue", "queue": true, "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "pre-prompt-threshold", "seed": {"input": 950}, "responses": [{}]}, {"name": "pre-prompt-overflow", "seed": {"error": "prompt is too long", "input": 0}, "responses": [{}]}, {"name": "pre-prompt-aborted", "seed": {"reason": "aborted", "input": 950}, "responses": [{}]}, {"name": "stale-compaction", "seed": {"input": 950}, "stale": true, "responses": [{"error": "insufficient_quota", "input": 0}]}, {"name": "error-fallback", "seed": {"input": 899}, "responses": [{"error": "insufficient_quota", "input": 0}]}, {"name": "zero-usage-fallback", "seed": {"input": 899}, "responses": [{"input": 0}]}, {"name": "post-aborted", "responses": [{"reason": "aborted", "input": 950}]}, {"name": "cache-budget", "responses": [{"input": 500, "cacheRead": 300, "cacheWrite": 101}]}, {"name": "threshold-next-prompt", "secondPrompt": true, "responses": [{"input": 950}, {}]}, {"name": "retry-disabled-overflow", "retryDisabled": true, "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "threshold-end-queue", "queue": true, "queueEnd": true, "responses": [{"input": 950}]}, {"name": "summary-failed-queue", "queue": true, "summaryErrors": ["insufficient_quota"], "responses": [{"error": "prompt is too long", "input": 0}]}, {"name": "threshold-queue-one", "queue": true, "queueCount": 3, "responses": [{"input": 950}]}, {"name": "threshold-queue-all", "queue": true, "queueCount": 3, "queueAll": true, "responses": [{"input": 950}]}, {"name": "no-cut-overflow", "keep": 10000, "responses": [{"error": "prompt is too long", "input": 0}]}]};
 const project=m=>({role:m.role,text:m.summary??(typeof m.content === "string"?m.content:m.content.filter(b=>b.type==="text").map(b=>b.text).join("")),...(m.role==="assistant"?{stopReason:m.stopReason,error:m.errorMessage??""}:{})});
 let runs=[];
 try {
 const credentials=AuthStorage.inMemory();
 const runtime=await ModelRuntime.create({credentials,modelsPath:null,allowModelNetwork:false});
 runtime.registerProvider("auto-fixture",{api:"faux:auto",apiKey:"fixture",baseUrl:"https://invalid.test",models:[{id:"fixture",name:"fixture",reasoning:false,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:1000,maxTokens:100}]});
 for (const sdk of ["src/core/sdk.ts","dist/core/sdk.js"]) {
 const {createAgentSession}=await load("packages/coding-agent/"+sdk);
 const observed=[];
 for (const scenario of input.scenarios) {
 const model={id:"fixture",name:"fixture",provider:"auto-fixture",api:"faux:auto",baseUrl:"https://invalid.test",reasoning:false,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:1000,maxTokens:100};
 const manager=SessionManager.inMemory(dir);
 const assistant=(text,opts={})=>({role:"assistant",content:[{type:"text",text}],api:model.api,provider:opts.otherModel?"other":model.provider,model:model.id,usage:{input:opts.input??20,output:opts.output??0,cacheRead:opts.cacheRead??0,cacheWrite:opts.cacheWrite??0,totalTokens:(opts.input??20)+(opts.output??0)+(opts.cacheRead??0)+(opts.cacheWrite??0),cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:opts.reason??(opts.error?"error":"stop"),...(opts.error?{errorMessage:opts.error}:{}),timestamp:opts.timestamp??Date.now()});
 const seed=[{role:"user",content:"old request",timestamp:1},assistant("old answer",{timestamp:1}),{role:"user",content:"recent request long enough to keep",timestamp:1},assistant("recent answer",{...scenario.seed,timestamp:1})];
 for (const message of seed) manager.appendMessage(message);
 if(scenario.stale)manager.appendCompaction("prior checkpoint",manager.getEntries()[2].id,950);
 const settings=SettingsManager.inMemory({compaction:{enabled:!scenario.disabled,reserveTokens:100,keepRecentTokens:scenario.keep??10},retry:{enabled:!scenario.retryDisabled,maxRetries:2,baseDelayMs:1}});
 const {session}=await createAgentSession({cwd:dir,agentDir:dir,model,modelRuntime:runtime,sessionManager:manager,settingsManager:settings,tools:[]});
 if(scenario.queueAll){session.setSteeringMode("all");session.setFollowUpMode("all");}
 const requests=[],events=[];
 let generations=0,summaries=0,queued=false;
 session.agent.streamFunction=(m,c,o)=>{
 const summary=o.cacheRetention==="none";
 requests.push({kind:summary?"summary":"generation",messages:c.messages.map(project)});
 if(requests.length>15)throw new Error("unbounded recovery");
 const opts=summary?{error:scenario.summaryErrors?.[summaries++]}:(scenario.responses[generations++]??{});
 const response=assistant(summary?"checkpoint":`answer-${generations}`,opts);
 const stream=createAssistantMessageEventStream();
 setTimeout(()=>{response.timestamp=Date.now();stream.push(response.stopReason==="error"||response.stopReason==="aborted"?{type:"error",reason:response.stopReason,error:response}:{type:"done",reason:response.stopReason,message:response});},3);
 return stream;
 };
 session.subscribe(e=>{
 const v={type:e.type};
 if(e.type==="message_start"||e.type==="message_end")v.message=project(e.message);
 if(e.type==="agent_end")v.willRetry=e.willRetry;
 if(e.type==="compaction_start")v.reason=e.reason;
 if(e.type==="compaction_end")Object.assign(v,{reason:e.reason,aborted:e.aborted,willRetry:e.willRetry,error:e.errorMessage??"",summary:e.result?.summary??""});
 if(e.type==="auto_retry_start"||e.type==="summarization_retry_scheduled")Object.assign(v,{attempt:e.attempt,maxAttempts:e.maxAttempts,delayMs:e.delayMs,errorMessage:e.errorMessage});
 if(e.type==="auto_retry_end")Object.assign(v,{attempt:e.attempt,success:e.success,error:e.finalError??""});
 if(e.type==="summarization_retry_attempt_start")Object.assign(v,{source:e.source,reason:e.reason});
 if(e.type!=="message_update"&&e.type!=="queue_update")events.push(v);
 if(scenario.queue&&!queued&&e.type===(scenario.queueEnd?"compaction_end":"compaction_start")){queued=true;for(let i=0;i<(scenario.queueCount??1);i++){void session.steer("steering");void session.followUp("follow-up");}}
 });
 let error="";
 try {await session.prompt("go");} catch(e){error=e.message.toLowerCase();}
 if(scenario.secondPrompt)await session.prompt("next");
 observed.push({name:scenario.name,error,requests,events,messages:session.messages.map(project),entries:manager.getEntries().map(e=>e.type),pending:session.pendingMessageCount});
 session.dispose();
 }
 if(runs.length&&JSON.stringify(canonical(runs))!==JSON.stringify(canonical(observed)))throw new Error("source/dist mismatch");
 runs=observed;
 }
 const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts#prompt";
 const observation={outcome:{runs},side_effects:[]};
 const c={schema_version:"1.0.0",id:"go-sdk/codingagent/auto-compaction",catalog_id:"contract:codingagent/compaction",surface:"go-sdk",input,observe:["outcome","side_effects"]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/auto-compaction.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 }finally{rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
