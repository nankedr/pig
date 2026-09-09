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
 {type:"set_auto_compaction",enabled:false}, {type:"get_fork_messages"},
 {type:"get_entries",since:"missing"}, {type:"fork",entryId:"missing"},
 {type:"set_session_name",name:"   "}, {type:"set_session_name",name:"  named  "},
 {type:"get_state"}, {type:"new_session"}, {type:"get_state"},
 {type:"navigate_tree",targetId:"missing"}, {type:"abort_compaction"},
];
const dir=mkdtempSync(join(tmpdir(),"pi-rpc-lifecycle-"));
try {
 const child=spawn(process.execPath,["--experimental-strip-types",join(root,"parity/oracle/rpc-lifecycle-child.mjs"),pi,"src",dir],{stdio:["pipe","pipe","pipe"]});
 let stderr="",buffer="",records=[];
 child.stderr.on("data",x=>stderr+=x);
 child.stdout.on("data",x=>{buffer+=x;let i;while((i=buffer.indexOf("\n"))>=0){records.push(JSON.parse(buffer.slice(0,i)));buffer=buffer.slice(i+1);}});
 const dispatch=[];
 try {
  for (const [i,input] of inputs.entries()) {
   child.stdin.write(JSON.stringify({...input,id:String(i)})+"\n");
   const end=Date.now()+15000;
   while(!records.some(x=>x.type==="response"&&x.id===String(i))){if(child.exitCode!==null||Date.now()>end)throw Error("timeout/exit "+stderr);await new Promise(r=>setTimeout(r,5));}
   const response=records.find(x=>x.type==="response"&&x.id===String(i));
   if(input.type==="get_state")response.data=Object.fromEntries(["messageCount","isStreaming","autoCompactionEnabled","sessionName"].filter(k=>k in response.data).map(k=>[k,response.data[k]]));
   dispatch.push(response);records=[];
  }
 } finally {if(child.exitCode===null){const ended=once(child,"exit");child.kill("SIGTERM");await ended;}}
 const {RpcClient}=await import(pathToFileURL(join(pi,"packages/coding-agent/src/modes/rpc/rpc-client.ts")));
 const client=new RpcClient({cliPath:join(root,"parity/oracle/rpc-lifecycle-child.mjs"),cwd:dir,env:{PIG_RPC_ORACLE_PI:pi,PIG_RPC_ORACLE_MODE:"src",PIG_RPC_ORACLE_DIR:dir,PIG_RPC_LIFECYCLE_PERSIST:"1"}});
 let lifecycle;
 try {
  await client.start();
  await client.promptAndWait("first"); await client.promptAndWait("second"); await client.setSessionName("  source  ");
  const state=await client.getState(), source=state.sessionFile;
  const {entries,leafId}=await client.getEntries(), tail=await client.getEntries(entries[0].id), tree=await client.getTree(), users=await client.getForkMessages();
  const before=readFileSync(source,"utf8");
  const forkResult=await client.fork(users[1].entryId), fork=await client.getState();
  await client.promptAndWait("fork continued");
  const sourceUnchanged=before===readFileSync(source,"utf8");
  await client.clone(); const clone=await client.getState();
  await client.newSession(source); const fresh=await client.getState(); await client.promptAndWait("fresh continued");
  const header=JSON.parse(readFileSync(fresh.sessionFile,"utf8").split("\n")[0]);
  await client.switchSession(source); const restored=await client.getState();
  const compact=await client.compact("preserve important details"), compacted=await client.getEntries();
  await client.setAutoCompaction(true); const auto=(await client.getState()).autoCompactionEnabled; await client.setAutoCompaction(false);
  await client.promptAndWait("after summary"); const expected=await client.getMessages();
  await client.stop(); await client.start(); await client.switchSession(source); const reopened=await client.getMessages();
  await client.promptAndWait("reopened continued");
  lifecycle={users:users.map(x=>x.text),name:state.sessionName,forkText:forkResult.text,forkMessages:fork.messageCount,forkIdentityChanged:fork.sessionId!==state.sessionId,sourceUnchanged,cloneMessages:clone.messageCount,cloneIdentityChanged:clone.sessionId!==fork.sessionId,newMessages:fresh.messageCount,parentPreserved:header.parentSession===source,restoredIdentity:restored.sessionId===state.sessionId,restoredMessages:restored.messageCount,cursorMatches:JSON.stringify(tail.entries)===JSON.stringify(entries.slice(1))&&tail.leafId===leafId,treeLeafMatches:tree.leafId===leafId,compactionRetainsHistory:JSON.stringify(compacted.entries.slice(0,entries.length))===JSON.stringify(entries),summaryPresent:!!compact.summary,autoEnabled:auto,reopenMatches:JSON.stringify(expected)===JSON.stringify(reopened)};
 } finally {await client.stop();}
 const c={schema_version:"1.0.0",id:"rpc/codingagent/lifecycle",catalog_id:"contract:rpc/session-lifecycle",surface:"cli",input:{commands:inputs},observe:["outcome","side_effects"]};
 const observation={outcome:{dispatch,lifecycle},side_effects:[]};
 const reference="packages/coding-agent/src/modes/rpc/rpc-mode.ts#runRpcMode";
 const fixture={schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/rpc-lifecycle.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
 const output=join(root,"parity/oracle/fixtures/rpc-lifecycle.json");
 if(process.argv.includes("--check")){const old=JSON.parse(readFileSync(output));fixture.environment.node=old.environment.node;if(JSON.stringify(old)!==JSON.stringify(fixture))throw Error("fixture drift");console.log("verified RPC lifecycle source fixture");}
 else {writeFileSync(output,JSON.stringify(fixture,null,2)+"\n");console.log("wrote RPC lifecycle source fixture");}
} finally {rmSync(dir,{recursive:true,force:true});}
