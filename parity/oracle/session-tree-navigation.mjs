import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-tree-navigation.json");

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
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("baseline mismatch");
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain=v1", "--untracked-files=no"], {encoding:"utf8"}).trim()) throw new Error("dirty Pi sources");
 globalThis.fetch = async () => { throw new Error("unexpected network"); };
 const load = p => import(pathToFileURL(join(args.pi, p)).href);
 const {ModelRuntime} = await load("packages/coding-agent/src/core/model-runtime.ts");
 const {AuthStorage} = await load("packages/coding-agent/src/core/auth-storage.ts");
 const {SessionManager} = await load("packages/coding-agent/src/core/session-manager.ts");
 const {SettingsManager} = await load("packages/coding-agent/src/core/settings-manager.ts");
 const {createFauxCore, fauxAssistantMessage} = await load("packages/ai/src/providers/faux.ts");
 const dir = mkdtempSync(join(tmpdir(), "pi-tree-navigation-"));
 const input = {sequence:["first", "second", "new branch", "original continuation"], projection:"Entry IDs are mapped to semantic names; model and thinking remain live across navigation, while SessionContext follows the selected path."};
 input.history = [
  {type:"message",id:"root",parentId:null,message:{role:"user",content:[{type:"text",text:"hello"},{type:"image",mimeType:"image/png",data:"aGk="},{type:"text",text:" world"}],timestamp:1}},
  {type:"message",id:"answer",parentId:"root",message:{role:"assistant",content:[{type:"text",text:"answer"}],api:"openai-completions",provider:"deepseek",model:"deepseek-v4-flash",usage:{input:0,output:0,cacheRead:0,cacheWrite:0,totalTokens:0,cost:{input:0,output:0,cacheRead:0,cacheWrite:0,total:0}},stopReason:"stop",timestamp:2}},
  {type:"custom_message",id:"custom",parentId:"answer",customType:"note",content:[{type:"text",text:"custom"},{type:"text",text:" text"}],display:true},
  {type:"message",id:"tool",parentId:"custom",message:{role:"toolResult",toolCallId:"call",toolName:"read",content:[{type:"text",text:"file"}],isError:false,timestamp:3}},
  {type:"compaction",id:"compact",parentId:"tool",summary:"compressed",firstKeptEntryId:"answer",tokensBefore:100},
  {type:"branch_summary",id:"summary",parentId:"compact",summary:"other branch",fromId:"root"},
  {type:"label",id:"label",parentId:"summary",targetId:"root",label:"start"},
  {type:"model_change",id:"model",parentId:"label",provider:"deepseek",modelId:"deepseek-v4-flash"},
  {type:"thinking_level_change",id:"thinking",parentId:"model",thinkingLevel:"off"}
 ].map(e=>({...e,timestamp:"2026-01-01T00:00:00Z"}));
 let outcome;
 try {
  for (const path of ["src/core/sdk.ts", "dist/core/sdk.js"]) {
   const {createAgentSession} = await load("packages/coding-agent/" + path);
   const runtime = await ModelRuntime.create({credentials:AuthStorage.inMemory(), modelsPath:null, allowModelNetwork:false});
   runtime.registerProvider("deepseek", {api:"openai-completions", apiKey:"fixture", baseUrl:"https://invalid.test", models:["deepseek-v4-flash", "deepseek-v4-pro"].map(id=>({id,name:id,reasoning:true,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:2048}))});
   const manager = SessionManager.create(dir, join(dir, path.startsWith("src") ? "source" : "dist"));
   const settings = SettingsManager.inMemory({compaction:{enabled:false},retry:{enabled:false}});
   const {session} = await createAgentSession({cwd:dir,agentDir:dir,model:runtime.getModel("deepseek","deepseek-v4-flash"),modelRuntime:runtime,sessionManager:manager,settingsManager:settings,thinkingLevel:"off",tools:[]});
   const core = createFauxCore({}); session.agent.streamFunction = core.streamSimple;
   const names = new Map(), states = [], requests = [];
   const text = m => typeof m.content === "string" ? m.content : (m.content??[]).filter(b=>b.type==="text").map(b=>b.text).join("");
   const messages = ms => ms.map(m=>({role:m.role,text:text(m)}));
   const request = async prompt => {core.setResponses([(c,o,_s,m)=>{requests.push({messages:messages(c.messages),model:m.id,thinking:o?.reasoning??"off"});return fauxAssistantMessage("reply:"+prompt)}]);await session.prompt(prompt);};
   const capture = (name,result) => {const c=manager.buildSessionContext();states.push({name,editorText:result.editorText??null,cancelled:result.cancelled,leaf:names.get(manager.getLeafId())??manager.getLeafEntry()?.type??null,messages:messages(session.messages),model:session.model.id,thinking:session.thinkingLevel,pathModel:c.model?.modelId??null,pathThinking:c.thinkingLevel,labels:["u2","a1"].map(n=>manager.getLabel([...names].find(([,v])=>v===n)[0])??null),summaryCount:manager.getEntries().filter(e=>e.type==="branch_summary").length});};
   await request(input.sequence[0]);
   let entries=manager.getEntries();const u1=entries.find(e=>e.type==="message"&&e.message.role==="user").id;const a1=manager.getLeafId();names.set(u1,"u1");names.set(a1,"a1");
   await session.setModel(runtime.getModel("deepseek","deepseek-v4-pro"));session.setThinkingLevel("high");
   await request(input.sequence[1]);
   const u2=manager.getEntries().filter(e=>e.type==="message"&&e.message.role==="user")[1].id;const a2=manager.getLeafId();names.set(u2,"u2");names.set(a2,"a2");
   const count=manager.getEntries().length;capture("same",await session.navigateTree(a2,{summarize:true,label:"ignored"}));
   const sameCount=manager.getEntries().length===count;
   capture("user",await session.navigateTree(u2,{label:"retry",customInstructions:"ignored",replaceInstructions:true}));
   capture("ancestor",await session.navigateTree(a1));
   await request(input.sequence[2]);const branch=manager.getLeafId();names.set(branch,"branch");
   capture("other",await session.navigateTree(a2));await request(input.sequence[3]);
   capture("return",await session.navigateTree(branch));
   const before=manager.getLeafId();let invalid=false;try{await session.navigateTree("missing")}catch{invalid=manager.getLeafId()===before}
   const reopened=SessionManager.open(manager.getSessionFile());
   const persisted={original:!!reopened.getEntry(a2),branch:!!reopened.getEntry(branch),label:reopened.getLabel(u2),messages:messages(reopened.buildSessionContext().messages),thinking:reopened.buildSessionContext().thinkingLevel,model:reopened.buildSessionContext().model.modelId};
   await session.navigateTree(a1);
   await session.navigateTree(branch,{label:"branch end"});
   const reopenedBranch=SessionManager.open(manager.getSessionFile());
   const {session:restored}=await createAgentSession({cwd:dir,agentDir:dir,modelRuntime:runtime,sessionManager:reopenedBranch,settingsManager:settings,tools:[]});
   const restoredBranch={model:restored.model.id,thinking:restored.thinkingLevel,messages:messages(restored.messages)};
   restored.dispose();
   session.dispose();
   const historical = [];
   for (const target of ["root","custom","tool","compact","summary","label","model","thinking"]) {
    const file=join(dir, "history.jsonl");
    writeFileSync(file,[{type:"session",version:3,id:"history",cwd:dir,timestamp:"2026-01-01T00:00:00Z"},...input.history].map(e=>JSON.stringify(e)).join("\n")+"\n");
    const m=SessionManager.open(file);
    const {session:h}=await createAgentSession({cwd:dir,agentDir:dir,model:runtime.getModel("deepseek","deepseek-v4-pro"),modelRuntime:runtime,sessionManager:m,settingsManager:settings,thinkingLevel:"high",tools:[]});
    const r=await h.navigateTree(target);
    historical.push({target,leaf:m.getLeafId(),editorText:r.editorText??null,messages:messages(h.messages),model:h.model.id,thinking:h.thinkingLevel});
    h.dispose();
   }
   const captured={states,requests,sameCount,invalid,persisted,historical,restoredBranch};
   if(outcome&&JSON.stringify(canonical(outcome))!==JSON.stringify(canonical(captured)))throw new Error("source/dist mismatch");outcome=captured;
  }
  const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts#navigateTree";
  const observation={outcome,side_effects:[]};
  const c={schema_version:"1.0.0",id:"go-sdk/codingagent/session-tree-navigation",catalog_id:"contract:codingagent/session-tree-navigation",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/session-tree-navigation.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`)}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`)}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(e=>{console.error(e);process.exit(1)});
