import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "usage-cost-cache.json");

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

function usageSSE(usage, choiceUsage) {
	const chunk = { choices: choiceUsage ? [{ delta: {}, usage: choiceUsage }] : [], ...(usage ? { usage } : {}) };
	return [
		'data: {"choices":[{"delta":{"content":"done"},"finish_reason":"stop"}]}',
		`data: ${JSON.stringify(chunk)}`, "data: [DONE]", "",
	].join("\n\n");
}

function declaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/ai/usage-cost-cache",
		catalog_id: "contract:ai/faux-provider",
		surface: "go-sdk",
		input: {
			api: "faux:usage-parity", provider: "faux-usage-parity", model_id: "usage-model", response: "done",
			context: { systemPrompt: "rules", messages: [{ role: "user", content: "hello", timestamp: 1 }] },
			model_cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
			requests: [
				{ id: "first", entrypoint: "stream", sessionId: "session-a" },
				{ id: "hit", entrypoint: "complete", sessionId: "session-a" },
				{ id: "isolated", entrypoint: "stream", sessionId: "session-b" },
				{ id: "disabled", entrypoint: "stream", sessionId: "session-a", cacheRetention: "none" },
			],
			cost_cases: [
				{ id: "zero-base", model: { cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 } }, usage: { input: 1, output: 1, cacheRead: 1, cacheWrite: 1, totalTokens: 4, cost: {} } },
				{ id: "base", model: { cost: { input: 1, output: 2, cacheRead: 3, cacheWrite: 4 } }, usage: { input: 1000000, output: 500000, cacheRead: 1000000, cacheWrite: 1000000, totalTokens: 3500000, cost: {} } },
				{ id: "empty-tier", model: { cost: { input: 1, output: 2, cacheRead: 3, cacheWrite: 4, tiers: [] } }, usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, cacheWrite1h: 0, totalTokens: 0, cost: {} } },
				{ id: "zero-tier", model: { cost: { input: 1, output: 2, cacheRead: 3, cacheWrite: 4, tiers: [{ inputTokensAbove: 0, input: 0, output: 0, cacheRead: 0, cacheWrite: 0 }] } }, usage: { input: 1, output: 1, cacheRead: 1, cacheWrite: 1, totalTokens: 4, cost: {} } },
				{ id: "tier", model: { cost: { input: 1000000, output: 2000000, cacheRead: 3000000, cacheWrite: 4000000, tiers: [
					{ inputTokensAbove: 300, input: 31000000, output: 32000000, cacheRead: 33000000, cacheWrite: 34000000 },
					{ inputTokensAbove: 100, input: 11000000, output: 12000000, cacheRead: 13000000, cacheWrite: 14000000 },
					{ inputTokensAbove: 200, input: 21000000, output: 22000000, cacheRead: 23000000, cacheWrite: 24000000 },
				] } }, usage: { input: 100, output: 1, cacheRead: 100, cacheWrite: 100, totalTokens: 301, cost: {} } },
				{ id: "one-hour", model: { cost: { input: 1, output: 0, cacheRead: 0, cacheWrite: 3, tiers: [{ inputTokensAbove: 0, input: 2, output: 0, cacheRead: 0, cacheWrite: 4 }] } }, usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 1000000, cacheWrite1h: 250000, totalTokens: 1000000, cost: {} } },
			],
			openai: {
				model: {
					id: "usage-model", name: "Usage Model", api: "openai-completions", provider: "local-openai",
					baseUrl: "https://local.invalid/v1", reasoning: true, input: ["text"], contextWindow: 32000, maxTokens: 2048,
					cost: { input: 1000000, output: 2000000, cacheRead: 3000000, cacheWrite: 4000000 },
				},
				cases: [
					{ id: "detailed", sse: usageSSE(
						{ prompt_tokens: 10, completion_tokens: 5, prompt_cache_hit_tokens: 99, prompt_tokens_details: { cached_tokens: 3, cache_write_tokens: 1 }, completion_tokens_details: { reasoning_tokens: 2 } },
						{ prompt_tokens: 999, completion_tokens: 999, completion_tokens_details: { reasoning_tokens: 999 } },
					) },
					{ id: "legacy", sse: usageSSE({ prompt_tokens: 8, completion_tokens: 1, prompt_cache_hit_tokens: 2, completion_tokens_details: { reasoning_tokens: 0 } }) },
					{ id: "legacy-zero", sse: usageSSE({ prompt_tokens: 0, completion_tokens: 0, prompt_cache_hit_tokens: 0, completion_tokens_details: { reasoning_tokens: 0 } }) },
					{ id: "zero", sse: usageSSE({ prompt_tokens: 0, completion_tokens: 0, prompt_cache_hit_tokens: 0, prompt_tokens_details: { cached_tokens: 0, cache_write_tokens: 0 }, completion_tokens_details: { reasoning_tokens: 0 } }, { prompt_tokens: 0, completion_tokens: 0 }) },
					{ id: "choice", sse: usageSSE(undefined, { prompt_tokens: 6, completion_tokens: 2, prompt_cache_hit_tokens: 1, completion_tokens_details: { reasoning_tokens: 1 } }) },
					{ id: "choice-zero", sse: usageSSE(undefined, { prompt_tokens: 0, completion_tokens: 0, prompt_cache_hit_tokens: 0, completion_tokens_details: { reasoning_tokens: 0 } }) },
					{ id: "missing-tokens", sse: usageSSE({ completion_tokens_details: { reasoning_tokens: 0 } }) },
					{ id: "clamp", sse: usageSSE({ prompt_tokens: 1, completion_tokens: 0, prompt_tokens_details: { cached_tokens: 3, cache_write_tokens: 1 }, completion_tokens_details: { reasoning_tokens: 0 } }) },
				],
			},
		},
		observe: ["outcome", "side_effects"],
	};
}

