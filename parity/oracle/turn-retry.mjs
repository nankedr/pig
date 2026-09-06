import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "turn-retry.json");

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
 if(execFileSync("git",["-C",args.pi,"status","--porcelain=v1","--untracked-files=no"],{encoding:"utf8"}).trim()) throw new Error("Pi tracked sources are dirty");
 globalThis.fetch = async () => { throw new Error("unexpected Oracle network request"); };
 const load = p => import(pathToFileURL(join(args.pi, p)).href);
 const sourceSDK = await load("packages/coding-agent/src/core/sdk.ts");
 const distSDK = await load("packages/coding-agent/dist/core/sdk.js");
 const {SessionManager} = await load("packages/coding-agent/src/core/session-manager.ts");
 const {SettingsManager} = await load("packages/coding-agent/src/core/settings-manager.ts");
 const {ModelRuntime} = await load("packages/coding-agent/src/core/model-runtime.ts");
 const {AuthStorage} = await load("packages/coding-agent/src/core/auth-storage.ts");
 const {createFauxCore, fauxAssistantMessage, fauxToolCall} = await load("packages/ai/src/providers/faux.ts");
 const dir = mkdtempSync(join(tmpdir(), "pi-turn-retry-"));
 const project = m => ({role:m.role, text:typeof m.content === "string" ? m.content : m.content.filter(c=>c.type === "text").map(c=>c.text).join(""), ...(m.role === "assistant" ? {stopReason:m.stopReason, errorMessage:m.errorMessage ?? ""} : {})});
 const input = {
  "scenarios": [
    {
      "name": "recovered",
      "errors": [
        "overloaded_error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "large-delay",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 2147483648,
      "enabled": true
    },
    {
      "name": "negative-delay",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": -1,
      "enabled": true
    },
    {
      "name": "zero-delay",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 0,
      "enabled": true
    },
    {
      "name": "backoff",
      "errors": [
        "overloaded_error",
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "exhausted",
      "errors": [
        "503",
        "503",
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "permanent-after-retry",
      "errors": [
        "503",
        "invalid_api_key"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "disabled",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": false
    },
    {
      "name": "zero-budget",
      "errors": [
        "503"
      ],
      "maxRetries": 0,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "tool-before-error",
      "errors": [
        "@tool",
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "tool-after-error",
      "errors": [
        "503",
        "@tool"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "budget-resets-at-tool",
      "errors": [
        "503",
        "503",
        "@tool",
        "503",
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "cancel-wait",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 50,
      "enabled": true,
      "cancel": true
    },
    {
      "name": "classification-0",
      "errors": [
        "GoUsageLimitError 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-1",
      "errors": [
        "FreeUsageLimitError 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-2",
      "errors": [
        "Monthly usage limit reached 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-3",
      "errors": [
        "available balance 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-4",
      "errors": [
        "insufficient_quota 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-5",
      "errors": [
        "out of budget 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-6",
      "errors": [
        "quota exceeded 429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-7",
      "errors": [
        "billing 500"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-8",
      "errors": [
        "503 prompt is too long"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-9",
      "errors": [
        "invalid_api_key"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-10",
      "errors": [
        "overloaded"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-11",
      "errors": [
        "rate-limit"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-12",
      "errors": [
        "too many requests"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-13",
      "errors": [
        "429"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-14",
      "errors": [
        "500"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-15",
      "errors": [
        "502"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-16",
      "errors": [
        "503"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-17",
      "errors": [
        "504"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-18",
      "errors": [
        "524"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-19",
      "errors": [
        "service unavailable"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-20",
      "errors": [
        "server error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-21",
      "errors": [
        "internal error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-22",
      "errors": [
        "Provider returned error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-23",
      "errors": [
        "exceeded request buffer limit while retrying upstream"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-24",
      "errors": [
        "Provider finish_reason: network_error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-25",
      "errors": [
        "connection error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-26",
      "errors": [
        "connection refused"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-27",
      "errors": [
        "connection lost"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-28",
      "errors": [
        "other side closed"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-29",
      "errors": [
        "fetch failed"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-30",
      "errors": [
        "getaddrinfo"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-31",
      "errors": [
        "ENOTFOUND"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-32",
      "errors": [
        "EAI_AGAIN"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-33",
      "errors": [
        "upstream connect"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-34",
      "errors": [
        "reset before headers"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-35",
      "errors": [
        "socket hang up"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-36",
      "errors": [
        "socket connection was closed"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-37",
      "errors": [
        "timed out"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-38",
      "errors": [
        "time out"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-39",
      "errors": [
        "timeout"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-40",
      "errors": [
        "terminated"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-41",
      "errors": [
        "websocket closed"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-42",
      "errors": [
        "websocket error"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-43",
      "errors": [
        "ended without"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-44",
      "errors": [
        "stream ended before message_stop"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-45",
      "errors": [
        "stream ended before a terminal response event"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-46",
      "errors": [
        "http2 request did not get a response"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-47",
      "errors": [
        "retry delay"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-48",
      "errors": [
        "you can retry your request"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-49",
      "errors": [
        "try your request again"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-50",
      "errors": [
        "please retry your request"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    },
    {
      "name": "classification-51",
      "errors": [
        "ResourceExhausted"
      ],
      "maxRetries": 2,
      "baseDelayMs": 1,
      "enabled": true
    }
  ]
};
 let runs = [];
 try {
  const runtime = await ModelRuntime.create({credentials:AuthStorage.inMemory(),modelsPath:null,allowModelNetwork:false});
  runtime.registerProvider("retry-fixture", {api:"faux:retry-fixture",apiKey:"fixture",baseUrl:"https://invalid.test",models:[{id:"fixture",name:"fixture",reasoning:false,input:["text"],cost:{input:0,output:0,cacheRead:0,cacheWrite:0},contextWindow:32000,maxTokens:2048}]});
  for (const {createAgentSession} of [sourceSDK,distSDK]) {
  const captured = [];
  for (const scenario of input.scenarios) {
   const core = createFauxCore({api:"faux:retry-fixture",provider:"retry-fixture",models:[{id:"fixture"}],tokenSize:{min:100,max:100}});
   const manager = SessionManager.inMemory(dir);
   const {session} = await createAgentSession({cwd:dir,agentDir:dir,model:core.getModel(),modelRuntime:runtime,sessionManager:manager,settingsManager:SettingsManager.inMemory({retry:{enabled:scenario.enabled,maxRetries:scenario.maxRetries,baseDelayMs:scenario.baseDelayMs},compaction:{enabled:false}}),tools:["write"]});
   session.agent.streamFunction = core.streamSimple;
   const contexts = [], events = [];
   core.setResponses([...scenario.errors.map(errorMessage=>errorMessage === "@tool" ? fauxAssistantMessage(fauxToolCall("write", {path:"once.txt",content:"once"}, {id:"call-once"}), {stopReason:"toolUse"}) : fauxAssistantMessage("partial", {stopReason:"error",errorMessage})),fauxAssistantMessage("recovered")].map(message=>context=>{contexts.push(context.messages.map(project));return message;}));
   session.subscribe(event=>{
    if(event.type === "message_update") { if(events.at(-1)?.type !== "message_update") events.push({type:event.type}); return; }
    const e = {type:event.type};
    if(event.type === "message_end") e.message=project(event.message);
    if(event.type === "agent_end") e.willRetry=event.willRetry;
    if(event.type === "auto_retry_start" || event.type === "auto_retry_end") Object.assign(e,event,{retryAttempt:session.retryAttempt,historyCount:manager.getEntries().filter(e=>e.type === "message").length});
    events.push(e);
    if(scenario.cancel && event.type === "auto_retry_start") setTimeout(()=>session.abortRetry(),0);
   });
   await session.prompt("retry please");
   captured.push({name:scenario.name,events,contexts,toolExecutions:events.filter(e=>e.type === "tool_execution_end").length,messages:session.messages.map(project),history:manager.getEntries().filter(e=>e.type === "message").map(e=>project(e.message)),attempt:session.retryAttempt,retrying:session.isRetrying});
   session.dispose();
  }
  if(runs.length && JSON.stringify(canonical(captured))!==JSON.stringify(canonical(runs))) throw new Error("Pi source/dist retry observations differ");
  runs=captured;
  }
  const reference="packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts";
  const observation={outcome:{runs},side_effects:[]};
  const c={schema_version:"1.0.0",id:"go-sdk/codingagent/turn-retry",catalog_id:"contract:codingagent/turn-retry",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/turn-retry.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if(args.check){const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;if(JSON.stringify(fixture)!==JSON.stringify(committed))throw new Error("fixture drift");console.log(`verified ${args.out}`);}else{writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");console.log(`wrote ${args.out}`);}
 } finally {rmSync(dir,{recursive:true,force:true});}
}
main().catch(e=>{console.error(e);process.exit(1)});
