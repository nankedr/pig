import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-tools.json");
const readArguments = String.raw`{"path":"docs/\u76ee\u5f55/README.md","query":"line\n\"quoted\"\\tail","flags":[true,false,null],"nested":{"count":12.5},"native":"原生😀"}`;
const writeArguments = String.raw`{"path":"out.txt","content":"\u4f60\u597d"}`;
const catalogIDs = [
	"matrix:ai/openai-completions/content/assistant-message-content-tool-call",
	"matrix:ai/openai-completions/delta/delta-tool-calls",
	"matrix:ai/openai-completions/delta/delta-tool-calls-cleanup-success",
	"matrix:ai/openai-completions/delta/delta-tool-calls-coalesce-id",
	"matrix:ai/openai-completions/delta/delta-tool-calls-coalesce-index",
	"matrix:ai/openai-completions/delta/delta-tool-calls-coalesce-interleaved",
	"matrix:ai/openai-completions/delta/delta-tool-calls-coalesce-late-index",
	"matrix:ai/openai-completions/delta/delta-tool-calls-coalesce-mutable-id",
	"matrix:ai/openai-completions/delta/delta-tool-calls-finalize-function",
	"matrix:ai/openai-completions/delta/delta-tool-calls-items-function",
	"matrix:ai/openai-completions/delta/delta-tool-calls-items-function-arguments",
	"matrix:ai/openai-completions/delta/delta-tool-calls-items-function-name",
	"matrix:ai/openai-completions/delta/delta-tool-calls-items-id",
	"matrix:ai/openai-completions/delta/delta-tool-calls-items-index",
	"matrix:ai/openai-completions/delta/partial-json-complete",
	"matrix:ai/openai-completions/delta/partial-json-empty",
	"matrix:ai/openai-completions/delta/partial-json-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-delta",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-delta-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-delta-delta",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-delta-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-delta-type",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-end",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-end-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-end-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-end-tool-call",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-end-type",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-start",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-start-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-start-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-toolcall-start-type",
	"matrix:ai/openai-completions/error/finish-tool-calls",
	"matrix:ai/openai-completions/request/request-tool-choice",
	"matrix:ai/openai-completions/request/request-tools",
	"matrix:ai/openai-completions/tool/tool-call-arguments",
	"matrix:ai/openai-completions/tool/tool-call-id",
	"matrix:ai/openai-completions/tool/tool-call-name",
	"matrix:ai/openai-completions/tool/tool-call-type",
];
const model = {
	id: "tool-model",
	name: "Tool Model",
	api: "openai-completions",
	provider: "local-openai",
	baseUrl: "https://local.invalid/v1",
	reasoning: false,
	input: ["text"],
	cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
	contextWindow: 32000,
	maxTokens: 2048,
};
const tools = [
	{
		name: "read",
		description: "Read a file",
		parameters: { type: "object", properties: { path: { type: "string" }, query: { type: "string" } }, required: ["path"] },
	},
	{
		name: "write",
		description: "Write a file",
		parameters: { type: "object", properties: { path: { type: "string" }, content: { type: "string" } }, required: ["path", "content"] },
	},
];
const context = { messages: [{ role: "user", content: "Use tools", timestamp: 1 }], tools };

function chunk(toolCalls, finishReason = null) {
	return `data: ${JSON.stringify({ id: "chatcmpl-tools", choices: [{ delta: { tool_calls: toolCalls }, finish_reason: finishReason }] })}\n\n`;
}

function buildSSE() {
	let sse = `data: ${JSON.stringify({ id: "chatcmpl-tools", choices: [{ delta: {}, finish_reason: null }] })}\n\n`;
	sse += chunk([
		{ index: 0, id: "call-read", type: "function", function: { name: "read", arguments: "" } },
		{ id: "call-write", type: "function", function: { name: "write", arguments: "" } },
	]);
	sse += chunk([{ index: 0, id: "", function: {} }, { index: 1, id: "call-write" }]);
	sse += chunk([{ index: 0, function: { name: "" } }]);
	for (let i = 0; i < Math.max(readArguments.length, writeArguments.length); i++) {
		const calls = [];
		if (i < readArguments.length)
			calls.push({ index: 0, ...(i === 0 && { id: "changed-read" }), function: { arguments: readArguments[i] } });
		if (i < writeArguments.length) calls.push({ index: 1, function: { arguments: writeArguments[i] } });
		sse += chunk(calls);
	}
	sse += chunk([], "tool_calls");
	return `${sse}data: [DONE]\n\n`;
}

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
	if (typeof value === "string")
		return value.replace(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])|(?<![\uD800-\uDBFF])[\uDC00-\uDFFF]/g, "�");
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

async function capture(piRoot, sse) {
	const api = await import(pathToFileURL(join(piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	let requestBody;
	const fetch = async (_input, init = {}) => {
		requestBody = JSON.parse(init.body);
		return new Response(sse, { status: 200, headers: { "content-type": "text/event-stream" } });
	};
	const stream = api.stream(model, context, { apiKey: "test-key", fetch, toolChoice: "required" });
	const events = [];
	for await (const event of stream) {
		const observed = { type: event.type };
		if (typeof event.contentIndex === "number") {
			observed.contentIndex = event.contentIndex;
			observed.partialArguments = event.partial.content[event.contentIndex]?.arguments;
			const native = observed.partialArguments?.native;
			if (typeof native === "string")
				observed.partialNativeCodeUnits = Array.from({ length: native.length }, (_, i) => native.charCodeAt(i));
		}
		if ("delta" in event) observed.delta = event.delta;
		if ("toolCall" in event) observed.toolCall = event.toolCall;
		if ("reason" in event) observed.reason = event.reason;
		events.push(canonical(observed));
	}
	const outcome = canonical(await stream.result());
	const deltas = events.filter((event) => event.type === "toolcall_delta");
	if (deltas.length !== 5 + readArguments.length + writeArguments.length) throw new Error("tool delta count mismatch");
	if (outcome.stopReason !== "toolUse" || outcome.content.length !== 2) throw new Error("tool outcome mismatch");
	if (stableJSON(outcome.content[0].arguments) !== stableJSON(JSON.parse(readArguments))) throw new Error("read arguments mismatch");
	if (stableJSON(outcome.content[1].arguments) !== stableJSON(JSON.parse(writeArguments))) throw new Error("write arguments mismatch");
	return canonical({ request: { body: requestBody }, events, outcome });
}

function fixtureFacts(fixture) {
	return stableJSON({
		schema_version: fixture.schema_version,
		id: fixture.id,
		catalog_ids: fixture.catalog_ids,
		baseline_id: fixture.baseline_id,
		baseline_commit: fixture.baseline_commit,
		deterministic: fixture.deterministic,
		normalization: fixture.normalization,
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
	const sse = buildSSE();
	const input = canonical({ model, context, options: { apiKey: "test-key", toolChoice: "required" }, arguments: { read: readArguments, write: writeArguments }, sse });
	const actual = await capture(args.piRoot, sse);
	const fixture = {
		schema_version: "1.0.0",
		id: "ai/openai-completions/m1-streaming-tools",
		catalog_ids: catalogIDs,
		baseline_id: lock.baselineID,
		baseline_commit: lock.commit,
		deterministic: true,
		normalization: { unpaired_surrogates: "JSON strings use U+FFFD; partialNativeCodeUnits retains exact UTF-16 units" },
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
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-tools.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed tools fixture does not reproduce");
		console.error("oracle: committed tools fixture reproduces");
		return;
	}
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
