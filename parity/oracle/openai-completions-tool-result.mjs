import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-tool-result.json");
const catalogIDs = [
	"matrix:ai/openai-completions/content/assistant-message-content-tool-call",
	"matrix:ai/openai-completions/content/tool-result-message-content-text",
	"matrix:ai/openai-completions/entrypoint/convert-messages",
	"matrix:ai/openai-completions/message/conversion-assistant-function-tool-call",
	"matrix:ai/openai-completions/message/conversion-tool-call-id-open-ai",
	"matrix:ai/openai-completions/message/conversion-tool-call-id-pipe",
	"matrix:ai/openai-completions/message/conversion-tool-call-id-result-remap",
	"matrix:ai/openai-completions/message/conversion-tool-result-empty",
	"matrix:ai/openai-completions/message/conversion-tool-result-text",
	"matrix:ai/openai-completions/message/tool-result-message-content",
	"matrix:ai/openai-completions/message/tool-result-message-is-error",
	"matrix:ai/openai-completions/message/tool-result-message-role",
	"matrix:ai/openai-completions/message/tool-result-message-tool-call-id",
	"matrix:ai/openai-completions/request/context-messages",
	"matrix:ai/openai-completions/request/request-messages",
];
const readID = "call+/read|item=read";
const emptyID = "call-empty+with/special|item-empty=abcdefghijklmnopqrstuvwxyz0123456789";
const errorID = "call-error-abcdefghijklmnopqrstuvwxyz-0123456789";
const sourceModel = {
	id: "source-model",
	name: "Source Model",
	api: "openai-completions",
	provider: "local-openai",
	baseUrl: "https://local.invalid/v1",
	reasoning: false,
	input: ["text"],
	cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
	contextWindow: 32000,
	maxTokens: 2048,
};
const targetModel = { ...sourceModel, id: "target-model", name: "Target Model", provider: "openai" };
const tools = [
	{ name: "read", description: "Read a file", parameters: { type: "object", properties: { path: { type: "string" } }, required: ["path"] } },
	{ name: "empty", description: "Return no output", parameters: { type: "object", properties: {} } },
	{ name: "fail", description: "Return an error", parameters: { type: "object", properties: {} } },
];
const user = { role: "user", content: "Read README", timestamp: 1 };
const firstSSE = [
	`data: ${JSON.stringify({
		id: "chatcmpl-tool-result",
		choices: [{
			delta: { tool_calls: [
				{ index: 0, id: readID, type: "function", function: { name: "read", arguments: '{"path":"README.md"}' } },
				{ index: 1, id: emptyID, type: "function", function: { name: "empty", arguments: "{}" } },
				{ index: 2, id: errorID, type: "function", function: { name: "fail", arguments: "{}" } },
			] },
			finish_reason: "tool_calls",
		}],
	})}`,
	"data: [DONE]",
	"",
].join("\n\n");
const finalSSE = [
	`data: ${JSON.stringify({ id: "chatcmpl-final", choices: [{ delta: { content: "done" }, finish_reason: "stop" }] })}`,
	"data: [DONE]",
	"",
].join("\n\n");

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

async function collect(stream) {
	const events = [];
	for await (const event of stream) events.push({ type: event.type });
	return { events, outcome: await stream.result() };
}

async function capture(piRoot) {
	const api = await import(pathToFileURL(join(piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	const requests = [];
	const fetch = async (_input, init = {}) => {
		requests.push(canonical(JSON.parse(init.body)));
		return new Response(requests.length === 1 ? firstSSE : finalSSE, {
			status: 200,
			headers: { "content-type": "text/event-stream" },
		});
	};
	const first = await collect(api.stream(sourceModel, { messages: [user], tools }, { apiKey: "test-key", fetch, toolChoice: "required" }));
	const calls = first.outcome.content.filter((block) => block.type === "toolCall");
	if (first.outcome.stopReason !== "toolUse" || calls.length !== 3) throw new Error("first streamed ToolCall outcome mismatch");
	const results = [
		{ role: "toolResult", toolCallId: calls[0].id, toolName: calls[0].name, content: [{ type: "text", text: "README contents" }], isError: false, timestamp: 2 },
		{ role: "toolResult", toolCallId: calls[1].id, toolName: calls[1].name, content: [], isError: false, timestamp: 3 },
		{ role: "toolResult", toolCallId: calls[2].id, toolName: calls[2].name, content: [{ type: "text", text: "permission denied" }], isError: true, timestamp: 4 },
	];
	const second = await collect(api.stream(
		targetModel,
		{ messages: [user, first.outcome, ...results], tools },
		{ apiKey: "test-key", fetch },
	));
	const secondMessages = requests[1].messages;
	const expectedIDs = ["call__read_item_read", "call-empty_with_special_ru6r3z1u", "call-error-abcdefghijklmnopqrstuvwxyz-01"];
	const replayedIDs = secondMessages[1].tool_calls.map((call) => call.id);
	const resultIDs = secondMessages.slice(2).map((result) => result.tool_call_id);
	if (stableJSON(replayedIDs) !== stableJSON(expectedIDs) || stableJSON(resultIDs) !== stableJSON(expectedIDs)) {
		throw new Error("normalized ToolCall and ToolResult IDs are not linked");
	}
	if (stableJSON(secondMessages.slice(2).map((result) => result.content)) !== stableJSON(["README contents", "(no tool output)", "permission denied"])) {
		throw new Error("text, empty, or error ToolResult conversion mismatch");
	}
	if (second.outcome.stopReason !== "stop" || second.outcome.content[0]?.text !== "done") throw new Error("final outcome mismatch");
	return canonical({ requests, first: { events: first.events, outcome: first.outcome }, second: { events: second.events, outcome: second.outcome } });
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
	const input = canonical({ sourceModel, targetModel, tools, user, results: ["text", "empty", "error"], firstSSE, finalSSE });
	const actual = await capture(args.piRoot);
	const fixture = {
		schema_version: "1.0.0",
		id: "ai/openai-completions/m1-tool-result-round-trip",
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
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-tool-result.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed ToolResult fixture does not reproduce");
		console.error("oracle: committed ToolResult fixture reproduces");
		return;
	}
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
