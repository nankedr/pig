import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "message-handoff.json");

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

const source = { id: "source", api: "openai-responses", provider: "github-copilot", input: ["text"], reasoning: true };
const target = { ...source, id: "target", api: "anthropic-messages" };
const usage = { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } };
const assistant = (content, extra = {}) => ({ role: "assistant", api: source.api, provider: source.provider, model: source.id, content, usage, stopReason: "toolUse", timestamp: 1, ...extra });
const call = (id, extra = {}) => ({ type: "toolCall", id, name: "read", arguments: { path: "README.md" }, ...extra });
const result = (id) => ({ role: "toolResult", toolCallId: id, toolName: "read", content: [{ type: "text", text: "done" }], isError: false, timestamp: 2 });
const user = { role: "user", content: "continue", timestamp: 3 };

function declaration() {
 const blocks = [
  { type: "thinking", thinking: "plan", thinkingSignature: "reasoning_content" },
  { type: "thinking", thinking: "", thinkingSignature: "encrypted" },
  { type: "thinking", thinking: " \n", thinkingSignature: "signed-space" },
  { type: "thinking", thinking: "", redacted: true, thinkingSignature: "opaque" },
  { type: "thinking", thinking: "hidden", redacted: true },
  { type: "thinking", thinking: " \n" },
  { type: "thinking", thinking: "\uFEFF" },
  { type: "thinking", thinking: "\u0085" },
  { type: "text", text: "answer", textSignature: "text-id" },
  call("call|item", { thoughtSignature: "tool-signature", namespace: "files" }),
 ];
 const scenarios = [
  ...[["same", source], ["different-model", { ...source, id: "other" }], ["different-api", { ...source, api: "anthropic-messages" }], ["different-provider", { ...source, provider: "other" }]].map(([id, model]) => ({ id, model, normalize: "anthropic", messages: [assistant(blocks), result("call|item")] })),
  { id: "no-normalizer", model: target, normalize: "none", messages: [assistant([call("call|item")]), result("call|item")] },
  { id: "identity-normalizer", model: target, normalize: "identity", messages: [assistant([call("call|item")]), result("call|item")] },
  { id: "long-anthropic-id", model: target, normalize: "anthropic", messages: [assistant([call("call|" + "+/=".repeat(30))]), result("call|" + "+/=".repeat(30))] },
  ...[["end", []], ["user", [user]], ["assistant", [assistant([{ type: "text", text: "next" }], { stopReason: "stop" })]], ["error", [assistant([call("failed")], { stopReason: "error" })]], ["aborted", [assistant([call("failed")], { stopReason: "aborted" })]]].map(([id, tail]) => ({ id: `orphan-${id}`, model: target, normalize: "anthropic", messages: [assistant([call("call|done"), call("call|missing")]), result("call|done"), ...tail] })),
  { id: "all-results", model: target, normalize: "anthropic", messages: [assistant([call("call|one"), call("call|two")]), result("call|two"), result("call|one"), user] },
  { id: "result-after-interruption", model: target, normalize: "anthropic", messages: [assistant([call("call|one")]), user, result("call|one")] },
  { id: "failed-mapping", model: target, normalize: "anthropic", messages: [assistant([call("call|failed")], { stopReason: "error" }), result("call|failed"), assistant([call("cancelled")], { stopReason: "aborted" }), user] },
  { id: "nil-content", model: target, normalize: "none", messages: [{ ...user, content: null }, assistant(null, { stopReason: "stop" }), { ...result("missing"), content: null }, { role: "user", timestamp: 4 }, { role: "toolResult", toolCallId: "old", toolName: "read", isError: false, timestamp: 5 }] },
  { id: "empty-signatures", model: target, normalize: "none", messages: [assistant([call("empty", { thoughtSignature: "" }), call("null", { thoughtSignature: null }), { type: "text", text: "", textSignature: null }]), result("empty"), result("null")] },
 ];
 const ids = ["call|item", "call|" + "+/=".repeat(150), "call|" + "+/=".repeat(149) + "other", "call|💡item", "call|", "raw-id", "x".repeat(70), "é".repeat(50)];
 for (const provider of ["openai", "other"]) {
  scenarios.push({ id: `wire-${provider}`, wire: true, model: { ...source, id: "target", api: "openai-completions", provider }, normalize: "none", messages: [assistant([...ids.map((id) => call(id)), { type: "thinking", thinking: "\uFEFF" }, { type: "thinking", thinking: "\u0085" }]), ...ids.map(result), assistant([{ type: "thinking", thinking: "\uFEFF", thinkingSignature: "reasoning" }, { type: "thinking", thinking: "\u0085", thinkingSignature: "reasoning_content" }, { type: "text", text: "\uFEFF" }, { type: "text", text: "\u0085" }], { model: "target", api: "openai-completions", provider, stopReason: "stop" })] });
 }
 return { schema_version: "1.0.0", id: "go-sdk/ai/message-handoff", catalog_id: "contract:ai/message-handoff", surface: "go-sdk", input: { scenarios, projection: "Full transformed messages, callback metadata and Chat Completions wire. Only synthetic ToolResult timestamps become 0. Missing/null content maps to Go zero-value content at the typed SDK boundary; strict JSON codecs remain unchanged. Images remain M12." }, observe: ["outcome", "side_effects"] };
}

async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
 const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
 if (head !== lock.upstream.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.upstream.commit}`);
 const status = spawnSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
 if (status.status !== 0 || status.stdout !== "") throw new Error("locked Pi checkout has tracked changes");
 const reference = "packages/ai/src/api/transform-messages.ts";
 const { transformMessages } = await import(pathToFileURL(join(args.pi, reference)).href);
 const { convertMessages } = await import(pathToFileURL(join(args.pi, "packages/ai/src/api/openai-completions.ts")).href);
 const c = declaration();
 const outcome = c.input.scenarios.map((scenario) => {
  const calls = [];
  const normalize = scenario.normalize === "none" ? undefined : (id, model, source) => {
   calls.push({ id, target: model.id, source: source.model, api: source.api, provider: source.provider });
   return scenario.normalize === "identity" ? id : id.replace(/[^a-zA-Z0-9_-]/g, "_").slice(0, 64);
  };
  const before = JSON.stringify(scenario.messages);
  const messages = scenario.wire ? convertMessages(scenario.model, { messages: scenario.messages }, {}) : transformMessages(scenario.messages, scenario.model, normalize);
  if (JSON.stringify(scenario.messages) !== before) throw new Error(`source mutated: ${scenario.id}`);
  return { id: scenario.id, messages: messages.map((message) => message.role === "toolResult" && message.timestamp > 5 ? { ...message, timestamp: 0 } : message), calls };
 });
 const observation = { outcome, side_effects: [] };
 const fixture = {
  schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
  upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: `${reference}#transformMessages` },
  case: c, observation, input_hash: caseDigest(c), observation_hash: digest(observation),
  execution_method: "node --experimental-strip-types parity/oracle/message-handoff.mjs <locked-pi-checkout>", platform: "any",
  environment: { node: process.version, oracle_entry: reference },
 };
 if (args.check) {
  const committed = JSON.parse(readFileSync(args.out, "utf8"));
  fixture.environment.node = committed.environment.node;
  if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error(`committed fixture does not reproduce: ${args.out}`);
  console.log(`verified ${args.out}`);
 } else {
  writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
  console.log(`wrote ${args.out}`);
 }
}
main().catch((error) => { console.error(`oracle: ${error.message}`); process.exit(1); });
