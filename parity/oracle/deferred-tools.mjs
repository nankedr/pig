import { createHash } from "node:crypto";
import { execFileSync, spawnSync } from "node:child_process";
import { readFileSync, writeFileSync } from "node:fs";
import { dirname, join } from "node:path";
import { fileURLToPath, pathToFileURL } from "node:url";

const here = dirname(fileURLToPath(import.meta.url));
const root = join(here, "..", "..");
const defaultPi = join(root, ".upstream", "pi");
const defaultOutput = join(here, "fixtures", "deferred-tools.json");

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

function tool(name, description = name) {
	return { name, description, parameters: { type: "object", properties: {} } };
}

function call(name) {
	return { role: "assistant", content: [{ type: "toolCall", id: "call-1", name, arguments: {} }], api: "faux", provider: "faux", model: "faux-1", usage: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, totalTokens: 0, cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0, total: 0 } }, stopReason: "toolUse", timestamp: 1 };
}

function added(names) {
	return { role: "toolResult", toolCallId: "call-1", toolName: "discover", content: [], addedToolNames: names, isError: false, timestamp: 2 };
}

function declarations() {
	const tools = [tool("discover"), tool("Read", "old"), tool("spare"), tool("read", "replacement"), tool("write")];
	const markers = added(["READ", "read", "write", "missing"]);
	const scenarios = [
		{ id: "disabled", enabled: false, normalize: true, context: { tools, messages: [markers] } },
		{ id: "discovered", enabled: true, normalize: true, context: { tools, messages: [call("discover"), markers] } },
		{ id: "used-before-marker", enabled: true, normalize: true, context: { tools, messages: [call("READ"), markers] } },
		{ id: "identity", enabled: true, normalize: false, context: { tools, messages: [markers] } },
		{ id: "empty", enabled: true, normalize: false, context: { messages: [] } },
		{ id: "unmarked", enabled: true, normalize: false, context: { tools: [tool("read"), tool("read", "latest")], messages: [added(null), added([])] } },
	];
	const declaration = (id, scenarios) => ({
		schema_version: "1.0.0", id: `go-sdk/ai/${id}`, catalog_id: "contract:ai/deferred-tools", surface: "go-sdk",
		input: { scenarios, projection: "Tool metadata in stable order; Pi deferred Map values become an ordered array. Normalizer lowercases ASCII test names only." },
		observe: ["outcome", "side_effects"],
	});
	return [
		declaration("deferred-tools", scenarios),
		declaration("deferred-tools-used-deviation", [{ id: "used-after-marker", enabled: true, normalize: true, context: { tools, messages: [markers, call("READ"), markers] } }]),
	];
}

async function main() {
	const args = parseArgs(process.argv);
	const lock = JSON.parse(readFileSync(join(root, "parity", "baseline", "upstream.lock.json"), "utf8"));
	const head = execFileSync("git", ["-C", args.pi, "rev-parse", "HEAD"], { encoding: "utf8" }).trim();
	if (head !== lock.upstream.commit) throw new Error(`checkout HEAD ${head} != locked commit ${lock.upstream.commit}`);
	const status = spawnSync("git", ["-C", args.pi, "status", "--porcelain", "--untracked-files=no"], { encoding: "utf8" });
	if (status.status !== 0 || status.stdout !== "") throw new Error("locked Pi checkout has tracked changes");
	const reference = "packages/ai/src/utils/deferred-tools.ts";
	const { splitDeferredTools } = await import(pathToFileURL(join(args.pi, reference)).href);
	for (const caseValue of declarations()) {
		const outcome = caseValue.input.scenarios.map(({ id, context, enabled, normalize }) => {
			const { immediate, deferred } = splitDeferredTools(context, enabled, normalize ? (name) => name.toLowerCase() : undefined);
			return { id, immediate, deferred: [...deferred.values()] };
		});
		const observation = { outcome, side_effects: [] };
		const fixture = {
			schema_version: "1.0.0", deterministic: true, baseline_id: lock.baseline_id, baseline_commit: lock.upstream.commit,
			upstream: { repository: lock.upstream.repository, commit: lock.upstream.commit, reference: `${reference}#splitDeferredTools` },
			case: caseValue, observation, input_hash: caseDigest(caseValue), observation_hash: digest(observation),
			execution_method: "node --experimental-strip-types parity/oracle/deferred-tools.mjs <locked-pi-checkout>", platform: "any",
			environment: { node: process.version, oracle_entry: reference },
		};
		const output = caseValue.id.endsWith("-deviation") ? args.out.replace(/\.json$/, "-used-deviation.json") : args.out;
		if (args.check) {
			const committed = JSON.parse(readFileSync(output, "utf8"));
			fixture.environment.node = committed.environment.node;
			if (JSON.stringify(fixture) !== JSON.stringify(committed)) throw new Error(`committed fixture does not reproduce: ${output}`);
			console.log(`verified ${output}`);
		} else {
			writeFileSync(output, `${JSON.stringify(fixture, null, 2)}\n`);
			console.log(`wrote ${output}`);
		}
	}
}

main().catch((error) => { console.error(`oracle: ${error.message}`); process.exit(1); });
