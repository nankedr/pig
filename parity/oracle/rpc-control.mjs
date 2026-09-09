import { createHash } from "node:crypto";
import { spawn, execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";
import { once } from "node:events";
const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
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
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value).replaceAll("\u2028", "\\u2028").replaceAll("\u2029", "\\u2029");
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


const pi = process.argv[2];
const lock = JSON.parse(readFileSync(join(root,"parity/baseline/upstream.lock.json")));
if (execFileSync("git", ["-C", pi, "rev-parse", "HEAD"], {encoding:"utf8"}).trim() !== lock.upstream.commit) throw Error("baseline mismatch");
if (execFileSync("git", ["-C", pi, "status", "--porcelain=v1", "--untracked-files=no"], {encoding:"utf8"}).trim()) throw Error("dirty Pi sources");
const inputs = [
 {type:"set_steering_mode",mode:"all"},
 {type:"set_follow_up_mode",mode:"one-at-a-time"},
 {type:"get_state"},
 {type:"set_model",provider:"missing",modelId:"missing"},
 {type:"set_model"},
 {type:"set_model",provider:["deepseek"],modelId:"deepseek-v4-flash"},
 {type:"set_auto_retry",enabled:false},
 {type:"abort_retry"},
 {type:"bash",command:"printf rpc-bash",excludeFromContext:true},
 {type:"abort_bash"},
 {type:"get_session_stats"},
 {type:"get_last_assistant_text"},
 {type:"set_active_tools",tools:[]},
];
const dir = mkdtempSync(join(tmpdir(),"pi-rpc-control-"));
let outcomes;
let lifecycle;
try {
 for (const mode of ["src"]) {
  const child = spawn(process.execPath,["--experimental-strip-types",join(root,"parity/oracle/rpc-child.mjs"),pi,mode,dir],{stdio:["pipe","pipe","pipe"]});
  let stderr="", buffer="", records=[];
  child.stderr.on("data",x=>stderr+=x);
  child.stdout.on("data",x=>{buffer+=x;let i;while((i=buffer.indexOf("\n"))>=0){records.push(JSON.parse(buffer.slice(0,i)));buffer=buffer.slice(i+1);}});
  const wait = async predicate => {const end=Date.now()+15000;while(!predicate()){if(child.exitCode!==null)throw Error(stderr);if(Date.now()>end)throw Error("timeout "+stderr);await new Promise(r=>setTimeout(r,5));}};
  const result=[];
  try {
   for (const [i,input] of inputs.entries()) {
    child.stdin.write(JSON.stringify({...input,id:String(i)})+"\n");
    await wait(()=>records.some(x=>x.type==="response"&&x.id===String(i)));
    const response=records.find(x=>x.type==="response"&&x.id===String(i));
    if(input.type==="get_state") response.data=Object.fromEntries(["steeringMode","followUpMode","pendingMessageCount","messageCount","isStreaming"].map(k=>[k,response.data[k]]));
    if(input.type==="get_session_stats") {delete response.data.sessionId;delete response.data.sessionFile;delete response.data.contextUsage;}
    result.push({response,events:records.filter(x=>x.type==="queue_update").map(({type,steering,followUp})=>({type,steering,followUp}))}); records=[];
   }
   if(outcomes && JSON.stringify(outcomes)!==JSON.stringify(result))throw Error("source/dist mismatch");
   outcomes=result;
   for(let i=0;i<200;i++){
    child.stdin.write(JSON.stringify({type:"set_steering_mode",mode:"one-at-a-time",id:"reset"})+"\n");
    await wait(()=>records.some(x=>x.id==="reset"));records=[];
    child.stdin.write(JSON.stringify({type:"set_steering_mode",mode:"all",id:"set"})+"\n"+JSON.stringify({type:"get_state",id:"get"})+"\n");
    await wait(()=>records.filter(x=>x.type==="response").length===2);
    if(records.find(x=>x.id==="get").data.steeringMode!=="all")throw Error("stale pipelined state");records=[];
   }
  } finally {if(child.exitCode===null){const ended=once(child,"exit");child.kill("SIGTERM");await ended;}}
 }
 const {RpcClient}=await import(pathToFileURL(join(pi,"packages/coding-agent/src/modes/rpc/rpc-client.ts")));
 const client=new RpcClient({cliPath:join(root,"parity/oracle/rpc-control-child.mjs"),cwd:dir,env:{PIG_RPC_ORACLE_PI:pi,PIG_RPC_ORACLE_MODE:"src",PIG_RPC_ORACLE_DIR:dir}});
 try {
  await client.start();
  const finished=client.collectEvents();
  await client.setSteeringMode("all"); await client.setFollowUpMode("one-at-a-time");
  await client.prompt("start"); await client.steer("steering"); await client.followUp("follow-up");
  const pending=(await client.getState()).pendingMessageCount;
  writeFileSync(join(dir,"release"),""); await finished;
  await client.setModel("deepseek","deepseek-v4-pro"); await client.setThinkingLevel("max");
  await client.promptAndWait("configured");
  const messages=await client.getMessages(), stats=await client.getSessionStats();
  const users=messages.filter(m=>m.role==="user").map(m=>m.content.map(c=>c.text??"").join(""));
  lifecycle={pending,users,text:await client.getLastAssistantText(),userMessages:stats.userMessages,assistantMessages:stats.assistantMessages,totalMessages:stats.totalMessages,tokens:stats.tokens};
 } finally {await client.stop();}
 const c={schema_version:"1.0.0",id:"rpc/codingagent/control",catalog_id:"contract:rpc/session-control",surface:"cli",input:{commands:inputs,lifecyclePrompts:["start","steering","follow-up","configured"],pipelinedQueries:200},observe:["outcome","side_effects"]};
 const observation={outcome:{dispatch:outcomes,lifecycle,pipelinedQueries:200},side_effects:[]};
 const reference="packages/coding-agent/src/modes/rpc/rpc-mode.ts#runRpcMode";
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/rpc-control.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 const output=join(root,"parity/oracle/fixtures/rpc-control.json");
 if(process.argv.includes("--check")) {const old=JSON.parse(readFileSync(output));fixture.environment.node=old.environment.node;if(JSON.stringify(old)!==JSON.stringify(fixture))throw Error("fixture drift");console.log("verified RPC control source fixture");}
 else {writeFileSync(output,JSON.stringify(fixture,null,2)+"\n");console.log("wrote RPC control source fixture");}
} finally {rmSync(dir,{recursive:true,force:true});}
