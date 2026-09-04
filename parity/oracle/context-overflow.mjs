import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "context-overflow.json");

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
	if (typeof value === "string" || typeof value === "boolean") return JSON.stringify(value).replace(/\u2028/g, "\\u2028").replace(/\u2029/g, "\\u2029");
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

function usage(input = 0, output = 0, cacheRead = 0, cacheWrite = 0, totalTokens = 0) {
	return { input, output, cacheRead, cacheWrite, totalTokens, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } };
}

function assistant(timestamp, tokens, extra = {}) {
	return { role: "assistant", content: [{ type: "text", text: "kept" }], api: "faux", provider: "faux", model: "faux-1", usage: usage(tokens), stopReason: "stop", timestamp, ...extra };
}

function user(timestamp, content) { return { role: "user", content, timestamp }; }
function tool(name) { return { name, description: name, parameters: { type: "object", properties: {} } }; }
function added(timestamp, names) {
	return { role: "toolResult", toolCallId: "call-1", toolName: "discover", content: [], addedToolNames: names, isError: false, timestamp };
}

function declaration() {
	const estimates = [
		{ id: "empty", context: { messages: [] } },
		{ id: "all-content-no-usage", context: { systemPrompt: "system😀", tools: [tool("read")], messages: [
			user(1, [{ type: "text", text: "👋你好abc" }, { type: "image", data: "AA==", mimeType: "image/png" }]),
			assistant(2, 0, { content: [{ type: "thinking", thinking: "plan😀", thinkingSignature: "excluded" }, { type: "text", text: "ok" }, { type: "toolCall", id: "call-1", name: "read", arguments: { path: "<a>&😀", n: 1e-7 } }] }),
			{ ...added(3, []), content: [{ type: "text", text: "done!" }, { type: "image", data: "AA==", mimeType: "image/png" }] },
		] } },
		{ id: "total-precedence", context: { systemPrompt: "already counted", tools: [tool("read")], messages: [assistant(1, 1, { usage: usage(10, 20, 30, 40, 500) }), user(2, "tail")] } },
		{ id: "usage-buckets", context: { messages: [assistant(1, 1, { usage: usage(10, 20, 30, 40) })] } },
		{ id: "failed-usage-skipped", context: { messages: [assistant(1, 500), assistant(2, 900, { stopReason: "error" }), assistant(3, 1000, { stopReason: "aborted" }), assistant(4, 0), user(5, "tail")] } },
		{ id: "equal-timestamp", context: { messages: [user(1, "prompt"), assistant(1, 500)] } },
		{ id: "only-new-tool-definitions", context: { systemPrompt: "already counted", tools: [tool("old"), tool("new"), tool("unused")], messages: [added(1, ["old"]), assistant(2, 500), added(3, ["new", "new", "missing"]), added(4, ["new"])] } },
		{ id: "next-usage-covers-tools", context: { tools: [tool("old"), tool("new")], messages: [added(1, ["old"]), assistant(2, 500), added(3, ["new"]), assistant(4, 800)] } },
		{ id: "null-empty-added-names", context: { tools: [tool("unused")], messages: [assistant(1, 500), added(2, null), added(3, [])] } },
		{ id: "schema-json-semantics", context: { tools: [{ name: "a<&>", description: "line\u2028para\u2029 and literal \\u2028", parameters: { type: "object", properties: { x: { type: "number", minimum: 1e-7, maximum: 1e21 } } }, constrainedSampling: { type: "json_schema", strict: "require" } }], messages: [] } },
		{ id: "stale-prefix", context: { systemPrompt: "system", messages: [user(200, "summary"), assistant(100, 9500), user(300, "x".repeat(4000))] } },
		{ id: "fresh-after-summary", context: { messages: [user(200, "summary"), assistant(100, 9500), user(300, "new prompt"), assistant(400, 2000), user(500, "tail")] } },
	];
	const errors = [
		["anthropic", "prompt is too long: 213462 tokens > 200000 maximum"],
		["anthropic-bytes", '413 {"error":{"type":"request_too_large","message":"Request exceeds the maximum size"}}'],
		["bedrock", "Input is too long for requested model"],
		["openai", "Your input exceeds the context window of this model"],
		["litellm", "Error: 503 litellm.ServiceUnavailableError: Requested token count exceeds the model's maximum context length of 131072 tokens."],
		["compatible", "Input length (265330) exceeds model's maximum context length (262144)."],
		["gemini", "The input token count (1196265) exceeds the maximum number of tokens allowed (1048575)"],
		["xai", "This model's maximum prompt length is 131072 but the request contains 537812 tokens"],
		["groq", "Please reduce the length of the messages or completion"],
		["openrouter", "This endpoint's maximum context length is 100 tokens. However, you requested about 200 tokens"],
		["poolside", "Input length 131393 exceeds the maximum allowed input length of 131040 tokens."],
		["together", "The input (516368 tokens) is longer than the model's context length (262144 tokens)."],
		["copilot", "prompt token count of 200 exceeds the limit of 100"],
		["llama-cpp", "the request exceeds the available context size, try increasing it"],
		["lm-studio", "tokens to keep from the initial prompt is greater than the context length"],
		["minimax", "invalid params, context window exceeds limit"],
		["kimi", "Your request exceeded model token limit: 100 (requested: 200)"],
		["mistral", "Prompt contains 200 tokens ... too large for model with 100 maximum context length"],
		["ds4", "Prompt has 5,958,968 tokens, but the configured context size is 256,000 tokens"],
		["zai", "model_context_window_exceeded"],
		["ollama", "400 prompt too long; exceeded max context length by 100918 tokens"],
		["qwen", "Range of input length should be [1, 100]"],
		["generic-code", "context_length_exceeded"],
		["generic-spaces", "CONTEXT LENGTH EXCEEDED"],
		["generic-many", "too many tokens"],
		["generic-limit", "token limit exceeded"],
		["cerebras-400", "400 status code (no body)"],
		["cerebras-413", "413 (no body)"],
		["throttling", "Throttling error: Too many tokens, please wait before trying again."],
		["unavailable", "Service unavailable: too many tokens"],
		["rate-limit", "Rate limit exceeded: token limit exceeded"],
		["requests", "Too many requests: too many tokens"],
		["crashed", "500 model runner crashed unexpectedly"],
		["bodyless-500", "500 status code (no body)"],
		["bodyless-unanchored", "wrapped 400 status code (no body)"],
	].map(([id, errorMessage]) => ({ id, message: assistant(1, 0, { stopReason: "error", errorMessage }) }));
	const overflows = [
		...errors,
		...["stop", "length", "toolUse", "aborted", "error"].flatMap((stopReason) => [98, 99, 100, 101].map((input) => ({
			id: `${stopReason}-${input}`, contextWindow: 100, desiredMaxOutput: 16,
			message: assistant(1, 0, { stopReason, usage: usage(1, 0, input - 1, 9999, 10000) }),
		}))),
		...[undefined, 0, 100].map((contextWindow) => ({ id: `window-${contextWindow ?? "absent"}`, ...(contextWindow === undefined ? {} : { contextWindow }), desiredMaxOutput: 16, message: assistant(1, 101) })),
		...[0, 15, 16, 17].flatMap((output) => [0, 16, 128000].map((desiredMaxOutput) => ({
			id: `length-output-${output}-desired-${desiredMaxOutput}`, contextWindow: 100, desiredMaxOutput,
			message: assistant(1, 0, { stopReason: "length", usage: usage(100, output) }),
		}))),
		{ id: "ignore-error-text-on-success", message: assistant(1, 0, { errorMessage: "too many tokens" }) },
		{ id: "cache-write-is-not-input", contextWindow: 100, message: assistant(1, 0, { usage: usage(1, 999, 0, 1000) }) },
	];
	return {
		schema_version: "1.0.0", id: "go-sdk/ai/context-overflow", catalog_id: "contract:ai/context-overflow", surface: "go-sdk",
		input: { estimates, overflows }, observe: ["outcome", "side_effects"],
	};
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
	const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.upstream.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.upstream.commit}`);
	const status = spawnSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
	if (status.status !== 0 || status.stdout !== "") throw new Error("locked Pi checkout has tracked changes");
	const estimateRef = "packages/ai/src/utils/estimate.ts";
	const overflowRef = "packages/ai/src/utils/overflow.ts";
	const { estimateContextTokens } = await import(pathToFileURL(join(args.pi, estimateRef)).href);
	const { isContextOverflow, isRecoverableLength, getOverflowPatterns } = await import(pathToFileURL(join(args.pi, overflowRef)).href);
	const caseValue = declaration();
	const outcome = {
		estimates: caseValue.input.estimates.map(({ id, context }) => ({ id, ...estimateContextTokens(context) })),
		overflows: caseValue.input.overflows.map(({ id, message, contextWindow, desiredMaxOutput }) => ({
			id, overflow: isContextOverflow(message, contextWindow), recoverable: isRecoverableLength(message, desiredMaxOutput ?? 0),
			matches: getOverflowPatterns().map((pattern, index) => pattern.test(message.errorMessage ?? "") ? index : -1).filter((index) => index >= 0),
		})),
	};
	const observation = { outcome, side_effects: [] };
	const fixture = {
		schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
		upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: `${estimateRef};${overflowRef}` },
		case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation),
		execution_method: "node --experimental-strip-types parity/oracle/context-overflow.mjs <locked-pi-checkout>", platform: "any",
		environment: { node: process.version, oracle_entry: `${estimateRef};${overflowRef}` },
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
