// Captures the deterministic Faux core stream/outcome contract from locked Pi source.
// A prepared checkout can additionally require the built module differential with --require-dist.

import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { existsSync, mkdirSync, readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const repoRoot = join(here, "..", "..");
const defaultPiRoot = join(repoRoot, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "faux-stream-outcomes.json");
const factoryStateOutput = join(here, "fixtures", "faux-factory-state-snapshot-deviation.json");

function parseArgs(argv) {
	const result = { piRoot: defaultPiRoot, out: "", caseName: "stream-outcomes", check: false, requireDist: false };
	const args = argv.slice(2);
	for (let index = 0; index < args.length; index++) {
		if (args[index] === "--check") result.check = true;
		else if (args[index] === "--require-dist") result.requireDist = true;
		else if (args[index] === "--case") result.caseName = args[++index];
		else if (args[index] === "--out") result.out = args[++index];
		else if (result.piRoot === defaultPiRoot) result.piRoot = args[index];
		else throw new Error(`unexpected argument: ${args[index]}`);
	}
	if (!["stream-outcomes", "factory-state"].includes(result.caseName)) {
		throw new Error(`unknown case: ${result.caseName}`);
	}
	if (!result.out) result.out = result.caseName === "factory-state" ? factoryStateOutput : defaultOutput;
	return result;
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
				.sort(([left], [right]) => left.localeCompare(right))
				.map(([key, child]) => [key, canonical(child)]),
		);
	}
	return value;
}

function digestJSON(value) {
	return `sha256:${createHash("sha256").update(JSON.stringify(value)).digest("hex")}`;
}

function hashCase(declaration) {
	const value = {
		schema_version: declaration.schema_version,
		id: declaration.id,
		catalog_id: declaration.catalog_id,
		surface: declaration.surface,
		input: canonical(declaration.input),
		observe: declaration.observe,
	};
	return digestJSON(value);
}

function hashObservation(observation) {
	return digestJSON(canonical(observation));
}

function fileDigest(path) {
	return `sha256:${createHash("sha256").update(readFileSync(path)).digest("hex")}`;
}

function caseDeclaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/ai/faux-stream-outcomes",
		catalog_id: "contract:ai/faux-provider/core-stream",
		surface: "go-sdk",
		input: {
			api: "faux:parity",
			context: { messages: [{ role: "user", content: "hi", timestamp: 1 }] },
			entrypoints: { stream: "Provider.stream", complete: "Models.complete" },
			model_id: "faux-1",
			network: "forbidden",
			projection: {
				event: ["scenario", "type", "reason", "contentIndex", "delta", "content", "toolCall", "partial", "message", "error"],
				message: ["role", "content", "api", "provider", "model", "stopReason", "errorMessage"],
				partial: ["role", "api", "provider", "model", "stopReason", "errorMessage"],
			},
			provider: "faux-parity",
			repeat_result: 2,
			scenarios: [
				{
					id: "stream",
					method: "stream",
					content: [
						{ type: "text", text: "answer" },
						{ type: "toolCall", id: "tool-1", name: "echo", arguments: { count: 12, text: "hi" } },
					],
					stop_reason: "toolUse",
					timestamp: 2,
				},
				{
					id: "complete",
					method: "complete",
					content: [
						{ type: "text", text: "answer" },
						{ type: "toolCall", id: "tool-1", name: "echo", arguments: { count: 12, text: "hi" } },
					],
					stop_reason: "toolUse",
					timestamp: 2,
				},
				{
					id: "error",
					method: "stream",
					content: [{ type: "text", text: "partial response" }],
					error_message: "provider failed",
					stop_reason: "error",
					timestamp: 3,
				},
				{
					id: "aborted",
					method: "stream",
					abort_after: "text_delta",
					content: [{ type: "text", text: "abcdefghijklmnop" }],
					stop_reason: "stop",
					timestamp: 4,
				},
			],
			token_size: { min: 1, max: 1 },
			tokens_per_second: 64,
		},
		observe: ["events", "outcome", "side_effects"],
	};
}

