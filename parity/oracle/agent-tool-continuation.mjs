// Captures the deterministic Agent ToolResult continuation contract from the
// locked Pi source. A prepared checkout also checks the built modules.

import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, mkdtempSync, readFileSync, rmSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "agent-tool-continuation.json");

function parseArgs(argv) {
	const result = { piRoot: defaultPiRoot, out: defaultOutput, check: false, requireDist: false };
	const args = argv.slice(2);
	for (let index = 0; index < args.length; index++) {
		if (args[index] === "--check") result.check = true;
		else if (args[index] === "--require-dist") result.requireDist = true;
		else if (args[index] === "--batch") result.batch = true;
		else if (args[index] === "--read") result.read = true;
		else if (args[index] === "--out") result.out = args[++index];
		else if (result.piRoot === defaultPiRoot) result.piRoot = args[index];
		else throw new Error(`unexpected argument: ${args[index]}`);
	}
	return result;
}

function readDeclaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/codingagent/read-tool-continuation",
		catalog_id: "contract:codingagent/read-tool",
		surface: "go-sdk",
		input: {
			entrypoint: "createReadTool",
			network: "forbidden",
			provider: { api: "faux:read-continuation", id: "faux-read-continuation", model_id: "faux-1" },
			prompt: { role: "user", content: "read the sentinel", timestamp: 1 },
			file: { path: "sentinel.txt", content: "issue-54-real-read-sentinel" },
			tool_call_id: "read-sentinel",
			final_text: "The file contains issue-54-real-read-sentinel.",
			token_size: { min: 100, max: 100 },
		},
		observe: ["events", "outcome", "side_effects"],
	};
}

function loadLock() {
	const lock = JSON.parse(readFileSync(join(repoRoot, "parity", "baseline", "upstream.lock.json"), "utf8"));
	if (!lock?.baseline_id || !lock?.upstream?.commit || !lock?.upstream?.repository) {
		throw new Error("baseline lock is incomplete");
	}
	return lock;
}

function assertLockedCheckout(piRoot, commit) {
	const head = execFileSync("git", ["-C", piRoot, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== commit) throw new Error(`Pi checkout HEAD ${head} != locked commit ${commit}`);
	const dirty = spawnSync("git", ["-C", piRoot, "status", "--porcelain", "--untracked-files=no"], {
		encoding: "utf8",
	});
	if (dirty.error || dirty.status !== 0) throw dirty.error ?? new Error("unable to inspect Pi checkout");
	if (dirty.stdout !== "") throw new Error(`Pi checkout has tracked changes; expected ${commit}`);
}

function canonical(value) {
	if (Array.isArray(value)) return value.map(canonical);
	if (value && typeof value === "object") {
		return Object.fromEntries(
			Object.entries(value)
				.filter(([, child]) => child !== undefined)
				.sort(([left], [right]) => left.localeCompare(right))
				.map(([key, child]) => [key, canonical(child)]),
		);
	}
	return value;
}

function canonicalNumber(value) {
	if (!Number.isFinite(value)) throw new Error(`non-finite JSON number: ${value}`);
	if (value === 0) return "0";
	let text = String(value).toLowerCase();
	let sign = "";
	if (text.startsWith("-")) {
		sign = "-";
		text = text.slice(1);
	}
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
	if (typeof value === "object") {
		return `{${Object.entries(value)
			.filter(([, child]) => child !== undefined)
			.map(([key, child]) => `${JSON.stringify(key)}:${canonicalJSONString(child)}`)
			.join(",")}}`;
	}
	throw new Error(`unsupported JSON value: ${typeof value}`);
}

function digestJSON(value) {
	return `sha256:${createHash("sha256").update(canonicalJSONString(value)).digest("hex")}`;
}

function hashCase(declaration) {
	return digestJSON({
		schema_version: declaration.schema_version,
		id: declaration.id,
		catalog_id: declaration.catalog_id,
		surface: declaration.surface,
		input: canonical(declaration.input),
		observe: declaration.observe,
	});
}

function hashObservation(observation) {
	return digestJSON(canonical(observation));
}

function fileDigest(path) {
	return `sha256:${createHash("sha256").update(readFileSync(path)).digest("hex")}`;
}

function declaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/agent/tool-result-continuation",
		catalog_id: "contract:agent/runtime",
		surface: "go-sdk",
		input: {
			entrypoint: "runAgentLoop",
			network: "forbidden",
			provider: { api: "faux:agent-continuation", id: "faux-agent-continuation", model_id: "faux-1" },
			prompt: { role: "user", content: "look it up", timestamp: 1 },
			tool: {
				call: { id: "call-52", name: "lookup", arguments: {} },
				result: { content: [{ type: "text", text: "42" }], details: { value: 42 } },
			},
			final_text: "The model saw the ToolResult: forty-two.",
			token_size: { min: 100, max: 100 },
		},
		observe: ["events", "outcome", "side_effects"],
	};
}