async function streamResult(stream) {
	let terminal;
	for await (const event of stream) if (event.type === "done") terminal = event.message.usage;
	const first = (await stream.result()).usage;
	const repeated = (await stream.result()).usage;
	return { usage: canonical(first), terminal_equal: JSON.stringify(terminal) === JSON.stringify(first), repeat_equal: JSON.stringify(repeated) === JSON.stringify(first) };
}

async function capture(pi, input) {
	const providerPath = join(pi, "packages", "ai", "src", "providers", "faux.ts");
	const modelsPath = join(pi, "packages", "ai", "src", "models.ts");
	const openAIPath = join(pi, "packages", "ai", "src", "api", "openai-completions.ts");
	const [fauxAPI, modelsAPI, openAIAPI] = await Promise.all([
		import(pathToFileURL(providerPath).href), import(pathToFileURL(modelsPath).href), import(pathToFileURL(openAIPath).href),
	]);
	const faux = fauxAPI.fauxProvider({ api: input.api, provider: input.provider, models: [{ id: input.model_id, cost: input.model_cost }] });
	const models = modelsAPI.createModels();
	models.setProvider(faux.provider);
	const model = faux.getModel(input.model_id);
	faux.setResponses(input.requests.map(() => fauxAPI.fauxAssistantMessage(input.response, { timestamp: 1 })));
	const requests = {};
	for (const request of input.requests) {
		const options = { sessionId: request.sessionId, ...(request.cacheRetention ? { cacheRetention: request.cacheRetention } : {}) };
		if (request.entrypoint === "complete") requests[request.id] = { usage: canonical((await models.complete(model, input.context, options)).usage) };
		else requests[request.id] = await streamResult(faux.provider.stream(model, input.context, options));
	}
	const costs = {};
	for (const item of input.cost_cases) {
		const usage = structuredClone(item.usage);
		const cost = modelsAPI.calculateCost(item.model, usage);
		costs[item.id] = { usage: canonical(usage), cost: canonical(cost) };
	}
	const openai = {};
	for (const item of input.openai.cases) {
		const fetch = async () => new Response(item.sse, { status: 200, headers: { "content-type": "text/event-stream" } });
		openai[item.id] = await streamResult(openAIAPI.stream(input.openai.model, { messages: [] }, { apiKey: "test-key", fetch }));
	}
	return { outcome: { requests, costs, openai }, side_effects: [] };
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
	const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.upstream.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.upstream.commit}`);
	const status = spawnSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
	if (status.status !== 0 || status.stdout !== "") throw new Error("locked Pi checkout has tracked changes");
	const caseValue = declaration();
	const observation = await capture(args.pi, caseValue.input);
	const fixture = {
		schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
		upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: "packages/ai/src/providers/faux.ts#withUsageEstimate + packages/ai/src/models.ts#calculateCost + packages/ai/src/api/openai-completions.ts#parseChunkUsage" },
		case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation),
		execution_method: "node --experimental-strip-types parity/oracle/usage-cost-cache.mjs <locked-pi-checkout>", platform: "any",
		environment: { node: process.version, oracle_entry: "packages/ai/src/providers/faux.ts + packages/ai/src/models.ts + packages/ai/src/api/openai-completions.ts" },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		fixture.environment.node = committed.environment.node;
		if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error("committed usage fixture does not reproduce");
		console.log(`verified ${args.out}`);
		return;
	}
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.log(`wrote ${args.out}`);
}

main().catch((error) => { console.error(`oracle: ${error.message}`); process.exit(1); });
