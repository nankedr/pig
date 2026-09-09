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
 {name:"empty-last-text",line:'{"id":"empty","type":"get_last_assistant_text"}'},
 {name:"surrogate-id",line:'{"id":"\\ud800","type":"foobar"}'},
 {name:"unknown-string",line:'{"id":"unknown","type":"foobar"}'},
 {name:"unknown-object-id",line:'{"id":{"key":[1,true]},"type":"foobar"}'},
 {name:"unknown-number-id",line:'{"id":17,"type":"foobar"}'},
 {name:"null-id",line:'{"id":null,"type":"foobar"}'},
 {name:"empty-id",line:'{"id":"","type":"foobar"}'},
 {name:"missing-type",line:'{"id":"missing"}'},
 {name:"number-type",line:'{"id":"number-type","type":7}'},
 {name:"object-type",line:'{"id":"object-type","type":{}}'},
 {name:"primitive",line:'42'}, {name:"array",line:'[]'},
 {name:"malformed",line:'{"id":"bad",'}, {name:"blank",line:''},
 {name:"missing-message",line:'{"id":"missing-message","type":"prompt"}'},
 {name:"null-message",line:'{"id":"null-message","type":"prompt","message":null}'},
 {name:"number-message",line:'{"id":"number-message","type":"prompt","message":5}'},
 {name:"extra-fields",line:'{"id":"extra","type":"get_messages","ignored":true}'},
 {name:"unicode",line:JSON.stringify({id:"中文😀\u2028\u2029",type:"foobar"})},
];
const dir = mkdtempSync(join(tmpdir(),"pi-rpc-"));
let outcomes;
let lifecycle;
try {
 for (const mode of ["src", "dist"]) {
  const child = spawn(process.execPath,["--experimental-strip-types",join(root,"parity/oracle/rpc-child.mjs"),pi,mode,dir],{stdio:["pipe","pipe","pipe"]});
  let stderr="", buffer="", records=[];
  child.stderr.on("data",x=>stderr+=x);
  child.stdout.on("data",x=>{buffer+=x;let i;while((i=buffer.indexOf("\n"))>=0){records.push(JSON.parse(buffer.slice(0,i)));buffer=buffer.slice(i+1);}});
  const wait = async predicate => {const end=Date.now()+15000;while(!predicate()){if(child.exitCode!==null)throw Error(stderr);if(Date.now()>end)throw Error("timeout "+stderr);await new Promise(r=>setTimeout(r,5));}};
  const result=[];
  try {
   child.stdin.write('{"id":"ready","type":"get_state"}\n');
   await wait(()=>records.some(x=>x.id==="ready")); records=[];
   for (const input of inputs) {
    const wire=Buffer.from(input.line+"\r\n");
    for(const b of wire) child.stdin.write(Buffer.from([b]));
    await wait(()=>records.some(x=>x.type==="response"));
    const response=records.find(x=>x.type==="response");
    if(input.name==="surrogate-id"){if(response.id!=="\ud800")throw Error("surrogate id was rewritten");response.id="<lone-high-surrogate>";}
    if(response.command==="parse") response.error="Failed to parse command: <syntax>";
    result.push({name:input.name,response}); records=[];
   }
   if(outcomes && JSON.stringify(outcomes)!==JSON.stringify(result))throw Error("source/dist mismatch");
   outcomes=result;
  } finally {if(child.exitCode===null){const ended=once(child,"exit");child.kill("SIGTERM");await ended;}}
 }
 for (const mode of ["src","dist"]) {
  const {RpcClient} = await import(pathToFileURL(join(pi,"packages/coding-agent",mode,"modes/rpc/rpc-client."+(mode==="src"?"ts":"js"))));
  const client=new RpcClient({cliPath:join(root,"parity/oracle/rpc-child.mjs"),cwd:dir,env:{PIG_RPC_ORACLE_PI:pi,PIG_RPC_ORACLE_MODE:mode,PIG_RPC_ORACLE_DIR:dir}});
  try {
   await client.start();
   const events=await client.promptAndWait("hello");
   const messages=await client.getMessages();
   const text=await client.getLastAssistantText();
   const observation={roles:messages.map(m=>m.role),text,eventTypes:[...new Set(events.map(e=>e.type))].sort(),projected:events.filter(e=>e.type==="message_update").every(e=>!("message" in e)&&!("partial" in e.assistantMessageEvent))};
   if(lifecycle&&JSON.stringify(lifecycle)!==JSON.stringify(observation))throw Error("client source/dist mismatch");
   lifecycle=observation;
  } finally {await client.stop();}
 }
 const c={schema_version:"1.0.0",id:"rpc/codingagent/basic",catalog_id:"contract:rpc/jsonl-transport",surface:"cli",input:{scenarios:inputs,lifecyclePrompt:"hello"},observe:["outcome","side_effects"]};
 const observation={outcome:{dispatch:outcomes,lifecycle},side_effects:[]};
 const reference="packages/coding-agent/src/modes/rpc/rpc-mode.ts#runRpcMode";
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/rpc.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 const output=join(root,"parity/oracle/fixtures/rpc.json");
 if(process.argv.includes("--check")) {const old=JSON.parse(readFileSync(output));fixture.environment.node=old.environment.node;if(JSON.stringify(old)!==JSON.stringify(fixture))throw Error("fixture drift");console.log("verified RPC source/dist fixture");}
 else {writeFileSync(output,JSON.stringify(fixture,null,2)+"\n");console.log("wrote RPC source/dist fixture");}
} finally {rmSync(dir,{recursive:true,force:true});}