function factoryStateDeclaration() {
	return {
		schema_version: "1.0.0",
		id: "go-sdk/ai/faux-factory-state-snapshot-deviation",
		catalog_id: "contract:ai/faux-provider",
		surface: "go-sdk",
		input: {
			api: "faux:factory-state-parity",
			calls: 2,
			entrypoint: "createFauxCore.stream",
			model_id: "faux-1",
			network: "forbidden",
			provider: "faux-factory-state",
		},
		observe: ["outcome", "side_effects"],
	};
}

function projectContent(content) {
	if (content.type === "text") return { type: content.type, text: content.text };
	if (content.type === "toolCall") {
		return { type: content.type, id: content.id, name: content.name, arguments: canonical(content.arguments) };
	}
	throw new Error(`out-of-scope Faux content: ${content.type}`);
}

function projectMessage(message) {
	const projected = {
		role: message.role,
		content: message.content.map(projectContent),
		api: message.api,
		provider: message.provider,
		model: message.model,
		stopReason: message.stopReason,
	};
	if (message.errorMessage !== undefined) projected.errorMessage = message.errorMessage;
	return projected;
}

function projectPartial(message) {
	const projected = {
		role: message.role,
		api: message.api,
		provider: message.provider,
		model: message.model,
		stopReason: message.stopReason,
	};
	if (message.errorMessage !== undefined) projected.errorMessage = message.errorMessage;
	return projected;
}

function projectEvent(scenario, event) {
	const projected = { scenario, type: event.type };
	if (event.reason !== undefined) projected.reason = event.reason;
	if (event.contentIndex !== undefined) projected.contentIndex = event.contentIndex;
	if (event.delta !== undefined) projected.delta = event.delta;
	if (event.content !== undefined) projected.content = event.content;
	if (event.toolCall !== undefined) projected.toolCall = projectContent(event.toolCall);
	if (event.partial !== undefined) projected.partial = projectPartial(event.partial);
	if (event.message !== undefined) projected.message = projectMessage(event.message);
	if (event.error !== undefined) projected.error = projectMessage(event.error);
	return projected;
}

function scriptedMessage(ai, scenario) {
	const content = scenario.content.map((block) => {
		if (block.type === "text") return ai.fauxText(block.text);
		if (block.type === "toolCall") return ai.fauxToolCall(block.name, block.arguments, { id: block.id });
		throw new Error(`out-of-scope scripted content: ${block.type}`);
	});
	return ai.fauxAssistantMessage(content, {
		stopReason: scenario.stop_reason,
		...(scenario.error_message === undefined ? {} : { errorMessage: scenario.error_message }),
		timestamp: scenario.timestamp,
	});
}

async function streamResult(stream, scenario, events, abort) {
	for await (const event of stream) {
		events.push(projectEvent(scenario.id, event));
		if (scenario.abort_after === event.type) abort?.abort();
	}
	return [projectMessage(await stream.result()), projectMessage(await stream.result())];
}

