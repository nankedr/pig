import { createHash } from "node:crypto";
import { execFileSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "openai-completions-thinking.json");
const catalogIDs = [
	"matrix:ai/openai-completions/compat/open-aicompletions-compat-requires-reasoning-content-on-assistant-messages",
	"matrix:ai/openai-completions/compat/open-aicompletions-compat-requires-thinking-as-text",
	"matrix:ai/openai-completions/compat/open-aicompletions-compat-supports-reasoning-effort",
	"matrix:ai/openai-completions/compat/open-aicompletions-compat-supports-thinking-token-budget",
	"matrix:ai/openai-completions/compat/open-aicompletions-compat-thinking-format",
	"matrix:ai/openai-completions/content/assistant-message-content-thinking",
	"matrix:ai/openai-completions/content/text-content-text-signature",
	"matrix:ai/openai-completions/content/thinking-content-thinking",
	"matrix:ai/openai-completions/content/thinking-content-thinking-signature",
	"matrix:ai/openai-completions/content/thinking-content-type",
	"matrix:ai/openai-completions/delta/delta-reasoning",
	"matrix:ai/openai-completions/delta/delta-reasoning-content",
	"matrix:ai/openai-completions/delta/delta-reasoning-details",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-attach-after-tool-call",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-invalid-element",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-items-data",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-items-id",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-items-type",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-pending-before-tool-call",
	"matrix:ai/openai-completions/delta/delta-reasoning-details-pending-last-wins",
	"matrix:ai/openai-completions/delta/delta-reasoning-text",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-delta",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-delta-type",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-end",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-content",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-end-type",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-start",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-start-content-index",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-start-partial",
	"matrix:ai/openai-completions/event/assistant-message-event-thinking-start-type",
	"matrix:ai/openai-completions/message/conversion-assistant-reasoning-content-required",
	"matrix:ai/openai-completions/message/conversion-assistant-reasoning-details-invalid",
	"matrix:ai/openai-completions/message/conversion-assistant-reasoning-details-valid",
	"matrix:ai/openai-completions/message/conversion-assistant-thinking-as-text",
	"matrix:ai/openai-completions/message/conversion-assistant-thinking-signature",
	"matrix:ai/openai-completions/message/conversion-assistant-thinking-signature-opencode-go",
	"matrix:ai/openai-completions/message/conversion-history-cross-model-thinking",
	"matrix:ai/openai-completions/message/conversion-history-cross-model-thought-signature",
	"matrix:ai/openai-completions/option/open-aicompletions-options-reasoning-effort",
	"matrix:ai/openai-completions/option/open-aicompletions-options-thinking-budgets",
	"matrix:ai/openai-completions/option/simple-stream-options-reasoning",
	"matrix:ai/openai-completions/option/thinking-budgets-high",
	"matrix:ai/openai-completions/request/model-reasoning",
	"matrix:ai/openai-completions/request/model-thinking-level-map",
	"matrix:ai/openai-completions/request/model-thinking-level-map-high",
	"matrix:ai/openai-completions/request/model-thinking-level-map-medium",
	"matrix:ai/openai-completions/request/request-enable-thinking",
	"matrix:ai/openai-completions/request/request-reasoning",
	"matrix:ai/openai-completions/request/request-reasoning-effort",
	"matrix:ai/openai-completions/request/request-thinking",
	"matrix:ai/openai-completions/request/request-thinking-token-budget",
	"matrix:ai/openai-completions/tool/tool-call-thought-signature",
	"matrix:ai/openai-completions/usage/usage-completion-tokens-details-reasoning-tokens",
	"matrix:ai/openai-completions/usage/usage-output-includes-reasoning",
	"matrix:ai/openai-completions/usage/usage-reasoning",
];
const model = {
	id: "reasoning-model",
	name: "Reasoning Model",
	api: "openai-completions",
	provider: "opencode-go",
	baseUrl: "https://local.invalid/v1",
	reasoning: true,
	thinkingLevelMap: { medium: null, high: "hard" },
	input: ["text"],
	cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
	contextWindow: 32000,
	maxTokens: 16384,
	compat: {
		supportsReasoningEffort: true,
		requiresReasoningContentOnAssistantMessages: true,
		thinkingFormat: "openai",
		supportsThinkingTokenBudget: true,
	},
};
const encryptedHistory = JSON.stringify({ type: "reasoning.encrypted", id: "history-call", data: "history-secret" });
const context = {
	messages: [
		{ role: "user", content: "No thinking", timestamp: 1 },
		{
			role: "assistant", content: [{ type: "text", text: "plain" }], api: model.api, provider: model.provider, model: model.id,
			usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
			stopReason: "stop", timestamp: 2,
		},
		{ role: "user", content: "Earlier question", timestamp: 1 },
		{
			role: "assistant",
			content: [
				{ type: "thinking", thinking: "plan", thinkingSignature: "reasoning" },
				{ type: "thinking", thinking: "check", thinkingSignature: "reasoning_text" },
				{ type: "text", text: "answer", textSignature: '{"v":1,"id":"ignored"}' },
				{ type: "toolCall", id: "history-call", name: "read", arguments: { path: "README.md" }, thoughtSignature: encryptedHistory },
			],
			api: model.api,
			provider: model.provider,
			model: model.id,
			usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
			stopReason: "toolUse",
			timestamp: 2,
		},
		{ role: "toolResult", toolCallId: "history-call", toolName: "read", content: [{ type: "text", text: "file" }], isError: false, timestamp: 3 },
		{ role: "user", content: "Continue", timestamp: 4 },
	],
	tools: [{ name: "read", description: "Read a file", parameters: { type: "object", properties: { path: { type: "string" } }, required: ["path"] } }],
};
const sse = [
	'data: {"id":"chatcmpl-thinking","model":"reasoning-model-v2","choices":[{"delta":{"reasoning_content":"first","reasoning":"duplicate","reasoning_text":"duplicate-text"}}]}',
	'data: {"choices":[{"delta":{"reasoning":" second"}}]}',
	'data: {"choices":[{"delta":{"reasoning_text":" fallback"}}]}',
	'data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","id":"call-1","data":"old"},{"type":"invalid","id":"call-1","data":"ignored"}]}}]}',
	'data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","id":"call-1","data":"secret"}]}}]}',
	`data: ${JSON.stringify({ choices: [{ delta: { tool_calls: [{ index: 0, id: "call-1", function: { name: "read", arguments: '{"path":"README.md"}' } }] } }] })}`,
	`data: ${JSON.stringify({ choices: [{ delta: { tool_calls: [{ index: 1, id: "call-2", function: { name: "read", arguments: "{}" } }] } }] })}`,
	'data: {"choices":[{"delta":{"reasoning_details":[{"type":"reasoning.encrypted","id":"call-2","data":"later"}]}}]}',
	'data: {"choices":[{"delta":{"content":"answer"},"finish_reason":"tool_calls"}]}',
	'data: {"choices":[],"usage":{"prompt_tokens":5,"completion_tokens":8,"completion_tokens_details":{"reasoning_tokens":6}}}',
	"data: [DONE]",
	"",
].join("\n\n");
const doneSSE = 'data: {"choices":[{"delta":{},"finish_reason":"stop"}]}\n\ndata: [DONE]\n\n';

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
	return { baselineID: lock.baseline_id, commit: lock.source_verification.expected_commit, repository: lock.upstream.repository };
}

