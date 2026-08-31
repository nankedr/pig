import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-sse.json");
const encoder = new TextEncoder();
const catalogIDs = [
	"matrix:ai/openai-completions/sse/sse-data",
	"matrix:ai/openai-completions/sse/sse-data-multiline",
	"matrix:ai/openai-completions/sse/sse-done",
	"matrix:ai/openai-completions/sse/sse-done-post-events",
	"matrix:ai/openai-completions/sse/sse-done-prefix",
	"matrix:ai/openai-completions/sse/sse-frame-cr",
	"matrix:ai/openai-completions/sse/sse-frame-crlf",
	"matrix:ai/openai-completions/sse/sse-frame-lf",
	"matrix:ai/openai-completions/sse/sse-frame-trailing",
	"matrix:ai/openai-completions/sse/sse-frame-truncated-json",
	"matrix:ai/openai-completions/sse/sse-frame-truncated-valid-json",
	"matrix:ai/openai-completions/sse/sse-json-malformed",
	"matrix:ai/openai-completions/sse/sse-line-comment",
	"matrix:ai/openai-completions/sse/sse-line-newline",
	"matrix:ai/openai-completions/sse/sse-line-split-cr",
	"matrix:ai/openai-completions/sse/sse-line-unknown-field",
	"matrix:ai/openai-completions/sse/sse-response-empty-body",
	"matrix:ai/openai-completions/sse/sse-response-premature-eof",
];
const lines = [
	": heartbeat",
	"id: ignored",
	"retry: 1000",
	"unknown: ignored",
	'data: {"id":"boundary","choices":[',
	'data: {"delta":{"content":"你"}}]}',
	"",
	'data: {"choices":[{"delta":{"content":"好"},"finish_reason":"stop"}]}',
	"",
	"data: [DONE] suffix ignored",
	"",
	"data: {malformed after done",
	"",
];
const scenarios = {
	trailing: 'data: {"id":"trailing","choices":[{"delta":{"content":"尾"},"finish_reason":"stop"}]}',
	malformed: "data: {\n\n",
	malformed_non_object: "data: nope\n\n",
	truncated: 'data: {"choices":[',
	empty: "",
	premature_close: 'data: {"choices":[{"delta":{"content":"半"}}]}\n\n',
	blocked: 'data: {"id":"partial","choices":[{"delta":{"content":"你"}}]}\n\ndata: {"choices":[',
};
const model = {
	id: "sse-model",
	name: "SSE Model",
	api: "openai-completions",
	provider: "local-openai",
	baseUrl: "https://local.invalid/v1",
	reasoning: false,
	input: ["text"],
	cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
	contextWindow: 32000,
	maxTokens: 2048,
};
const context = { messages: [{ role: "user", content: "test", timestamp: 1 }] };

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

function bodyFromChunks(chunks) {
	return new ReadableStream({
		start(controller) {
			for (const chunk of chunks) controller.enqueue(typeof chunk === "string" ? encoder.encode(chunk) : chunk);
			controller.close();
		},
	});
}

async function observe(api, chunks, options = {}) {
	const fetch = async () => new Response(bodyFromChunks(chunks), { status: 200, headers: { "content-type": "text/event-stream" } });
	const stream = api.stream(model, context, { apiKey: "test-key", fetch, ...options });
	const events = [];
	for await (const event of stream) events.push(canonical(event));
	return canonical({ events, outcome: await stream.result() });
}

async function observeBlocked(api, mode) {
	const caller = new AbortController();
	const fetch = async (input, init = {}) => {
		const signal = init.signal ?? input.signal;
		return new Response(
			new ReadableStream({
				start(controller) {
					controller.enqueue(encoder.encode(scenarios.blocked));
					const fallback = setTimeout(() => controller.close(), 60);
					signal?.addEventListener(
						"abort",
						() => {
							clearTimeout(fallback);
							controller.error(new DOMException("Aborted", "AbortError"));
						},
						{ once: true },
					);
				},
			}),
			{ status: 200, headers: { "content-type": "text/event-stream" } },
		);
	};
	const options = { apiKey: "test-key", fetch };
	if (mode === "cancel") options.signal = caller.signal;
	else options.timeoutMs = 20;
	const stream = api.stream(model, context, options);
	const events = [];
	for await (const event of stream) {
		events.push(canonical(event));
		if (mode === "cancel" && event.type === "text_delta") caller.abort();
	}
	return canonical({ events, outcome: await stream.result() });
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
	const api = await import(pathToFileURL(join(args.piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	const sse = lines.join("\r\n");
	const whole = await observe(api, [sse]);
	const bytes = encoder.encode(sse);
	const oneByte = await observe(api, [...bytes].map((byte) => Uint8Array.of(byte)));
	const splitIndexes = [1, sse.indexOf("\r\n") + 1, bytes.indexOf(0xe4) + 1, bytes.length - 1];
	const splitEquivalent = [];
	for (const index of splitIndexes) {
		const observation = await observe(api, [bytes.slice(0, index), bytes.slice(index)]);
		splitEquivalent.push(stableJSON(observation) === stableJSON(whole));
	}
	const oldError = console.error;
	console.error = () => {};
	const malformed = await observe(api, [scenarios.malformed]);
	const malformedNonObject = await observe(api, [scenarios.malformed_non_object]);
	const truncated = await observe(api, [scenarios.truncated]);
	console.error = oldError;
	const input = canonical({ lines, scenarios, split_indexes: splitIndexes });
	const actual = canonical({
		line_endings: {
			lf: stableJSON(await observe(api, [lines.join("\n")])) === stableJSON(whole),
			crlf: true,
			cr: stableJSON(await observe(api, [lines.join("\r")])) === stableJSON(whole),
		},
		fragmentation: {
			one_byte: stableJSON(oneByte) === stableJSON(whole),
			split_indexes: splitIndexes,
			split_equivalent: splitEquivalent,
		},
		whole,
		trailing: await observe(api, [scenarios.trailing]),
		malformed,
		malformed_non_object: malformedNonObject,
		truncated,
		empty: await observe(api, [scenarios.empty]),
		premature_close: await observe(api, [scenarios.premature_close]),
		cancel: await observeBlocked(api, "cancel"),
		timeout: await observeBlocked(api, "timeout"),
	});
	const fixture = {
		schema_version: "1.0.0",
		id: "ai/openai-completions/m1-sse-boundaries",
		catalog_ids: catalogIDs,
		baseline_id: lock.baselineID,
		baseline_commit: lock.commit,
		deterministic: true,
		upstream: {
			module: "ai",
			repository: lock.repository,
			commit: lock.commit,
			reference: "packages/ai/package.json#openai@6.40.0",
		},
		input,
		input_sha256: sha256(input),
		actual,
		hash: { algorithm: "sha256", observations_sha256: sha256(actual) },
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-sse.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed SSE fixture does not reproduce");
		console.error("oracle: committed SSE fixture reproduces");
		return;
	}
	writeFileSync(args.out, JSON.stringify(fixture, null, 2) + "\n");
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