async function captureModules(ai, modelsModule, declaration) {
	const input = declaration.input;
	const faux = ai.fauxProvider({
		api: input.api,
		provider: input.provider,
		models: [{ id: input.model_id }],
		tokenSize: input.token_size,
		tokensPerSecond: input.tokens_per_second,
	});
	const models = modelsModule.createModels();
	models.setProvider(faux.provider);
	faux.setResponses(input.scenarios.map((scenario) => scriptedMessage(ai, scenario)));
	const model = faux.getModel(input.model_id);
	if (!model) throw new Error(`configured model ${input.model_id} is unavailable`);

	const events = [];
	let streamPair;
	let completeResult;
	let errorPair;
	let abortedPair;
	for (const scenario of input.scenarios) {
		if (scenario.method === "complete") {
			completeResult = projectMessage(await models.complete(model, input.context));
			continue;
		}
		const abort = scenario.abort_after ? new AbortController() : undefined;
		const stream = faux.provider.stream(model, input.context, abort ? { signal: abort.signal } : undefined);
		const pair = await streamResult(stream, scenario, events, abort);
		if (scenario.id === "stream") streamPair = pair;
		else if (scenario.id === "error") errorPair = pair;
		else if (scenario.id === "aborted") abortedPair = pair;
	}
	if (!streamPair || !completeResult || !errorPair || !abortedPair) throw new Error("incomplete Faux scenario set");
	const streamCompleteEqual = JSON.stringify(canonical(streamPair[0])) === JSON.stringify(canonical(completeResult));
	if (!streamCompleteEqual) throw new Error("Pi Stream and Complete outcomes differ");
	return {
		events,
		outcome: {
			stream_result: streamPair[0],
			stream_result_repeat: streamPair[1],
			complete_result: completeResult,
			stream_complete_equal: streamCompleteEqual,
			error_result: errorPair[0],
			error_result_repeat: errorPair[1],
			aborted_result: abortedPair[0],
			aborted_result_repeat: abortedPair[1],
			state: { call_count: faux.state.callCount, pending_response_count: faux.getPendingResponseCount() },
		},
		side_effects: [],
	};
}

async function capture(piRoot, requireDist) {
	const declaration = caseDeclaration();
	const sourceProvider = join(piRoot, "packages", "ai", "src", "providers", "faux.ts");
	const sourceModels = join(piRoot, "packages", "ai", "src", "models.ts");
	const distProvider = join(piRoot, "packages", "ai", "dist", "providers", "faux.js");
	const distModels = join(piRoot, "packages", "ai", "dist", "models.js");
	const sourceObservation = await withForbiddenFetch(() =>
		captureModulesFromPaths(sourceProvider, sourceModels, declaration),
	);
	const hasDist = existsSync(distProvider) && existsSync(distModels);
	if (requireDist && !hasDist) throw new Error("prepared Pi dist is required but packages/ai/dist is absent");
	if (hasDist) {
		const distObservation = await withForbiddenFetch(() =>
			captureModulesFromPaths(distProvider, distModels, declaration),
		);
		if (JSON.stringify(canonical(distObservation)) !== JSON.stringify(canonical(sourceObservation))) {
			throw new Error("Pi built modules differ from locked source modules");
		}
	}
	return { declaration, observation: sourceObservation, sourceProvider, sourceModels, hasDist };
}

async function captureModulesFromPaths(providerPath, modelsPath, declaration) {
	const [ai, models] = await Promise.all([
		import(pathToFileURL(providerPath).href),
		import(pathToFileURL(modelsPath).href),
	]);
	return captureModules(ai, models, declaration);
}

async function captureFactoryStateModule(providerPath, declaration) {
	const ai = await import(pathToFileURL(providerPath).href);
	const input = declaration.input;
	let release;
	const barrier = new Promise((resolve) => {
		release = resolve;
	});
	const factory = async (_context, _options, state) => {
		await barrier;
		return ai.fauxAssistantMessage(String(state.callCount), { timestamp: 1 });
	};
	const faux = ai.createFauxCore({
		api: input.api,
		provider: input.provider,
		models: [{ id: input.model_id }],
		tokenSize: { min: 1, max: 1 },
	});
	faux.setResponses(Array.from({ length: input.calls }, () => factory));
	const model = faux.getModel(input.model_id);
	if (!model) throw new Error(`configured model is unavailable: ${input.model_id}`);
	const streams = Array.from({ length: input.calls }, () => faux.stream(model, { messages: [] }, {}));
	release();
	const messages = await Promise.all(streams.map((stream) => stream.result()));
	const factoryCallCounts = messages.map((message) => {
		const content = message.content[0];
		const count = content?.type === "text" ? Number(content.text) : Number.NaN;
		if (!Number.isInteger(count)) throw new Error("factory did not return an integer state snapshot");
		return count;
	});
	return {
		outcome: {
			factory_call_counts: factoryCallCounts,
			provider_call_count: faux.state.callCount,
		},
		side_effects: [],
	};
}