function projectEvent(event) {
	const projected = { type: event.type };
	if (typeof event.contentIndex === "number") projected.contentIndex = event.contentIndex;
	if (typeof event.delta === "string") projected.delta = event.delta;
	if (typeof event.content === "string") projected.content = event.content;
	if (event.type.startsWith("thinking_")) {
		const block = event.partial.content[event.contentIndex];
		projected.thinkingSignature = block.thinkingSignature;
		if (event.type !== "thinking_start") projected.partialThinking = block.thinking;
	}
	if (event.type === "toolcall_end" && event.toolCall.thoughtSignature) {
		projected.thoughtSignature = event.toolCall.thoughtSignature;
	}
	if (event.type === "done") projected.reason = event.reason;
	return canonical(projected);
}

async function captureRequestCases(api) {
	const high = "high";
	const cases = [
		{ id: "openai", compat: { supportsReasoningEffort: true, thinkingFormat: "openai" }, reasoningEffort: high, thinkingLevelMap: { high: "hard" } },
		{ id: "deepseek-enabled", compat: { supportsReasoningEffort: true, thinkingFormat: "deepseek" }, reasoningEffort: high },
		{ id: "deepseek-disabled", compat: { thinkingFormat: "deepseek" } },
		{ id: "openrouter-off", compat: { thinkingFormat: "openrouter" }, thinkingLevelMap: { off: "disabled" } },
		{ id: "qwen", compat: { supportsReasoningEffort: true, thinkingFormat: "qwen" }, reasoningEffort: high },
		{ id: "zai", compat: { thinkingFormat: "zai" }, reasoningEffort: high },
		{ id: "together", compat: { thinkingFormat: "together" }, reasoningEffort: high },
		{ id: "string-thinking", compat: { thinkingFormat: "string-thinking" }, reasoningEffort: high },
		{ id: "ant-ling", compat: { thinkingFormat: "ant-ling" }, reasoningEffort: high, thinkingLevelMap: { high: "enabled" } },
	];
	const result = {};
	for (const testCase of cases) {
		let body;
		const fetch = async (_input, init = {}) => {
			body = JSON.parse(init.body);
			return new Response(doneSSE, { status: 200, headers: { "content-type": "text/event-stream" } });
		};
		const stream = api.stream(
			{ ...model, provider: "local", baseUrl: "https://example.test/v1", compat: testCase.compat, thinkingLevelMap: testCase.thinkingLevelMap },
			{ messages: [] },
			{ apiKey: "test-key", fetch, reasoningEffort: testCase.reasoningEffort },
		);
		for await (const _event of stream) {}
		await stream.result();
		result[testCase.id] = Object.fromEntries(
			["enable_thinking", "reasoning", "reasoning_effort", "thinking"].filter((field) => field in body).map((field) => [field, body[field]]),
		);
	}
	return canonical(result);
}

