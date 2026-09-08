import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "branch-summary.json");

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
 const args=parseArgs(process.argv), lock=JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json"),"utf8"));
 if(execFileSync("git",["-C",args.pi,"rev-parse","HEAD"],{encoding:"utf8"}).trim()!==lock.upstream.commit)throw Error("baseline mismatch");
 if(execFileSync("git",["-C",args.pi,"status","--porcelain=v1","--untracked-files=no"],{encoding:"utf8"}).trim())throw Error("dirty baseline");
 globalThis.fetch=async()=>{throw Error("unexpected network")};
 const load=p=>import(pathToFileURL(join(args.pi,p)).href);
 const {ModelRuntime}=await load("packages/coding-agent/src/core/model-runtime.ts");
 const {AuthStorage}=await load("packages/coding-agent/src/core/auth-storage.ts");
 const {SessionManager}=await load("packages/coding-agent/src/core/session-manager.ts");
 const {SettingsManager}=await load("packages/coding-agent/src/core/settings-manager.ts");
 const {createFauxCore,fauxAssistantMessage}=await load("packages/ai/src/providers/faux.ts");
 const dir=mkdtempSync(join(tmpdir(),"pi-branch-summary-"));
 const usage={input:80,output:20,cacheRead:0,cacheWrite:0,totalTokens:100,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}};
 const user=text=>({role:"user",content:[{type:"text",text}],timestamp:1});
 const assistant=text=>({role:"assistant",content:[{type:"text",text}],api:"openai-completions",provider:"deepseek",model:"deepseek-v4-flash",usage,stopReason:"stop",timestamp:1});
 const history=[
 {type:"message",id:"root",parentId:null,message:user("shared request")},
 {type:"message",id:"shared",parentId:"root",message:assistant("shared answer")},
 {type:"message",id:"target",parentId:"shared",message:user("original path")},
 {type:"message",id:"other",parentId:"target",message:assistant("original answer")},
 {type:"message",id:"explore",parentId:"shared",message:user("explore alternative")},
 {type:"branch_summary",id:"prior",parentId:"explore",summary:"prior exploration",fromId:"shared",fromHook:false,details:{readFiles:["prior.txt","shared.go"],modifiedFiles:["prior.go"]}},
 {type:"message",id:"tools",parentId:"prior",message:{...assistant("exploration result"),content:[{type:"text",text:"exploration result"},...["read","edit","read","write"].map((name,i)=>({type:"toolCall",id:"call"+i,name,arguments:{path:["shared.go","shared.go","read.go","write.go"][i]}}))]}},
 {type:"message",id:"toolresult",parentId:"tools",message:{role:"toolResult",toolCallId:"call0",toolName:"read",content:[{type:"text",text:"tool output excluded"}],isError:false,timestamp:1}},
 {type:"message",id:"leaf",parentId:"toolresult",message:assistant("finished exploration")}
 ].map(e=>({...e,timestamp:"2026-01-01T00:00:00Z"}));
 const input={history,scenarios:[
 {name:"other",target:"other",label:"explored",instructions:"focus files",replace:false,reserve:1000},
 {name:"root",target:"root",instructions:"only conclusions",replace:true},
 {name:"nested-user",target:"target"},
 {name:"ancestor",target:"shared",replace:true},
 {name:"same",target:"leaf",label:"ignored"},
 {name:"budget",target:"other",reserve:31990},
 {name:"summary-budget",target:"other",reserve:31955},
 {name:"oversized-reserve",target:"other",reserve:33000},
 {name:"no-content",target:"leaf",metadata:true},
 {name:"forward",target:"other",start:"shared"},
 {name:"hook-details",target:"other",hook:true},
 {name:"compaction",target:"other",compaction:true},
 {name:"custom",target:"target",custom:true},
 {name:"roundtrip",target:"other",roundtrip:true},
 {name:"retry",target:"other",errors:["503 overloaded"]},
 {name:"exhausted",target:"other",errors:["503 overloaded","503 overloaded","503 overloaded"]},
 {name:"quota",target:"other",errors:["429 insufficient_quota"]},
 {name:"abort",target:"other",abort:true},
 {name:"abort-retry",target:"other",errors:["503 overloaded"],abortRetry:true}
 ].map(x=>({label:"",instructions:"",replace:false,reserve:1000,errors:[],abort:false,abortRetry:false,metadata:false,start:"",hook:false,compaction:false,custom:false,roundtrip:false,...x}))};
 let outcome;
 try{
 for(const sdk of ["src/core/sdk.ts","dist/core/sdk.js"]){
 const {createAgentSession}=await load("packages/coding-agent/"+sdk);
 const runtime=await ModelRuntime.create({credentials:AuthStorage.inMemory(),modelsPath:null,allowModelNetwork:false});
 runtime.registerProvider("deepseek",{api:"openai-completions",apiKey:"fixture",baseUrl:"https://invalid.test",models:[{id:"deepseek-v4-flash",name:"fixture",reasoning:true,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:4096}]});
 const runs=[];
 for(const scenario of input.scenarios){
 const seed=structuredClone(history);
 if(scenario.metadata)seed.push({type:"custom",id:"metadata",parentId:"leaf",timestamp:"2026-01-01T00:00:00Z",customType:"state",data:{}});
 if(scenario.hook)seed.find(e=>e.id==="prior").fromHook=true;
 if(scenario.compaction)Object.assign(seed.find(e=>e.id==="prior"),{type:"compaction",firstKeptEntryId:"tools",tokensBefore:100});
 if(scenario.custom)Object.assign(seed.find(e=>e.id==="target"),{type:"custom_message",customType:"note",content:[{type:"text",text:"custom target"}],display:true});
 const file=join(dir,"session.jsonl");writeFileSync(file,[{type:"session",version:3,id:"history",cwd:dir,timestamp:"2026-01-01T00:00:00Z"},...seed].map(e=>JSON.stringify(e)).join("\n")+"\n");
 const manager=SessionManager.open(file),settings=SettingsManager.inMemory({compaction:{enabled:false},branchSummary:{reserveTokens:scenario.reserve},retry:{enabled:true,maxRetries:2,baseDelayMs:1}});
 const {session}=await createAgentSession({cwd:dir,agentDir:dir,model:runtime.getModel("deepseek","deepseek-v4-flash"),modelRuntime:runtime,sessionManager:manager,settingsManager:settings,thinkingLevel:"high",tools:[]});
 const core=createFauxCore({}),requests=[],events=[],routing=[];
 session.agent.streamFunction=core.streamSimple;
 const text=m=>typeof m.content==="string"?m.content:(m.content??[]).filter(b=>b.type==="text").map(b=>b.text).join("");
 const messages=ms=>ms.map(m=>({role:m.role,text:text(m)}));
 session.subscribe(e=>{if(e.type.startsWith("summarization_")||e.type.startsWith("compaction_"))events.push(e);if(scenario.abortRetry&&e.type==="summarization_retry_scheduled")session.abortBranchSummary()});
 let attempt=0;
 core.setResponses(Array.from({length:scenario.errors.length+1},()=> (c,o)=>{routing.push(o.sessionId);requests.push({system:c.systemPrompt,messages:messages(c.messages),maxTokens:o.maxTokens,cache:o.cacheRetention,reasoning:o.reasoning??null,apiKey:o.apiKey??null});
 if(scenario.abort){session.abortBranchSummary();return {...fauxAssistantMessage(""),stopReason:"aborted"}}
 const error=scenario.errors[attempt++];return {...fauxAssistantMessage("summary result"),usage,...(error?{stopReason:"error",errorMessage:error}:{})};}));
 if(scenario.start)await session.navigateTree(scenario.start);
 const before=manager.getEntries().length;
 let r={cancelled:false},error=null;try{r=await session.navigateTree(scenario.target,{summarize:true,label:scenario.label,customInstructions:scenario.instructions,replaceInstructions:scenario.replace})}catch(e){error=e.message}
 const summary=r.summaryEntry;
 const state={error,editorText:r.editorText??null,cancelled:r.cancelled,aborted:r.aborted??false,summary:summary?{parent:summary.parentId,from:summary.fromId,text:summary.summary,details:summary.details,usage:summary.usage??null,fromHook:summary.fromHook,label:manager.getLabel(summary.id)??null}:null,messages:messages(session.messages),added:manager.getEntries().length-before,original:!!manager.getEntry("leaf")};
 const reopened=SessionManager.open(file);
 const {session:restored}=await createAgentSession({cwd:dir,agentDir:dir,modelRuntime:runtime,sessionManager:reopened,settingsManager:settings,tools:[]});
 const persisted=messages(restored.messages);restored.dispose();
 let continuation=[];core.setResponses([(c)=>{continuation=messages(c.messages);return fauxAssistantMessage("continued")}]);await session.prompt("continue target");
 let roundtrip=null;
 if(scenario.roundtrip){
 let request;core.setResponses([(c)=>{request=messages(c.messages);return {...fauxAssistantMessage("return summary"),usage}}]);
 const next=await session.navigateTree("leaf",{summarize:true});
 const count=manager.getEntries().length;await session.navigateTree(manager.getLeafId(),{summarize:true});
 roundtrip={request,summary:next.summaryEntry.summary,details:next.summaryEntry.details,noDuplicate:manager.getEntries().length===count,original:!!manager.getEntry("other")};
 }else await session.navigateTree("leaf");
 const returned=messages(session.messages);
 session.dispose();runs.push({name:scenario.name,roundtrip,requests,events,state,persisted,continuation,returned,routing:routing.every(id=>!!id&&id===routing[0])});
 }
 if(outcome&&JSON.stringify(canonical(outcome))!==JSON.stringify(canonical(runs)))throw Error("source/dist mismatch");outcome=runs;
 }
 const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts#navigateTree";
 const observation={outcome,side_effects:[]};
 const c={schema_version:"1.0.0",id:"go-sdk/codingagent/branch-summary",catalog_id:"contract:codingagent/branch-summary",surface:"go-sdk",input,observe:["outcome","side_effects"]};
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/branch-summary.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 }finally{rmSync(dir,{recursive:true,force:true})}
}
main().catch(e=>{console.error(e);process.exit(1)});