async function captureFactoryState(piRoot, requireDist) {
	const declaration = factoryStateDeclaration();
	const sourceProvider = join(piRoot, "packages", "ai", "src", "providers", "faux.ts");
	const distProvider = join(piRoot, "packages", "ai", "dist", "providers", "faux.js");
	const sourceObservation = await withForbiddenFetch(() => captureFactoryStateModule(sourceProvider, declaration));
	const hasDist = existsSync(distProvider);
	if (requireDist && !hasDist) throw new Error("prepared Pi dist is required but packages/ai/dist is absent");
	if (hasDist) {
		const distObservation = await withForbiddenFetch(() => captureFactoryStateModule(distProvider, declaration));
		if (JSON.stringify(canonical(distObservation)) !== JSON.stringify(canonical(sourceObservation))) {
			throw new Error("Pi built Faux module differs from the locked source module");
		}
	}
	return { declaration, observation: sourceObservation, sourceProvider };
}

async function withForbiddenFetch(run) {
	const originalFetch = globalThis.fetch;
	const calls = [];
	globalThis.fetch = async (input) => {
		calls.push(String(input));
		throw new Error(`unexpected network access: ${input}`);
	};
	try {
		const observation = await run();
		if (calls.length !== 0) throw new Error(`Faux attempted ${calls.length} network request(s)`);
		return observation;
	} finally {
		globalThis.fetch = originalFetch;
	}
}

async function buildFixture(piRoot, lock, requireDist) {
	const captured = await capture(piRoot, requireDist);
	return {
		schema_version: "1.0.0",
		deterministic: true,
		baseline_id: lock.baseline_id,
		baseline_commit: lock.upstream.commit,
		upstream: {
			repository: lock.upstream.repository,
			commit: lock.upstream.commit,
			reference: "packages/ai/src/providers/faux.ts#createFauxCore",
		},
		case: captured.declaration,
		observation: captured.observation,
		input_hash: hashCase(captured.declaration),
		observation_hash: hashObservation(captured.observation),
		execution_method:
			"node --experimental-strip-types parity/oracle/faux-stream-outcomes.mjs <locked-pi-checkout>; source modules with optional built-dist differential",
		platform: "any",
		environment: {
			node: process.version,
			oracle_entry: "packages/ai/src/providers/faux.ts + packages/ai/src/models.ts",
			provider_source_sha256: fileDigest(captured.sourceProvider),
			models_source_sha256: fileDigest(captured.sourceModels),
		},
	};
}

async function buildFactoryStateFixture(piRoot, lock, requireDist) {
	const captured = await captureFactoryState(piRoot, requireDist);
	return {
		schema_version: "1.0.0",
		deterministic: true,
		baseline_id: lock.baseline_id,
		baseline_commit: lock.upstream.commit,
		upstream: {
			repository: lock.upstream.repository,
			commit: lock.upstream.commit,
			reference: "packages/ai/src/providers/faux.ts#createFauxCore",
		},
		case: captured.declaration,
		observation: captured.observation,
		input_hash: hashCase(captured.declaration),
		observation_hash: hashObservation(captured.observation),
		execution_method:
			"node --experimental-strip-types parity/oracle/faux-stream-outcomes.mjs <locked-pi-checkout> --case factory-state; source module with optional built-module differential",
		platform: "any",
		environment: {
			node: process.version,
			oracle_entry: "packages/ai/src/providers/faux.ts#createFauxCore",
			provider_source_sha256: fileDigest(captured.sourceProvider),
		},
	};
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = loadLock();
	assertLockedCheckout(args.piRoot, lock.upstream.commit);
	const fixture =
		args.caseName === "factory-state"
			? await buildFactoryStateFixture(args.piRoot, lock, args.requireDist)
			: await buildFixture(args.piRoot, lock, args.requireDist);
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