function projectContent(content) {
	if (content.type === "text") return { type: content.type, text: content.text };
	if (content.type === "toolCall") {
		return { type: content.type, id: content.id, name: content.name, arguments: canonical(content.arguments) };
	}
	throw new Error(`out-of-scope content: ${content.type}`);
}

function projectMessage(message) {
	if (message.role === "user") {
		return { role: message.role, content: message.content };
	}
	if (message.role === "assistant") {
		return { role: message.role, content: message.content.map(projectContent), stopReason: message.stopReason };
	}
	if (message.role === "toolResult") {
		return {
			role: message.role,
			toolCallId: message.toolCallId,
			toolName: message.toolName,
			content: message.content.map(projectContent),
			details: canonical(message.details),
			isError: message.isError,
		};
	}
	throw new Error(`out-of-scope message: ${message.role}`);
}

function projectResult(result) {
	const projected = { content: (result.content ?? []).map(projectContent), details: canonical(result.details) };
	if (result.terminate !== undefined) projected.terminate = result.terminate;
	return projected;
}

function projectEvent(event) {
	const projected = { type: event.type };
	if (event.type === "message_start" || event.type === "message_end") {
		projected.message = projectMessage(event.message);
	} else if (event.type === "message_update") {
		projected.assistantEventType = event.assistantMessageEvent.type;
	} else if (event.type === "tool_execution_start") {
		projected.toolCallId = event.toolCallId;
		projected.toolName = event.toolName;
		projected.args = canonical(event.args);
	} else if (event.type === "tool_execution_end") {
		projected.toolCallId = event.toolCallId;
		projected.toolName = event.toolName;
		projected.result = projectResult(event.result);
		projected.isError = event.isError;
	} else if (event.type === "turn_end") {
		projected.message = projectMessage(event.message);
		projected.toolResults = event.toolResults.map(projectMessage);
	} else if (event.type === "agent_end") {
		projected.messages = event.messages.map(projectMessage);
	}
	return projected;
}

