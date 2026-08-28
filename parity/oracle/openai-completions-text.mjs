import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-text.json");
const catalogIDs = [
	"matrix:ai/openai-completions/entrypoint/stream",
	"matrix:ai/openai-completions/event/assistant-message-event-start",
	"matrix:ai/openai-completions/event/assistant-message-event-text-delta",
	"matrix:ai/openai-completions/event/assistant-message-event-done",
	"matrix:ai/openai-completions/usage/usage-cache-read-detailed",
];

const sse = [
	'data: {"id":"chatcmpl-44","model":"reply-model","choices":[{"index":0,"delta":{"content":"你"}}]}',
	'data: {"id":"chatcmpl-44","choices":[{"index":0,"delta":{"content":"好"},"finish_reason":"stop"}]}',
	'data: {"id":"chatcmpl-44","choices":[],"usage":{"prompt_tokens":10,"completion_tokens":2,"prompt_cache_hit_tokens":99,"prompt_tokens_details":{"cached_tokens":3,"cache_write_tokens":1}}}',
	"data: [DONE]",
	"",
].join("\n\n");

const model = {
	id: "text-model",
	name: "Text Model",
	api: "openai-completions",
	provider: "local-openai",
	baseUrl: "https://local.invalid/v1",
	reasoning: false,
	input: ["text"],
	cost: { input: 1, output: 2, cacheRead: 0.5, cacheWrite: 1.5 },
	contextWindow: 32000,
	maxTokens: 2048,
};
const context = {
	systemPrompt: "简洁回答",
	messages: [
		{ role: "user", content: "你好", timestamp: 1 },
		{
			role: "assistant",
			content: [{ type: "text", text: "此前回答" }],
			api: model.api,
			provider: model.provider,
			model: model.id,
			usage: {
				input: 0,
				output: 0,
				cacheRead: 0,
				cacheWrite: 0,
				totalTokens: 0,
				cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 },
			},
			stopReason: "stop",
			timestamp: 2,
		},
		{ role: "user", content: [{ type: "text", text: "继续" }], timestamp: 3 },
	],
};

function parseArgs(argv) {
	const parsed = { piRoot: defaultPiRoot, out: defaultOutput, check: false };
	const rest = argv.slice(2);
	for (let i = 0; i < rest.length; i++) {
		if (rest[i] === "--check") parsed.check = true;
		else if (rest[i] === "--out") parsed.out = rest[++i];
		else if (parsed.piRoot === defaultPiRoot) parsed.piRoot = rest[i];
		else throw new Error(`unexpected argument: ${rest[i]}`);
	}
	return parsed;
}

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(
			Object.entries(value)
				.filter(([key]) => key !== "timestamp")
				.sort(([left], [right]) => left.localeCompare(right))
				.map(([key, child]) => [key, canonical(child)]),
		);
	}
	return value;
}

function stableJSON(value) {
	return JSON.stringify(canonical(value));
}

function sha256(value) {
	return createHash("sha256").update(typeof value === "string" ? value : stableJSON(value)).digest("hex");
}

function readLock() {
	const lock = JSON.parse(readFileSync(join(repoRoot, "parity", "baseline", "upstream.lock.json"), "utf8"));
	return {
		baselineID: lock.baseline_id,
		commit: lock.source_verification.expected_commit,
		repository: lock.upstream.repository,
	};
}

function normalizedHeaders(headers) {
	return Object.fromEntries(
		[...new Headers(headers).entries()]
			.filter(([name]) => name !== "content-length" && name !== "user-agent" && !name.startsWith("x-stainless-"))
			.sort(([left], [right]) => left.localeCompare(right)),
	);
}

async function capture(piRoot) {
	const api = await import(pathToFileURL(join(piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	let request;
	const fetch = async (input, init = {}) => {
		request = canonical({
			url: String(input),
			method: init.method,
			headers: normalizedHeaders(init.headers),
			body: JSON.parse(init.body),
		});
		return new Response(sse, { status: 200, headers: { "content-type": "text/event-stream" } });
	};
	const stream = api.stream(model, context, { apiKey: "test-key", fetch, temperature: 0, maxTokens: 512 });
	const events = [];
	for await (const event of stream) events.push(canonical(event));
	const outcome = canonical(await stream.result());
	const types = events.map((event) => event.type);
	const expectedTypes = ["start", "text_start", "text_delta", "text_delta", "text_end", "done"];
	if (stableJSON(types) !== stableJSON(expectedTypes)) throw new Error(`event types ${stableJSON(types)}`);
	if (request.url !== "https://local.invalid/v1/chat/completions" || request.method !== "POST") throw new Error("request target mismatch");
	if (request.headers.authorization !== "Bearer test-key") throw new Error("authorization mismatch");
	if (outcome.content?.[0]?.text !== "你好" || outcome.stopReason !== "stop") throw new Error("terminal text mismatch");
	const usage = outcome.usage;
	if (usage.input !== 6 || usage.output !== 2 || usage.cacheRead !== 3 || usage.cacheWrite !== 1 || usage.totalTokens !== 12) {
		throw new Error(`usage mismatch: ${stableJSON(usage)}`);
	}
	return { request, events, outcome };
}

function fixtureFacts(fixture) {
	return stableJSON({
		schema_version: fixture.schema_version,
		id: fixture.id,
		catalog_ids: fixture.catalog_ids,
		baseline_id: fixture.baseline_id,
		baseline_commit: fixture.baseline_commit,
		deterministic: fixture.deterministic,
		upstream: fixture.upstream,
		input: fixture.input,
		input_sha256: fixture.input_sha256,
		actual: fixture.actual,
		hash: fixture.hash,
		exec_method: fixture.exec_method,
	});
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = readLock();
	const head = execFileSync("git", ["-C", args.piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.commit}`);
	const input = canonical({ model, context, options: { apiKey: "test-key", temperature: 0, maxTokens: 512 }, sse });
	const actual = await capture(args.piRoot);
	const fixture = {
		schema_version: "1.0.0",
		id: "ai/openai-completions/m1-text",
		catalog_ids: catalogIDs,
		baseline_id: lock.baselineID,
		baseline_commit: lock.commit,
		deterministic: true,
		upstream: {
			module: "ai",
			repository: lock.repository,
			commit: lock.commit,
			reference: "packages/ai/src/api/openai-completions.ts#stream",
		},
		input,
		input_sha256: sha256(input),
		actual,
		hash: { algorithm: "sha256", observations_sha256: sha256(actual) },
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-text.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed text fixture does not reproduce");
		console.error("oracle: committed text fixture reproduces");
		return;
	}
	writeFileSync(args.out, JSON.stringify(fixture, null, 2) + "\n");
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
