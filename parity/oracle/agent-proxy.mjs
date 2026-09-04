import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "agent-proxy.json");

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(Object.entries(value).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([key, child]) => [key, canonical(child)]));
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

function digest(value) {
	return `sha256:${createHash("sha256").update(canonicalJSONString(canonical(value))).digest("hex")}`;
}

function caseDigest(value) {
	return `sha256:${createHash("sha256").update(canonicalJSONString({
		schema_version: value.schema_version, id: value.id, catalog_id: value.catalog_id, surface: value.surface,
		input: canonical(value.input), observe: value.observe,
	})).digest("hex")}`;
}

function parseArgs(argv) {
	const result = { pi: defaultPi, out: defaultOutput, check: false };
	const args = argv.slice(2);
	for (let i = 0; i < args.length; i++) {
		if (args[i] === "--check") result.check = true;
		else if (args[i] === "--out") result.out = args[++i];
		else if (result.pi === defaultPi) result.pi = args[i];
		else throw new Error(`unexpected argument: ${args[i]}`);
	}
	return result;
}

function projectMessage(message) {
 const { timestamp, ...result } = structuredClone(message);
 result.content.forEach((block) => delete block.partialJson);
 return result;
}
function projectEvent(event) {
 const result = structuredClone(event);
 for (const key of ["partial", "message", "error"]) if (result[key]) result[key] = projectMessage(result[key]);
 return result;
}

async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json"), "utf8"));
 if (execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 if (execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], {encoding:"utf8"}) !== "") throw new Error("Pi checkout has tracked changes");
 const reference = "packages/agent/src/proxy.ts";
 const { streamProxy } = await import(pathToFileURL(join(args.pi, reference)).href);
 const usage = {input:3,output:2,cacheRead:1,cacheWrite:0,totalTokens:6,cost:{input:0.1,output:0.2,cacheRead:0.01,cacheWrite:0,total:0.31}};
 const prefix = [{type:"start"},{type:"text_start",contentIndex:0},{type:"text_delta",contentIndex:0,delta:"你好"},{type:"text_end",contentIndex:0,contentSignature:"text-signature"}];
 const toolCall = {type:"toolCall",id:"call-1",name:"read",arguments:{path:"hello",nested:{list:[1,true]}},thoughtSignature:"tool-signature"};
 const success = [...prefix,
  {type:"thinking_start",contentIndex:1},{type:"thinking_delta",contentIndex:1,delta:"plan"},{type:"thinking_end",contentIndex:1,contentSignature:"thinking-signature"},
  {type:"toolcall_start",contentIndex:2,id:toolCall.id,toolName:toolCall.name},
  {type:"toolcall_delta",contentIndex:2,delta:'{"path":"hel'},
  {type:"toolcall_delta",contentIndex:2,delta:'lo","nested":{"list":[1,true]}}'},
  {type:"toolcall_end",contentIndex:2,toolCall},{type:"done",reason:"toolUse",usage}];
 const input = {
  model:{id:"proxy-model",name:"Proxy model",api:"openai-completions",provider:"controlled",baseUrl:"https://provider.invalid",reasoning:true,input:["text"],cost:{input:1,output:2,cacheRead:0.1,cacheWrite:0.2},contextWindow:8192,maxTokens:1024},
  context:{systemPrompt:"system",messages:[{role:"user",content:"hi",timestamp:1}],tools:[{name:"read",description:"Read",parameters:{type:"object",properties:{path:{type:"string"}}}}]},
  options:{temperature:0,samplingParams:{top_p:0.8},maxTokens:128,reasoning:"high",cacheRetention:"long",sessionId:"session",headers:{"X-Provider":"key",Remove:null},metadata:{trace:[1,true]},transport:"sse",thinkingBudgets:{high:100},maxRetryDelayMs:0},
  scenarios:[{name:"success",events:success},{name:"error",events:[...prefix,{type:"error",reason:"error",errorMessage:"remote failure",usage}]},{name:"cancel",events:prefix.slice(0,3),cancel:true}]
 };
 const outcomes = [], originalFetch = globalThis.fetch;
 try {
  for (const scenario of input.scenarios) {
   let controller, index=0, ended=false, captured;
   const abort = new AbortController();
   const send = () => {
    if (ended) return;
    if (index === scenario.events.length) {
     ended=true;
     if (scenario.cancel) abort.abort(); else controller.close();
     return;
    }
    const bytes = new TextEncoder().encode(`data: ${JSON.stringify(scenario.events[index++])}\n\n`);
    controller.enqueue(bytes.slice(0, 3)); controller.enqueue(bytes.slice(3));
   };
   globalThis.fetch = async (url, request) => {
    captured = {url,method:request.method,headers:request.headers,body:JSON.parse(request.body)};
    return new Response(new ReadableStream({start(c){controller=c;send();}}));
   };
   const stream = streamProxy(input.model,input.context,{...input.options,proxyUrl:"https://proxy.invalid",authToken:"proxy-secret",signal:abort.signal});
   const pending = [stream.result(),stream.result(),stream.result()];
   const events=[];
   for await (const event of stream) {events.push(projectEvent(event));send();}
   const results = (await Promise.all(pending)).map(projectMessage);
   outcomes.push({name:scenario.name,request:captured,events,results});
  }
 } finally {globalThis.fetch=originalFetch;}
 const declaration = {schema_version:"1.0.0",id:"go-sdk/agent/proxy",catalog_id:"contract:agent/proxy",surface:"go-sdk",input,observe:["outcome","side_effects"]};
 const observation = {outcome:outcomes,side_effects:[]};
 const fixture = {
  schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,
  upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference:`${reference}#streamProxy`},case:declaration,observation,
  input_hash:caseDigest(declaration),observation_hash:digest(observation),execution_method:"node --experimental-strip-types parity/oracle/agent-proxy.mjs <locked-pi-checkout>",platform:"any",
  environment:{node:process.version,oracle_entry:reference}
 };
 if (args.check) {
  const committed=JSON.parse(readFileSync(args.out,"utf8"));fixture.environment.node=committed.environment.node;
  if (JSON.stringify(fixture)!==JSON.stringify(committed)) throw new Error("proxy fixture does not reproduce");
  console.log(`verified ${args.out}`);
 } else {writeFileSync(args.out,`${JSON.stringify(fixture,null,2)}\n`);console.log(`wrote ${args.out}`);}
}
main().catch((error)=>{console.error(error);process.exit(1);});