function captureConversions(api) {
	const message = {
		role: "assistant", api: "openai-completions", provider: "source", model: "source", stopReason: "toolUse", timestamp: 1,
		usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } },
		content: [
			{ type: "thinking", thinking: "visible reasoning", thinkingSignature: "reasoning_content" },
			{ type: "thinking", thinking: "secret", thinkingSignature: "opaque", redacted: true },
			{ type: "thinking", thinking: "   " },
			{ type: "text", text: "answer", textSignature: '{"v":1,"id":"ignored"}' },
			{ type: "toolCall", id: "call", name: "read", arguments: {}, thoughtSignature: '{"type":"reasoning.encrypted","id":"call","data":"secret"}' },
		],
	};
	const target = { ...model, id: "target", provider: "target" };
	const crossModel = api.convertMessages(target, { messages: [message] }, {});
	const sameModelMessage = {
		...message, provider: target.provider, model: target.id, stopReason: "stop",
		content: [
			{ type: "thinking", thinking: "plan" },
			{ type: "thinking", thinking: "check" },
			{ type: "text", text: "answer" },
			{ type: "toolCall", id: "valid", name: "read", arguments: {}, thoughtSignature: '{"type":"reasoning.encrypted","id":"valid","data":"ok"}' },
			{ type: "toolCall", id: "invalid", name: "read", arguments: {}, thoughtSignature: "false" },
		],
	};
	const thinkingAsText = api.convertMessages(target, { messages: [sameModelMessage] }, { requiresThinkingAsText: true });
	return canonical({ cross_model: crossModel[0], thinking_as_text: thinkingAsText[0] });
}

