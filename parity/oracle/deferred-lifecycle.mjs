import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "deferred-lifecycle.json");

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

function declaration() {
	return {
		schema_version: "1.0.0", id: "go-sdk/ai/deferred-lifecycle", catalog_id: "contract:ai/faux-provider", surface: "go-sdk",
		input: {
			api: "faux:deferred-parity", provider: "deferred-parity", model_id: "faux-1",
			pending_fetches: 2, poll_after_ms: 0, response: "ready", failure: "script failure",
			context: { messages: [{ role: "user", content: "hello", timestamp: 1 }] },
			actions: ["sync", "submit", "pending-1", "pending-2", "final", "repeat-final", "submit-cancel", "cancel", "fetch-cancelled", "unknown", "mismatch", "submit-error", "error-pending-1", "error-pending-2", "error-final", "repeat-error"],
			projection: "Message identity, content, usage, stopReason, handle fields and error text; timestamps and opaque IDs projected to presence/stability assertions. Invalid cancellation and concurrent resolution are Go deviations tested separately.",
		},
		observe: ["outcome", "side_effects"],
	};
}

function project(message, handle) {
	const result = { role: message.role, api: message.api, provider: message.provider, model: message.model, content: message.content, usage: message.usage, stopReason: message.stopReason };
	if (message.deferred) {
		const { id, ...metadata } = message.deferred;
		result.deferred = { ...metadata, id_present: id.length > 0, same_handle: !handle || id === handle.id };
	}
	if (message.errorMessage !== undefined) result.errorMessage = handle ? message.errorMessage.replace(handle.id, "<handle>") : message.errorMessage;
	return result;
}

async function capture(pi, input) {
	const ai = await import(pathToFileURL(join(pi, "packages/ai/src/providers/faux.ts")).href);
	const { createModels } = await import(pathToFileURL(join(pi, "packages/ai/src/models.ts")).href);
	const faux = ai.fauxProvider({ api: input.api, provider: input.provider, deferred: { pendingFetches: input.pending_fetches, pollAfterMs: input.poll_after_ms }, tokenSize: { min: 3, max: 3 } });
	const models = createModels();
	models.setProvider(faux.provider);
	const model = faux.getModel(input.model_id);
	let factoryCalls = 0;
	const response = () => ai.fauxAssistantMessage(input.response, { timestamp: 42 });
	faux.setResponses([response(), () => { factoryCalls++; return response(); }, response(), () => { factoryCalls++; throw new Error(input.failure); }]);
	const steps = {}, hooks = [];
	let handle, final, failure;
	for (const action of input.actions) {
		const options = { onResponse: (response) => hooks.push(`${action}:${response.status}`) };
		if (action === "sync" || action.startsWith("submit")) {
			const stream = faux.provider.streamSimple(model, input.context, { ...options, deferred: action !== "sync" });
			const events = [];
			for await (const event of stream) events.push(event.type === "done" || event.type === "error" ? `${event.type}:${event.reason}` : event.type);
			const result = await stream.result();
			steps[action] = { events, message: project(result), repeat_equal: JSON.stringify(result) === JSON.stringify(await stream.result()) };
			handle = result.deferred;
		} else if (action === "cancel") {
			await models.cancelDeferred(model, handle, options);
			steps[action] = { cancelled_count: faux.state.cancelledDeferred.length };
		} else {
			const requested = action === "unknown" ? { ...handle, id: "unknown" } : action === "mismatch" ? { ...handle, provider: "foreign" } : handle;
			const result = await models.fetchDeferred(model, requested, options);
			steps[action] = { message: project(result, requested) };
			if (action === "final") final = result;
			if (action === "repeat-final") steps[action].stable = JSON.stringify(result) === JSON.stringify(final);
			if (action === "error-final") failure = result;
			if (action === "repeat-error") steps[action].stable = JSON.stringify(result) === JSON.stringify(failure);
		}
	}
	return { outcome: { steps, hooks, factory_calls: factoryCalls, call_count: faux.state.callCount, fetch_count: faux.state.deferredFetchCount, queue_remaining: faux.getPendingResponseCount() }, side_effects: [] };
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
		upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: "packages/ai/src/providers/faux.ts#createFauxCore + packages/ai/src/models.ts#fetchDeferred" },
		case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation),
		execution_method: "node --experimental-strip-types parity/oracle/deferred-lifecycle.mjs <locked-pi-checkout>", platform: "any",
		environment: { node: process.version, oracle_entry: "packages/ai/src/providers/faux.ts + packages/ai/src/models.ts" },
	};
	if (args.check) {
		const committed = JSON.parse(readFileSync(args.out, "utf8"));
		fixture.environment.node = committed.environment.node;
		if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error("committed deferred fixture does not reproduce");
		console.log(`verified ${args.out}`);
		return;
	}
	writeFileSync(args.out, `${JSON.stringify(fixture, null, 2)}\n`);
	console.log(`wrote ${args.out}`);
}

main().catch((error) => { console.error(`oracle: ${error.message}`); process.exit(1); });