async function captureModule(agentModule, fauxModule, caseDeclaration) {
	const agent = await import(pathToFileURL(agentModule).href);
	const ai = await import(pathToFileURL(fauxModule).href);
	const input = caseDeclaration.input;
	const providerContexts = [];
	const executions = [];
	const events = [];
	const core = ai.createFauxCore({
		api: input.provider.api,
		provider: input.provider.id,
		models: [{ id: input.provider.model_id }],
		tokenSize: input.token_size,
	});
	const firstResponse = ai.fauxAssistantMessage(
		ai.fauxToolCall(input.tool.call.name, input.tool.call.arguments, { id: input.tool.call.id }),
		{ stopReason: "toolUse", timestamp: 2 },
	);
	const finalResponse = ai.fauxAssistantMessage(input.final_text, { timestamp: 4 });
	core.setResponses([
		(context) => {
			providerContexts.push(context.messages.map(projectMessage));
			return firstResponse;
		},
		(context) => {
			providerContexts.push(context.messages.map(projectMessage));
			return finalResponse;
		},
	]);
	const tool = {
		name: input.tool.call.name,
		label: "Lookup",
		description: "Look up a deterministic value",
		parameters: { type: "object", properties: {} },
		async execute(toolCallId, args) {
			executions.push({ toolCallId, args: canonical(args) });
			return structuredClone(input.tool.result);
		},
	};
	const prompt = structuredClone(input.prompt);
	const messages = await agent.runAgentLoop(
		[prompt],
		{ systemPrompt: "", messages: [], tools: [tool] },
		{
			model: core.getModel(),
			convertToLlm: (values) => values.filter((message) => ["user", "assistant", "toolResult"].includes(message.role)),
		},
		(event) => events.push(projectEvent(event)),
		undefined,
		core.streamSimple,
	);
	return {
		events,
		outcome: {
			messages: messages.map(projectMessage),
			providerContexts,
			executions,
			state: { callCount: core.state.callCount, pendingResponseCount: core.getPendingResponseCount() },
		},
		side_effects: [],
	};
}

async function captureReadModule(agentModule, fauxModule, readModule, caseDeclaration) {
	const agent = await import(pathToFileURL(agentModule).href);
	const ai = await import(pathToFileURL(fauxModule).href);
	const codingAgent = await import(pathToFileURL(readModule).href);
	const input = caseDeclaration.input;
	const providerContexts = [];
	const events = [];
	const cwd = mkdtempSync(join(tmpdir(), "pig-read-oracle-"));
	try {
		writeFileSync(join(cwd, input.file.path), input.file.content);
		const core = ai.createFauxCore({
			api: input.provider.api,
			provider: input.provider.id,
			models: [{ id: input.provider.model_id }],
			tokenSize: input.token_size,
		});
		const toolResponse = ai.fauxAssistantMessage(
			ai.fauxToolCall("read", { path: input.file.path }, { id: input.tool_call_id }),
			{ stopReason: "toolUse", timestamp: 2 },
		);
		const finalResponse = ai.fauxAssistantMessage(input.final_text, { timestamp: 4 });
		core.setResponses([
			(context) => {
				providerContexts.push(context.messages.map(projectMessage));
				return toolResponse;
			},
			(context) => {
				providerContexts.push(context.messages.map(projectMessage));
				const result = context.messages.at(-1);
				if (result?.role !== "toolResult" || result.content?.[0]?.text !== input.file.content) {
					throw new Error("read ToolResult did not contain the real sentinel file");
				}
				return finalResponse;
			},
		]);
		const messages = await agent.runAgentLoop(
			[structuredClone(input.prompt)],
			{ systemPrompt: "", messages: [], tools: [codingAgent.createReadTool(cwd)] },
			{
				model: core.getModel(),
				convertToLlm: (values) => values.filter((message) => ["user", "assistant", "toolResult"].includes(message.role)),
			},
			(event) => events.push(projectEvent(event)),
			undefined,
			core.streamSimple,
		);
		return {
			events,
			outcome: {
				messages: messages.map(projectMessage),
				providerContexts,
				file: { path: input.file.path, content: readFileSync(join(cwd, input.file.path), "utf8") },
				state: { callCount: core.state.callCount, pendingResponseCount: core.getPendingResponseCount() },
			},
			side_effects: [],
		};
	} finally {
		rmSync(cwd, { recursive: true, force: true });
	}
}

function batchDeclaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/agent/multi-tool-batch",
		catalog_id: "contract:agent/runtime",
		surface: "go-sdk",
		input: {
			entrypoint: "runAgentLoop",
			network: "forbidden",
			provider: { api: "faux:agent-tool-batch", id: "faux-agent-tool-batch", model_id: "faux-1" },
			prompt: { role: "user", content: "run both", timestamp: 1 },
			tool: {
				name: "work",
				calls: [
					{ id: "call-1", arguments: { value: "first" } },
					{ id: "call-2", arguments: { value: "second" } },
				],
			},
			token_size: { min: 100, max: 100 },
		},
		observe: ["events", "outcome", "side_effects"],
	};
}