async function capture(piRoot) {
	const api = await import(pathToFileURL(join(piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	let requestBody;
	const fetch = async (_input, init = {}) => {
		requestBody = canonical(JSON.parse(init.body));
		return new Response(sse, { status: 200, headers: { "content-type": "text/event-stream" } });
	};
	const stream = api.streamSimple(model, context, {
		apiKey: "test-key", fetch, maxTokens: 4096, reasoning: "medium", thinkingBudgets: { high: 8192 },
	});
	const events = [];
	for await (const event of stream) events.push(projectEvent(event));
	const outcome = canonical(await stream.result());
	const expectedTypes = [
		"start", "thinking_start", "thinking_delta", "thinking_delta", "thinking_delta",
		"toolcall_start", "toolcall_delta", "toolcall_start", "toolcall_delta", "text_start",
		"text_delta", "thinking_end", "toolcall_end", "toolcall_end", "text_end", "done",
	];
	if (stableJSON(events.map((event) => event.type)) !== stableJSON(expectedTypes)) throw new Error("thinking event order mismatch");
	if (requestBody.reasoning_effort !== "hard" || requestBody.thinking_token_budget !== 3072) throw new Error("reasoning request mapping mismatch");
	const replay = requestBody.messages.find((message) => message.role === "assistant");
	const signedReplay = requestBody.messages.find((message) => message.reasoning_details);
	if (replay.reasoning_content !== "" || signedReplay.reasoning_content !== "plan\ncheck" || signedReplay.textSignature !== undefined || signedReplay.reasoning_details[0]?.data !== "history-secret") {
		throw new Error("thinking history replay mismatch");
	}
	if (outcome.content[0]?.thinking !== "first second fallback" || outcome.usage.reasoning !== 6) throw new Error("thinking outcome mismatch");
	if (outcome.content[1]?.thoughtSignature !== '{"type":"reasoning.encrypted","id":"call-1","data":"secret"}') throw new Error("pending reasoning detail mismatch");
	if (outcome.content[2]?.thoughtSignature !== '{"type":"reasoning.encrypted","id":"call-2","data":"later"}') throw new Error("late reasoning detail mismatch");
	return canonical({ request: { body: requestBody }, events, outcome });
}

function fixtureFacts(fixture) {
	return stableJSON({
		schema_version: fixture.schema_version, id: fixture.id, catalog_ids: fixture.catalog_ids, baseline_id: fixture.baseline_id,
		baseline_commit: fixture.baseline_commit, deterministic: fixture.deterministic, upstream: fixture.upstream, input: fixture.input,
		input_sha256: fixture.input_sha256, actual: fixture.actual, hash: fixture.hash, exec_method: fixture.exec_method,
	});
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = readLock();
	const head = execFileSync("git", ["-C", args.piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.commit}`);
	const input = canonical({ model, context, options: { apiKey: "test-key", maxTokens: 4096, reasoning: "medium", thinkingBudgets: { high: 8192 } }, sse });
	const actual = await capture(args.piRoot);
	const api = await import(pathToFileURL(join(args.piRoot, "packages", "ai", "src", "api", "openai-completions.ts")).href);
	actual.request_cases = await captureRequestCases(api);
	actual.conversions = captureConversions(api);
	const fixture = {
		schema_version: "1.0.0", id: "ai/openai-completions/m2-thinking-signatures", catalog_ids: catalogIDs,
		baseline_id: lock.baselineID, baseline_commit: lock.commit, deterministic: true,
		upstream: { module: "ai", repository: lock.repository, commit: lock.commit, reference: "packages/ai/src/api/openai-completions.ts#streamSimple" },
		input, input_sha256: sha256(input), actual, hash: { algorithm: "sha256", observations_sha256: sha256(actual) },
		exec_method: "node --experimental-strip-types parity/oracle/openai-completions-thinking.mjs",
		env: { node_version: process.version, platform: process.platform, arch: process.arch },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		if (fixtureFacts(committed) !== fixtureFacts(fixture)) throw new Error("committed thinking fixture does not reproduce");
		console.error("oracle: committed thinking fixture reproduces");
		return;
	}
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.error(`oracle: captured ${fixture.id} -> ${args.out}`);
}

main().catch((error) => {
	console.error("oracle:", error.message);
	process.exit(1);
});
