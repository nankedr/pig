import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const root = join(dirname(fileURLToPath(import.meta.url)), "../..");
const pi = process.argv[2];
const lock = JSON.parse(readFileSync(join(root, "parity/baseline/upstream.lock.json")));
if (execFileSync("git", ["-C", pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim() !== lock.upstream.commit) throw Error("wrong Pi baseline");
if (execFileSync("git", ["-C", pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" })) throw Error("dirty Pi baseline");
const reference = "packages/coding-agent/src/core/sdk.ts";
const load = (path) => import(pathToFileURL(join(pi, path)));
const { createAgentSession } = await load(reference);
const { SessionManager } = await load("packages/coding-agent/src/core/session-manager.ts");
const { SettingsManager } = await load("packages/coding-agent/src/core/settings-manager.ts");
const { ModelRuntime } = await load("packages/coding-agent/src/core/model-runtime.ts");
const { AuthStorage } = await load("packages/coding-agent/src/core/auth-storage.ts");
const { createFauxCore, fauxAssistantMessage } = await load("packages/ai/src/providers/faux.ts");
const text = (m) => typeof m.content === "string" ? m.content : m.content.filter(c => c.type === "text").map(c => c.text).join("");
const canonical = (v) => Array.isArray(v) ? v.map(canonical) : v && typeof v === "object" ? Object.fromEntries(Object.entries(v).sort(([a], [b]) => a < b ? -1 : a > b ? 1 : 0).map(([k, v]) => [k, canonical(v)])) : v;
const hash = (v) => `sha256:${createHash("sha256").update(JSON.stringify(v).replace(/"(?:[^"\\]|\\.)*"|-?\d+/g, token => token[0] === '"' || !/[^0]0+$/.test(token) ? token : token.replace(/0+$/, "") + "e" + token.match(/0+$/)[0].length)).digest("hex")}`;
const dir = mkdtempSync(join(tmpdir(), "pi-session-bash-"));
const input = { commands: [
 { command: "custom", chunks: ["\u001b[31mhello\u001b[0m\r\n\u0000", "world\ufffa"], code: 7, id: "bash-1" },
 { command: "private", chunks: ["secret"], code: 0, exclude: true },
 { command: "empty", chunks: [], code: 0 },
 { command: "unicode", chunks: [[239,187,191,228], [184,150,231], [149,140]], code: 0 },
 { command: "cancel", chunks: ["partial"], code: 0, cancel: true },
 { command: "failure", chunks: ["before failure"], failure: true }
] };
try {
 const core = createFauxCore({ tokenSize: { min: 100, max: 100 } });
 core.setResponses(Array(3).fill(fauxAssistantMessage("reply", { timestamp: 2 })));
 const model = { ...core.getModel(), provider: "anthropic" };
 const runtime = await ModelRuntime.create({ credentials: AuthStorage.inMemory({ anthropic: { type: "api_key", key: "fixture" } }), modelsPath: null, allowModelNetwork: false });
 const manager = SessionManager.create(dir, dir);
 const { session } = await createAgentSession({ cwd: dir, agentDir: dir, model, modelRuntime: runtime, sessionManager: manager, settingsManager: SettingsManager.inMemory({ compaction: { enabled: false }, retry: { enabled: false }, shellCommandPrefix: "prefix" }), noTools: "all" });
 const events = [], results = [], requests = [];
 session.subscribe(e => { if (e.type === "bash_execution_update") events.push({id: e.id ?? null, delta:e.delta}); });
 for (const c of input.commands) {
  const chunks = [], calls = [];
  let result, failed = false;
  try { result = await session.executeBash(c.command, x => chunks.push(x), { id:c.id, excludeFromContext:c.exclude, operations:{exec:async(command,cwd,opts)=>{
   calls.push({command,cwd:cwd === dir, timeout:opts.timeout ?? null, running:session.isBashRunning});
   for (const chunk of c.chunks) opts.onData(Buffer.from(chunk));
   if (c.cancel) session.abortBash();
   if (c.failure || c.cancel) throw Error("failed");
   return {exitCode:c.code};
  }}}); } catch { failed = true; }
  results.push({chunks,calls,failed,result:result ? {output:result.output,exitCode:result.exitCode ?? null,cancelled:result.cancelled,truncated:result.truncated,fullOutputPath:result.fullOutputPath ?? null}:null,running:session.isBashRunning});
 }
 const summarize = messages => messages.map(m => m.role === "bashExecution" ? {role:m.role,command:m.command,output:m.output,exitCode:m.exitCode ?? null,cancelled:m.cancelled,truncated:m.truncated,exclude:!!m.excludeFromContext} : {role:m.role,text:text(m)});
 const history = summarize(session.messages);
 let deferred;
 runtime.streamSimple = async(m,context,opts)=>{
  requests.push(context.messages.map(m=>({role:m.role,text:text(m)})));
  if(requests.length===1){session.recordBashResult("during",{output:"queued",exitCode:0,cancelled:false,truncated:false});deferred={pending:session.hasPendingBashMessages,history:summarize(session.messages)};}
  return core.streamSimple(m,context,opts);
 };
 await session.prompt("first");
 const settled={pending:session.hasPendingBashMessages,history:summarize(session.messages)};
 await session.prompt("next");
 const reopened=summarize(SessionManager.open(manager.getSessionFile()).buildSessionContext().messages);
 session.dispose();
 const truncations=[];
 for (const [name, chunk, count] of [["lines", "line\n", 2501], ["bytes", "x".repeat(32769), 5], ["sanitized", "\u001b[31m\u0000\r".repeat(10001), 1]]) {
  const {session: large}=await createAgentSession({cwd:dir,agentDir:dir,model,modelRuntime:runtime,sessionManager:SessionManager.inMemory(dir),settingsManager:SettingsManager.inMemory(),noTools:"all"});
  const r=await large.executeBash(name,undefined,{operations:{exec:async(_,__,o)=>{for(let i=0;i<count;i++)o.onData(Buffer.from(chunk));return {exitCode:0};}}});
  large.dispose();
  let full;
  for(let i=0;i<100;i++){try{full=readFileSync(r.fullOutputPath,"utf8");if(full.length===(name==="sanitized"?0:chunk.length*count))break;}catch{}await new Promise(r=>setTimeout(r,5));}
  truncations.push({name,outputLength:r.output.length,outputStart:r.output.slice(0,5),outputEnd:r.output.slice(-5),truncated:r.truncated,hasFile:!!r.fullOutputPath,fullLength:full?.length,fullStart:full?.slice(0,5),fullEnd:full?.slice(-5)});
  if(r.fullOutputPath)rmSync(r.fullOutputPath,{force:true});
 }
 const outcome = {results,events,history,requests,deferred,settled,reopened,truncations};
 const c = { schema_version: "1.0.0", id: "go-sdk/codingagent/session-bash", catalog_id: "contract:codingagent/session-bash", surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
 const fixture = { schema_version: "1.0.0", deterministic:true, baseline_id:lock.baseline_id, baseline_commit:lock.upstream.commit, upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference}, case:c, observation:{outcome,side_effects:[]},input_hash:hash({...c,input:canonical(input)}),observation_hash:hash({outcome:canonical(outcome),side_effects:[]}),execution_method:"node --experimental-strip-types parity/oracle/session-bash.mjs <locked-pi-checkout>",platform:"any",environment:{oracle_entry:reference}};
 const out=join(root,"parity/oracle/fixtures/session-bash.json");
 if(process.argv.includes("--check")){if(JSON.stringify(fixture)!==JSON.stringify(JSON.parse(readFileSync(out))))throw Error("fixture drift");}
 else writeFileSync(out,JSON.stringify(fixture,null,2)+"\n");
 console.log(`verified ${out}`);
} finally { rmSync(dir,{recursive:true,force:true}); }