async function captureBatchModule(agentModule, fauxModule, caseDeclaration) {
	const agent = await import(pathToFileURL(agentModule).href);
	const ai = await import(pathToFileURL(fauxModule).href);
	const input = caseDeclaration.input;
	const providerContexts = [];
	const preflights = [];
	const executions = [];
	const events = [];
	let releaseFirst;
	const firstReleased = new Promise((resolve) => {
		releaseFirst = resolve;
	});
	const core = ai.createFauxCore({
		api: input.provider.api,
		provider: input.provider.id,
		models: [{ id: input.provider.model_id }],
		tokenSize: input.token_size,
	});
	const response = ai.fauxAssistantMessage(
		input.tool.calls.map((call) => ai.fauxToolCall(input.tool.name, call.arguments, { id: call.id })),
		{ stopReason: "toolUse", timestamp: 2 },
	);
	core.setResponses([
		(context) => {
			providerContexts.push(context.messages.map(projectMessage));
			return response;
		},
	]);
	const tool = {
		name: input.tool.name,
		label: "Work",
		description: "Run deterministic controlled work",
		parameters: { type: "object", required: ["value"], properties: { value: { type: "string" } } },
		async execute(toolCallId, args) {
			executions.push(toolCallId);
			if (toolCallId === "call-1") await firstReleased;
			return {
				content: [{ type: "text", text: args.value }],
				details: { value: args.value },
				terminate: true,
			};
		},
	};
	const messages = await agent.runAgentLoop(
		[structuredClone(input.prompt)],
		{ systemPrompt: "", messages: [], tools: [tool] },
		{
			model: core.getModel(),
			convertToLlm: (values) => values.filter((message) => ["user", "assistant", "toolResult"].includes(message.role)),
			toolExecution: "parallel",
			beforeToolCall: ({ toolCall }) => {
				preflights.push(toolCall.id);
			},
		},
		(event) => {
			events.push(projectEvent(event));
			if (event.type === "tool_execution_end" && event.toolCallId === "call-2") releaseFirst();
		},
		undefined,
		core.streamSimple,
	);
	return {
		events,
		outcome: {
			messages: messages.map(projectMessage),
			providerContexts,
			preflights,
			executionCount: executions.length,
			state: { callCount: core.state.callCount, pendingResponseCount: core.getPendingResponseCount() },
		},
		side_effects: [],
	};
}

async function capture(piRoot, requireDist, batch, read) {
	if (batch && read) throw new Error("--batch and --read are mutually exclusive");
	const caseDeclaration = read ? readDeclaration() : batch ? batchDeclaration() : declaration();
	const sourceAgent = join(piRoot, "packages", "agent", "src", "agent-loop.ts");
	const sourceFaux = join(piRoot, "packages", "ai", "src", "providers", "faux.ts");
	const sourceRead = join(piRoot, "packages", "coding-agent", "src", "core", "tools", "read.ts");
	const captureSelectedModule = batch ? captureBatchModule : captureModule;
	const observation = read
		? await captureReadModule(sourceAgent, sourceFaux, sourceRead, caseDeclaration)
		: await captureSelectedModule(sourceAgent, sourceFaux, caseDeclaration);
	const distAgent = join(piRoot, "packages", "agent", "dist", "agent-loop.js");
	const distFaux = join(piRoot, "packages", "ai", "dist", "providers", "faux.js");
	const distRead = join(piRoot, "packages", "coding-agent", "dist", "core", "tools", "read.js");
	const hasDist = existsSync(distAgent) && existsSync(distFaux) && (!read || existsSync(distRead));
	if (requireDist && !hasDist) throw new Error("prepared Pi dist is required but Agent/Faux dist is absent");
	if (hasDist) {
		const distObservation = read
			? await captureReadModule(distAgent, distFaux, distRead, caseDeclaration)
			: await captureSelectedModule(distAgent, distFaux, caseDeclaration);
		if (JSON.stringify(canonical(observation)) !== JSON.stringify(canonical(distObservation))) {
			throw new Error("Pi built Agent/Faux modules differ from locked source modules");
		}
	}
	return { caseDeclaration, observation, sourceAgent, sourceFaux, sourceRead: read ? sourceRead : undefined };
}

