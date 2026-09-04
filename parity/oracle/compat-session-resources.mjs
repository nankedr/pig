import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "compat-session-resources.json");

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

async function main() {
 const args = parseArgs(process.argv);
 const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
 const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
 if (head !== lock.upstream.commit) throw new Error("Pi checkout does not match Code Baseline");
 const status = execFileSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
 if (status !== "") throw new Error("Pi checkout has tracked changes");
 const reference = "packages/ai/src/compat.ts";
 const compat = await import(pathToFileURL(join(args.pi, reference)).href);
 const faux = await import(pathToFileURL(join(args.pi, "packages/ai/src/providers/faux.ts")).href);
 const resources = await import(pathToFileURL(join(args.pi, "packages/ai/src/session-resources.ts")).href);
 const input = { api: "compat-fixture", provider: "fixture", text: "reply", session: "one", projection: "Registry order, Faux event types and outcome text/reason, counters, cache isolation, and cleanup order/errors; times and provider-generated identifiers are not observed." };
 compat.resetApiProviders();
 const apis = () => compat.getApiProviders().map(p => p.api);
 const initial = apis();
 const core = faux.createFauxCore({ api: input.api, provider: input.provider });
 const custom = api => ({ api, stream: core.stream, streamSimple: core.streamSimple });
 compat.registerApiProvider(custom("first"), "old");
 compat.registerApiProvider(custom("second"), "second");
 compat.registerApiProvider(custom("first"), "new");
 compat.unregisterApiProviders("old");
 const overwritten = apis();
 compat.registerApiProvider(custom(initial[0]), "override");
 compat.registerBuiltInApiProviders();
 compat.unregisterApiProviders("override");
 const removed = apis();
 compat.registerBuiltInApiProviders();
 const restored = apis();
 compat.resetApiProviders();
 const reset = apis();
 const options = { api: input.api, provider: input.provider, tokenSize: { min: 100, max: 100 } };
 const old = compat.registerFauxProvider(options);
 const registered = compat.registerFauxProvider(options);
 old.unregister();
 const model = registered.getModel();
 const reply = faux.fauxAssistantMessage(input.text, { timestamp: 1 });
 const factoryOptions = [];
 const factory = (_input, options, state) => {
  factoryOptions.push({ session: options.sessionId, maxTokens: options.maxTokens, reasoning: options.reasoning, count: state.callCount });
  return reply;
 };
 registered.setResponses([reply]);
 registered.appendResponses([factory, reply, reply, reply]);
 const pendingBefore = registered.getPendingResponseCount();
 const context = { messages: [{ role: "user", content: "prompt", timestamp: 1 }] };
 const stream = compat.stream(model, context, { sessionId: input.session });
 const events = [];
 for await (const event of stream) events.push(event.type);
 const first = await stream.result();
 const second = await compat.completeSimple(model, context, { sessionId: input.session, maxTokens: 0, reasoning: "high" });
 const third = await compat.complete(model, context, { sessionId: "two" });
 const direct = faux.fauxProvider(options);
 direct.setResponses([reply]);
 const directResult = await direct.provider.stream(direct.getModel(), context, { sessionId: "two" }).result();
 const projected = result => ({ text: result.content.map(c => c.text ?? "").join(""), reason: result.stopReason, cacheRead: result.usage.cacheRead });
 const pendingAfter = registered.getPendingResponseCount();
 registered.unregister();
 registered.unregister();
 let missing = false;
 try { await compat.complete(model, context); } catch { missing = true; }
 const fresh = compat.registerFauxProvider(options);
 const freshState = { calls: fresh.state.callCount, pending: fresh.getPendingResponseCount() };
 fresh.unregister();
 const calls = [];
 const register = (name, fail = false) => resources.registerSessionResourceCleanup(id => { calls.push(`${name}:${id ?? "<all>"}`); if (fail) throw new Error(name); });
 const removeFirst = register("first");
 const removeFailure = register("failure", true);
 const removeLast = register("last");
 const failures = [];
 for (const id of [input.session, input.session]) {
  try { resources.cleanupSessionResources(id); } catch (error) { failures.push(error.errors.map(e => e.message)); }
 }
 removeFailure(); removeFailure();
 resources.cleanupSessionResources("two");
 resources.cleanupSessionResources();
 removeFirst(); removeLast();
 const observation = { outcome: { registry: { initial, overwritten, removed, restored, reset }, faux: { events, results: [first, second, third, directResult].map(projected), factoryOptions, pendingBefore, pendingAfter, calls: registered.state.callCount, oldCalls: old.state.callCount, missing, fresh: freshState }, cleanup: { calls, failures } }, side_effects: [] };
 await save("go-sdk/ai/compat-session-resources", "contract:ai/compat", input, observation, args.out);

 // Explicitly record the two execution differences requested by issue #65.
 const aliasCore = faux.createFauxCore({ api: "openai-completions", provider: input.provider });
 aliasCore.setResponses([faux.fauxAssistantMessage("registered", { timestamp: 1 })]);
 compat.registerApiProvider({ api: aliasCore.api, stream: aliasCore.stream, streamSimple: aliasCore.streamSimple }, "alias");
 const aliasModel = aliasCore.getModel();
 const transport = { apiKey: "fixture", maxRetries: 0, fetch: async () => new Response('data: {"choices":[{"delta":{"content":"adapter"},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n', { headers: { "content-type": "text/event-stream" } }) };
 const fromCompat = await compat.complete(aliasModel, { messages: [] }, transport);
 const fromAlias = await compat.streamOpenAICompletions(aliasModel, { messages: [] }, transport).result();
 const mutation = [];
 let removeTail, removeAdded;
 const removeHead = resources.registerSessionResourceCleanup(() => { mutation.push("first"); removeTail(); if (!removeAdded) removeAdded = resources.registerSessionResourceCleanup(() => mutation.push("added")); });
 removeTail = resources.registerSessionResourceCleanup(() => mutation.push("last"));
 resources.cleanupSessionResources();
 removeHead(); removeTail(); removeAdded();
 compat.resetApiProviders();
 const deviationInput = { projection: "Pi deprecated aliases bind the adapter and cleanup iterates a live Set; Pig issue #65 aliases follow the registry and cleanup snapshots registrations." };
 await save("go-sdk/ai/compat-session-resources-deviation", "contract:ai/compat", deviationInput, { outcome: { compat: fromCompat.content[0].text, alias: fromAlias.content[0].text, mutation }, side_effects: [] }, args.out.replace(".json", "-deviation.json"));

 async function save(id, catalogId, input, observation, output) {
  const caseValue = { schema_version: "1.0.0", id, catalog_id: catalogId, surface: "go-sdk", input, observe: ["outcome", "side_effects"] };
  const fixture = { schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit, upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference }, case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation), execution_method: "node --experimental-strip-types parity/oracle/compat-session-resources.mjs <locked-pi-checkout>", platform: "any", environment: { node: process.version, oracle_entry: reference } };
  if (args.check) {
   const committed = JSON.parse(readFileSync(output, "utf8"));
   fixture.environment.node = committed.environment.node;
   if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error(`committed fixture does not reproduce: ${output}`);
   console.log(`verified ${output}`);
  } else { writeFileSync(output, `${JSON.stringify(fixture, null, 2)}\n`); console.log(`wrote ${output}`); }
 }
}
main().catch(error => { console.error(error); process.exit(1); });
