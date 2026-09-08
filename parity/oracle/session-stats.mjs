import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "session-stats.json");

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
 const sourceFixture = "parity/oracle/fixtures/session-interop.json";
 const interop = JSON.parse(readFileSync(join(root, sourceFixture), "utf8"));
 const base = interop.case.input.sessions.find(s => s.name === "pi-writer-v3").content.trim().split("\n").map(JSON.parse);
 const usage = {input:10, output:20, cacheRead:30, cacheWrite:40, totalTokens:999, cost:{input:0.1, output:0.2, cacheRead:0.3, cacheWrite:0.4, total:2}};
 const assistant = (content, overrides = {}) => ({role:"assistant", content, api:"openai-completions", provider:"deepseek", model:"deepseek-v4-flash", usage, stopReason:"stop", timestamp:2, ...overrides});
 const text = text => ({type:"text", text});
 const user = content => ({role:"user", content, timestamp:1});
 const append = (records, message) => [...records, {type:"message", id:`added-${records.length}`, parentId:records.at(-1)?.id ?? null, timestamp:"2025-01-01T00:00:00.000Z", message}];
 const scenario = (name, records, contextWindow = 32000) => ({name, records, contextWindow});
 const empty = [base[0]];
 const simple = append(append(empty, user("hi")), assistant([text(" \ufeffanswer"), {type:"thinking", thinking:"hidden"}, text(" end \ufeff")]));
 const paid = structuredClone(base);
 for (const e of paid) {
  if (e.type === "compaction" || e.type === "branch_summary") e.usage = usage;
  if (e.type === "message" && e.message.role === "toolResult") e.message.usage = {...usage, reasoning:0, cacheWrite1h:0};
  if (e.type === "message" && e.message.role === "assistant") e.message = assistant([{type:"toolCall", id:"call-1", name:"read", arguments:{path:"a"}}, text("old answer")]);
 }
 const zeroUsage = {...usage, input:0, output:0, cacheRead:0, cacheWrite:0, totalTokens:0};
 const post = append(paid, assistant([text("new answer")]));
 const bad = append(append(post, user("continue 😀")), assistant([text("partial")], {stopReason:"error"}));
 const aborted = append(bad, assistant([], {stopReason:"aborted"}));
 const sibling = append(paid, assistant([text("sibling")]));
 sibling.at(-1).parentId = "entry-3";
 const scenarios = [
  scenario("empty", empty), scenario("no-context-window", simple, 0), scenario("simple-native-total", simple),
  scenario("existing-v3", base), scenario("paid-compaction-unknown", paid), scenario("post-compaction", post),
  scenario("post-compaction-zero", append(paid, assistant([text("zero")], {usage:zeroUsage}))),
  scenario("post-compaction-error", append(paid, assistant([text("error")], {stopReason:"error"}))),
  scenario("error-trailing", bad), scenario("empty-abort-skipped", aborted), scenario("branch-without-compaction", sibling),
  scenario("tool-only-last", append(simple, assistant([{type:"toolCall", id:"x", name:"read", arguments:{}}]))),
  scenario("empty-error-not-skipped", append(simple, assistant([], {stopReason:"error"}))),
  scenario("zero-fallback", append(simple, assistant([text("zero")], {usage:{...usage, totalTokens:0}}))),
  scenario("all-estimated", append(append(empty, user([{type:"text", text:"😀x"}, {type:"image", data:"AA==", mimeType:"image/png"}])), assistant([text("tail")], {usage:zeroUsage}))),
  scenario("whitespace-last", append(simple, assistant([text(" \ufeff\n")]))),
  scenario("non-js-whitespace", append(simple, assistant([text("\u0085")]))),
 ];
 const input = {sourceFixture, sourceHash:`sha256:${createHash("sha256").update(readFileSync(join(root,sourceFixture))).digest("hex")}`, scenarios};
 const dir = mkdtempSync(join(tmpdir(), "pi-stats-"));
 let runs;
 try {
  for (const path of ["src/core/sdk.ts", "dist/core/sdk.js"]) {
   const {createAgentSession} = await load("packages/coding-agent/" + path);
   const runtime = await ModelRuntime.create({credentials:AuthStorage.inMemory(), modelsPath:null, allowModelNetwork:false});
   runtime.registerProvider("deepseek", {api:"openai-completions", apiKey:"fixture", baseUrl:"https://invalid.test", models:[{id:"deepseek-v4-flash", name:"fixture", reasoning:false, input:["text"], cost:{input:0,output:0,cacheRead:0,cacheWrite:0}, contextWindow:32000, maxTokens:2048}]});
   const captured = [];
   for (const s of scenarios) {
    const file = join(dir, s.name+".jsonl");
    writeFileSync(file, s.records.map(e=>JSON.stringify(e)).join("\n")+"\n");
    const manager = SessionManager.open(file, dir);
    const settings = SettingsManager.inMemory({compaction:{enabled:false},retry:{enabled:false}});
    const model = {...runtime.getModel("deepseek", "deepseek-v4-flash"), contextWindow:s.contextWindow};
    const {session} = await createAgentSession({cwd:dir, agentDir:dir, model, modelRuntime:runtime, sessionManager:manager, settingsManager:settings, thinkingLevel:"off", tools:[]});
    const before = readFileSync(file,"utf8");
    const entries = JSON.stringify(manager.getEntries()), messages = JSON.stringify(session.messages);
    const stats = session.getSessionStats();
    const context = session.getContextUsage() ?? null, last = session.getLastAssistantText() ?? null;
    captured.push({name:s.name, stats:{...stats, sessionFile:stats.sessionFile === file, contextUsage:stats.contextUsage ?? null}, context, last, roles:session.messages.map(m=>m.role)});
    if (before !== readFileSync(file,"utf8") || entries !== JSON.stringify(manager.getEntries()) || messages !== JSON.stringify(session.messages)) throw new Error("query mutation");
    session.dispose();
   }
   if (runs && JSON.stringify(canonical(runs)) !== JSON.stringify(canonical(captured))) throw new Error("source/dist mismatch");
   runs = captured;
  }
  const reference = "packages/coding-agent/src/core/sdk.ts#createAgentSession + agent-session.ts#getSessionStats/getContextUsage/getLastAssistantText";
  const observation = {outcome:runs, side_effects:[]};
  const c = {schema_version:"1.0.0",id:"go-sdk/codingagent/session-stats",catalog_id:"contract:codingagent/session-stats",surface:"go-sdk",input,observe:["outcome","side_effects"]};
  const fixture = {schema_version:"1.0.0",deterministic:true,baseline_id:lock.baseline_id,baseline_commit:lock.upstream.commit,upstream:{repository:lock.upstream.repository,commit:lock.upstream.commit,reference},case:c,observation,input_hash:caseDigest(c),observation_hash:observationDigest(observation),execution_method:"node --experimental-strip-types parity/oracle/session-stats.mjs <locked-pi-checkout>",platform:"any",environment:{node:process.version,oracle_entry:reference}};
  if (args.check) {
   const committed = JSON.parse(readFileSync(args.out,"utf8"));
   fixture.environment.node = committed.environment.node;
   if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error("fixture drift");
   console.log(`verified ${args.out}`);
  } else {
   writeFileSync(args.out,JSON.stringify(fixture,null,2)+"\n");
   console.log(`wrote ${args.out}`);
  }
 } finally { rmSync(dir,{recursive:true,force:true}); }
}
main().catch(e=>{console.error(e);process.exit(1)});