async function withForbiddenFetch(run) {
	const originalFetch = globalThis.fetch;
	const calls = [];
	globalThis.fetch = async (request) => {
		calls.push(String(request));
		throw new Error(`unexpected network access: ${request}`);
	};
	try {
		const result = await run();
		if (calls.length !== 0) throw new Error(`Agent continuation attempted ${calls.length} network request(s)`);
		return result;
	} finally {
		globalThis.fetch = originalFetch;
	}
}

async function buildFixture(piRoot, lock, requireDist, batch, read) {
	const captured = await withForbiddenFetch(() => capture(piRoot, requireDist, batch, read));
	return {
		schema_version: "1.0.0",
		deterministic: true,
		baseline_id: lock.baseline_id,
		baseline_commit: lock.upstream.commit,
		upstream: {
			repository: lock.upstream.repository,
			commit: lock.upstream.commit,
			reference: read ? "packages/coding-agent/src/core/tools/read.ts#createReadTool" : "packages/agent/src/agent-loop.ts#runAgentLoop",
		},
		case: captured.caseDeclaration,
		observation: {
			events: captured.observation.events,
			outcome: captured.observation.outcome,
			side_effects: captured.observation.side_effects,
		},
		input_hash: hashCase(captured.caseDeclaration),
		observation_hash: hashObservation({
			events: captured.observation.events,
			outcome: captured.observation.outcome,
			side_effects: captured.observation.side_effects,
		}),
		execution_method: read
			? "node --experimental-strip-types parity/oracle/agent-tool-continuation.mjs --read <locked-pi-checkout>; source modules with built-dist differential"
			: batch
			? "node --experimental-strip-types parity/oracle/agent-tool-continuation.mjs --batch <locked-pi-checkout>; source modules with built-dist differential"
			: "node --experimental-strip-types parity/oracle/agent-tool-continuation.mjs <locked-pi-checkout>; source modules with built-dist differential",
		platform: "any",
		environment: {
			node: process.version,
				oracle_entry: read
					? "packages/coding-agent/src/core/tools/read.ts + packages/agent/src/agent-loop.ts + packages/ai/src/providers/faux.ts"
					: "packages/agent/src/agent-loop.ts + packages/ai/src/providers/faux.ts",
			agent_source_sha256: fileDigest(captured.sourceAgent),
			faux_source_sha256: fileDigest(captured.sourceFaux),
				...(captured.sourceRead ? { read_source_sha256: fileDigest(captured.sourceRead) } : {}),
		},
	};
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = loadLock();
	assertLockedCheckout(args.piRoot, lock.upstream.commit);
	const fixture = await buildFixture(args.piRoot, lock, args.requireDist, args.batch, args.read);
	if (args.check) {
		const recorded = JSON.parse(readFileSync(args.out, "utf8"));
		fixture.environment.node = recorded.environment.node;
		if (JSON.stringify(fixture) !== JSON.stringify(recorded)) {
			throw new Error(`fixture does not reproduce: ${args.out}`);
		}
		console.log(`verified ${args.out}`);
		return;
	}
	mkdirSync(dirname(args.out), { recursive: true });
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.log(`wrote ${args.out}`);
}

main().catch((error) => {
	console.error(`oracle: ${error.message}`);
	process.exit(1);
});
